import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  AlertTriangle, ArrowLeft, Check, ChevronDown, ChevronLeft, ChevronRight, CircleAlert, Code2,
  Copy, Download, FileText, History, Image, LayoutPanelTop, LoaderCircle, MonitorPlay, Plus, RotateCcw, Trash2, WandSparkles, X,
} from 'lucide-react'
import { api, ApiError, bodySlots, primaryBodySlot, textToParagraphs, type DeckFinding } from '../api/client'
import { BrandMark } from '../branding/BrandContext'
import { AssetLibrary, type Asset } from '../components/AssetLibrary'
import { FreeformCanvas } from '../components/FreeformCanvas'
import { GridLibrary } from '../components/GridLibrary'
import { SlidePreview } from '../components/SlidePreview'
import { Button, EmptyState, ErrorState, LoadingState, Modal, Select, Textarea } from '../components/UI'
import { useToast } from '../components/Toast'
import { navigate } from '../router'
import type {
  Presentation, PresentationRevision, Slide, SlideBlock, SlideElement, SlideParagraph, SlotFrame, SlotStyle,
  Template, TemplateLayout,
} from '../types'
import { displayError, relativeDate } from '../utils'
import { roleLabel } from './TemplatesPage'

const MAX_SLIDES = 50
const defaultSlide = (order: number, layoutId?: string): Slide => ({
  id: `new-${crypto.randomUUID()}`, order, layout: 'content', layoutId,
  title: '새로운 슬라이드', body: '핵심 메시지를 입력하세요.', bullets: ['핵심 메시지를 입력하세요.'],
  fields: { title: [{ text: '새로운 슬라이드' }], body: [{ text: '핵심 메시지를 입력하세요.' }] },
  elements: [],
})

const slideBody = (slide?: Slide) => slide?.body || slide?.bullets?.join('\n') || ''
/** What a slide holds besides prose, named in the workspace's language. */
interface SlideHolding { slot: string; kind: 'block' | 'image' | 'element'; label: string; detail: string }

function slideHoldings(slide?: Slide): SlideHolding[] {
  if (!slide) return []
  const holdings: SlideHolding[] = Object.entries(slide.blocks || {}).map(([slot, block]) => ({
    slot, kind: 'block', label: blockLabel(String(block.kind)),
    detail: String(block.caption || block.heading || '') || `${(block.items?.length ?? block.rows?.length ?? 0)}개 항목`,
  }))
  for (const [slot, image] of Object.entries(slide.images || {})) {
    holdings.push({ slot, kind: 'image', label: '이미지', detail: String(image.name || image.caption || slot) })
  }
  if ((slide.elements || []).length > 0) {
    holdings.push({ slot: 'freeform', kind: 'element', label: '자유 배치 개체', detail: `${slide.elements!.length}개` })
  }
  return holdings.sort((a, b) => a.slot.localeCompare(b.slot))
}

/** blockLabel names a component the way the source language does. */
function blockLabel(kind: string) {
  switch (kind) {
    case 'kpi': return '핵심 지표'
    case 'hero': return '대표 숫자'
    case 'steps': return '단계'
    case 'timeline': return '타임라인'
    case 'comparison': return '비교'
    case 'columnChart': return '세로 막대 차트'
    case 'barChart': return '가로 막대 차트'
    case 'lineChart': return '추이 차트'
    case 'shareBar': return '비중 바'
    case 'meter': return '달성률'
    case 'table': return '표'
    case 'quote': return '인용'
    case 'callout': return '강조'
    case 'grid': return '격자'
    case 'bullets': return '목록'
  }
  return kind
}
const slideBodyLines = (slide?: Slide) => slideBody(slide).split(/\r?\n/).map((line) => line.trim()).filter(Boolean)

/** The slots a component or an image occupies. A slot holds one thing. */
function drawnSlots(slide: Slide) {
  return new Set([...Object.keys(slide.blocks || {}), ...Object.keys(slide.images || {})])
}

/**
 * proseSlot is the slot the body textarea writes to: the first body slot no
 * drawing occupies. Writing prose into a component's slot would put two things in
 * one place, and the server keeps whichever it decides — silently losing one.
 */
function proseSlot(slide: Slide, layout?: TemplateLayout) {
  const drawn = drawnSlots(slide)
  const free = bodySlots(slide.fields).filter((slot) => !drawn.has(slot))
  if (free.length > 0) return free[0]
  const fromLayout = layout?.placeholders.find((placeholder) =>
    placeholder.kind === 'text' && placeholder.slot !== 'title' && placeholder.slot !== 'subtitle' && !drawn.has(placeholder.slot))
  return fromLayout?.slot || primaryBodySlot(slide, layout)
}

/**
 * Rebuilds the template fields for a slide from the edited title and body,
 * leaving every other slot the generator filled untouched.
 */
function slideFields(slide: Slide, layout?: TemplateLayout) {
  const fields: Record<string, { text: string; level?: number }[]> = { ...(slide.fields || {}) }
  const bodySlot = proseSlot(slide, layout)
  if (slide.title.trim()) fields.title = [{ text: slide.title.trim() }]
  else delete fields.title
  if (slide.subtitle?.trim() && (fields.subtitle || layout?.placeholders.some((placeholder) => placeholder.slot === 'subtitle'))) {
    fields.subtitle = [{ text: slide.subtitle.trim() }]
  }
  const paragraphs = textToParagraphs(slideBody(slide))
  if (paragraphs.length > 0) fields[bodySlot] = paragraphs
  else delete fields[bodySlot]
  // Slots the chosen layout does not expose would be dropped by the server
  // anyway; removing them here keeps the editor state honest.
  if (layout) {
    const allowed = new Set(layout.placeholders.filter((placeholder) => placeholder.kind === 'text').map((placeholder) => placeholder.slot))
    for (const slot of Object.keys(fields)) if (!allowed.has(slot)) delete fields[slot]
  }
  return fields
}

/**
 * Rebuilds the prose the text editors show from one slot's paragraphs.
 *
 * The slot is passed in rather than guessed: saving writes the body textarea back
 * to the slide's prose slot, so reading it from a different slot would copy one
 * region's words over another's on the next save.
 */
function bodyFromFields(fields: Record<string, SlideParagraph[]>, slot: string) {
  const bullets = (fields[slot] || []).map((paragraph) => `${'  '.repeat(paragraph.level || 0)}${paragraph.text}`)
  return { body: bullets.join('\n'), bullets }
}

/** The same, from text the canvas just typed into the prose slot. */
function bodyFromText(text: string) {
  return { body: text, bullets: text.split(/\r?\n/).map((line) => line.trim()).filter(Boolean) }
}

function toApiSlides(slides: Slide[], layouts: TemplateLayout[]) {
  return slides.map((slide, index) => {
    const layout = layouts.find((candidate) => candidate.id === slide.layoutId)
    return {
      id: slide.id.startsWith('new-') ? undefined : slide.id,
      position: index + 1,
      title: slide.title,
      subtitle: slide.subtitle,
      speakerNotes: slide.speakerNotes,
      layout: slide.layout,
      layoutId: slide.layoutId || '',
      content: {
        type: 'template',
        layoutId: slide.layoutId || '',
        fields: slideFields(slide, layout),
        // The drawings the generator made are the deck's design. They travel with
        // every save; a save that only carried text used to delete them.
        blocks: slide.blocks || {},
        images: slide.images || {},
        elements: slide.elements || [],
        // Where the author dragged a template region, and how they set its type.
        // Empty maps are the deck sitting exactly as its template puts it.
        frames: slide.frames || {},
        styles: slide.styles || {},
        bullets: slideBodyLines(slide),
        accent: slide.accent,
      },
    }
  })
}

// findingLabel names a measured defect in the workspace's language.
function findingLabel(kind: string) {
  switch (kind) {
    case 'overflow': return '텍스트 넘침'
    case 'outside': return '슬라이드 밖으로 나감'
    case 'collision': return '겹침'
    case 'contrast': return '대비 부족'
    case 'orphan': return '줄 끝에 한 음절만 남음'
    case 'density': return '한 장에 너무 많음'
    case 'notes': return '발표 노트 없음'
    case 'repeat': return '같은 말을 두 번 함'
  }
  return kind
}

/**
 * The measurement, in the workspace's language.
 *
 * The API states findings in English, because that is what an API and a log
 * should say. A person reading their own deck should not have to. Anything this
 * does not recognise is shown as the server wrote it, so a new measurement is
 * never swallowed.
 */
const componentNames: Record<string, string> = {
  kpi: '핵심 지표', hero: '대표 숫자', steps: '단계', timeline: '타임라인', comparison: '비교',
  columnChart: '세로 막대 차트', barChart: '가로 막대 차트', lineChart: '추이 차트', shareBar: '비중 바',
  meter: '달성률', table: '표', quote: '인용', callout: '강조', grid: '격자', bullets: '목록',
  text: '텍스트', component: '컴포넌트', picture: '이미지',
}
const named = (value: string) => componentNames[value] || value

