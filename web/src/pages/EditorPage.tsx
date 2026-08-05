import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  AlertTriangle, ArrowLeft, Check, ChevronDown, ChevronLeft, ChevronRight, CircleAlert, Code2,
  Copy, Download, FileText, Image, LayoutPanelTop, LoaderCircle, MonitorPlay, Plus, Trash2, X,
} from 'lucide-react'
import { api, bodySlots, primaryBodySlot, textToParagraphs, type DeckFinding } from '../api/client'
import { BrandMark } from '../branding/BrandContext'
import { AssetLibrary } from '../components/AssetLibrary'
import { GridLibrary } from '../components/GridLibrary'
import { SlidePreview } from '../components/SlidePreview'
import { Button, EmptyState, ErrorState, LoadingState, Modal, Select, Textarea } from '../components/UI'
import { useToast } from '../components/Toast'
import { navigate } from '../router'
import type { Presentation, Slide, Template, TemplateLayout } from '../types'
import { displayError } from '../utils'
import { roleLabel } from './TemplatesPage'

const MAX_SLIDES = 50
const defaultSlide = (order: number, layoutId?: string): Slide => ({
  id: `new-${crypto.randomUUID()}`, order, layout: 'content', layoutId,
  title: '새로운 슬라이드', body: '핵심 메시지를 입력하세요.', bullets: ['핵심 메시지를 입력하세요.'],
  fields: { title: [{ text: '새로운 슬라이드' }], body: [{ text: '핵심 메시지를 입력하세요.' }] },
})

const slideBody = (slide?: Slide) => slide?.body || slide?.bullets?.join('\n') || ''
/** What a slide holds besides prose, named in the workspace's language. */
interface SlideHolding { slot: string; kind: 'block' | 'image'; label: string; detail: string }

