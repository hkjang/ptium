import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  ChevronLeft, ChevronRight, Grid3X3, Keyboard, LoaderCircle, Maximize2, Minimize2, MonitorPlay, Pointer, Square, X,
} from 'lucide-react'
import { api } from '../api/client'
import { ShortcutSheet, presentationShortcuts, useShortcutSheet } from './Shortcuts'
import type { Slide } from '../types'

/**
 * Presenting a deck.
 *
 * The audience sees the slide and nothing else — the speaker notes used to be
 * printed under it on the projector, which is exactly backwards. Everything the
 * presenter needs lives in a second window: the notes, what is coming next, and
 * how long they have been talking.
 */

export type Blackout = 'none' | 'black' | 'white'

export interface PresentState {
  index: number
  total: number
  blackout: Blackout
  startedAt: number
  title: string
}

type PresentMessage =
  | ({ type: 'state' } & PresentState)
  | { type: 'go'; index: number }
  | { type: 'step'; delta: number }
  | { type: 'hello' }
  | { type: 'exit' }

/**
 * The link between the two windows.
 *
 * A BroadcastChannel is same-origin and needs no server round trip, so the
 * presenter view answers a key press as fast as the projector does. Where it is
 * missing the presentation still runs; only the second window does not.
 */
export function usePresentChannel(presentationId: string, onMessage: (message: PresentMessage) => void) {
  const handler = useRef(onMessage)
  handler.current = onMessage
  const channel = useRef<BroadcastChannel | null>(null)
  useEffect(() => {
    if (typeof BroadcastChannel === 'undefined' || !presentationId) return
    const opened = new BroadcastChannel(`ptium-present-${presentationId}`)
    opened.onmessage = (event) => handler.current(event.data as PresentMessage)
    channel.current = opened
    return () => { opened.close(); channel.current = null }
  }, [presentationId])
  return useCallback((message: PresentMessage) => channel.current?.postMessage(message), [])
}

/**
 * Slide images, with the neighbours already fetched.
 *
 * Advancing a slide in front of a room is the one moment a spinner is
 * unacceptable, so the next and previous slides are pulled while the current one
 * is on screen.
 */
export function useSlideImages(presentationId: string, total: number, version: string | number, index: number, width = 1600) {
  const [images, setImages] = useState<Record<number, string>>({})
  const cache = useRef<Map<string, string>>(new Map())
  useEffect(() => {
    // A saved change invalidates every drawing at once.
    for (const url of cache.current.values()) URL.revokeObjectURL(url)
    cache.current = new Map()
    setImages({})
  }, [presentationId, version, width])
  useEffect(() => {
    let active = true
    const wanted = [index, index + 1, index - 1, index + 2].filter((position) => position >= 0 && position < total)
    void (async () => {
      for (const position of wanted) {
        const key = `${position}`
        if (cache.current.has(key)) continue
        try {
          const url = await api.slidePreview(presentationId, position + 1, width)
          if (!active) { URL.revokeObjectURL(url); return }
          cache.current.set(key, url)
          setImages((current) => ({ ...current, [position]: url }))
        } catch { /* a slide that will not draw shows as an empty frame */ }
      }
    })()
    return () => { active = false }
  }, [presentationId, total, index, version, width])
  useEffect(() => () => { for (const url of cache.current.values()) URL.revokeObjectURL(url) }, [])
  return images
}

function elapsed(from: number, now: number) {
  const seconds = Math.max(0, Math.floor((now - from) / 1000))
  const pad = (value: number) => String(value).padStart(2, '0')
  return `${pad(Math.floor(seconds / 3600))}:${pad(Math.floor(seconds / 60) % 60)}:${pad(seconds % 60)}`
}