function findingDetail(detail: string) {
  const rules: [RegExp, (match: RegExpMatchArray) => string][] = [
    [/^(\w+) region extends ([\d.]+)cm past the slide edge$/,
      (m) => `${named(m[1])} 영역이 슬라이드 밖으로 ${m[2]}cm 나갔습니다`],
    [/^(\d+) lines of text in room for (\d+); it must shrink to (\d+)% of the template's size$/,
      (m) => `${m[2]}줄 자리에 ${m[1]}줄이 들어가 템플릿 크기의 ${m[3]}%로 줄여야 합니다`],
    [/^(\d+) lines of text in room for (\d+); it does not fit even at (\d+)%$/,
      (m) => `${m[2]}줄 자리에 ${m[1]}줄이라 ${m[3]}%로 줄여도 들어가지 않습니다`],
    [/^(\w+) overlaps (\w+) by (\d+)%$/, (m) => `${named(m[1])}이 ${m[2]} 영역과 ${m[3]}% 겹칩니다`],
    [/^text covers (\d+)% of the layout's own (.+)$/, (m) => `글이 템플릿 자체의 ${m[2]}를 ${m[1]}% 덮습니다`],
    [/^text (\w+) on (\w+) is ([\d.]+):1, below 4\.5:1$/,
      (m) => `글자색 #${m[1]}과 배경 #${m[2]}의 대비가 ${m[3]}:1로, 기준 4.5:1에 못 미칩니다`],
    [/^(\d+) points on one slide; past (\d+) an audience reads instead of listening$/,
      (m) => `한 장에 요점이 ${m[1]}개입니다. ${m[2]}개를 넘으면 듣지 않고 읽습니다`],
    [/^the region is (\d+)% full; a slide needs room to breathe$/,
      (m) => `영역이 ${m[1]}% 찼습니다. 슬라이드에는 여백이 필요합니다`],
    [/^the same point twice: "(.+)" and "(.+)"$/, (m) => `같은 말을 두 번 합니다: "${m[1]}"와 "${m[2]}"`],
    [/^no speaker notes: .+$/, () => '발표 노트가 없습니다. 이 슬라이드에서 무엇을 말할지 적혀 있지 않습니다'],
    [/^the last line holds (\d+)% of a line; .+$/,
      (m) => `마지막 줄에 한 줄의 ${m[1]}%만 남았습니다. 조금 줄이거나 고쳐 쓰면 사라집니다`],
    [/^(\w+) had too little room to draw anything$/, (m) => `${named(m[1])}을 그릴 자리가 없었습니다`],
    [/^(\w+) draws "(.+)" ([\d.]+)cm taller than the room it reserved$/,
      (m) => `${named(m[1])}의 "${m[2]}"가 확보한 자리보다 ${m[3]}cm 큽니다`],
    [/^two lines of the (\w+) overlap$/, (m) => `${named(m[1])}의 두 줄이 서로 겹칩니다`],
    [/^(\w+) draws ([\d.]+)cm past the slide edge$/, (m) => `${named(m[1])}이 슬라이드 밖으로 ${m[2]}cm 나갔습니다`],
    [/^(\w+) draws ([\d.]+)cm outside its region$/, (m) => `${named(m[1])}이 자기 영역 밖으로 ${m[2]}cm 나갔습니다`],
  ]
  for (const [pattern, write] of rules) {
    const match = detail.match(pattern)
    if (match) return write(match)
  }
  return detail
}

function revisionReason(reason: string) {
  switch (reason) {
    case 'edit': return '자동 편집 체크포인트'
    case 'source': return '코드 적용 전'
    case 'generation': return '재생성 전'
    case 'restore': return '버전 복원 전'
  }
  return reason
}

export function EditorPage({ id }: { id: string }) {
  const [presentation, setPresentation] = useState<Presentation | null>(null)
  const [slides, setSlides] = useState<Slide[]>([])
  const [template, setTemplate] = useState<Template | null>(null)
  const [templates, setTemplates] = useState<Template[]>([])
  const [activeId, setActiveId] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [dirty, setDirty] = useState(false)
  const [saving, setSaving] = useState(false)
  const [lastSaved, setLastSaved] = useState<Date | null>(null)
  // Rendered slides come from the server, so they are only worth refetching when
  // the server's copy changed. Every save and every applied source bumps this.
  const [railVersion, setRailVersion] = useState(0)
  const [savedSlideCount, setSavedSlideCount] = useState(0)
  const [deckFindings, setDeckFindings] = useState<DeckFinding[] | null>(null)
  const [findingsOpen, setFindingsOpen] = useState(false)
	const [historyOpen, setHistoryOpen] = useState(false)
	const [historyLoading, setHistoryLoading] = useState(false)
	const [history, setHistory] = useState<PresentationRevision[]>([])
	const [restoringRevision, setRestoringRevision] = useState('')
	const [conflictOpen, setConflictOpen] = useState(false)
	const [conflictKind, setConflictKind] = useState<'canvas' | 'source'>('canvas')
  const [panel, setPanel] = useState<'content' | 'design' | 'notes' | 'images' | 'grids'>('content')
  const [canvasMode, setCanvasMode] = useState<'edit' | 'preview' | 'source'>('edit')
  // The deck as text. It is the same deck the canvas shows: applying it
  // recompiles the slides, and opening it reads them back out.
  const [source, setSource] = useState('')
  const [sourceLoaded, setSourceLoaded] = useState(false)
  const [sourceBusy, setSourceBusy] = useState(false)
  const [sourceWarnings, setSourceWarnings] = useState<string[]>([])
  const [sourceFindings, setSourceFindings] = useState<DeckFinding[]>([])
  const [sourceSlide, setSourceSlide] = useState(1)
  const [sourcePreview, setSourcePreview] = useState<{ url: string; slide: number; count: number } | null>(null)
  const [sourcePreviewError, setSourcePreviewError] = useState('')
  const [presenting, setPresenting] = useState(false)
  const [presentIndex, setPresentIndex] = useState(0)
  const [exportOpen, setExportOpen] = useState(false)
  const [exporting, setExporting] = useState(false)
  const [retrying, setRetrying] = useState(false)
  const [editVersion, setEditVersion] = useState(0)
  const saveTimer = useRef<number | null>(null)
  const revision = useRef(0)
  const savePromise = useRef<Promise<boolean> | null>(null)
  const editorState = useRef({ presentation, slides, dirty })
  editorState.current = { presentation, slides, dirty }
  const layoutsRef = useRef<TemplateLayout[]>([])
  layoutsRef.current = template?.layouts || []
  const { showToast } = useToast()
  const markEdited = () => { revision.current += 1; setEditVersion((value) => value + 1); setSourceLoaded(false) }

  // A presenter's hands are on the arrow keys, not on the footer buttons.
  useEffect(() => {
    if (!presenting) return
    const onKey = (event: KeyboardEvent) => {
      switch (event.key) {
        case 'ArrowRight': case 'PageDown': case ' ': case 'Enter':
          event.preventDefault()
          setPresentIndex((value) => Math.min(slides.length - 1, value + 1))
          break
        case 'ArrowLeft': case 'PageUp':
          event.preventDefault()
          setPresentIndex((value) => Math.max(0, value - 1))
          break
        case 'Home': setPresentIndex(0); break
        case 'End': setPresentIndex(Math.max(0, slides.length - 1)); break
        case 'Escape': setPresenting(false); break
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [presenting, slides.length])

  // The measurement follows the saved deck, not the keystrokes.
  useEffect(() => {
    if (railVersion === 0 || savedSlideCount === 0) return
    let active = true
    api.inspectPresentation(id)
      .then((result) => { if (active) setDeckFindings(result.findings) })
      .catch(() => { if (active) setDeckFindings(null) })
    return () => { active = false }
  }, [id, railVersion, savedSlideCount])

  const load = useCallback(async () => {
    setLoading(true); setError('')
    try {
      const data = await api.presentation(id)
      setPresentation(data); setSlides(data.slides || []); setActiveId(data.slides?.[0]?.id || '')
			setDirty(false)
			setSourceLoaded(false)
      setSavedSlideCount((data.slides || []).length)
      // What the deck looks like once drawn, so a finished deck says whether it is
      // actually finished rather than only that generation ended.
      if ((data.slides || []).length > 0) {
        api.inspectPresentation(id).then((result) => setDeckFindings(result.findings)).catch(() => setDeckFindings(null))
      }
      if (data.templateId) {
        api.template(data.templateId).then(setTemplate).catch(() => setTemplate(null))
      }
      api.templates().then(setTemplates).catch(() => setTemplates([]))
    } catch (err) { setError(displayError(err)) } finally { setLoading(false) }
  }, [id])
  useEffect(() => { void load() }, [load])

  useEffect(() => {
    if (presentation?.status !== 'generating') return
    const interval = window.setInterval(async () => {
      try { const data = await api.presentation(id); setPresentation(data); if (data.slides?.length) { setSlides(data.slides); setActiveId((current) => data.slides!.some((slide) => slide.id === current) ? current : data.slides![0].id) } } catch { /* polling resumes */ }
    }, 3000)
    return () => window.clearInterval(interval)
  }, [id, presentation?.status])

  const save = useCallback((): Promise<boolean> => {
    if (savePromise.current) return savePromise.current
    if (!editorState.current.presentation || !editorState.current.dirty) return Promise.resolve(true)
    const operation = (async () => {
      setSaving(true)
      try {
        while (editorState.current.presentation && editorState.current.dirty) {
          const snapshot = editorState.current
          const snapshotPresentation = snapshot.presentation
          if (!snapshotPresentation) break
          const snapshotRevision = revision.current
          const updated = await api.updatePresentation(id, {
            title: snapshotPresentation.title,
            theme: snapshotPresentation.theme,
            templateId: snapshotPresentation.templateId,
						version: snapshotPresentation.version,
            ...(snapshot.slides.length > 0 ? { slides: toApiSlides(snapshot.slides, layoutsRef.current) } : {}),
          })
						if (snapshotRevision !== revision.current) {
							// A keystroke landed while this request was in flight. The server
							// accepted the older snapshot, so carry its new version forward while
							// keeping the newer local slides for the next loop iteration.
							const current = editorState.current
							if (current.presentation) {
								const rebased = { ...current.presentation, version: updated.version, updatedAt: updated.updatedAt }
								editorState.current = { ...current, presentation: rebased }
								setPresentation(rebased)
							}
							continue
						}
          const updatedSlides = updated.slides?.length ? updated.slides : snapshot.slides
          const merged = { ...snapshotPresentation, ...updated, slides: updatedSlides }
          editorState.current = { presentation: merged, slides: updatedSlides, dirty: false }
          setPresentation(merged)
          setSlides(updatedSlides)
          setActiveId((current) => updatedSlides.some((slide) => slide.id === current) ? current : updatedSlides[0]?.id || '')
          setDirty(false)
          setLastSaved(new Date())
          setSavedSlideCount(updatedSlides.length)
          setRailVersion((value) => value + 1)
        }
				return true
			} catch (err) {
				if (err instanceof ApiError && err.status === 409) {
					setConflictKind('canvas')
					setConflictOpen(true)
				}
				throw err
      } finally {
        setSaving(false)
        savePromise.current = null
      }
    })()
    savePromise.current = operation
    return operation
  }, [id])

  useEffect(() => {
    if (!dirty) return
    if (saveTimer.current) window.clearTimeout(saveTimer.current)
    saveTimer.current = window.setTimeout(() => { void save().catch((err) => showToast(`저장하지 못했습니다: ${displayError(err)}`, 'error')) }, 1000)
    return () => { if (saveTimer.current) window.clearTimeout(saveTimer.current) }
  }, [dirty, editVersion, save, showToast])

  useEffect(() => {
    const warnBeforeUnload = (event: BeforeUnloadEvent) => {
      if (!editorState.current.dirty && !savePromise.current) return
      event.preventDefault()
      event.returnValue = ''
    }
    window.addEventListener('beforeunload', warnBeforeUnload)
    return () => window.removeEventListener('beforeunload', warnBeforeUnload)
  }, [])

  useEffect(() => () => {
    if (saveTimer.current) window.clearTimeout(saveTimer.current)
    if (editorState.current.dirty) void save().catch(() => { /* A full-page exit is guarded by beforeunload. */ })
  }, [save])

  const activeIndex = Math.max(0, slides.findIndex((slide) => slide.id === activeId))
  const active = slides[activeIndex]
  const activeHoldings = useMemo(() => slideHoldings(active), [active])
  const defects = (deckFindings || []).filter((finding) => !finding.advisory)
  const advisories = (deckFindings || []).filter((finding) => finding.advisory)
  const updateSlide = (slideId: string, updates: Partial<Slide>) => {
    markEdited()
    setSlides((current) => current.map((slide) => slide.id === slideId ? { ...slide, ...updates } : slide)); setDirty(true)
  }
  const updateActive = (updates: Partial<Slide>) => {
    if (!active) return
    updateSlide(activeId, updates)
  }
  const activeLayout = useMemo(() => (template?.layouts || []).find((layout) => layout.id === active?.layoutId), [template, active?.layoutId])

  // ── Editing what the generator wrote ───────────────────────────────────────
  // The canvas edits the slide's own regions. Every change lands in the same
  // slide state the text editors use, so the two views never disagree.
  /**
   * One undo stack for everything the canvas does.
   *
   * Objects had their own history and the regions had none, so dragging a title
   * by accident could not be taken back — and Ctrl+Z quietly undid something
   * else instead. A checkpoint is the whole slide before a change; a run of the
   * same kind of change inside a second is one step, so typing is not thirty.
   */
  const undoStack = useRef<{ slideId: string; slide: Slide; reason: string; at: number }[]>([])
  const redoStack = useRef<{ slideId: string; slide: Slide }[]>([])
  const [historyDepth, setHistoryDepth] = useState({ undo: 0, redo: 0 })
  const trackDepth = () => setHistoryDepth({ undo: undoStack.current.length, redo: redoStack.current.length })

  const checkpoint = useCallback((reason: string) => {
    const slide = editorState.current.slides.find((candidate) => candidate.id === activeId)
    if (!slide) return
    const top = undoStack.current[undoStack.current.length - 1]
    const now = Date.now()
    if (top && top.slideId === slide.id && top.reason === reason && now - top.at < 900) {
      top.at = now
      return
    }
    undoStack.current = [...undoStack.current.slice(-59), { slideId: slide.id, slide, reason, at: now }]
    redoStack.current = []
    trackDepth()
  }, [activeId])

  const stepHistory = (from: typeof undoStack, to: typeof redoStack) => {
    const entry = from.current.pop()
    if (!entry) return
    const current = editorState.current.slides.find((candidate) => candidate.id === entry.slideId)
    if (current) to.current.push({ slideId: entry.slideId, slide: current })
    setActiveId(entry.slideId)
    updateSlide(entry.slideId, entry.slide)
    trackDepth()
  }
  const undoCanvas = () => stepHistory(undoStack, redoStack as unknown as typeof undoStack)
  const redoCanvas = () => {
    const entry = redoStack.current.pop()
    if (!entry) return
    const current = editorState.current.slides.find((candidate) => candidate.id === entry.slideId)
    if (current) undoStack.current.push({ slideId: entry.slideId, slide: current, reason: 'redo', at: Date.now() })
    setActiveId(entry.slideId)
    updateSlide(entry.slideId, entry.slide)
    trackDepth()
  }

  /**
   * Keeps the body textarea's slot and its text in step with the regions.
   *
   * Saving writes that textarea back to the slide's prose slot, and which slot
   * that is depends on what the other regions hold. Editing one region can move
   * it — filling an empty region, or turning a region into a component — so the
   * prose is re-read from wherever it will be written, or one region's words end
   * up overwriting another's on the next save.
   */
  const withProse = (next: Slide, updates: Partial<Slide>, typed?: { slot: string; text: string }) => {
    const slot = proseSlot(next, activeLayout)
    return {
      ...updates,
      ...(typed && typed.slot === slot ? bodyFromText(typed.text) : bodyFromFields(next.fields || {}, slot)),
    }
  }

  const writeRegionText = (slot: string, text: string) => {
    if (!active) return
    checkpoint(`text:${slot}`)
    const paragraphs = textToParagraphs(text)
    const fields = { ...(active.fields || {}) }
    if (paragraphs.length > 0) fields[slot] = paragraphs
    else delete fields[slot]
    const updates: Partial<Slide> = { fields }
    if (slot === 'title') {
      // A slide must keep a title: emptying the region would fail the save, and
      // failing a save while someone types is worse than keeping the old words.
      if (paragraphs[0]?.text) updates.title = paragraphs[0].text
      else fields.title = active.fields?.title || [{ text: active.title }]
    } else if (slot === 'subtitle') {
      updates.subtitle = text.trim()
    }
    updateActive(withProse({ ...active, fields }, updates, { slot, text }))
  }
  const writeRegionBlock = (slot: string, block: SlideBlock) => {
    if (!active) return
    checkpoint(`block:${slot}`)
    const blocks = { ...(active.blocks || {}), [slot]: block }
    updateActive(withProse({ ...active, blocks }, { blocks }))
  }
  const clearRegion = (slot: string) => {
    if (!active) return
    checkpoint(`clear:${slot}`)
    const fields = { ...(active.fields || {}) }
    const blocks = { ...(active.blocks || {}) }
    const images = { ...(active.images || {}) }
    delete fields[slot]; delete blocks[slot]; delete images[slot]
    const updates: Partial<Slide> = { fields, blocks, images }
    if (slot === 'subtitle') updates.subtitle = ''
    updateActive(withProse({ ...active, fields, blocks, images }, updates))
  }
  const writeRegionFrames = (frames: Record<string, SlotFrame>) => {
    checkpoint('frames')
    updateActive({ frames })
  }
  /** Type a slide sets for one region: size, colour, weight, alignment. */
  const writeRegionStyle = (slot: string, patch: SlotStyle | null) => {
    if (!active) return
    checkpoint(`style:${slot}`)
    const styles = { ...(active.styles || {}) }
    if (patch === null) delete styles[slot]
    else {
      const merged = { ...(styles[slot] || {}), ...patch }
      for (const [key, value] of Object.entries(merged)) {
        if (value === undefined) delete (merged as Record<string, unknown>)[key]
      }
      if (Object.keys(merged).length === 0) delete styles[slot]
      else styles[slot] = merged
    }
    updateActive({ styles })
  }

  const revisionSnapshot = useRef<Slide | null>(null)
  const [canUndoRevise, setCanUndoRevise] = useState(false)
  const reviseSlideAt = async (index: number, input: { action: string; instruction: string; slot: string }) => {
    const slide = slides[index]
    if (!slide || !presentation) return
    // The model is asked to revise what the server has, so anything typed a
    // moment ago is part of the draft it works from.
    if (editorState.current.dirty) await save()
    try {
      const result = await api.reviseSlide(id, index + 1, input)
      const current = editorState.current.slides.find((candidate) => candidate.id === slide.id) || slide
      revisionSnapshot.current = current
      setCanUndoRevise(true)
      const fields = result.slide.fields
      const written: Slide = { ...current, fields, blocks: result.slide.blocks, images: result.slide.images }
      const layout = (template?.layouts || []).find((candidate) => candidate.id === (result.slide.layoutId || current.layoutId))
      updateSlide(current.id, {
        title: result.slide.title || current.title,
        subtitle: result.slide.subtitle,
        speakerNotes: result.slide.speakerNotes ?? current.speakerNotes,
        layoutId: result.slide.layoutId || current.layoutId,
        fields,
        blocks: result.slide.blocks,
        images: result.slide.images,
        accent: result.slide.accent || current.accent,
        ...bodyFromFields(fields, proseSlot(written, layout)),
      })
      const trouble = result.findings.filter((finding) => !finding.advisory).length
      showToast(trouble > 0
        ? `AI가 다시 썼습니다. 아직 맞지 않는 부분이 ${trouble}곳 있습니다.`
        : result.warnings.length > 0 ? `AI가 다시 썼습니다. ${result.warnings[0]}` : 'AI가 이 슬라이드를 다시 썼습니다.')
    } catch (err) {
      showToast(err instanceof ApiError && err.status === 503
        ? '이 배포에는 AI 공급자가 설정되어 있지 않습니다. 관리자 설정에서 연결하세요.'
        : displayError(err))
    }
  }
  const reviseActiveSlide = (input: { action: string; instruction: string; slot: string }) => reviseSlideAt(activeIndex, input)
  const undoRevise = () => {
    const snapshot = revisionSnapshot.current
    if (!snapshot) return
    markEdited()
    setSlides((current) => current.map((slide) => slide.id === snapshot.id ? snapshot : slide))
    setDirty(true)
    setCanUndoRevise(false)
    revisionSnapshot.current = null
  }

  const contentLayoutId = useMemo(() => {
    const layouts = template?.layouts || []
    return (layouts.find((layout) => layout.role === 'content') || layouts[0])?.id
  }, [template])
  const addSlide = () => { if (slides.length >= MAX_SLIDES) return; markEdited(); const next = defaultSlide(slides.length + 1, contentLayoutId); setSlides((current) => [...current, next]); setActiveId(next.id); setDirty(true) }
  const duplicateSlide = () => {
    if (!active || slides.length >= MAX_SLIDES) return
    markEdited()
    const groups = new Map<string, string>()
    const elements = (active.elements || []).map((element) => {
      let groupId = element.groupId
      if (groupId) {
        if (!groups.has(groupId)) groups.set(groupId, `group-${crypto.randomUUID()}`)
        groupId = groups.get(groupId)
      }
      return { ...element, id: `element-${crypto.randomUUID()}`, groupId }
    })
    const next = {
      ...active, id: `new-${crypto.randomUUID()}`, order: slides.length + 1, title: `${active.title} 복사본`,
      fields: structuredClone(active.fields || {}), blocks: structuredClone(active.blocks || {}), images: structuredClone(active.images || {}), elements,
    }
    setSlides((current) => [...current, next]); setActiveId(next.id); setDirty(true)
  }
  const removeSlide = () => { if (!active || slides.length <= 1) return; markEdited(); const next = slides.filter((slide) => slide.id !== active.id).map((slide, index) => ({ ...slide, order: index + 1 })); setSlides(next); setActiveId(next[Math.min(activeIndex, next.length - 1)]?.id || ''); setDirty(true) }
  const openSource = async () => {
    setCanvasMode('source')
    if (dirty) {
      // The source is read from the stored deck, so unsaved edits are written first.
      try { await save() } catch { /* the source falls back to the saved deck */ }
    }
    if (sourceLoaded) return
    setSourceBusy(true)
    try {
      const loaded = await api.presentationSource(id)
      setSource(loaded.source)
      setSourceLoaded(true)
    } catch (err) { showToast(displayError(err), 'error') } finally { setSourceBusy(false) }
  }

  // The slide is drawn from the text as it is typed, a moment after typing stops.
  // Nothing is stored: this renders the compiled source directly.
  useEffect(() => {
    if (canvasMode !== 'source' || !source.trim()) return
    let active = true
    let url = ''
    const timer = window.setTimeout(() => {
      void api.sourcePreview(id, source, sourceSlide)
        .then((result) => {
          if (!active) { URL.revokeObjectURL(result.url); return }
          url = result.url
          setSourcePreview((current) => { if (current) URL.revokeObjectURL(current.url); return { url: result.url, slide: sourceSlide, count: result.slideCount } })
          setSourcePreviewError('')
        })
        .catch((err) => { if (active) setSourcePreviewError(displayError(err)) })
    }, 500)
    return () => { active = false; window.clearTimeout(timer); if (url) URL.revokeObjectURL(url) }
  }, [canvasMode, source, sourceSlide, id])

  // Writing `::image <name>` is the other half of uploading one, so the library
  // hands the reference to the code editor rather than making anyone retype it.
  const insertImageReference = (name: string) => {
    const directive = `::image ${name}`
    if (canvasMode !== 'source') {
      void navigator.clipboard?.writeText(directive).catch(() => { /* the toast still tells them what to write */ })
      showToast(`${directive} 을 복사했습니다. 코드 탭에 붙여 넣으세요.`)
      return
    }
    setSource((current) => {
      const separator = current.length === 0 || current.endsWith('\n') ? '' : '\n'
      return current + separator + directive + '\n'
    })
    showToast(`${directive} 을 코드 끝에 넣었습니다.`)
  }

  const insertGridReference = (name: string) => {
    const directive = `::grid ${name}\n- 항목 | 담당 A | 담당 B\n- 첫 번째 활동 | R | C\n::`
    if (canvasMode !== 'source') {
      void navigator.clipboard?.writeText(directive).catch(() => { /* the toast still says what to write */ })
      showToast(`::grid ${name} 예시를 복사했습니다. 코드 탭에 붙여 넣으세요.`)
      return
    }
    setSource((current) => {
      const separator = current.length === 0 || current.endsWith('\n') ? '' : '\n'
      return current + separator + directive + '\n'
    })
    showToast(`::grid ${name} 예시를 코드 끝에 넣었습니다.`)
  }

  const applySource = async (dryRun: boolean) => {
    setSourceBusy(true)
    try {
      const result = await api.applyPresentationSource(id, source, dryRun, presentation?.version)
      setSourceWarnings(result.warnings)
      setSourceFindings(result.findings)
      if (dryRun) {
        // A slide drawn wrong and a slide left unfinished are different news.
        const defects = result.findings.filter((finding) => !finding.advisory).length
        const advisories = result.findings.length - defects
        const slides = `${result.slideCount ?? 0}장으로 컴파일됩니다`
        showToast(defects > 0
          ? `${slides}. 검사에서 ${defects}건이 나왔습니다.`
          : advisories > 0
            ? `${slides}. 그려지는 데는 문제가 없고, 다듬을 곳 ${advisories}건이 있습니다.`
            : `${slides}. 검사 통과.`,
          defects > 0 ? 'error' : 'success')
        return
      }
      if (result.presentation) {
        setPresentation(result.presentation)
        const compiled = result.presentation.slides || []
			editorState.current = { presentation: result.presentation, slides: compiled, dirty: false }
        setSlides(compiled)
        setActiveId(compiled[0]?.id || '')
        setDirty(false)
        setLastSaved(new Date())
        setSavedSlideCount(compiled.length)
        setRailVersion((value) => value + 1)
			setSourceLoaded(true)
      }
      showToast('코드를 적용했습니다.')
      setCanvasMode('preview')
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        setConflictKind('source')
        setConflictOpen(true)
      }
      showToast(displayError(err), 'error')
    } finally { setSourceBusy(false) }
  }

  const moveSlide = (direction: -1 | 1) => { const nextIndex = activeIndex + direction; if (nextIndex < 0 || nextIndex >= slides.length) return; markEdited(); const next = [...slides]; [next[activeIndex], next[nextIndex]] = [next[nextIndex], next[activeIndex]]; setSlides(next.map((slide, index) => ({ ...slide, order: index + 1 }))); setDirty(true) }

	const canSafelyFix = (finding: DeckFinding) => {
		if (finding.kind === 'notes') return Boolean(slides[finding.slide - 1])
		if (finding.kind !== 'density' && finding.kind !== 'overflow') return false
		return slides.length < MAX_SLIDES && slideBodyLines(slides[finding.slide - 1]).length >= 4
	}

	// What cannot be fixed without rewriting words is handed to the model, with
	// the measurement attached: "0.9cm too tall" is something a rewrite can aim at.
	const [aiFixing, setAiFixing] = useState(0)
	const fixFindingWithAI = async (finding: DeckFinding) => {
		const target = slides[finding.slide - 1]
		if (!target || aiFixing) return
		setActiveId(target.id)
		setFindingsOpen(false)
		setAiFixing(finding.slide)
		try {
			await reviseSlideAt(finding.slide - 1, {
				action: finding.kind === 'notes' ? 'notes' : finding.kind === 'repeat' ? 'shorten' : 'fit',
				instruction: `측정 결과: ${finding.detail}`,
				slot: finding.slot || '',
			})
		} finally {
			setAiFixing(0)
		}
	}

	// Every measured defect, handed to the model one slide at a time.
	//
	// A deck is judged as a deck, so fixing them one by one from the list is the
	// tedious half of the same job. Slides are taken in order and each is a
	// separate undo step, so a rewrite that went wrong can be taken back alone.
	const [sweeping, setSweeping] = useState({ done: 0, total: 0 })
	const fixEverythingWithAI = async () => {
		const targets = [...new Set((deckFindings || []).filter((finding) => !canSafelyFix(finding))
			.map((finding) => finding.slide))].sort((first, second) => first - second)
		if (targets.length === 0 || sweeping.total > 0) return
		setFindingsOpen(false)
		setSweeping({ done: 0, total: targets.length })
		try {
			for (const [index, slide] of targets.entries()) {
				const worst = (deckFindings || []).find((finding) => finding.slide === slide && !canSafelyFix(finding))
				setSweeping({ done: index, total: targets.length })
				await reviseSlideAt(slide - 1, {
					action: worst?.kind === 'repeat' ? 'shorten' : worst?.kind === 'notes' ? 'notes' : 'fit',
					instruction: worst ? `측정 결과: ${worst.detail}` : '',
					slot: worst?.slot || '',
				})
			}
			showToast(`${targets.length}장을 AI로 다시 썼습니다. 결과를 다시 측정합니다.`)
		} finally {
			setSweeping({ done: 0, total: 0 })
		}
	}

	// Safe fixes never discard content: missing notes receive a draft, while a
	// crowded prose slide is split and every line moves to one of the two slides.
	const safelyFixFinding = (finding: DeckFinding) => {
		const index = finding.slide - 1
		const target = slides[index]
		if (!target || !canSafelyFix(finding)) return
		markEdited()
		if (finding.kind === 'notes') {
			const lead = target.subtitle || slideBodyLines(target)[0] || target.title
			setSlides((current) => current.map((slide, slideIndex) => slideIndex === index
				? { ...slide, speakerNotes: `${slide.title}: ${lead}`.slice(0, 4000) }
				: slide))
			setActiveId(target.id)
			setDirty(true)
			setFindingsOpen(false)
			showToast('발표 노트 초안을 추가했습니다.')
			return
		}
		const lines = slideBodyLines(target)
		const splitAt = Math.ceil(lines.length / 2)
		const firstLines = lines.slice(0, splitAt)
		const secondLines = lines.slice(splitAt)
		const continuation: Slide = {
			...target,
			id: `new-${crypto.randomUUID()}`,
			order: index + 2,
			title: `${target.title} (계속)`.slice(0, 200),
			body: secondLines.join('\n'),
			bullets: secondLines,
			fields: {},
			blocks: {},
			images: {},
			elements: [],
			speakerNotes: target.speakerNotes ? `${target.speakerNotes} (계속)` : undefined,
		}
		const next = [
			...slides.slice(0, index),
			{ ...target, body: firstLines.join('\n'), bullets: firstLines },
			continuation,
			...slides.slice(index + 1),
		].map((slide, slideIndex) => ({ ...slide, order: slideIndex + 1 }))
		setSlides(next)
		setActiveId(continuation.id)
		setDirty(true)
		setFindingsOpen(false)
		showToast('내용을 버리지 않고 두 슬라이드로 나눴습니다.')
	}

	const openHistory = async () => {
		setHistoryOpen(true)
		setHistoryLoading(true)
		try {
			await save()
			setHistory(await api.presentationRevisions(id))
		} catch (err) { showToast(displayError(err), 'error') } finally { setHistoryLoading(false) }
	}

	const restoreRevision = async (checkpoint: PresentationRevision) => {
		setRestoringRevision(checkpoint.id)
		try {
			const restored = await api.restorePresentationRevision(id, checkpoint.id)
			const restoredSlides = restored.slides || []
			editorState.current = { presentation: restored, slides: restoredSlides, dirty: false }
			setPresentation(restored)
			setSlides(restoredSlides)
			setActiveId(restoredSlides[0]?.id || '')
			setDirty(false)
			setSourceLoaded(false)
			setSavedSlideCount(restoredSlides.length)
			setRailVersion((value) => value + 1)
			setLastSaved(new Date())
			setHistory(await api.presentationRevisions(id))
			if (restored.templateId) api.template(restored.templateId).then(setTemplate).catch(() => setTemplate(null))
			api.inspectPresentation(id).then((result) => setDeckFindings(result.findings)).catch(() => setDeckFindings(null))
			showToast(`버전 ${checkpoint.version}의 내용으로 복원했습니다.`)
		} catch (err) { showToast(displayError(err), 'error') } finally { setRestoringRevision('') }
	}

	const useServerVersion = async () => {
		setConflictOpen(false)
		setDirty(false)
		editorState.current = { ...editorState.current, dirty: false }
		if (conflictKind === 'source') {
			setCanvasMode('edit')
			setSourceLoaded(false)
		}
		await load()
		setConflictKind('canvas')
		showToast('서버에 저장된 최신 버전을 불러왔습니다.')
	}

	const keepLocalVersion = async () => {
		const kind = conflictKind
		if (kind === 'source') setSourceBusy(true)
		try {
			const latest = await api.presentation(id)
			if (kind === 'source') {
				const result = await api.applyPresentationSource(id, source, false, latest.version)
				if (!result.presentation) throw new Error('코드를 적용한 프레젠테이션을 받지 못했습니다.')
				const retained = result.presentation
				const retainedSlides = retained.slides || []
				setSourceWarnings(result.warnings)
				setSourceFindings(result.findings)
				editorState.current = { presentation: retained, slides: retainedSlides, dirty: false }
				setPresentation(retained)
				setSlides(retainedSlides)
				setActiveId(retainedSlides[0]?.id || '')
				setDirty(false)
				setSourceLoaded(true)
				setSavedSlideCount(retainedSlides.length)
				setRailVersion((value) => value + 1)
				setLastSaved(new Date())
				setCanvasMode('preview')
				if (retained.templateId) api.template(retained.templateId).then(setTemplate).catch(() => setTemplate(null))
				setConflictOpen(false)
				setConflictKind('canvas')
				showToast('내 코드를 최신 버전 위에 적용했습니다.')
				return
			}
			const local = editorState.current.presentation
			if (!local) return
			const rebased = { ...local, version: latest.version }
			editorState.current = { presentation: rebased, slides: editorState.current.slides, dirty: true }
			setPresentation(rebased)
			setDirty(true)
			markEdited()
			setConflictOpen(false)
			showToast('내 변경을 최신 버전 위에 다시 저장합니다.')
		} catch (err) { showToast(displayError(err), 'error') } finally {
			if (kind === 'source') setSourceBusy(false)
		}
	}
  // Switching template keeps each slide's narrative role and rebinds it to the
  // equivalent layout in the new design, so content survives the change.
  const chooseTemplate = async (next: Template) => {
    if (!presentation || next.id === presentation.templateId) return
    try {
      const detail = next.layouts ? next : await api.template(next.id)
      const layouts = detail.layouts || []
      markEdited()
      setTemplate(detail)
      setPresentation((current) => current ? { ...current, templateId: detail.id, templateName: detail.name, theme: detail.paletteKey || current.theme } : current)
      setSlides((current) => current.map((slide) => {
        const role = template?.layouts?.find((layout) => layout.id === slide.layoutId)?.role || slide.layout
        const replacement = layouts.find((layout) => layout.role === role) || layouts.find((layout) => layout.role === 'content') || layouts[0]
        return replacement ? { ...slide, layoutId: replacement.id, layout: String(replacement.role) } : slide
      }))
      setDirty(true)
      showToast(`"${detail.name}" 템플릿으로 전환했습니다.`)
    } catch (err) { showToast(displayError(err), 'error') }
  }

  const chooseLayout = (layout: TemplateLayout) => updateActive({ layoutId: layout.id, layout: String(layout.role) })

  // A region waiting for a picture. Choosing one then fills that region rather
  // than dropping a floating image on top of the slide.
  const [imageTarget, setImageTarget] = useState('')
  const pickImageFor = (slot: string) => {
    setImageTarget(slot)
    setPanel('images')
    showToast('이미지를 고르면 선택한 영역에 넣습니다.')
  }

  const placeImage = (asset: Asset) => {
    if (!active) return
    if (imageTarget) {
      checkpoint(`image:${imageTarget}`)
      const fields = { ...(active.fields || {}) }
      const blocks = { ...(active.blocks || {}) }
      delete fields[imageTarget]; delete blocks[imageTarget]
      const images = { ...(active.images || {}), [imageTarget]: { assetId: asset.id, name: asset.name, caption: asset.name } }
      updateActive(withProse({ ...active, fields, blocks, images }, { fields, blocks, images }))
      setImageTarget('')
      setCanvasMode('edit')
      showToast(`${asset.name}을 ${imageTarget} 영역에 넣었습니다.`)
      return
    }
    const ratio = asset.width > 0 && asset.height > 0 ? asset.width / asset.height : 16 / 9
    const width = 30
    const height = Math.min(45, Math.max(8, width * (16 / 9) / ratio))
    const highest = Math.max(0, ...(active.elements || []).map((element) => element.zIndex || 0))
    const image: SlideElement = {
      id: `element-${crypto.randomUUID()}`, kind: 'image', assetId: asset.id, name: asset.name,
      caption: asset.name, x: 35, y: Math.max(5, (100 - height) / 2), width, height, zIndex: highest + 1,
      rotation: 0, opacity: 100, fit: 'cover',
    }
    updateActive({ elements: [...(active.elements || []), image] })
    setCanvasMode('edit')
    showToast(`${asset.name}을 현재 슬라이드에 배치했습니다.`)
  }

  const exportDeck = async (format: 'pptx' | 'pdf') => {
    setExporting(true)
    try {
      if (dirty) await save()
      const blob = await api.exportPresentation(id, format)
      const url = URL.createObjectURL(blob); const anchor = document.createElement('a'); anchor.href = url; anchor.download = `${presentation?.title || 'ptium'}.${format}`; anchor.click(); URL.revokeObjectURL(url)
      showToast(`${format.toUpperCase()} 파일을 다운로드했습니다.`); setExportOpen(false)
    } catch (err) { showToast(displayError(err), 'error') } finally { setExporting(false) }
  }

  const retryGeneration = async () => {
    setRetrying(true)
    try { setPresentation(await api.retryPresentation(id)) } catch (err) { showToast(displayError(err), 'error') } finally { setRetrying(false) }
  }

  const leaveEditor = async () => {
    try {
      await save()
      navigate('/presentations')
    } catch (err) {
      showToast(`저장하지 못해 편집기를 닫지 않았습니다: ${displayError(err)}`, 'error')
    }
  }

  if (loading) return <main className="editor-loading"><BrandMark /><LoadingState label="프레젠테이션을 준비하는 중…" /></main>
  if (error || !presentation) return <main className="editor-loading"><ErrorState message={error || '프레젠테이션을 찾을 수 없습니다.'} onRetry={() => void load()} /><Button variant="secondary" onClick={() => navigate('/presentations')}>목록으로 돌아가기</Button></main>
  if (presentation.status === 'generating') return <main className="editor-loading generation-wait"><div className="generation-visual small"><div className={`generating-slide theme-${presentation.theme || 'aurora'}`}><span>PTIUM ENGINE</span><div><i /><i /><i /></div><strong>{presentation.title}</strong><em /></div></div><h1>슬라이드를 생성하고 있어요</h1><p>완성되는 대로 자동으로 편집기를 열어드릴게요.</p><LoaderCircle className="spin" size={22} /></main>
  if (presentation.status === 'failed') return <main className="editor-loading"><CircleAlert size={30} /><h1>슬라이드 생성에 실패했습니다</h1><p>{presentation.errorMessage || '관리자에게 오류 센터의 요청 기록을 확인해 달라고 요청하세요.'}</p><div><Button variant="secondary" onClick={() => navigate('/presentations')}>목록으로 돌아가기</Button> <Button disabled={retrying} onClick={() => void retryGeneration()}>{retrying ? '다시 등록 중…' : '생성 다시 시도'}</Button></div></main>

  return (
    <main className="editor-page">
      <header className="editor-header">
        <div className="editor-header-left"><button className="icon-button" aria-label="저장하고 프레젠테이션 목록으로 이동" onClick={() => void leaveEditor()}><ArrowLeft size={18} /></button><span className="editor-brand"><BrandMark size="tiny" /></span><span className="header-divider" /><input className="deck-title-input" maxLength={200} value={presentation.title} onChange={(event) => { markEdited(); setPresentation({ ...presentation, title: event.target.value }); setDirty(true) }} aria-label="프레젠테이션 제목" /></div>
        <div className="editor-history"><button
            className={`deck-state ${defects.length > 0 ? 'has-defects' : advisories.length > 0 ? 'has-advisories' : 'clean'}`}
            onClick={() => setFindingsOpen(true)}
            disabled={deckFindings === null}
            title="그려진 슬라이드를 측정한 결과입니다"
          >{sweeping.total > 0
            ? <><LoaderCircle className="spin" size={13} /> AI 수정 {sweeping.done + 1}/{sweeping.total}</>
            : deckFindings === null
            ? <><LoaderCircle className="spin" size={13} /> 측정 중</>
            : defects.length > 0
              ? <><CircleAlert size={13} /> 결함 {defects.length}</>
              : advisories.length > 0
                ? <><AlertTriangle size={13} /> 다듬을 곳 {advisories.length}</>
                : <><Check size={13} /> 결함 없음</>}
          </button><button className="save-status" disabled={saving || !dirty} onClick={() => void save().catch((err) => showToast(`저장하지 못했습니다: ${displayError(err)}`, 'error'))}>{saving ? <><LoaderCircle className="spin" size={13} /> 저장 중</> : dirty ? <><CircleAlert size={13} /> 지금 저장</> : <><Check size={13} /> {lastSaved ? '저장됨' : '모든 변경 저장됨'}</>}</button></div>
		<div className="editor-actions"><Button variant="ghost" size="small" onClick={() => void openHistory()}><History size={16} /> 버전 이력</Button><Button variant="ghost" size="small" disabled={slides.length === 0} onClick={() => { setPresentIndex(0); setPresenting(true) }}><MonitorPlay size={16} /> 발표</Button><Button variant="secondary" size="small" disabled={slides.length === 0} onClick={() => setExportOpen(true)}><Download size={16} /> 내보내기 <ChevronDown size={14} /></Button></div>
      </header>

      <div className="editor-workspace">
        <aside className="slide-rail">
          <div className="slide-rail-head"><strong>슬라이드</strong><span>{slides.length} / {MAX_SLIDES}</span></div>
          <div className="slide-list">{slides.map((slide, index) => {
            const holdings = slideHoldings(slide)
            const drawn = !slide.id.startsWith('new-') && index < savedSlideCount
            return <button key={slide.id} className={`slide-thumbnail-row ${activeId === slide.id ? 'active' : ''}`} onClick={() => setActiveId(slide.id)}>
              <span className="slide-number">{index + 1}</span>
              <div className="slide-thumbnail">
                {/* The template's own drawing, not an approximation of it. */}
                {drawn
                  ? <SlidePreview cacheKey={`${id}-rail-${index}-${railVersion}`} alt={`${index + 1}번 슬라이드`} load={() => api.slidePreview(id, index + 1, 260)} />
                  : <div className="slide-thumbnail-pending"><span>{slide.title || '제목 없음'}</span><small>저장하면 그려집니다</small></div>}
              </div>
              <span className="slide-thumbnail-meta">
                <strong>{slide.title || '제목 없음'}</strong>
                {holdings.length > 0 && <em>{holdings.map((holding) => holding.label).join(' · ')}</em>}
              </span>
            </button>
          })}</div>
          <button className="add-slide-button" disabled={slides.length >= MAX_SLIDES} title={slides.length >= MAX_SLIDES ? `최대 ${MAX_SLIDES}장까지 편집할 수 있습니다.` : undefined} onClick={addSlide}><Plus size={15} /> {slides.length >= MAX_SLIDES ? '최대 슬라이드 수 도달' : '슬라이드 추가'}</button>
        </aside>

        <section className="canvas-area">
          <div className="canvas-toolbar"><div className="canvas-mode-switch">
            <button className={canvasMode === 'edit' ? 'active' : ''} onClick={() => setCanvasMode('edit')}>편집</button>
            <button className={canvasMode === 'preview' ? 'active' : ''} onClick={() => { if (dirty) void save().catch(() => { /* the preview falls back to the saved state */ }); setCanvasMode('preview') }}>템플릿 미리보기</button>
            <button className={canvasMode === 'source' ? 'active' : ''} onClick={() => void openSource()}><Code2 size={13} /> 코드</button>
          </div><div><button className="icon-button small" onClick={() => moveSlide(-1)} disabled={!active || activeIndex === 0} aria-label="왼쪽으로 이동"><ChevronLeft size={16} /></button><button className="icon-button small" onClick={() => moveSlide(1)} disabled={!active || activeIndex >= slides.length - 1} aria-label="오른쪽으로 이동"><ChevronRight size={16} /></button><button className="icon-button small" onClick={duplicateSlide} disabled={!active || slides.length >= MAX_SLIDES} title={slides.length >= MAX_SLIDES ? `최대 ${MAX_SLIDES}장까지 편집할 수 있습니다.` : undefined} aria-label="복제"><Copy size={15} /></button><button className="icon-button small danger-hover" onClick={removeSlide} disabled={!active || slides.length <= 1} aria-label="삭제"><Trash2 size={15} /></button></div></div>
          {active ? <div className={`canvas-stage ${canvasMode === 'edit' ? 'detail-mode' : ''}`}>
            {canvasMode === 'edit' ? <FreeformCanvas
              presentationId={id}
              position={activeIndex + 1}
              slideId={active.id}
              elements={active.elements || []}
              frames={active.frames || {}}
              styles={active.styles || {}}
              baseVersion={`${activeIndex}-${railVersion}`}
              onChange={(elements) => updateActive({ elements })}
              onRegionText={writeRegionText}
              onRegionBlock={writeRegionBlock}
              onRegionFrames={writeRegionFrames}
              onRegionStyle={writeRegionStyle}
              onPickImage={pickImageFor}
              onRegionClear={clearRegion}
              onCheckpoint={checkpoint}
              onUndo={undoCanvas}
              onRedo={redoCanvas}
              canUndo={historyDepth.undo > 0}
              canRedo={historyDepth.redo > 0}
              onRevise={reviseActiveSlide}
              onUndoRevise={undoRevise}
              canUndoRevise={canUndoRevise}
            /> : canvasMode === 'source' ? <div className="source-editor">
              <div className="source-editor-head">
                <div>
                  <strong>덱 소스</strong>
                  <span># 제목 · @cover · &gt; 리드 · - 항목 · ::steps … :: · ::image 이름 형식으로 씁니다. 적용하면 템플릿에 맞춰 다시 그립니다.</span>
                </div>
                <div className="source-editor-actions">
                  <Button variant="ghost" onClick={() => void applySource(true)} disabled={sourceBusy || !source.trim()}>검사</Button>
                  <Button onClick={() => void applySource(false)} disabled={sourceBusy || !source.trim()}>
                    {sourceBusy ? '적용 중…' : '적용'}
                  </Button>
                </div>
              </div>
              <div className="source-editor-body">
              <textarea
                className="source-editor-code"
                spellCheck={false}
                value={source}
                onChange={(event) => setSource(event.target.value)}
                aria-label="덱 소스"
                placeholder={'# 슬라이드 제목\n@content\n> 한 줄 리드\n- 핵심 요점\n::kpi 핵심 지표\n- 전환 대상 | 42개\n::\n::image 로고 | 브랜드 마크'}
              />
              <div className="source-editor-preview">
                {sourcePreview ? <img src={sourcePreview.url} alt={`${sourceSlide}번 슬라이드 미리보기`} /> : <div className="source-editor-preview-empty">{sourcePreviewError || '입력을 멈추면 이 자리에 슬라이드가 그려집니다.'}</div>}
                <div className="source-editor-preview-nav">
                  <button type="button" className="icon-button small" onClick={() => setSourceSlide((current) => Math.max(1, current - 1))} disabled={sourceSlide <= 1} aria-label="이전 슬라이드"><ChevronLeft size={15} /></button>
                  <span>{sourceSlide} / {sourcePreview?.count || 1}</span>
                  <button type="button" className="icon-button small" onClick={() => setSourceSlide((current) => Math.min(sourcePreview?.count || 1, current + 1))} disabled={sourceSlide >= (sourcePreview?.count || 1)} aria-label="다음 슬라이드"><ChevronRight size={15} /></button>
                </div>
              </div>
              </div>
              {(sourceWarnings.length > 0 || sourceFindings.length > 0) && <ul className="source-editor-warnings">
                {sourceWarnings.map((warning) => <li key={warning}><AlertTriangle size={13} /> {warning}</li>)}
                {sourceFindings.map((finding) => (
                  <li key={`${finding.slide}-${finding.slot}-${finding.kind}`}
                    className={finding.advisory ? 'source-editor-advisory' : 'source-editor-finding'}>
                    <AlertTriangle size={13} /> {finding.slide}번 슬라이드 {finding.slot} · {findingLabel(finding.kind)}: {finding.detail}
                  </li>
                ))}
              </ul>}
            </div> : <div className="canvas-template-preview">
              <SlidePreview
                cacheKey={`${id}-${activeIndex}-${lastSaved?.getTime() || 0}`}
                alt={`${active.title} 슬라이드 미리보기`}
                load={() => api.slidePreview(id, activeIndex + 1, 1200)}
              />
              <small>{dirty ? '저장된 내용을 기준으로 그린 미리보기입니다. 방금 수정한 내용은 저장 후 반영됩니다.' : '실제 템플릿의 배치·색·글꼴을 그대로 사용한 미리보기입니다.'}</small>
            </div>}
          </div> : <EmptyState title="슬라이드가 없습니다" description="새 슬라이드를 추가해 편집을 시작하세요." action={<Button onClick={addSlide}><Plus size={15} /> 추가</Button>} />}
        </section>

        <aside className="inspector-panel">
          <div className="inspector-tabs"><button className={panel === 'content' ? 'active' : ''} onClick={() => setPanel('content')}>내용</button><button className={panel === 'design' ? 'active' : ''} onClick={() => setPanel('design')}>디자인</button><button className={panel === 'images' ? 'active' : ''} onClick={() => setPanel('images')}>이미지</button><button className={panel === 'notes' ? 'active' : ''} onClick={() => setPanel('notes')}>노트</button><button className={panel === 'grids' ? 'active' : ''} onClick={() => setPanel('grids')}>격자</button></div>
          {panel === 'content' ? <div className="inspector-content">
            <section className="template-text-fields">
              <div className="inspector-section-head"><strong>템플릿 텍스트</strong><span className="inspector-hint">배경 레이어</span></div>
              <p className="inspector-help">템플릿 슬롯의 글을 편집합니다. 캔버스 위 텍스트 상자는 자유롭게 이동·회전할 수 있습니다.</p>
              <label className="slide-edit-field"><span>제목</span><input disabled={!active} value={active?.title || ''} maxLength={200} onChange={(event) => updateActive({ title: event.target.value })} aria-label="슬라이드 제목" placeholder="슬라이드 제목" /></label>
              <label className="slide-edit-field"><span>리드 문장</span><input disabled={!active} value={active?.subtitle || ''} maxLength={300} onChange={(event) => updateActive({ subtitle: event.target.value })} aria-label="리드 문장" placeholder="제목 아래 한 줄" /></label>
              <label className="slide-edit-field grow"><span>본문 {activeHoldings.some((holding) => holding.kind !== 'element') ? '(컴포넌트 옆 영역)' : ''}</span><Textarea disabled={!active} value={slideBody(active)} onChange={(event) => updateActive({ body: event.target.value, bullets: event.target.value.split(/\r?\n/).map((line) => line.trim()).filter(Boolean) })} className="slide-body-editor" aria-label="슬라이드 본문" placeholder="핵심 메시지를 줄마다 입력하세요." /></label>
              {activeHoldings.length > 0 && <div className="slide-holdings"><strong>슬라이드 개체</strong><ul>{activeHoldings.map((holding) => <li key={holding.slot}>{holding.kind === 'image' ? <Image size={13} /> : <LayoutPanelTop size={13} />}<span>{holding.label}</span><small>{holding.detail}</small></li>)}</ul></div>}
            </section>
          </div> : panel === 'design' ? <div className="inspector-content">
            <section>
              <div className="inspector-section-head"><strong>템플릿</strong>{template && <span className="inspector-hint">{template.layoutCount}개 레이아웃</span>}</div>
              <Select
                aria-label="디자인 템플릿"
                value={presentation.templateId || ''}
                onChange={(event) => {
                  const next = templates.find((candidate) => candidate.id === event.target.value)
                  if (next) void chooseTemplate(next)
                }}
              >
                {!presentation.templateId && <option value="">기본 디자인</option>}
                {templates.map((candidate) => <option key={candidate.id} value={candidate.id}>{candidate.name}{candidate.kind === 'builtin' ? ' (기본)' : ''}</option>)}
              </Select>
              <p className="inspector-help">템플릿을 바꾸면 각 슬라이드가 같은 역할의 레이아웃으로 다시 연결됩니다.</p>
            </section>
            <section>
              <div className="inspector-section-head"><strong>슬라이드 레이아웃</strong></div>
              <p className="inspector-help">{!active ? '레이아웃을 바꾸려면 슬라이드를 먼저 추가하세요.' : (template?.layouts?.length ? '이 템플릿에 정의된 실제 레이아웃입니다. 내보낼 때 그대로 사용됩니다.' : '템플릿 레이아웃 정보를 불러오는 중입니다.')}</p>
              <div className="layout-choice-list">{(template?.layouts || []).map((layout) => (
                <button
                  key={layout.id}
                  disabled={!active}
                  className={active?.layoutId === layout.id ? 'active' : ''}
                  onClick={() => chooseLayout(layout)}
                >
                  <SlidePreview cacheKey={`${template?.id}-${layout.id}-inspector`} alt={`${layout.name} 레이아웃`} load={() => api.templateLayoutPreview(template!.id, layout.id, 260)} />
                  <span><strong>{layout.name}</strong><small>{roleLabel(String(layout.role))} · 슬롯 {layout.placeholders.filter((placeholder) => placeholder.kind === 'text').length}개</small></span>
                  {active?.layoutId === layout.id && <em><Check size={11} /></em>}
                </button>
              ))}</div>
            </section>
          </div> : panel === 'grids' ? <div className="inspector-content">
            <section>
              <strong>격자 정의</strong>
              <p className="inspector-help">RACI·위험 매트릭스처럼 조직 고유의 표를 정의합니다. 소스에서는 <code>::grid 이름</code>으로 부릅니다. 색은 직접 쓰지 않고 역할만 고르므로, 같은 정의가 템플릿마다 그 회사 색으로 나옵니다.</p>
              <GridLibrary onInsert={insertGridReference} notify={showToast} />
            </section>
          </div> : panel === 'images' ? <div className="inspector-content">
            <section>
              <strong>이미지</strong>
              <p className="inspector-help">현재 슬라이드에 자유 배치하거나, 코드의 <code>::image 이름</code>으로 템플릿 슬롯에 넣을 수 있습니다.</p>
              <AssetLibrary onPlace={placeImage} onInsert={insertImageReference} notify={showToast} />
            </section>
          </div> : <div className="inspector-content"><section><strong>발표자 노트</strong><p className="inspector-help">{active ? '슬라이드와 별도로 저장되는 내부 발표 메모입니다.' : '노트를 작성하려면 슬라이드를 먼저 추가하세요.'}</p><Textarea className="notes-editor" disabled={!active} maxLength={4000} value={active?.speakerNotes || ''} onChange={(event) => updateActive({ speakerNotes: event.target.value })} placeholder="이 슬라이드에서 전달할 포인트, 참고할 숫자 등을 기록하세요." /></section></div>}
        </aside>
      </div>

      {presenting && <div className="presentation-mode" role="dialog" aria-modal="true">
        <header><span>{presentation.title}</span><button onClick={() => setPresenting(false)}><X size={20} /> 닫기</button></header>
        {/* The slide as it will be shown, drawn by the same renderer as the export. */}
        <div className="present-render">
          <SlidePreview
            cacheKey={`${id}-present-${presentIndex}-${railVersion}`}
            alt={`${presentIndex + 1}번 슬라이드`}
            load={() => api.slidePreview(id, presentIndex + 1, 1600)}
          />
        </div>
        {slides[presentIndex]?.speakerNotes && <div className="present-notes"><strong>발표 노트</strong><p>{slides[presentIndex]?.speakerNotes}</p></div>}
        <footer>
          <button disabled={presentIndex === 0} onClick={() => setPresentIndex((value) => value - 1)}><ChevronLeft size={22} /></button>
          <span>{presentIndex + 1} / {slides.length}<small>← → 로 이동 · ESC 로 종료</small></span>
          <button disabled={presentIndex === slides.length - 1} onClick={() => setPresentIndex((value) => value + 1)}><ChevronRight size={22} /></button>
        </footer>
      </div>}
      <Modal
        open={findingsOpen}
        onClose={() => setFindingsOpen(false)}
        title="그려진 슬라이드 측정 결과"
        description="결함은 잘못 그려진 것, 다듬을 곳은 제대로 그려졌지만 더 좋아질 수 있는 것입니다."
        footer={<>
          {(deckFindings || []).some((finding) => !canSafelyFix(finding)) && (
            <Button variant="secondary" disabled={sweeping.total > 0} onClick={() => void fixEverythingWithAI()}>
              <WandSparkles size={14} /> 전부 AI로 고치기
            </Button>
          )}
          <Button variant="secondary" onClick={() => setFindingsOpen(false)}>닫기</Button>
        </>}
      >
        {(deckFindings || []).length === 0
          ? <p className="modal-note">모든 슬라이드가 템플릿 안에 제대로 들어갑니다.</p>
          : <ul className="deck-findings">{(deckFindings || []).map((finding) => (
			  <li key={`${finding.slide}-${finding.slot}-${finding.kind}`} className={finding.advisory ? 'advisory' : 'defect'}>
				<button type="button" className="finding-target" onClick={() => { const target = slides[finding.slide - 1]; if (target) setActiveId(target.id); setFindingsOpen(false) }}>
				  <strong>{finding.slide}번 슬라이드</strong>
				  <span>{findingLabel(finding.kind)}</span>
				  <small>{findingDetail(finding.detail)}</small>
				</button>
				{canSafelyFix(finding)
				  ? <button type="button" className="finding-safe-fix" onClick={() => safelyFixFinding(finding)}><WandSparkles size={13} /> 안전 수정</button>
				  : <button type="button" className="finding-safe-fix ai" disabled={aiFixing === finding.slide} onClick={() => void fixFindingWithAI(finding)}>
					  {aiFixing === finding.slide ? <LoaderCircle className="spin" size={13} /> : <WandSparkles size={13} />} AI로 고치기
					</button>}
			  </li>
			))}</ul>}
      </Modal>
		<Modal
			open={historyOpen}
			onClose={() => { if (!restoringRevision) setHistoryOpen(false) }}
			title="버전 이력"
			description="자동 편집은 5분 단위로 묶고, 코드 적용·재생성·복원 전에는 별도 체크포인트를 남깁니다. 복원 직전 상태도 다시 기록됩니다."
			footer={<Button variant="secondary" disabled={Boolean(restoringRevision)} onClick={() => setHistoryOpen(false)}>닫기</Button>}
		>
			{historyLoading ? <LoadingState compact label="버전 이력을 불러오는 중…" /> : history.length === 0 ? <EmptyState icon={<History size={24} />} title="아직 이전 버전이 없습니다" description="첫 변경을 저장하면 복원 가능한 체크포인트가 만들어집니다." /> : <ol className="revision-list">
				<li className="revision-current"><span><Check size={14} /></span><div><strong>현재 버전 {presentation.version}</strong><small>지금 편집 중인 내용</small></div></li>
				{history.map((checkpoint) => <li key={checkpoint.id}>
					<span><History size={14} /></span>
					<div><strong>버전 {checkpoint.version} · {revisionReason(checkpoint.reason)}</strong><small>{checkpoint.slideCount}장 · {relativeDate(checkpoint.createdAt)}</small></div>
					<Button variant="secondary" size="small" disabled={Boolean(restoringRevision)} onClick={() => void restoreRevision(checkpoint)}>{restoringRevision === checkpoint.id ? <LoaderCircle className="spin" size={13} /> : <RotateCcw size={13} />} 복원</Button>
				</li>)}
			</ol>}
		</Modal>
		<Modal
			open={conflictOpen}
			onClose={() => setConflictOpen(false)}
			title="다른 창에서 변경된 내용이 있습니다"
			description={conflictKind === 'source' ? '코드를 조용히 덮어쓰지 않았습니다. 서버의 최신 덱을 불러오거나, 내 코드를 최신 버전 위에 적용할 수 있습니다.' : '현재 편집 내용을 조용히 덮어쓰지 않았습니다. 서버의 최신 내용을 불러오거나, 내 변경을 최신 버전 위에 다시 저장할 수 있습니다.'}
			footer={<><Button variant="secondary" disabled={sourceBusy} onClick={() => void useServerVersion()}>서버 버전 불러오기</Button><Button disabled={sourceBusy} onClick={() => void keepLocalVersion()}>{conflictKind === 'source' ? '내 코드 적용' : '내 변경 유지'}</Button></>}
		>
			<p className="modal-note">두 버전을 모두 보존하려면 먼저 내 변경을 유지한 뒤 버전 이력에서 이전 체크포인트를 확인할 수 있습니다.</p>
		</Modal>
      <Modal open={exportOpen} onClose={() => setExportOpen(false)} title="프레젠테이션 내보내기" description="사용할 형식을 선택하세요." footer={<Button variant="secondary" onClick={() => setExportOpen(false)}>취소</Button>}><div className="export-options"><button disabled={exporting} onClick={() => void exportDeck('pptx')}><span className="export-icon ppt"><FileText size={22} /></span><div><strong>PowerPoint (.pptx)</strong><p>Microsoft PowerPoint와 호환되는 편집 가능한 파일</p></div><Download size={18} /></button><button disabled title="추후 제공 예정"><span className="export-icon pdf"><FileText size={22} /></span><div><strong>PDF 문서 (.pdf) · 곧 제공</strong><p>읽기 전용 PDF 내보내기는 준비 중입니다.</p></div></button></div>{exporting && <LoadingState compact label="파일을 준비하고 있어요…" />}</Modal>
    </main>
  )
}