function slideHoldings(slide?: Slide): SlideHolding[] {
  if (!slide) return []
  const holdings: SlideHolding[] = Object.entries(slide.blocks || {}).map(([slot, block]) => ({
    slot, kind: 'block', label: blockLabel(String(block.kind)),
    detail: String(block.caption || block.heading || '') || `${(block.items?.length ?? block.rows?.length ?? 0)}개 항목`,
  }))
  for (const [slot, image] of Object.entries(slide.images || {})) {
    holdings.push({ slot, kind: 'image', label: '이미지', detail: String(image.name || image.caption || slot) })
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
  }
  return kind
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
  const [panel, setPanel] = useState<'design' | 'notes' | 'images' | 'grids'>('design')
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
  const markEdited = () => { revision.current += 1; setEditVersion((value) => value + 1) }

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
            ...(snapshot.slides.length > 0 ? { slides: toApiSlides(snapshot.slides, layoutsRef.current) } : {}),
          })
          if (snapshotRevision !== revision.current) continue
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
  const updateActive = (updates: Partial<Slide>) => {
    if (!active) return
    markEdited()
    setSlides((current) => current.map((slide) => slide.id === activeId ? { ...slide, ...updates } : slide)); setDirty(true)
  }
  const contentLayoutId = useMemo(() => {
    const layouts = template?.layouts || []
    return (layouts.find((layout) => layout.role === 'content') || layouts[0])?.id
  }, [template])
  const addSlide = () => { if (slides.length >= MAX_SLIDES) return; markEdited(); const next = defaultSlide(slides.length + 1, contentLayoutId); setSlides((current) => [...current, next]); setActiveId(next.id); setDirty(true) }
  const duplicateSlide = () => { if (!active || slides.length >= MAX_SLIDES) return; markEdited(); const next = { ...active, id: `new-${crypto.randomUUID()}`, order: slides.length + 1, title: `${active.title} 복사본` }; setSlides((current) => [...current, next]); setActiveId(next.id); setDirty(true) }
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
      const result = await api.applyPresentationSource(id, source, dryRun)
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
        setSlides(compiled)
        setActiveId(compiled[0]?.id || '')
        setDirty(false)
        setLastSaved(new Date())
        setSavedSlideCount(compiled.length)
        setRailVersion((value) => value + 1)
      }
      showToast('코드를 적용했습니다.')
      setCanvasMode('preview')
    } catch (err) { showToast(displayError(err), 'error') } finally { setSourceBusy(false) }
  }

  const moveSlide = (direction: -1 | 1) => { const nextIndex = activeIndex + direction; if (nextIndex < 0 || nextIndex >= slides.length) return; markEdited(); const next = [...slides]; [next[activeIndex], next[nextIndex]] = [next[nextIndex], next[activeIndex]]; setSlides(next.map((slide, index) => ({ ...slide, order: index + 1 }))); setDirty(true) }
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
          >{deckFindings === null
            ? <><LoaderCircle className="spin" size={13} /> 측정 중</>
            : defects.length > 0
              ? <><CircleAlert size={13} /> 결함 {defects.length}</>
              : advisories.length > 0
                ? <><AlertTriangle size={13} /> 다듬을 곳 {advisories.length}</>
                : <><Check size={13} /> 결함 없음</>}
          </button><button className="save-status" disabled={saving || !dirty} onClick={() => void save().catch((err) => showToast(`저장하지 못했습니다: ${displayError(err)}`, 'error'))}>{saving ? <><LoaderCircle className="spin" size={13} /> 저장 중</> : dirty ? <><CircleAlert size={13} /> 지금 저장</> : <><Check size={13} /> {lastSaved ? '저장됨' : '모든 변경 저장됨'}</>}</button></div>
        <div className="editor-actions"><Button variant="ghost" size="small" disabled={slides.length === 0} onClick={() => { setPresentIndex(0); setPresenting(true) }}><MonitorPlay size={16} /> 발표</Button><Button variant="secondary" size="small" disabled={slides.length === 0} onClick={() => setExportOpen(true)}><Download size={16} /> 내보내기 <ChevronDown size={14} /></Button></div>
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
          {active ? <div className="canvas-stage">
            {canvasMode === 'edit' ? <div className="slide-edit-pane">
              <div className="slide-edit-fields">
                <label className="slide-edit-field">
                  <span>제목</span>
                  <input value={active.title} maxLength={200} onChange={(event) => updateActive({ title: event.target.value })} className="slide-title-editor" aria-label="슬라이드 제목" placeholder="슬라이드 제목" />
                </label>
                <label className="slide-edit-field">
                  <span>리드 문장</span>
                  <input value={active.subtitle || ''} maxLength={300} onChange={(event) => updateActive({ subtitle: event.target.value })} aria-label="리드 문장" placeholder="제목 아래 한 줄" />
                </label>
                <label className="slide-edit-field grow">
                  <span>본문 {activeHoldings.length > 0 ? '(컴포넌트 옆 영역)' : ''}</span>
                  <Textarea value={slideBody(active)} onChange={(event) => updateActive({ body: event.target.value, bullets: event.target.value.split(/\r?\n/).map((line) => line.trim()).filter(Boolean) })} className="slide-body-editor" aria-label="슬라이드 본문" placeholder="핵심 메시지를 줄마다 입력하세요." />
                </label>
                {activeHoldings.length > 0 && <div className="slide-holdings">
                  <strong>이 슬라이드가 담고 있는 것</strong>
                  <ul>{activeHoldings.map((holding) => <li key={holding.slot}>
                    {holding.kind === 'image' ? <Image size={13} /> : <LayoutPanelTop size={13} />}
                    <span>{holding.label}</span><small>{holding.detail}</small>
                  </li>)}</ul>
                  <p>컴포넌트와 이미지는 텍스트로 고칠 수 없습니다. 내용을 바꾸려면 <button type="button" onClick={() => void openSource()}>코드</button>에서 편집하세요. 여기서 글을 고쳐도 지워지지 않습니다.</p>
                </div>}
              </div>
              <div className="slide-edit-render">
                {/* The saved slide as the template draws it, so editing is not blind. */}
                <SlidePreview cacheKey={`${id}-edit-${activeIndex}-${railVersion}`} alt={`${activeIndex + 1}번 슬라이드`} load={() => api.slidePreview(id, activeIndex + 1, 900)} />
                <small>{dirty ? '저장 후 이 그림에 반영됩니다.' : '실제 템플릿으로 그린 현재 상태입니다.'}</small>
              </div>
            </div> : canvasMode === 'source' ? <div className="source-editor">
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
          <div className="inspector-tabs"><button className={panel === 'design' ? 'active' : ''} onClick={() => setPanel('design')}>디자인</button><button className={panel === 'notes' ? 'active' : ''} onClick={() => setPanel('notes')}>발표 노트</button><button className={panel === 'images' ? 'active' : ''} onClick={() => setPanel('images')}>이미지</button><button className={panel === 'grids' ? 'active' : ''} onClick={() => setPanel('grids')}>격자</button></div>
          {panel === 'design' ? <div className="inspector-content">
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
              <p className="inspector-help">올린 이미지는 코드에서 <code>::image 이름</code>으로 불러 씁니다. 레이아웃의 그림 영역, 없으면 가장 넓은 본문 영역에 들어갑니다.</p>
              <AssetLibrary onInsert={insertImageReference} notify={showToast} />
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
        footer={<Button variant="secondary" onClick={() => setFindingsOpen(false)}>닫기</Button>}
      >
        {(deckFindings || []).length === 0
          ? <p className="modal-note">모든 슬라이드가 템플릿 안에 제대로 들어갑니다.</p>
          : <ul className="deck-findings">{(deckFindings || []).map((finding) => (
              <li key={`${finding.slide}-${finding.slot}-${finding.kind}`} className={finding.advisory ? 'advisory' : 'defect'}>
                <button type="button" onClick={() => { const target = slides[finding.slide - 1]; if (target) setActiveId(target.id); setFindingsOpen(false) }}>
                  <strong>{finding.slide}번 슬라이드</strong>
                  <span>{findingLabel(finding.kind)}</span>
                  <small>{finding.detail}</small>
                </button>
              </li>
            ))}</ul>}
      </Modal>
      <Modal open={exportOpen} onClose={() => setExportOpen(false)} title="프레젠테이션 내보내기" description="사용할 형식을 선택하세요." footer={<Button variant="secondary" onClick={() => setExportOpen(false)}>취소</Button>}><div className="export-options"><button disabled={exporting} onClick={() => void exportDeck('pptx')}><span className="export-icon ppt"><FileText size={22} /></span><div><strong>PowerPoint (.pptx)</strong><p>Microsoft PowerPoint와 호환되는 편집 가능한 파일</p></div><Download size={18} /></button><button disabled title="추후 제공 예정"><span className="export-icon pdf"><FileText size={22} /></span><div><strong>PDF 문서 (.pdf) · 곧 제공</strong><p>읽기 전용 PDF 내보내기는 준비 중입니다.</p></div></button></div>{exporting && <LoadingState compact label="파일을 준비하고 있어요…" />}</Modal>
    </main>
  )
}
