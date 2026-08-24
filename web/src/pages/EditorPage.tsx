import type React from 'react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  AlertTriangle, ArrowLeft, Check, ChevronDown, ChevronLeft, ChevronRight, CircleAlert, Code2,
  Copy, Download, EyeOff, FileText, History, Image, Keyboard, LayoutPanelTop, LifeBuoy, LoaderCircle, MessageSquare, Plus, RotateCcw, Trash2, Link2, MonitorPlay, WandSparkles, X, MessageSquareText } from 'lucide-react'
import { api, ApiError, bodySlots, primaryBodySlot, textToParagraphs, type DeckFinding, type DeckScore } from '../api/client'
import { BrandMark } from '../branding/BrandContext'
import { AssetLibrary, type Asset } from '../components/AssetLibrary'
import { FreeformCanvas } from '../components/FreeformCanvas'
import { GridLibrary } from '../components/GridLibrary'
import { PresentationView } from '../components/Presentation'
import { SlideLibrary } from '../components/SlideLibrary'
import { SlidePreview } from '../components/SlidePreview'
import { ShortcutSheet, editorShortcuts, useShortcutSheet } from '../components/Shortcuts'
import { Button, EmptyState, ErrorState, Input, LoadingState, Modal, Select, Textarea } from '../components/UI'
import { useToast } from '../components/Toast'
import { navigate } from '../router'
import type {
  Presentation, PresentationRevision, Slide, SlideBlock, SlideChange, SlideElement, SlideParagraph, Snippet, SlotFrame, SlotStyle,
  Template, TemplateLayout,
} from '../types'
import { displayError, relativeDate } from '../utils'
import { roleLabel } from './TemplatesPage'