/** The audience screen: the slide, and nothing that is not the slide. */
export function PresentationView({ presentationId, title, slides, version, startIndex = 0, onClose }: {
  presentationId: string
  title: string
  slides: Slide[]
  version: string | number
  startIndex?: number
  onClose: () => void
}) {
  const [index, setIndex] = useState(Math.min(Math.max(startIndex, 0), Math.max(slides.length - 1, 0)))
  const [blackout, setBlackout] = useState<Blackout>('none')
  const [overview, setOverview] = useState(false)
  const [laser, setLaser] = useState(false)
  const shortcuts = useShortcutSheet()
  const [pointer, setPointer] = useState({ x: -100, y: -100 })
  const [idle, setIdle] = useState(false)
  const [fullscreen, setFullscreen] = useState(false)
  const [jump, setJump] = useState('')
  const [presenterOpen, setPresenterOpen] = useState(false)
  const startedAt = useRef(Date.now())
  const stage = useRef<HTMLDivElement>(null)
  const idleTimer = useRef(0)
  const total = slides.length
  const images = useSlideImages(presentationId, total, version, index)
  const thumbs = useSlideImages(presentationId, overview ? total : 0, version, 0, 320)

  const step = useCallback((delta: number) => {
    setIndex((value) => Math.min(total - 1, Math.max(0, value + delta)))
    setBlackout('none')
  }, [total])

  const post = usePresentChannel(presentationId, (message) => {
    switch (message.type) {
      case 'hello': break
      case 'go': setIndex(Math.min(total - 1, Math.max(0, message.index))); setBlackout('none'); break
      case 'step': step(message.delta); break
      case 'exit': onClose(); break
    }
  })

  // The presenter window mirrors this one, so every change is announced.
  useEffect(() => {
    post({ type: 'state', index, total, blackout, startedAt: startedAt.current, title })
  }, [post, index, total, blackout, title])

  const openPresenter = useCallback(() => {
    const opened = window.open(`/presentations/${encodeURIComponent(presentationId)}/presenter`,
      `ptium-presenter-${presentationId}`, 'width=1180,height=760')
    setPresenterOpen(Boolean(opened))
    window.setTimeout(() => post({ type: 'state', index, total, blackout, startedAt: startedAt.current, title }), 700)
  }, [presentationId, post, index, total, blackout, title])

  const toggleFullscreen = useCallback(() => {
    const element = stage.current
    if (!element) return
    if (document.fullscreenElement) void document.exitFullscreen().catch(() => undefined)
    else void element.requestFullscreen?.().catch(() => undefined)
  }, [])

  useEffect(() => {
    const onChange = () => setFullscreen(Boolean(document.fullscreenElement))
    document.addEventListener('fullscreenchange', onChange)
    return () => document.removeEventListener('fullscreenchange', onChange)
  }, [])

  // A room is dark and the pointer is a distraction: both the cursor and the
  // controls step out of the way until the presenter moves the mouse.
  useEffect(() => {
    const wake = () => {
      setIdle(false)
      window.clearTimeout(idleTimer.current)
      idleTimer.current = window.setTimeout(() => setIdle(true), 2600)
    }
    wake()
    window.addEventListener('mousemove', wake)
    window.addEventListener('keydown', wake)
    return () => {
      window.clearTimeout(idleTimer.current)
      window.removeEventListener('mousemove', wake)
      window.removeEventListener('keydown', wake)
    }
  }, [])

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.ctrlKey || event.metaKey || event.altKey) return
      // Typing a number and pressing Enter jumps to that slide, the way every
      // presentation tool has done for thirty years.
      if (/^[0-9]$/.test(event.key)) {
        event.preventDefault()
        setJump((value) => (value + event.key).slice(0, 3))
        return
      }
      switch (event.key) {
        case 'ArrowRight': case 'PageDown': case ' ': case 'Enter': {
          event.preventDefault()
          if (jump) {
            const wanted = Number(jump) - 1
            setJump('')
            if (Number.isFinite(wanted)) { setIndex(Math.min(total - 1, Math.max(0, wanted))); setBlackout('none') }
            return
          }
          if (overview) { setOverview(false); return }
          step(1)
          break
        }
        case 'ArrowLeft': case 'PageUp': case 'Backspace': event.preventDefault(); step(-1); break
        case 'Home': setIndex(0); break
        case 'End': setIndex(Math.max(0, total - 1)); break
        case 'b': case 'B': case '.': setBlackout((value) => value === 'black' ? 'none' : 'black'); break
        case 'w': case 'W': case ',': setBlackout((value) => value === 'white' ? 'none' : 'white'); break
        case 'g': case 'G': setOverview((value) => !value); break
        case 'l': case 'L': setLaser((value) => !value); break
        case 'f': case 'F': toggleFullscreen(); break
        case 'p': case 'P': openPresenter(); break
        case 'Escape':
          if (shortcuts.open) { shortcuts.close(); return }
          if (overview) { setOverview(false); return }
          if (jump) { setJump(''); return }
          if (document.fullscreenElement) void document.exitFullscreen().catch(() => undefined)
          post({ type: 'exit' })
          onClose()
          break
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [step, total, overview, jump, onClose, post, toggleFullscreen, openPresenter, shortcuts])

  const image = images[index]
  return <div
    ref={stage}
    className={`presentation-mode ${idle && !overview ? 'idle' : ''} ${blackout !== 'none' ? `blackout-${blackout}` : ''}`}
    role="dialog"
    aria-modal="true"
    onMouseMove={(event) => laser && setPointer({ x: event.clientX, y: event.clientY })}
    onClick={(event) => { if (!overview && event.target === event.currentTarget) step(1) }}
  >
    <div className="present-stage" onClick={() => !overview && step(1)}>
      {image
        ? <img src={image} alt={`${index + 1}번 슬라이드`} />
        : <span className="present-loading"><LoaderCircle className="spin" size={26} /></span>}
    </div>

    {laser && <span className="present-laser" style={{ left: pointer.x, top: pointer.y }} aria-hidden="true" />}
    {jump && <div className="present-jump">{jump}<small>Enter</small></div>}

    {overview && <div className="present-overview" role="listbox" aria-label="슬라이드 목록">
      <div className="present-overview-grid">
        {slides.map((slide, position) => (
          <button key={slide.id} type="button" className={position === index ? 'active' : ''}
            onClick={() => { setIndex(position); setOverview(false); setBlackout('none') }}>
            {thumbs[position] ? <img src={thumbs[position]} alt="" /> : <span className="present-thumb-empty" />}
            <em>{position + 1}</em>
          </button>
        ))}
      </div>
    </div>}

    <div className="present-controls">
      <button type="button" disabled={index === 0} onClick={() => step(-1)} title="이전 (←)"><ChevronLeft size={20} /></button>
      <span>{index + 1} <i>/</i> {total}</span>
      <button type="button" disabled={index >= total - 1} onClick={() => step(1)} title="다음 (→)"><ChevronRight size={20} /></button>
      <span className="present-divider" />
      <button type="button" className={overview ? 'active' : ''} onClick={() => setOverview((value) => !value)} title="슬라이드 목록 (G)"><Grid3X3 size={17} /></button>
      <button type="button" className={blackout === 'black' ? 'active' : ''} onClick={() => setBlackout((value) => value === 'black' ? 'none' : 'black')} title="검은 화면 (B)"><Square size={17} /></button>
      <button type="button" className={laser ? 'active' : ''} onClick={() => setLaser((value) => !value)} title="레이저 포인터 (L)"><Pointer size={17} /></button>
      <button type="button" className={presenterOpen ? 'active' : ''} onClick={openPresenter} title="발표자 보기 (P)"><MonitorPlay size={17} /></button>
      <button type="button" onClick={toggleFullscreen} title="전체 화면 (F)">{fullscreen ? <Minimize2 size={17} /> : <Maximize2 size={17} />}</button>
      <button type="button" className={shortcuts.open ? 'active' : ''} onClick={() => shortcuts.setOpen((value) => !value)} title="단축키 (?)"><Keyboard size={17} /></button>
      <button type="button" onClick={() => { post({ type: 'exit' }); onClose() }} title="종료 (ESC)"><X size={18} /></button>
    </div>

    <ShortcutSheet open={shortcuts.open} onClose={shortcuts.close} groups={presentationShortcuts} title="발표 중 단축키" />
  </div>
}

