import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { MouseEvent as ReactMouseEvent } from 'react'
import {
  ChevronLeft, ChevronRight, Grid3X3, Keyboard, LoaderCircle, Maximize2, Minimize2, MonitorPlay, Pointer, Square, X,
} from 'lucide-react'
import { api } from '../api/client'
import { ShortcutSheet, presentationShortcuts, useShortcutSheet } from './Shortcuts'
import type { Slide } from '../types'
import { showPositions } from '../pages/editor/model/slides'

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
/** No slides to draw: one array, so a closed overview does not refetch. */
const noPositions: number[] = []

export function useSlideImages(presentationId: string, positions: number[], version: string | number, index: number, width = 1600, whole = false) {
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
    // The neighbours are enough for the slide on the wall. The overview is the
    // other case: it is the grid a speaker opens to find a slide to jump to, so
    // every slide has to be in it — asking for the neighbours drew three
    // pictures and left the rest of the deck as empty grey boxes with a number.
    // Nearest first, because that is where the speaker is looking.
    const wanted = whole
      ? positions.map((_, at) => at).sort((a, b) => Math.abs(a - index) - Math.abs(b - index))
      : [index, index + 1, index - 1, index + 2].filter((at) => at >= 0 && at < positions.length)
    void (async () => {
      for (const position of wanted) {
        const key = `${position}`
        if (cache.current.has(key)) continue
        try {
          const url = await api.slidePreview(presentationId, positions[position], width)
          if (!active) { URL.revokeObjectURL(url); return }
          cache.current.set(key, url)
          setImages((current) => ({ ...current, [position]: url }))
        } catch { /* a slide that will not draw shows as an empty frame */ }
      }
    })()
    return () => { active = false }
  }, [presentationId, positions.join(','), index, version, width, whole]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => () => { for (const url of cache.current.values()) URL.revokeObjectURL(url) }, [])
  return images
}

/**
 * The drawings for presenting: a picture, unless there is something to click.
 *
 * A slide with a link on it has to be drawn by the page, because nothing inside
 * an <img> can ever be clicked. Drawing it by the page is not free: one slide
 * carrying a photograph is most of a megabyte of base64 inside its markup, and
 * holding four of those as strings — this slide and the ones either side —
 * costs what an image the browser decodes once does not. So the rule is what a
 * reader would say it is: a slide is a picture until it has a link in it.
 *
 * Either way it is one request for the same drawing.
 */
export function useSlideDrawings(presentationId: string, positions: number[], version: string | number, index: number, width = 1600) {
  const [drawings, setDrawings] = useState<Record<number, { markup?: string; url?: string }>>({})
  const known = useRef<Map<number, { markup?: string; url?: string }>>(new Map())
  const release = () => {
    for (const drawing of known.current.values()) if (drawing.url) URL.revokeObjectURL(drawing.url)
    known.current = new Map()
  }
  useEffect(() => {
    release()
    setDrawings({})
  }, [presentationId, version, width])
  useEffect(() => {
    let active = true
    const wanted = [index, index + 1, index - 1, index + 2].filter((at) => at >= 0 && at < positions.length)
    void (async () => {
      for (const position of wanted) {
        if (known.current.has(position)) continue
        try {
          const markup = await api.slidePreviewMarkup(presentationId, positions[position], width)
          const drawing = markup.includes('<a href=')
            ? { markup }
            : { url: URL.createObjectURL(new Blob([markup], { type: 'image/svg+xml' })) }
          // Kept even if the slide moved on while this was in flight: it is the
          // drawing of a slide that is still in the deck, and the next time it
          // is wanted there is nothing to fetch.
          known.current.set(position, drawing)
          setDrawings((current) => ({ ...current, [position]: drawing }))
          if (!active) return
        } catch { /* a slide that will not draw shows as an empty frame */ }
      }
    })()
    return () => { active = false }
  }, [presentationId, positions.join(','), index, version, width]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => release, [])
  return drawings
}

/**
 * What the presenter's window remembers between talks.
 *
 * The target length and the size of the notes belong to the person, not to the
 * deck: somebody who needs bigger notes needs them on every deck, and the
 * twenty minutes they are given is usually the same twenty minutes next week.
 * Storage can be refused outright — a private window, a locked-down browser —
 * so every read and write is allowed to fail into the default.
 */
function remembered(key: string, fallback: number) {
  try {
    const held = Number(window.localStorage.getItem(key))
    return Number.isFinite(held) && held > 0 ? held : fallback
  } catch { return fallback }
}

function remember(key: string, value: number) {
  try { window.localStorage.setItem(key, String(value)) } catch { /* nothing to do about it */ }
}

/**
 * How far behind (positive) or ahead (negative) a talk is, in seconds.
 *
 * The schedule is where the deck should be by now, and where the deck is is
 * measured from the slide the speaker is on. The half is the point: a slide is
 * not a moment, it is a stretch of time the speaker is somewhere inside, and
 * reading it from its start told a speaker keeping perfect time that they were
 * late — further behind with every second, right up to the moment they pressed
 * the arrow and it snapped back to on-time, having learnt nothing about their
 * pace. Over a talk kept exactly to plan the old reading ran from zero to two
 * minutes late and never once said ahead; from the middle of the slide it runs
 * a minute either side of zero, which is what being on time looks like.
 */