import {
  MAX_SLIDES, blockLabel, moveSlideTo, presentIndexOf, slidesToPresent, bodyFromFields, bodyFromText, carryTrimmedEntries, defaultSlide, drawnSlots, proseSlot,
  slideBody, slideBodyLines, slideFields, slideHoldings, textRegions, toApiSlides,
} from './editor/model/slides'
import { findingDetail, findingLabel, revisionReason, scoreDimensionLabel, trimmedCounts, warningText } from './editor/model/findings'
import { replaceInDeck } from './editor/model/search'
import { versionToSend } from './editor/model/saving'
import { CommandDialog, type CommandPlan } from './editor/CommandDialog'
import { FindDialog } from './editor/FindDialog'
import { QualityDialog } from './editor/QualityDialog'
import { HistoryDialog } from './editor/HistoryDialog'
import { ShareDialog } from './editor/ShareDialog'
import { CommentsDialog } from './editor/CommentsDialog'
import { ExportDialog } from './editor/ExportDialog'
import { useAutosave, useUnsavedWarning } from './editor/hooks/useAutosave'

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
  // The same measurements, scored. A list says what to fix; the score says
  // whether the deck is ready, which is what anyone asks first.
  const [deckScore, setDeckScore] = useState<DeckScore | null>(null)
  // Telling the deck what to do, in words. The plan is shown before anything
  // changes: a command nobody can check is a command nobody should run.
  const [commandOpen, setCommandOpen] = useState(false)
  const [commandText, setCommandText] = useState('')
  const [commandBusy, setCommandBusy] = useState(false)
  const [commandPlan, setCommandPlan] = useState<CommandPlan | null>(null)
  const [findingsOpen, setFindingsOpen] = useState(false)
	const [historyOpen, setHistoryOpen] = useState(false)
	const [historyLoading, setHistoryLoading] = useState(false)
	const [history, setHistory] = useState<PresentationRevision[]>([])
	const [restoringRevision, setRestoringRevision] = useState('')
	const [conflictOpen, setConflictOpen] = useState(false)
	const [conflictKind, setConflictKind] = useState<'canvas' | 'source'>('canvas')
  const [panel, setPanel] = useState<'content' | 'design' | 'notes' | 'images' | 'grids' | 'library'>('content')
  const [canvasMode, setCanvasMode] = useState<'edit' | 'preview' | 'source'>('edit')
  // The deck as text. It is the same deck the canvas shows: applying it
  // recompiles the slides, and opening it reads them back out.
  const [source, setSource] = useState('')
  const [sourceLoaded, setSourceLoaded] = useState(false)
  const [sourceBusy, setSourceBusy] = useState(false)
  const [sourceWarnings, setSourceWarnings] = useState<string[]>([])
  // What changed since each checkpoint, asked for one at a time. Restoring a
  // version nobody has read is a leap in the dark.
  const [revisionChanges, setRevisionChanges] = useState<Record<string, SlideChange[] | 'loading'>>({})
  const [openChange, setOpenChange] = useState<string | null>(null)
  // What generation did differently from what was asked — a deck shorter than
  // the count requested, most often. It is the answer to "why is this only nine
  // slides", and the person who asked is the only one who can act on it, so it
  // stands above the deck until they have read it.
  const [generationNotes, setGenerationNotes] = useState<string[]>([])
  const [sourceFindings, setSourceFindings] = useState<DeckFinding[]>([])
  const [sourceSlide, setSourceSlide] = useState(1)
  const [sourcePreview, setSourcePreview] = useState<{ url: string; slide: number; count: number } | null>(null)
  const [sourcePreviewError, setSourcePreviewError] = useState('')
  const [presenting, setPresenting] = useState(false)
  const [rewriting, setRewriting] = useState(false)
  const shortcuts = useShortcutSheet()
  const [presentIndex, setPresentIndex] = useState(0)
  const [exportOpen, setExportOpen] = useState(false)
  // Links that open this deck for someone with no account here.
  const [shareOpen, setShareOpen] = useState(false)
  // What the people who were sent the link had to say, and how many of them are
  // still waiting for an answer.
  const [commentsOpen, setCommentsOpen] = useState(false)
  const [findOpen, setFindOpen] = useState(false)
  const [openComments, setOpenComments] = useState(0)
  const [exporting, setExporting] = useState(false)
  const [retrying, setRetrying] = useState(false)
  const [editVersion, setEditVersion] = useState(0)
  const revision = useRef(0)
  const savePromise = useRef<Promise<boolean> | null>(null)
  const editorState = useRef({ presentation, slides, dirty })
  editorState.current = { presentation, slides, dirty }
  // The deck's version as the server last reported it. A save that sends an
  // older number is refused with a conflict, and the number is easy to send by
  // accident: any handler that writes `{ ...presentation, … }` copies whatever
  // version was current when that render happened, which may be two saves ago.
  // Keeping the newest one in a ref means the save never asks with a stale one.
  const versionRef = useRef(0)
  if (presentation && presentation.version > versionRef.current) versionRef.current = presentation.version
  const layoutsRef = useRef<TemplateLayout[]>([])
  layoutsRef.current = template?.layouts || []
  const { showToast } = useToast()
  const markEdited = () => { revision.current += 1; setEditVersion((value) => value + 1); setSourceLoaded(false) }

  // The measurement follows the saved deck, not the keystrokes.
  useEffect(() => {
    if (railVersion === 0 || savedSlideCount === 0) return
    let active = true
    api.inspectPresentation(id)
      .then((result) => { if (active) { setDeckFindings(result.findings); setDeckScore(result.score) } })
      .catch(() => { if (active) { setDeckFindings(null); setDeckScore(null) } })
    return () => { active = false }
  }, [id, railVersion, savedSlideCount])

  const load = useCallback(async () => {
    setLoading(true); setError('')
    try {
      const data = await api.presentation(id)
      setPresentation(data); setSlides(data.slides || []); setActiveId(data.slides?.[0]?.id || '')
      setGenerationNotes(data.generationNotes || [])
			setDirty(false)
			setSourceLoaded(false)
      setSavedSlideCount((data.slides || []).length)
      // What the deck looks like once drawn, so a finished deck says whether it is
      // actually finished rather than only that generation ended.
      if ((data.slides || []).length > 0) {
        api.inspectPresentation(id).then((result) => { setDeckFindings(result.findings); setDeckScore(result.score) }).catch(() => { setDeckFindings(null); setDeckScore(null) })
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
      try { const data = await api.presentation(id); setPresentation(data); setGenerationNotes(data.generationNotes || []); if (data.slides?.length) { setSlides(data.slides); setActiveId((current) => data.slides!.some((slide) => slide.id === current) ? current : data.slides![0].id) } } catch { /* polling resumes */ }
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
						version: versionToSend(snapshotPresentation.version, versionRef.current),
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

  useAutosave({
    dirty,
    edits: editVersion,
    save,
    onError: (error) => showToast(`저장하지 못했습니다: ${displayError(error)}`, 'error'),
  })
  useUnsavedWarning(() => editorState.current.dirty || Boolean(savePromise.current))

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
  useEffect(() => {
    let active = true
    api.comments(id)
      .then((rows) => { if (active) setOpenComments(rows.filter((comment) => !comment.resolvedAt).length) })
      .catch(() => { /* a deck nobody has commented on is the ordinary case */ })
    return () => { active = false }
  }, [id, commentsOpen])

  const activeLayout = useMemo(() => (template?.layouts || []).find((layout) => layout.id === active?.layoutId), [template, active?.layoutId])
  // Every text region of the slide, so a two-column slide can be read and edited
  // from the panel rather than only from the canvas. The first is the prose slot
  // the body box has always written to; the rest used to be invisible here.
  const activeRegions = useMemo(() => textRegions(active, activeLayout), [active, activeLayout])

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
  // The rail scrolls, so the slide being edited is kept in sight. "nearest"
  // means an already visible thumbnail is left where it is rather than being
  // yanked to the middle on every click.
  const railRef = useRef<HTMLDivElement | null>(null)
  useEffect(() => {
    const row = railRef.current?.querySelector<HTMLElement>('.slide-thumbnail-row.active')
    row?.scrollIntoView({ block: 'nearest' })
  }, [activeId, slides.length])

  // Replacing across the deck is one edit, undone in one step.
  const replaceEverywhere = (query: string, replacement: string,
                             options: { matchCase: boolean; wholeWord: boolean },
                             only?: { slideId?: string }) => {
    const result = replaceInDeck(slides, query, replacement, options, blockLabel, only)
    if (result.replaced === 0) return 0
    markEdited()
    setSlides(result.slides)
    setDirty(true)
    return result.places
  }

  // Dragging a thumbnail to a new place. `dropAt` is a gap between slides —
  // 0 is before the first, slides.length is after the last — which is what the
  // line drawn between thumbnails is showing.
  const [dragging, setDragging] = useState<number | null>(null)
  const [dropAt, setDropAt] = useState<number | null>(null)
  const railScroll = useRef<number | null>(null)

  // A fifty-slide rail is taller than the window, so a drag has to be able to
  // reach the end of it: near the top or bottom edge the list keeps moving.
  const followEdges = (clientY: number) => {
    const rail = railRef.current
    if (!rail) return
    const box = rail.getBoundingClientRect()
    const edge = 48
    const speed = clientY < box.top + edge ? -12 : clientY > box.bottom - edge ? 12 : 0
    if (speed === 0) {
      if (railScroll.current !== null) { window.clearInterval(railScroll.current); railScroll.current = null }
      return
    }
    if (railScroll.current !== null) return
    railScroll.current = window.setInterval(() => { rail.scrollTop += speed }, 16)
  }
  const stopFollowing = () => {
    if (railScroll.current !== null) { window.clearInterval(railScroll.current); railScroll.current = null }
  }
  useEffect(() => stopFollowing, [])

  // Keeping a slide out of the talk without taking it out of the deck.
  const toggleSkipped = (slideId: string) => {
    markEdited()
    setSlides((current) => current.map((slide) =>
      slide.id === slideId ? { ...slide, skipped: !slide.skipped } : slide))
    setDirty(true)
  }

  const dropSlide = (gap: number) => {
    stopFollowing()
    const from = dragging
    setDragging(null)
    setDropAt(null)
    if (from === null) return
    const next = moveSlideTo(slides, from, gap)
    if (next === slides) return
    markEdited()
    setSlides(next)
    setDirty(true)
  }

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

  const planCommand = async () => {
    if (!commandText.trim()) return
    setCommandBusy(true)
    try {
      setCommandPlan(await api.commandPresentation(id, commandText, true))
    } catch (err) {
      setCommandPlan(null)
      showToast(displayError(err), 'error')
    } finally { setCommandBusy(false) }
  }

  const runCommand = async () => {
    setCommandBusy(true)
    try {
      const result = await api.commandPresentation(id, commandText, false)
      showToast([`${result.slides}장 → ${result.slidesAfter}장.`, ...result.notes].join(' '), 'success')
      setCommandOpen(false); setCommandText(''); setCommandPlan(null)
      setDirty(false)
      editorState.current = { ...editorState.current, dirty: false }
      await load()
    } catch (err) {
      showToast(displayError(err), 'error')
    } finally { setCommandBusy(false) }
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
  const selectSlide = (direction: -1 | 1) => {
    const next = slides[activeIndex + direction]
    if (next) setActiveId(next.id)
  }

  // The keys a deck is worked on with. Everything the canvas answers to is left
  // to the canvas — this is what belongs to the deck rather than to a drawing:
  // save, add, reorder, present. Typing is never interrupted, and the browser's
  // own Ctrl+S and F5 have to be taken deliberately or the page reloads.
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null
      const typing = Boolean(target?.matches?.('input, textarea, select, [contenteditable="true"]'))
      const control = event.ctrlKey || event.metaKey
      if (control && !event.altKey && event.key.toLowerCase() === 's') {
        event.preventDefault()
        void save().then((saved) => showToast(saved ? '저장했습니다.' : '변경할 내용이 없습니다.'))
          .catch((err) => showToast(`저장하지 못했습니다: ${displayError(err)}`, 'error'))
        return
      }
      // F5 presents even while typing. In a deck editor that is what the key
      // means, and reloading the page in the middle of a sentence is the worse
      // outcome by far.
      if (event.key === 'F5' && !control) {
        event.preventDefault()
        if (slides.length > 0) {
          (document.activeElement as HTMLElement | null)?.blur?.()
          setPresentIndex(presentIndexOf(slides, activeIndex))
          setPresenting(true)
        }
        return
      }
      if (control && !event.altKey && (event.key.toLowerCase() === 'f' || event.key.toLowerCase() === 'h')) {
        event.preventDefault()
        setFindOpen(true)
        return
      }
      if (typing) return
      if (control && event.key === 'Enter') { event.preventDefault(); addSlide(); return }
      if (event.altKey && !control) {
        if (event.key === 'ArrowUp') { event.preventDefault(); moveSlide(-1); return }
        if (event.key === 'ArrowDown') { event.preventDefault(); moveSlide(1); return }
        if (event.key === 'PageUp') { event.preventDefault(); selectSlide(-1); return }
        if (event.key === 'PageDown') { event.preventDefault(); selectSlide(1); return }
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  })

	const canSafelyFix = (finding: DeckFinding) => {
		if (finding.kind === 'notes') return Boolean(slides[finding.slide - 1])
		// A component that draws five of six entries is carried onto a second
		// slide: which five it drew is arithmetic, not judgement, so nothing has
		// to be rewritten to fix it.
		if (finding.kind === 'trimmed') return slides.length < MAX_SLIDES && Boolean(trimmedItems(finding))
		if (finding.kind !== 'density' && finding.kind !== 'overflow') return false
		return slides.length < MAX_SLIDES && slideBodyLines(slides[finding.slide - 1]).length >= 4
	}

	// trimmedItems is the component a "some of it is not drawn" finding is about,
	// and where it has to be cut, when the slide really holds what was measured.
	const trimmedItems = (finding: DeckFinding) => {
		const counts = trimmedCounts(finding.detail)
		const slide = slides[finding.slide - 1]
		const block = slide?.blocks?.[finding.slot]
		const items = Array.isArray(block?.items) ? block.items : []
		if (!counts || !block || items.length <= counts.drawn) return null
		return { slide, slot: finding.slot, block, items, drawn: counts.drawn }
	}

	// What cannot be fixed without rewriting words is handed to the model, with
	// the measurement attached: "0.9cm too tall" is something a rewrite can aim at.
	const [aiFixing, setAiFixing] = useState(0)
	// A group of findings is one job. Handed over one slide at a time so each
	// rewrite stays its own undo step, but asked for once.
	const fixFindingsWithAI = async (group: DeckFinding[]) => {
		if (group.length < 2) {
			await fixFindingWithAI(group[0])
			return
		}
		if (sweeping.total > 0 || aiFixing) return
		setFindingsOpen(false)
		setSweeping({ done: 0, total: group.length })
		try {
			for (const [index, finding] of group.entries()) {
				setSweeping({ done: index, total: group.length })
				await reviseSlideAt(finding.slide - 1, {
					action: finding.kind === 'notes' ? 'notes' : finding.kind === 'repeat' ? 'shorten' : 'fit',
					instruction: `측정 결과: ${finding.detail}`,
					slot: finding.slot || '',
				})
			}
			showToast(`${group.length}장을 AI로 다시 썼습니다. 결과를 다시 측정합니다.`)
		} finally {
			setSweeping({ done: 0, total: 0 })
		}
	}

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
	// Missing notes are the same omission on every slide that has them, so they
	// are drafted in one pass. Splitting a slide changes the deck under the
	// measurement, so those are still done one at a time and measured again.
	const safelyFixFindings = (group: DeckFinding[]) => {
		const notes = group.filter((finding) => finding.kind === 'notes' && canSafelyFix(finding))
		if (notes.length > 1) {
			const indexes = new Set(notes.map((finding) => finding.slide - 1))
			markEdited()
			setSlides((current) => current.map((slide, slideIndex) => indexes.has(slideIndex)
				? { ...slide, speakerNotes: `${slide.title}: ${slide.subtitle || slideBodyLines(slide)[0] || slide.title}`.slice(0, 4000) }
				: slide))
			setDirty(true)
			showToast(`${notes.length}장에 발표 노트 초안을 추가했습니다.`)
			return
		}
		safelyFixFinding(group[0])
	}

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
		if (finding.kind === 'trimmed') {
			const carried = trimmedItems(finding)
			const split = carried && carryTrimmedEntries(target, carried.slot, carried.drawn, `new-${crypto.randomUUID()}`)
			if (!carried || !split) return
			setSlides([...slides.slice(0, index), split.kept, split.rest, ...slides.slice(index + 1)]
				.map((slide, slideIndex) => ({ ...slide, order: slideIndex + 1 })))
			setActiveId(split.rest.id)
			setDirty(true)
			setFindingsOpen(false)
			showToast(`그려지지 않던 ${carried.items.length - carried.drawn}개를 다음 슬라이드로 옮겼습니다.`)
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

	// What changed since a checkpoint, fetched the first time someone asks and
	// kept after that: the answer cannot change while the dialog is open.
	const compareRevision = async (checkpoint: PresentationRevision) => {
		if (openChange === checkpoint.id) { setOpenChange(null); return }
		setOpenChange(checkpoint.id)
		if (revisionChanges[checkpoint.id]) return
		setRevisionChanges((current) => ({ ...current, [checkpoint.id]: 'loading' }))
		try {
			const changes = await api.presentationChanges(id, checkpoint.id)
			setRevisionChanges((current) => ({ ...current, [checkpoint.id]: changes }))
		} catch (err) {
			setRevisionChanges((current) => {
				const rest = { ...current }
				delete rest[checkpoint.id]
				return rest
			})
			showToast(`무엇이 바뀌었는지 불러오지 못했습니다: ${displayError(err)}`, 'error')
		}
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
			api.inspectPresentation(id).then((result) => { setDeckFindings(result.findings); setDeckScore(result.score) }).catch(() => { setDeckFindings(null); setDeckScore(null) })
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

  /**
   * An image that came from outside the workspace: pasted from the clipboard or
   * dropped onto the slide. It is uploaded like any other — it joins the image
   * library under its file name — and then placed where it was dropped.
   */
  const importImages = async (files: File[], at?: { x: number; y: number }) => {
    if (!active || files.length === 0) return
    showToast(files.length === 1 ? '이미지를 올리는 중…' : `이미지 ${files.length}장을 올리는 중…`)
    try {
      const uploaded: Asset[] = []
      for (const file of files) {
        // A pasted screenshot arrives as "image.png" every time, which would make
        // each one replace the last. Anything with a real file name keeps it.
        const pasted = !file.name || file.name.toLowerCase() === 'image.png'
        const name = pasted ? `붙여넣은 이미지 ${new Date().toISOString().slice(0, 19).replace(/[-:T]/g, '')}` : file.name
        const asset = await api.uploadAsset(file, name)
        uploaded.push({ ...asset, contentType: asset.contentType || file.type, sizeBytes: asset.sizeBytes || file.size })
      }
      // Placed in one edit: several separate ones would each start from the same
      // slide and only the last would survive.
      if (imageTarget || uploaded.length === 1) { placeImage(uploaded[0], at); return }
      const highest = Math.max(0, ...(active.elements || []).map((element) => element.zIndex || 0))
      const placed = uploaded.map((asset, index) => imageElement(asset, at && { x: at.x + index * 3, y: at.y + index * 3 }, highest + 1 + index))
      updateActive({ elements: [...(active.elements || []), ...placed] })
      setCanvasMode('edit')
      showToast(`이미지 ${placed.length}장을 배치했습니다.`)
    } catch (err) { showToast(displayError(err), 'error') }
  }

  /** One image as a floating object, sized from its own pixels. */
  const imageElement = (asset: Asset, at: { x: number; y: number } | undefined, zIndex: number): SlideElement => {
    const ratio = asset.width > 0 && asset.height > 0 ? asset.width / asset.height : 16 / 9
    const width = 30
    const height = Math.min(45, Math.max(8, width * (16 / 9) / ratio))
    // Dropped where the pointer was — centred on it, because that is where a
    // person aimed — and never hanging off the slide.
    const x = at ? Math.min(100 - width, Math.max(0, at.x - width / 2)) : 35
    const y = at ? Math.min(100 - height, Math.max(0, at.y - height / 2)) : Math.max(5, (100 - height) / 2)
    return {
      id: `element-${crypto.randomUUID()}`, kind: 'image', assetId: asset.id, name: asset.name,
      caption: asset.name, x, y, width, height, zIndex, rotation: 0, opacity: 100, fit: 'cover',
    }
  }

  const placeImage = (asset: Asset, at?: { x: number; y: number }) => {
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
    const highest = Math.max(0, ...(active.elements || []).map((element) => element.zIndex || 0))
    const image = imageElement(asset, at, highest + 1)
    updateActive({ elements: [...(active.elements || []), image] })
    setCanvasMode('edit')
    showToast(`${asset.name}을 현재 슬라이드에 배치했습니다.`)
  }

  /**
   * Puts a saved slide into this deck, after the one being edited.
   *
   * The server lays it out in this deck's template — the snippet is text, so the
   * company introduction comes out in whatever design this deck wears — and the
   * result is an ordinary slide from here on.
   */
  const insertSnippet = async (snippet: Snippet) => {
    if (slides.length >= MAX_SLIDES) { showToast(`한 덱은 ${MAX_SLIDES}장까지입니다.`, 'error'); return }
    if (dirty) await save().catch(() => { /* the snippet is compiled against the stored deck */ })
    const rendered = await api.renderSnippet(snippet.id, id)
    markEdited()
    const at = active ? activeIndex + 1 : slides.length
    const inserted = { ...rendered.slide, id: `new-${crypto.randomUUID()}` }
    const next = [...slides.slice(0, at), inserted, ...slides.slice(at)].map((slide, index) => ({ ...slide, order: index + 1 }))
    setSlides(next)
    setActiveId(inserted.id)
    setDirty(true)
    setCanvasMode('edit')
    showToast(rendered.warnings.length > 0
      ? `${snippet.name}을 넣었습니다. ${rendered.warnings[0]}`
      : `${snippet.name}을 넣었습니다.`)
  }

  /** Saves the slide being edited, as text, for use in any other deck. */
  const saveSlideToLibrary = async (name: string) => {
    if (!active) return
    if (dirty) await save().catch(() => { /* what is stored is what gets saved */ })
    const snippet = await api.saveSnippet({ name, presentationId: id, slide: activeIndex + 1 })
    showToast(`"${snippet.name}"을 라이브러리에 저장했습니다. 다른 덱에서도 쓸 수 있어요.`)
  }

  /**
   * Hands the whole deck to the model to rewrite: the facts stay, the craft
   * improves. Version history is what makes this safe to try, so the old deck is
   * one click away in 버전 이력.
   */
  const rewriteDeck = async () => {
    if (!window.confirm('덱 전체를 AI가 다시 씁니다. 숫자와 사실은 그대로 두고 제목·문장·구성을 다듬습니다. 이전 상태는 버전 이력에서 되돌릴 수 있습니다.')) return
    setRewriting(true)
    try {
      if (dirty) await save()
      setPresentation(await api.rewritePresentation(id))
      showToast('덱을 다시 쓰고 있습니다. 끝나면 이 화면이 바뀝니다.')
    } catch (err) { showToast(displayError(err), 'error') } finally { setRewriting(false) }
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
  // A deck with slides that is queued or generating is being rewritten: the words
  // say which is happening, because "생성 중" over a deck someone already has
  // reads like it is being replaced.
  if (presentation.status === 'generating') {
    const rewriting = (presentation.slideCount || 0) > 0
    return <main className="editor-loading generation-wait"><div className="generation-visual small"><div className={`generating-slide theme-${presentation.theme || 'aurora'}`}><span>PTIUM ENGINE</span><div><i /><i /><i /></div><strong>{presentation.title}</strong><em /></div></div>
      <h1>{rewriting ? '덱을 다시 쓰고 있어요' : '슬라이드를 생성하고 있어요'}</h1>
      <p>{rewriting
        ? '숫자와 사실은 그대로 두고 제목·문장·구성을 다듬는 중입니다. 끝나면 이 화면이 열립니다.'
        : '완성되는 대로 자동으로 편집기를 열어드릴게요.'}</p>
      <LoaderCircle className="spin" size={22} /></main>
  }
  if (presentation.status === 'failed') return <main className="editor-loading"><CircleAlert size={30} /><h1>슬라이드 생성에 실패했습니다</h1><p>{presentation.errorMessage || '관리자에게 오류 센터의 요청 기록을 확인해 달라고 요청하세요.'}</p><div><Button variant="secondary" onClick={() => navigate('/presentations')}>목록으로 돌아가기</Button> <Button disabled={retrying} onClick={() => void retryGeneration()}>{retrying ? '다시 등록 중…' : '생성 다시 시도'}</Button></div></main>

  return (
    <main className="editor-page">
      <header className="editor-header">
        <div className="editor-header-left"><button className="icon-button" aria-label="저장하고 프레젠테이션 목록으로 이동" onClick={() => void leaveEditor()}><ArrowLeft size={18} /></button><span className="editor-brand"><BrandMark size="tiny" /></span><span className="header-divider" /><input className="deck-title-input" maxLength={200} value={presentation.title} onChange={(event) => { const title = event.target.value; markEdited(); setPresentation((current) => current ? { ...current, title } : current); setDirty(true) }} aria-label="프레젠테이션 제목" /></div>
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
              ? <><CircleAlert size={13} /> 품질 {deckScore ? deckScore.total : '—'} · 결함 {defects.length}</>
              : advisories.length > 0
                ? <><AlertTriangle size={13} /> 품질 {deckScore ? deckScore.total : '—'} · 다듬을 곳 {advisories.length}</>
                : <><Check size={13} /> 품질 {deckScore ? deckScore.total : 100}</>}
          </button><button className="save-status" disabled={saving || !dirty} onClick={() => void save().catch((err) => showToast(`저장하지 못했습니다: ${displayError(err)}`, 'error'))}>{saving ? <><LoaderCircle className="spin" size={13} /> 저장 중</> : dirty ? <><CircleAlert size={13} /> 지금 저장</> : <><Check size={13} /> {lastSaved ? '저장됨' : '모든 변경 저장됨'}</>}</button></div>
		<div className="editor-actions"><Button variant="ghost" size="small" onClick={() => shortcuts.setOpen(true)} title="단축키 (?)"><Keyboard size={16} /> 단축키</Button><a className="button button-ghost button-small" href="/guide" target="_blank" rel="noreferrer" title="사용 가이드를 새 탭에서 엽니다"><LifeBuoy size={16} /> 도움말</a><Button variant="ghost" size="small" disabled={slides.length === 0} onClick={() => { setCommandPlan(null); setCommandOpen(true) }} title="말로 시킵니다. 예: 3번과 4번 합쳐줘 · 5번 삭제 · 10분 발표로 맞춰줘"><MessageSquare size={16} /> 명령</Button><Button variant="ghost" size="small" disabled={rewriting || slides.length === 0} onClick={() => void rewriteDeck()} title="숫자와 사실은 그대로 두고 제목·문장·구성을 다듬습니다"><WandSparkles size={16} /> {rewriting ? '보내는 중…' : 'AI로 다듬기'}</Button><Button variant="ghost" size="small" onClick={() => void openHistory()} title="이 덱의 체크포인트를 열어 되돌립니다"><History size={16} /> 버전 이력</Button><Button variant="ghost" size="small" onClick={() => setCommentsOpen(true)} title="공유 링크로 덱을 본 사람들이 남긴 의견"><MessageSquareText size={16} /> 의견{openComments > 0 ? ` ${openComments}` : ''}</Button><Button variant="ghost" size="small" disabled={slides.length === 0} onClick={() => setShareOpen(true)} title="계정이 없는 사람도 볼 수 있는 링크를 만듭니다"><Link2 size={16} /> 공유</Button><Button variant="ghost" size="small" disabled={slides.length === 0} onClick={() => { setPresentIndex(0); setPresenting(true) }} title="전체 화면으로 발표합니다"><MonitorPlay size={16} /> 발표</Button><Button variant="secondary" size="small" disabled={slides.length === 0} onClick={() => setExportOpen(true)}><Download size={16} /> 내보내기 <ChevronDown size={14} /></Button></div>
      </header>

      {generationNotes.length > 0 && <div className="generation-notes" role="status">
        <AlertTriangle size={15} />
        <div>{generationNotes.map((note) => <p key={note}>{note}</p>)}</div>
        <button className="icon-button" aria-label="안내 닫기" onClick={() => setGenerationNotes([])}><X size={15} /></button>
      </div>}
      <div className="editor-workspace">
        <aside className="slide-rail">
          <div className="slide-rail-head"><strong>슬라이드</strong><span>{slides.length} / {MAX_SLIDES}</span></div>
          <div className="slide-list" ref={railRef}>{slides.map((slide, index) => {
            const holdings = slideHoldings(slide)
            const drawn = !slide.id.startsWith('new-') && index < savedSlideCount
            return <button
              key={slide.id}
              className={`slide-thumbnail-row ${activeId === slide.id ? 'active' : ''}${slide.skipped ? ' skipped' : ''}${dragging === index ? ' dragging' : ''}${dropAt === index ? ' drop-before' : ''}${dropAt === index + 1 && index === slides.length - 1 ? ' drop-after' : ''}`}
              draggable
              onClick={() => setActiveId(slide.id)}
              onDragStart={(event) => {
                setDragging(index)
                event.dataTransfer.effectAllowed = 'move'
                // Firefox refuses to start a drag without payload.
                event.dataTransfer.setData('text/plain', String(index))
              }}
              onDragOver={(event) => {
                if (dragging === null) return
                event.preventDefault()
                event.dataTransfer.dropEffect = 'move'
                const box = event.currentTarget.getBoundingClientRect()
                setDropAt(event.clientY < box.top + box.height / 2 ? index : index + 1)
                followEdges(event.clientY)
              }}
              onDrop={(event) => { event.preventDefault(); dropSlide(dropAt ?? index) }}
              onDragEnd={() => { stopFollowing(); setDragging(null); setDropAt(null) }}
              title="끌어서 순서를 바꿉니다"
            >
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
          </div><div><button className="icon-button small" onClick={() => moveSlide(-1)} disabled={!active || activeIndex === 0} aria-label="왼쪽으로 이동"><ChevronLeft size={16} /></button><button className="icon-button small" onClick={() => moveSlide(1)} disabled={!active || activeIndex >= slides.length - 1} aria-label="오른쪽으로 이동"><ChevronRight size={16} /></button><button className="icon-button small" onClick={duplicateSlide} disabled={!active || slides.length >= MAX_SLIDES} title={slides.length >= MAX_SLIDES ? `최대 ${MAX_SLIDES}장까지 편집할 수 있습니다.` : undefined} aria-label="복제"><Copy size={15} /></button><button className={`icon-button small${active?.skipped ? ' active' : ''}`} onClick={() => { if (active) toggleSkipped(active.id) }} disabled={!active} aria-pressed={Boolean(active?.skipped)} title={active?.skipped ? '발표에서 건너뜁니다. 눌러서 다시 발표에 넣습니다' : '발표할 때 이 슬라이드를 건너뜁니다. 덱과 내려받은 파일에는 그대로 남습니다'} aria-label="발표에서 건너뛰기"><EyeOff size={15} /></button><button className="icon-button small danger-hover" onClick={removeSlide} disabled={!active || slides.length <= 1} aria-label="삭제"><Trash2 size={15} /></button></div></div>
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
              onImageFiles={(files, at) => { void importImages(files, at) }}
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
                {sourceWarnings.map((warning) => <li key={warning}><AlertTriangle size={13} /> {warningText(warning)}</li>)}
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
          <div className="inspector-tabs"><button className={panel === 'content' ? 'active' : ''} onClick={() => setPanel('content')}>내용</button><button className={panel === 'design' ? 'active' : ''} onClick={() => setPanel('design')}>디자인</button><button className={panel === 'images' ? 'active' : ''} onClick={() => setPanel('images')}>이미지</button><button className={panel === 'library' ? 'active' : ''} onClick={() => setPanel('library')}>슬라이드</button><button className={panel === 'notes' ? 'active' : ''} onClick={() => setPanel('notes')}>노트</button><button className={panel === 'grids' ? 'active' : ''} onClick={() => setPanel('grids')}>격자</button></div>
          {panel === 'content' ? <div className="inspector-content">
            <section className="template-text-fields">
              <div className="inspector-section-head"><strong>템플릿 텍스트</strong><span className="inspector-hint">배경 레이어</span></div>
              <p className="inspector-help">템플릿 슬롯의 글을 편집합니다. 캔버스 위 텍스트 상자는 자유롭게 이동·회전할 수 있습니다.</p>
              <label className="slide-edit-field"><span>제목</span><input disabled={!active} value={active?.title || ''} maxLength={200} onChange={(event) => updateActive({ title: event.target.value })} aria-label="슬라이드 제목" placeholder="슬라이드 제목" /></label>
              <label className="slide-edit-field"><span>리드 문장</span><input disabled={!active} value={active?.subtitle || ''} maxLength={300} onChange={(event) => updateActive({ subtitle: event.target.value })} aria-label="리드 문장" placeholder="제목 아래 한 줄" /></label>
              <label className="slide-edit-field grow"><span>{activeRegions.length > 1 ? activeRegions[0].label : '본문'} {activeHoldings.some((holding) => holding.kind !== 'element') ? '(컴포넌트 옆 영역)' : ''}</span><Textarea disabled={!active} value={slideBody(active)} onChange={(event) => updateActive({ body: event.target.value, bullets: event.target.value.split(/\r?\n/).map((line) => line.trim()).filter(Boolean) })} className="slide-body-editor" aria-label="슬라이드 본문" placeholder="핵심 메시지를 줄마다 입력하세요." /></label>
              {activeRegions.slice(1).map((region) => <label className="slide-edit-field grow" key={region.slot}><span>{region.label}</span><Textarea
                value={region.text}
                onChange={(event) => updateActive({ fields: { ...(active?.fields || {}), [region.slot]: textToParagraphs(event.target.value) } })}
                className="slide-body-editor" aria-label={region.label} placeholder="이 영역의 글" /></label>)}
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
          </div> : panel === 'library' ? <div className="inspector-content">
            <section>
              <strong>저장한 슬라이드</strong>
              <p className="inspector-help">회사 소개·팀·연락처처럼 매번 다시 만드는 슬라이드를 저장해 두고 어느 덱에나 넣습니다. 글로 저장하므로 <b>넣는 덱의 디자인</b>으로 다시 그려집니다.</p>
              <SlideLibrary
                presentationId={id}
                currentTitle={active?.title}
                onInsert={insertSnippet}
                onSaveCurrent={active ? saveSlideToLibrary : undefined}
                notify={showToast}
              />
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

      {/* The audience screen. Notes, the next slide and the clock live in the
          presenter's own window, which this one drives. */}
      <ShortcutSheet open={shortcuts.open} onClose={shortcuts.close} groups={editorShortcuts} title="편집기 단축키" />
      {presenting && <PresentationView
        presentationId={id}
        title={presentation.title}
        slides={slidesToPresent(slides)}
        version={`${railVersion}`}
        startIndex={presentIndex}
        onClose={() => setPresenting(false)}
      />}
      <CommandDialog
        open={commandOpen}
        text={commandText}
        plan={commandPlan}
        busy={commandBusy}
        onText={(value) => { setCommandText(value); setCommandPlan(null) }}
        onPlan={() => void planCommand()}
        onRun={() => void runCommand()}
        onClose={() => { setCommandOpen(false); setCommandPlan(null) }}
      />
      <QualityDialog
        open={findingsOpen}
        findings={deckFindings || []}
        score={deckScore}
        canSafelyFix={canSafelyFix}
        aiFixing={aiFixing}
        sweeping={sweeping}
        onOpenSlide={(position) => { const target = slides[position - 1]; if (target) setActiveId(target.id); setFindingsOpen(false) }}
        onSafeFix={(group) => safelyFixFindings(group)}
        onAIFix={(group) => void fixFindingsWithAI(group)}
        onFixEverything={() => void fixEverythingWithAI()}
        onClose={() => setFindingsOpen(false)}
      />
      <FindDialog
        open={findOpen}
        slides={slides}
        onClose={() => setFindOpen(false)}
        onOpenSlide={(position) => { const target = slides[position - 1]; if (target) setActiveId(target.id) }}
        onReplace={replaceEverywhere}
      />
      <CommentsDialog
        open={commentsOpen}
        deckId={id}
        slides={slides}
        onClose={() => setCommentsOpen(false)}
        onGo={(slideId) => { setActiveId(slideId); setCommentsOpen(false) }}
        load={api.comments}
        resolve={api.resolveComment}
        remove={api.deleteComment}
      />
      <ShareDialog
        open={shareOpen}
        deckId={id}
        onClose={() => setShareOpen(false)}
        load={api.shares}
        create={api.createShare}
        revoke={api.revokeShare}
      />
      <HistoryDialog
        open={historyOpen}
        loading={historyLoading}
        version={presentation.version}
        history={history}
        restoring={restoringRevision}
        changes={revisionChanges}
        openChange={openChange}
        onCompare={(checkpoint) => void compareRevision(checkpoint)}
        onRestore={(checkpoint) => void restoreRevision(checkpoint)}
        onClose={() => setHistoryOpen(false)}
      />
		<Modal
			open={conflictOpen}
			onClose={() => setConflictOpen(false)}
			title="다른 창에서 변경된 내용이 있습니다"
			description={conflictKind === 'source' ? '코드를 조용히 덮어쓰지 않았습니다. 서버의 최신 덱을 불러오거나, 내 코드를 최신 버전 위에 적용할 수 있습니다.' : '현재 편집 내용을 조용히 덮어쓰지 않았습니다. 서버의 최신 내용을 불러오거나, 내 변경을 최신 버전 위에 다시 저장할 수 있습니다.'}
			footer={<><Button variant="secondary" disabled={sourceBusy} onClick={() => void useServerVersion()}>서버 버전 불러오기</Button><Button disabled={sourceBusy} onClick={() => void keepLocalVersion()}>{conflictKind === 'source' ? '내 코드 적용' : '내 변경 유지'}</Button></>}
		>
			<p className="modal-note">두 버전을 모두 보존하려면 먼저 내 변경을 유지한 뒤 버전 이력에서 이전 체크포인트를 확인할 수 있습니다.</p>
		</Modal>
      <ExportDialog
        open={exportOpen}
        exporting={exporting}
        onExport={(format) => void exportDeck(format)}
        onClose={() => setExportOpen(false)}
      />
    </main>
  )
}