/** The presenter's own screen: notes, what is next, and the clock. */
export function PresenterScreen({ presentationId, slides, version }: {
  presentationId: string
  slides: Slide[]
  version: string | number
}) {
  const [state, setState] = useState<PresentState>({ index: 0, total: slides.length, blackout: 'none', startedAt: Date.now(), title: '' })
  const [now, setNow] = useState(Date.now())
  const [paused, setPaused] = useState(false)
  const [offset, setOffset] = useState(0)
  const images = useSlideImages(presentationId, state.total || slides.length, version, state.index, 900)

  const post = usePresentChannel(presentationId, (message) => {
    if (message.type === 'state') setState(message)
    // The talk is over: the presenter's window has no reason to outlive it.
    if (message.type === 'exit') window.close()
  })
  useEffect(() => { post({ type: 'hello' }) }, [post])
  useEffect(() => {
    const timer = window.setInterval(() => { if (!paused) setNow(Date.now()) }, 500)
    return () => window.clearInterval(timer)
  }, [paused])
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      switch (event.key) {
        case 'ArrowRight': case 'PageDown': case ' ': case 'Enter': event.preventDefault(); post({ type: 'step', delta: 1 }); break
        case 'ArrowLeft': case 'PageUp': event.preventDefault(); post({ type: 'step', delta: -1 }); break
        case 'Escape': post({ type: 'exit' }); window.close(); break
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [post])

  const current = slides[state.index]
  const next = slides[state.index + 1]
  const clock = useMemo(() => new Date(now).toLocaleTimeString('ko-KR', { hour: '2-digit', minute: '2-digit' }), [now])

  return <main className="presenter-screen">
    <header>
      <div>
        <span className="eyebrow">발표자 보기</span>
        <strong>{state.title || '프레젠테이션'}</strong>
      </div>
      <div className="presenter-clock">
        <button type="button" onClick={() => setPaused((value) => !value)} title={paused ? '타이머 재개' : '타이머 일시정지'}>
          {elapsed(state.startedAt + offset, paused ? state.startedAt + offset : now)}
        </button>
        <button type="button" className="ghost" onClick={() => setOffset(Date.now() - state.startedAt)} title="타이머 초기화">초기화</button>
        <span>{clock}</span>
      </div>
    </header>
    <section className="presenter-body">
      <div className="presenter-current">
        <div className="presenter-frame">
          {images[state.index] ? <img src={images[state.index]} alt={`${state.index + 1}번 슬라이드`} /> : <span className="present-thumb-empty" />}
        </div>
        <div className="presenter-step">
          <button type="button" disabled={state.index === 0} onClick={() => post({ type: 'step', delta: -1 })}><ChevronLeft size={18} /> 이전</button>
          <span>{state.index + 1} / {state.total || slides.length}</span>
          <button type="button" disabled={state.index >= (state.total || slides.length) - 1} onClick={() => post({ type: 'step', delta: 1 })}>다음 <ChevronRight size={18} /></button>
        </div>
      </div>
      <div className="presenter-side">
        <div className="presenter-next">
          <span>다음 슬라이드</span>
          <div className="presenter-frame small">
            {next
              ? images[state.index + 1] ? <img src={images[state.index + 1]} alt="다음 슬라이드" /> : <span className="present-thumb-empty" />
              : <p className="presenter-end">마지막 슬라이드입니다</p>}
          </div>
          {next && <strong>{next.title}</strong>}
        </div>
        <div className="presenter-notes">
          <span>발표 노트</span>
          {current?.speakerNotes
            ? <p>{current.speakerNotes}</p>
            : <p className="presenter-empty">이 슬라이드에는 발표 노트가 없습니다. 편집기의 노트 탭에서 적어 두면 여기에 보입니다.</p>}
        </div>
      </div>
    </section>
    <footer>← → 로 넘기고 ESC 로 종료합니다. 이 창은 발표자만 봅니다.</footer>
  </main>
}