export function pacingSeconds(spentSeconds: number, index: number, total: number, targetMinutes: number) {
  if (targetMinutes <= 0 || total <= 0) return null
  const at = Math.min(Math.max(index, 0), total - 1)
  return Math.round(spentSeconds - ((at + 0.5) / total) * targetMinutes * 60)
}

/** How far behind (positive) or ahead (negative) the talk is, said in words. */
export function pacing(seconds: number) {
  const off = Math.abs(seconds)
  if (off < 45) return { tone: 'on', text: '예정대로' }
  const minutes = Math.round(off / 60)
  const said = minutes >= 1 ? `${minutes}분` : `${off}초`
  return seconds > 0 ? { tone: 'late', text: `${said} 늦음` } : { tone: 'early', text: `${said} 이름` }
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
  /**
   * Which slide of the deck each slide of the show is.
   *
   * A show is not the deck: a slide marked 발표에서 건너뛰기 stays in the deck
   * and is taken out of the show, so the fourth slide the room sees can be the
   * fifth slide of the file. Everything that draws a slide asks the server for
   * it by its number in the deck, so the show has to carry those numbers rather
   * than count its own — counting its own drew the skipped slide, mislabelled
   * every slide after it, and left the last slide of the deck unreachable.
   */
  const positions = useMemo(() => showPositions(slides), [slides])
  const drawings = useSlideDrawings(presentationId, positions, version, index)
  const thumbs = useSlideImages(presentationId, overview ? positions : noPositions, version, index, 320, true)

  // A list handed to a room whole is read ahead of the speaker. A slide marked
  // !build gives up its points one at a time, and the arrow that would have
  // moved on brings the next line instead — until there are none left, when it
  // moves on as it always did.
  const [revealed, setRevealed] = useState(0)
  // Which way the talk was going when it left the last slide: stepping back
  // into a built slide arrives at its last line, where it was left, rather than
  // making the speaker click through it again.
  const heading = useRef(1)
  const built = (slide?: Slide) => Boolean(slide?.built)
  // The points a slide gives out: its body's paragraphs, less the lead sentence
  // that introduces them, which is not a point and is on the wall from the start.
  const pointsOn = (slide?: Slide) => {
    const fields = slide?.fields || {}
    for (const [slot, paragraphs] of Object.entries(fields)) {
      if (slot === 'title' || slot === 'subtitle') continue
      const points = paragraphs.filter((paragraph) => !(paragraph as { lead?: boolean }).lead).length
      if (points > 0) return points
    }
    return (slide?.bullets || []).length
  }
  useEffect(() => {
    const here = slides[index]
    if (!built(here)) { setRevealed(0); return }
    setRevealed(heading.current < 0 ? Math.max(1, pointsOn(here)) : 1)
  }, [index]) // eslint-disable-line react-hooks/exhaustive-deps

  const step = useCallback((delta: number) => {
    setBlackout('none')
    const here = slides[index]
    const points = pointsOn(here)
    if (built(here) && delta > 0 && revealed < points) { setRevealed(revealed + 1); return }
    if (built(here) && delta < 0 && revealed > 1) { setRevealed(revealed - 1); return }
    heading.current = delta < 0 ? -1 : 1
    setIndex((value) => Math.min(total - 1, Math.max(0, value + delta)))
  }, [total, index, revealed, slides]) // eslint-disable-line react-hooks/exhaustive-deps

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

  // While a slide is being built, its drawing is asked for again with the lines
  // said so far; the finished slide is the one the hook already holds.
  const [building, setBuilding] = useState<{ index: number; reveal: number; markup: string } | null>(null)
  useEffect(() => {
    const here = slides[index]
    if (!built(here)) { setBuilding(null); return }
    let active = true
    // Asked for even when every line is showing: the drawing with the last
    // point revealed is the whole slide, and handing back to the hook's copy
    // mid-talk is a flicker nobody needs to see.
    api.slidePreviewMarkup(presentationId, positions[index], 1600, Math.max(1, revealed))
      .then((markup) => { if (active) setBuilding({ index, reveal: revealed, markup }) })
      .catch(() => { if (active) setBuilding(null) })
    return () => { active = false }
  }, [presentationId, index, revealed, slides]) // eslint-disable-line react-hooks/exhaustive-deps

  const drawn = building && building.index === index && building.reveal === revealed
    ? { markup: building.markup }
    : drawings[index]

  /**
   * A click on a link in the slide. A jump goes to the slide it names; anything
   * else opens where it points, and neither one advances the deck — clicking a
   * link and losing your place is not what anybody meant by it.
   */
  const followLink = (event: ReactMouseEvent<HTMLDivElement>) => {
    const link = (event.target as HTMLElement | null)?.closest?.('a[href]') as HTMLAnchorElement | null
    if (!link) return
    event.stopPropagation()
    const href = link.getAttribute('href') || ''
    const jumped = href.match(/^#slide-(\d+)$/)
    if (!jumped) return
    event.preventDefault()
    // The link names a slide of the deck. If that slide is being skipped there
    // is nowhere to land on it, so the show goes to the next one it is giving.
    const named = Number(jumped[1])
    const at = positions.findIndex((position) => position >= named)
    if (at === -1) return
    setIndex(at)
    setBlackout('none')
  }
  return <div
    ref={stage}
    className={`presentation-mode ${idle && !overview ? 'idle' : ''} ${blackout !== 'none' ? `blackout-${blackout}` : ''}`}
    role="dialog"
    aria-modal="true"
    onMouseMove={(event) => laser && setPointer({ x: event.clientX, y: event.clientY })}
    onClick={(event) => { if (!overview && event.target === event.currentTarget) step(1) }}
  >
    <div className="present-stage" onClick={() => !overview && step(1)}>
      {drawn?.markup
        ? <div
            className="present-slide"
            role="img"
            aria-label={`${index + 1}번 슬라이드`}
            onClickCapture={followLink}
            dangerouslySetInnerHTML={{ __html: drawn.markup }}
          />
        : drawn?.url
          ? <img src={drawn.url} alt={`${index + 1}번 슬라이드`} />
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
  // A talk has a length somebody agreed to. Knowing the clock is not the same
  // as knowing whether you are behind, and the speaker cannot do that division
  // while talking.
  const [target, setTarget] = useState(() => remembered('ptium.presenter.target', 0))
  // Notes are read from a metre away, standing, in a dim room.
  const [notesScale, setNotesScale] = useState(() => remembered('ptium.presenter.notes', 100))
  const [showAll, setShowAll] = useState(false)
  // The same deck numbers the presenting window is drawing from. This window
  // holds no position of its own, so if it indexed a different list from the
  // one on the projector the two would disagree about which slide is up — which
  // is exactly what happened while this window was handed the whole deck.
  const positions = useMemo(() => showPositions(slides), [slides])
  const images = useSlideImages(presentationId, positions, version, state.index, 900)

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

  useEffect(() => { remember('ptium.presenter.target', target) }, [target])
  useEffect(() => { remember('ptium.presenter.notes', notesScale) }, [notesScale])

  const current = slides[state.index]
  const next = slides[state.index + 1]
  const total = state.total || slides.length
  const spent = Math.max(0, (paused ? state.startedAt + offset : now) - (state.startedAt + offset))
  // Behind or ahead against where the deck should be by now: a twenty-minute
  // talk of ten slides should be halfway through slide five at nine minutes.
  // It is the only arithmetic a speaker cannot do while speaking.
  const pace = pacingSeconds(spent / 1000, state.index, total, target)
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
        {pace !== null && <span className={`presenter-pace ${pacing(pace).tone}`} title={`목표 ${target}분 · ${state.index + 1}/${total}장`}>
          {pacing(pace).text}
        </span>}
        <label className="presenter-target" title="발표에 주어진 시간">
          <input type="number" min={0} max={240} value={target || ''} placeholder="목표"
            onChange={(event) => setTarget(Math.max(0, Math.min(240, Number(event.target.value) || 0)))} />
          <span>분</span>
        </label>
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
        <div className="presenter-notes" style={{ fontSize: `${notesScale}%` }}>
          <span>
            발표 노트
            <span className="presenter-notes-size">
              <button type="button" onClick={() => setNotesScale((value) => Math.max(80, value - 15))}
                disabled={notesScale <= 80} title="노트 글자 작게" aria-label="노트 글자 작게">가−</button>
              <button type="button" onClick={() => setNotesScale((value) => Math.min(200, value + 15))}
                disabled={notesScale >= 200} title="노트 글자 크게" aria-label="노트 글자 크게">가+</button>
            </span>
          </span>
          {current?.speakerNotes
            ? <p>{current.speakerNotes}</p>
            : <p className="presenter-empty">이 슬라이드에는 발표 노트가 없습니다. 편집기의 노트 탭에서 적어 두면 여기에 보입니다.</p>}
        </div>
      </div>
    </section>
    {/* Stepping one slide at a time is fine while the talk runs to plan. When
        somebody asks about the number on slide eleven, pressing the arrow six
        times in front of a room is the worst part of presenting. */}
    {showAll && <section className="presenter-grid" aria-label="모든 슬라이드">
      {slides.map((slide, index) => <button key={slide.id || index} type="button"
        className={index === state.index ? 'current' : ''}
        onClick={() => { post({ type: 'go', index }); setShowAll(false) }}>
        <span className="presenter-grid-frame">
          {images[index] ? <img src={images[index]} alt="" /> : <span className="present-thumb-empty" />}
        </span>
        <small>{index + 1}. {slide.title || '제목 없음'}</small>
      </button>)}
    </section>}
    <footer>
      <span>← → 로 넘기고 ESC 로 종료합니다. 이 창은 발표자만 봅니다.</span>
      <button type="button" className="presenter-all" onClick={() => setShowAll((value) => !value)}>
        {showAll ? '닫기' : '모든 슬라이드'}
      </button>
    </footer>
  </main>
}
