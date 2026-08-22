import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { CSSProperties, KeyboardEvent as ReactKeyboardEvent, PointerEvent as ReactPointerEvent } from 'react'
import {
  AlignEndHorizontal, AlignEndVertical, AlignHorizontalJustifyCenter, AlignStartHorizontal, AlignStartVertical,
  AlignVerticalJustifyCenter, ArrowRight, Bold, BringToFront, Copy, CornerUpLeft, Eraser, Eye, EyeOff, Grid3X3,
  Group, ImagePlus, Italic, Layers3, LayoutTemplate, Lock, Minus, MousePointer2, Plus, Redo2, RotateCw, SendToBack, Square,
  Table2, Trash2, Type, Underline, Ungroup, Unlock, Undo2, WandSparkles, ZoomIn, ZoomOut,
} from 'lucide-react'
import { api } from '../api/client'
import type { CanvasRegion, SlideBlock, SlideElement, SlotFrame, SlotStyle } from '../types'
import { SlidePreview } from './SlidePreview'

type Handle = 'nw' | 'n' | 'ne' | 'e' | 'se' | 's' | 'sw' | 'w'
type DragMode = 'move' | 'resize' | 'rotate'

interface Bounds { x: number; y: number; width: number; height: number }
interface DragState {
  mode: DragMode
  handle?: Handle
  pointerId: number
  startX: number
  startY: number
  bounds: Bounds
  originals: SlideElement[]
}

const clone = (elements: SlideElement[]) => elements.map((element) => ({ ...element }))
const clamp = (value: number, minimum: number, maximum: number) => Math.min(maximum, Math.max(minimum, value))
const cleanColor = (value?: string, fallback = '20242D') => (value || fallback).replace(/^#/, '').slice(0, 6)
const shapeNames: Record<string, string> = {
  rect: '사각형', roundRect: '둥근 사각형', ellipse: '원/타원', triangle: '삼각형', diamond: '마름모',
  rightArrow: '오른쪽 화살표', star5: '별', hexagon: '육각형',
}

function boundsOf(elements: SlideElement[]): Bounds {
  if (elements.length === 0) return { x: 0, y: 0, width: 0, height: 0 }
  const left = Math.min(...elements.map((element) => element.x))
  const top = Math.min(...elements.map((element) => element.y))
  const right = Math.max(...elements.map((element) => element.x + element.width))
  const bottom = Math.max(...elements.map((element) => element.y + element.height))
  return { x: left, y: top, width: right - left, height: bottom - top }
}

function nextID() { return `element-${crypto.randomUUID()}` }

/** What a template region holds, named the way the workspace names it. */
const regionNames: Record<string, string> = {
  title: '제목', subtitle: '리드 문장', body: '본문', body1: '본문', body2: '본문 2', body3: '본문 3',
  body4: '본문 4', notes: '노트', picture: '이미지', chart: '차트', table: '표',
}
const blockNames: Record<string, string> = {
  bullets: '목록', kpi: '핵심 지표', hero: '대표 숫자', steps: '단계', timeline: '타임라인',
  comparison: '비교', columnChart: '세로 막대', barChart: '가로 막대', lineChart: '추이',
  shareBar: '비중 바', meter: '달성률', table: '표', quote: '인용', callout: '강조', grid: '격자',
}
/** The components a person can switch between without losing what they wrote. */
const switchableBlocks = ['bullets', 'kpi', 'hero', 'steps', 'timeline', 'comparison', 'columnChart', 'barChart', 'lineChart', 'shareBar', 'meter', 'table', 'quote', 'callout']

function regionLabel(region: CanvasRegion) {
  if (region.kind === 'component') return blockNames[String(region.block?.kind || '')] || '컴포넌트'
  if (region.kind === 'picture') return '이미지'
  return regionNames[region.slot] || region.name || region.slot
}

/**
 * What a component looks like the moment it is added, so a region filled from the
 * canvas is a real component to edit rather than an empty frame to puzzle over.
 */
function starterBlock(kind: string): SlideBlock {
  switch (kind) {
    case 'kpi': return { kind, items: [{ label: '지표 1', value: '0' }, { label: '지표 2', value: '0' }, { label: '지표 3', value: '0' }] }
    case 'hero': return { kind, items: [{ label: '핵심 숫자', value: '0', detail: '무엇을 뜻하는지' }] }
    case 'meter': return { kind, items: [{ label: '달성률', value: '72%' }] }
    case 'steps': return { kind, items: [{ label: '준비', value: '무엇을 합니다' }, { label: '이행', value: '무엇을 합니다' }, { label: '안정화', value: '무엇을 합니다' }] }
    case 'timeline': return { kind, items: [{ label: '1분기', value: '무엇을 합니다' }, { label: '2분기', value: '무엇을 합니다' }, { label: '3분기', value: '무엇을 합니다' }] }
    case 'comparison': return { kind, items: [{ label: '선택 A', value: '한 줄 요약', detail: '근거' }, { label: '선택 B', value: '한 줄 요약', detail: '근거' }] }
    case 'columnChart': case 'barChart': case 'shareBar':
      return { kind, items: [{ label: '항목 1', value: '40' }, { label: '항목 2', value: '35' }, { label: '항목 3', value: '25' }] }
    case 'lineChart': return { kind, items: [{ label: '추이', value: '10, 14, 19, 26' }] }
    case 'table': return { kind, rows: [['항목', '값'], ['첫 번째', ''], ['두 번째', '']] }
    case 'quote': return { kind, text: '기억에 남을 한 문장', items: [{ label: '출처' }] }
    case 'callout': return { kind, text: '놓치면 안 되는 한 가지' }
  }
  return { kind: 'bullets', items: [{ label: '첫 번째 요점' }, { label: '두 번째 요점' }, { label: '세 번째 요점' }] }
}

/** The rows a component carries, as label / value / detail triples. */
interface BlockItem { label: string; value: string; detail: string }
function blockItems(block?: SlideBlock): BlockItem[] {
  if (!block?.items) return []
  return block.items.map((item) => ({
    label: String((item as Record<string, unknown>).label ?? ''),
    value: String((item as Record<string, unknown>).value ?? ''),
    detail: String((item as Record<string, unknown>).detail ?? ''),
  }))
}
/** Writes the edited rows back, keeping every field the renderer knows about. */
function withItems(block: SlideBlock, items: BlockItem[]): SlideBlock {
  const existing = block.items || []
  return {
    ...block,
    items: items.map((item, index) => {
      const carried = { ...(existing[index] as Record<string, unknown> | undefined) }
      delete carried.label; delete carried.value; delete carried.detail
      // A number the renderer plots was parsed from the value; re-parse it so a
      // chart follows the figure the author just typed.
      if (carried.number !== undefined) {
        const parsed = Number(item.value.replace(/[^0-9.-]/g, ''))
        carried.number = Number.isFinite(parsed) ? parsed : carried.number
      }
      const written: Record<string, unknown> = { ...carried }
      if (item.label) written.label = item.label
      if (item.value) written.value = item.value
      if (item.detail) written.detail = item.detail
      return written
    }),
  }
}

function elementLabel(element: SlideElement) {
  if (element.name) return element.name
  if (element.kind === 'text') return element.text?.split('\n')[0] || '텍스트 상자'
  if (element.kind === 'shape') return shapeNames[element.shape || 'rect'] || '도형'
  if (element.kind === 'line') return '선'
  if (element.kind === 'table') return element.name || '표'
  return '이미지'
}

function shapeStyle(element: SlideElement): CSSProperties {
  const style: CSSProperties = {
    width: '100%', height: '100%', background: element.fill && element.fill !== 'transparent' ? `#${cleanColor(element.fill, '725BD6')}` : 'transparent',
    border: element.stroke && element.stroke !== 'transparent' ? `${Math.max(.5, element.strokeWidth || 1)}px solid #${cleanColor(element.stroke, '4C3AA0')}` : 'none',
  }
  switch (element.shape) {
    case 'roundRect': style.borderRadius = '12%'; break
    case 'ellipse': style.borderRadius = '50%'; break
    case 'triangle': style.clipPath = 'polygon(50% 0, 100% 100%, 0 100%)'; break
    case 'diamond': style.clipPath = 'polygon(50% 0, 100% 50%, 50% 100%, 0 50%)'; break
    case 'rightArrow': style.clipPath = 'polygon(0 25%, 62% 25%, 62% 0, 100% 50%, 62% 100%, 62% 75%, 0 75%)'; break
    case 'star5': style.clipPath = 'polygon(50% 0, 61% 35%, 98% 35%, 68% 57%, 79% 94%, 50% 72%, 21% 94%, 32% 57%, 2% 35%, 39% 35%)'; break
    case 'hexagon': style.clipPath = 'polygon(25% 0, 75% 0, 100% 50%, 75% 100%, 25% 100%, 0 50%)'; break
  }
  return style
}

function LineMarker({ id, kind, color }: { id: string; kind?: string; color: string }) {
  if (!kind || kind === 'none') return null
  const shape = kind === 'diamond'
    ? <path d="M 0 5 L 5 0 L 10 5 L 5 10 z" fill={color} />
    : kind === 'oval'
      ? <circle cx="5" cy="5" r="4" fill={color} />
      : kind === 'stealth'
        ? <path d="M 0 1 L 10 5 L 0 9 L 3 5 z" fill={color} />
        : <path d="M 0 0 L 10 5 L 0 10 z" fill={color} />
  return <marker id={id} viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">{shape}</marker>
}

export function FreeformCanvas({
  presentationId, position, slideId, elements, frames, styles, baseVersion, aiEnabled = true,
  onChange, onRegionText, onRegionBlock, onRegionFrames, onRegionStyle, onPickImage, onImageFiles, onRegionClear,
  onCheckpoint, onUndo, onRedo, canUndo, canRedo, onRevise, onUndoRevise, canUndoRevise,
}: {
  presentationId: string
  /** 1-based position of the slide being edited. */
  position: number
  slideId: string
  elements: SlideElement[]
  frames: Record<string, SlotFrame>
  styles: Record<string, SlotStyle>
  /** Changes when the server's copy of the slide changes, which is when the
   * drawing and the regions are worth fetching again. */
  baseVersion: string | number
  aiEnabled?: boolean
  onChange: (elements: SlideElement[]) => void
  onRegionText: (slot: string, text: string) => void
  onRegionBlock: (slot: string, block: SlideBlock) => void
  onRegionFrames: (frames: Record<string, SlotFrame>) => void
  onRegionStyle: (slot: string, patch: SlotStyle | null) => void
  /** Opens the image library to fill one region. */
  onPickImage: (slot: string) => void
  /** Uploads image files and places them, at a point on the slide if given. */
  onImageFiles?: (files: File[], at?: { x: number; y: number }) => void
  onRegionClear: (slot: string) => void
  /** Records the slide as it stands, before the change about to be made. */
  onCheckpoint: (reason: string) => void
  onUndo: () => void
  onRedo: () => void
  canUndo: boolean
  canRedo: boolean
  onRevise: (input: { action: string; instruction: string; slot: string }) => Promise<void>
  onUndoRevise: () => void
  canUndoRevise: boolean
}) {
  const editorRoot = useRef<HTMLDivElement>(null)
  const [selected, setSelected] = useState<string[]>([])
  const [editing, setEditing] = useState('')
  const [zoom, setZoom] = useState(100)
  const [showGrid, setShowGrid] = useState(true)
  const [snap, setSnap] = useState(true)
  const [shape, setShape] = useState('rect')
  const [layersOpen, setLayersOpen] = useState(false)
  const [assetURLs, setAssetURLs] = useState<Record<string, string>>({})
  // The slide's own regions: what the generator wrote, as objects rather than
  // as pixels in a picture.
  const [regions, setRegions] = useState<CanvasRegion[]>([])
  const [slideHeightPoints, setSlideHeightPoints] = useState(540)
  const [aspect, setAspect] = useState(16 / 9)
  const [regionSlot, setRegionSlot] = useState('')
  const [regionEditing, setRegionEditing] = useState('')
  const [regionDraft, setRegionDraft] = useState('')
  const [regionOffset, setRegionOffset] = useState<{ x: number; y: number } | null>(null)
  const [guides, setGuides] = useState<{ axis: 'x' | 'y'; at: number }[]>([])
  // A rubber band drawn over empty page, and the menu a right-click opens.
  const [marquee, setMarquee] = useState<Bounds | null>(null)
  const marqueeStart = useRef<{ x: number; y: number } | null>(null)
  const [menu, setMenu] = useState<{ x: number; y: number; target: 'element' | 'region' | 'page' } | null>(null)
  const [regionBox, setRegionBox] = useState<SlotFrame | null>(null)
  const [spriteURL, setSpriteURL] = useState('')
  // Where the server drew the lifted region, which is what a drag offsets from.
  const [spriteAt, setSpriteAt] = useState<SlotFrame | null>(null)
  const [showRegions, setShowRegions] = useState(true)
  const [aiInstruction, setAiInstruction] = useState('')
  const [aiBusy, setAiBusy] = useState(false)
  const regionDrag = useRef<{ mode: DragMode; handle?: Handle; pointerId: number; startX: number; startY: number; frame: SlotFrame; slot: string } | null>(null)
  const page = useRef<HTMLDivElement>(null)
  const clipboard = useRef<SlideElement[]>([])
  const drag = useRef<DragState | null>(null)
  const elementsRef = useRef(elements)
  elementsRef.current = elements

  useEffect(() => {
    setSelected([])
    setEditing('')
    setRegionSlot('')
    setRegionEditing('')
  }, [slideId])

  // The regions come from the server, drawn through the real template, so what
  // the canvas offers to edit is exactly what the exported file contains.
  useEffect(() => {
    let active = true
    api.slideRegions(presentationId, position).then((result) => {
      if (!active) return
      setRegions(result.regions)
      setSlideHeightPoints(result.slideHeightPoints)
      setAspect(result.aspectRatio)
    }).catch(() => { if (active) setRegions([]) })
    return () => { active = false }
  }, [presentationId, position, baseVersion])

  const region = useMemo(() => regions.find((candidate) => candidate.slot === regionSlot), [regions, regionSlot])
  const regionDrawn = Boolean(region && region.kind !== 'empty')
  // A selected region is lifted off the page: the base is drawn without it and
  // the region itself is drawn above, so dragging moves the drawing rather than
  // an outline over a stale copy of it.
  const lifted = regionDrawn ? regionSlot : ''
  const regionFrame = useCallback((candidate: CanvasRegion) => frames[candidate.slot] || candidate.frame, [frames])

  // The lifted region is drawn where the server last saved it, so where it was
  // drawn has to be recorded with it: the offset a drag adds is measured from
  // there, and reading it from anywhere else applies the same move twice.
  const framesRef = useRef(frames)
  framesRef.current = frames
  const regionsRef = useRef(regions)
  regionsRef.current = regions
  useEffect(() => {
    if (!lifted) { setSpriteURL(''); setSpriteAt(null); return }
    let active = true
    let created = ''
    api.slidePreview(presentationId, position, 1400, false, { only: lifted }).then((url) => {
      if (!active) { URL.revokeObjectURL(url); return }
      created = url
      const drawn = regionsRef.current.find((candidate) => candidate.slot === lifted)
      setSpriteAt(framesRef.current[lifted] || drawn?.frame || null)
      setSpriteURL(url)
    }).catch(() => { if (active) setSpriteURL('') })
    return () => { active = false; if (created) URL.revokeObjectURL(created) }
  }, [presentationId, position, lifted, baseVersion])

  useEffect(() => {
    const assetIDs = [...new Set(elements.filter((element) => element.kind === 'image' && element.assetId).map((element) => element.assetId!))]
    let active = true
    const created: string[] = []
    void Promise.all(assetIDs.map(async (assetID) => {
      try {
        const url = await api.assetImage(assetID)
        created.push(url)
        if (active) setAssetURLs((current) => ({ ...current, [assetID]: url }))
      } catch { /* A missing asset is shown as an empty image frame. */ }
    }))
    return () => { active = false; created.forEach(URL.revokeObjectURL) }
  }, [slideId, elements.map((element) => element.assetId || '').join('|')])

  // Every change goes through one history, kept by the editor: objects, regions,
  // components and type are all the same slide, and undo has to mean that.
  const pushHistory = useCallback((reason = 'objects') => onCheckpoint(reason), [onCheckpoint])

  const commit = useCallback((next: SlideElement[], record = true) => {
    if (record) pushHistory()
    onChange(next.map((element, index) => ({ ...element, zIndex: element.zIndex ?? index })))
  }, [onChange, pushHistory])

  const selectedElements = useMemo(() => elements.filter((element) => selected.includes(element.id) && !element.hidden), [elements, selected])
  const selectedBounds = useMemo(() => boundsOf(selectedElements), [selectedElements])
  const primary = selectedElements[0]
  const layerElements = useMemo(() => [...elements].sort((a, b) => (b.zIndex || 0) - (a.zIndex || 0)), [elements])

  const resolveSelection = (element: SlideElement, additive: boolean) => {
    const grouped = element.groupId ? elements.filter((candidate) => candidate.groupId === element.groupId).map((candidate) => candidate.id) : [element.id]
    if (additive) {
      const next = new Set(selected)
      for (const id of grouped) next.has(id) ? next.delete(id) : next.add(id)
      return [...next]
    }
    return selected.includes(element.id) ? selected : grouped
  }

  // Double-click, detected here rather than left to the dblclick event.
  //
  // Dragging captures the pointer on the page, and a captured pointer retargets
  // the mouse events that follow it — so the browser's own dblclick lands on the
  // page instead of on the object that was pressed, and never opens it for
  // typing. Two presses on the same object inside 450ms is the same gesture and
  // does not depend on capture.
  const lastPress = useRef({ id: '', time: 0 })
  const doublePress = (id: string, time: number) => {
    const again = lastPress.current.id === id && time - lastPress.current.time < 450
    lastPress.current = { id, time: again ? 0 : time }
    return again
  }

  const point = (event: { clientX: number; clientY: number }) => {
    const rect = page.current?.getBoundingClientRect()
    if (!rect) return { x: 0, y: 0 }
    return { x: (event.clientX - rect.left) / rect.width * 100, y: (event.clientY - rect.top) / rect.height * 100 }
  }
  const snapped = (value: number) => snap ? Math.round(value * 2) / 2 : value

  // ── Images from outside the workspace ──────────────────────────────────────
  // A screenshot is on the clipboard and a photograph is in a folder. Both used
  // to mean: upload in the images panel, find it in the list, press place. Both
  // now mean: paste, or drop it where it goes.
  const [dropping, setDropping] = useState(false)
  const imageFiles = (list: FileList | File[] | null | undefined) =>
    Array.from(list || []).filter((file) => file.type.startsWith('image/'))

  // Ctrl+V is answered here rather than in the key handler, because a key
  // handler that prevents the default never learns what was on the clipboard —
  // and a screenshot is on the clipboard far more often than a copied shape.
  useEffect(() => {
    const onPaste = (event: ClipboardEvent) => {
      const target = event.target as HTMLElement | null
      // A paste into a text box is text, even when the clipboard also holds an
      // image: the person is typing.
      if (target?.matches?.('input, textarea, [contenteditable="true"]')) return
      if (!editorRoot.current?.contains(document.activeElement)) return
      const files = imageFiles(event.clipboardData?.files)
      if (files.length > 0 && onImageFiles) {
        event.preventDefault()
        onImageFiles(files)
        return
      }
      if (clipboard.current.length > 0) {
        event.preventDefault()
        paste()
      }
    }
    window.addEventListener('paste', onPaste)
    return () => window.removeEventListener('paste', onPaste)
  })

  // ── Alignment guides ───────────────────────────────────────────────────────
  // What makes a slide look composed is that things line up with each other, not
  // that they landed on a grid. So a dragged object is offered the edges and
  // centres of everything already on the slide, and the slide's own centre.
  const guideLines = (movingIDs: Set<string>, movingSlot: string) => {
    const vertical: number[] = [0, 50, 100]
    const horizontal: number[] = [0, 50, 100]
    for (const candidate of regions) {
      if (candidate.slot === movingSlot || candidate.spannedBy || candidate.kind === 'empty') continue
      const frame = regionFrame(candidate)
      vertical.push(frame.x, frame.x + frame.width / 2, frame.x + frame.width)
      horizontal.push(frame.y, frame.y + frame.height / 2, frame.y + frame.height)
    }
    for (const element of elements) {
      if (movingIDs.has(element.id) || element.hidden) continue
      vertical.push(element.x, element.x + element.width / 2, element.x + element.width)
      horizontal.push(element.y, element.y + element.height / 2, element.y + element.height)
    }
    return { vertical, horizontal }
  }

  const snapToGuides = (box: Bounds, movingIDs: Set<string>, movingSlot: string) => {
    if (!snap) return { dx: 0, dy: 0, guides: [] as { axis: 'x' | 'y'; at: number }[] }
    const tolerance = 0.8
    const lines = guideLines(movingIDs, movingSlot)
    const nearest = (edges: number[], candidates: number[]) => {
      let best: { shift: number; at: number } | null = null
      for (const edge of edges) {
        for (const candidate of candidates) {
          const distance = Math.abs(candidate - edge)
          if (distance > tolerance) continue
          if (!best || distance < Math.abs(best.shift)) best = { shift: candidate - edge, at: candidate }
        }
      }
      return best
    }
    const horizontalHit = nearest([box.x, box.x + box.width / 2, box.x + box.width], lines.vertical)
    const verticalHit = nearest([box.y, box.y + box.height / 2, box.y + box.height], lines.horizontal)
    const found: { axis: 'x' | 'y'; at: number }[] = []
    if (horizontalHit) found.push({ axis: 'x', at: horizontalHit.at })
    if (verticalHit) found.push({ axis: 'y', at: verticalHit.at })
    return { dx: horizontalHit?.shift || 0, dy: verticalHit?.shift || 0, guides: found }
  }

  // ── The slide's own regions ────────────────────────────────────────────────
  // A generated slide is edited here, not only drawn on: the title the model
  // wrote and a text box someone added answer the same click, the same drag and
  // the same Delete.
  const [regionDirty, setRegionDirty] = useState(false)
  const shown = region ? (regionBox || regionFrame(region)) : null
  /** Type sized the way the renderer sizes it: a point size is a fraction of the
   *  slide's height, whatever width the canvas happens to be drawn at. */
  const regionTextStyle = (candidate: CanvasRegion): CSSProperties => {
    // The region as the server drew it, adjusted by anything restyled since and
    // not yet saved — so typing shows the size and colour it will print at.
    const local = styles[candidate.slot] || {}
    const factor = (local.scale || 1) / (candidate.style?.scale || 1)
    const aligned = local.align || (candidate.align === 'ctr' ? 'center' : candidate.align === 'r' ? 'right' : candidate.align === 'just' ? 'justify' : undefined)
    return {
      fontSize: `${((candidate.fontSize || 18) * factor / slideHeightPoints * 100 / aspect).toFixed(3)}cqw`,
      color: `#${cleanColor(local.color || candidate.color, '20242D')}`,
      fontFamily: `${candidate.font && !candidate.font.startsWith('+') ? `${candidate.font}, ` : ''}'Noto Sans KR', sans-serif`,
      fontWeight: (local.bold ?? (candidate.bold || candidate.slot === 'title')) ? 700 : 400,
      fontStyle: (local.italic ?? candidate.italic) ? 'italic' : 'normal',
      textAlign: aligned as CSSProperties['textAlign'],
    }
  }

  const selectRegion = (slot: string) => {
    setSelected([])
    setEditing('')
    setRegionEditing('')
    setRegionSlot(slot)
  }

  const beginRegionDrag = (event: ReactPointerEvent, mode: DragMode, handle?: Handle) => {
    if (!region || !regionDrawn || regionEditing) return
    // Deliberately not preventDefault: cancelling the press would take the second
    // click with it, and the second click is how a region is opened for typing.
    event.stopPropagation()
    const start = point(event)
    regionDrag.current = { mode, handle, pointerId: event.pointerId, startX: start.x, startY: start.y, frame: regionFrame(region), slot: region.slot }
    page.current?.setPointerCapture(event.pointerId)
  }

  const onRegionPointerMove = (event: ReactPointerEvent) => {
    const operation = regionDrag.current
    if (!operation) return
    const current = point(event)
    const dx = snapped(current.x - operation.startX)
    const dy = snapped(current.y - operation.startY)
    if (operation.mode === 'move') {
      // A quarter of a region may hang off the slide — a deliberate bleed is a
      // real design — but no further: a region dragged past that is lost rather
      // than placed.
      const bleedX = operation.frame.width / 4
      const bleedY = operation.frame.height / 4
      const moved = {
        ...operation.frame,
        x: clamp(operation.frame.x + dx, -bleedX, 100 - operation.frame.width + bleedX),
        y: clamp(operation.frame.y + dy, -bleedY, 100 - operation.frame.height + bleedY),
      }
      const aligned = snapToGuides(moved, new Set(), operation.slot)
      setGuides(aligned.guides)
      setRegionBox({ ...moved, x: moved.x + aligned.dx, y: moved.y + aligned.dy })
      return
    }
    const handle = operation.handle || 'se'
    let left = operation.frame.x
    let top = operation.frame.y
    let right = left + operation.frame.width
    let bottom = top + operation.frame.height
    if (handle.includes('w')) left = clamp(snapped(left + dx), -20, right - 4)
    if (handle.includes('e')) right = clamp(snapped(right + dx), left + 4, 120)
    if (handle.includes('n')) top = clamp(snapped(top + dy), -20, bottom - 3)
    if (handle.includes('s')) bottom = clamp(snapped(bottom + dy), top + 3, 120)
    setRegionBox({ x: left, y: top, width: right - left, height: bottom - top })
  }

  const endRegionDrag = (event: ReactPointerEvent) => {
    setGuides([])
    const operation = regionDrag.current
    if (!operation) return
    regionDrag.current = null
    if (page.current?.hasPointerCapture(event.pointerId)) page.current.releasePointerCapture(event.pointerId)
    if (!region || !regionBox) return
    const round = (value: number) => Math.round(value * 100) / 100
    onRegionFrames({
      ...frames,
      [region.slot]: { x: round(regionBox.x), y: round(regionBox.y), width: round(regionBox.width), height: round(regionBox.height) },
    })
  }

  const resetRegionFrame = () => {
    if (!region) return
    const next = { ...frames }
    delete next[region.slot]
    onRegionFrames(next)
    setRegionBox(null)
  }

  const startRegionEdit = () => {
    if (!region || !region.acceptsText || region.kind === 'component' || region.kind === 'picture') return
    setRegionDraft(region.text || '')
    setRegionEditing(region.slot)
  }

  const commitRegionEdit = () => {
    if (!regionEditing) return
    const slot = regionEditing
    setRegionEditing('')
    const before = regions.find((candidate) => candidate.slot === slot)?.text || ''
    if (regionDraft === before) return
    setRegionDirty(true)
    onRegionText(slot, regionDraft)
  }

  // A refreshed slide is the answer to what was typed, so the draft stops
  // standing in for it.
  useEffect(() => { setRegionDirty(false); setRegionBox(null) }, [baseVersion])

  const style = region ? (styles[region.slot] || {}) : {}
  const patchStyle = (patch: SlotStyle | null) => {
    if (!region) return
    // Stand the region's own text in for the drawing until the server redraws it,
    // so a size, colour or alignment change is visible as it is made.
    setRegionDraft(region.text || '')
    setRegionDirty(true)
    onRegionStyle(region.slot, patch)
  }

  const patchBlock = (patch: Partial<SlideBlock>) => {
    if (!region?.block) return
    setRegionDirty(true)
    onRegionBlock(region.slot, { ...region.block, ...patch })
  }
  const patchBlockItems = (items: BlockItem[]) => {
    if (!region?.block) return
    setRegionDirty(true)
    onRegionBlock(region.slot, withItems(region.block, items))
  }

  const runRevision = async (action: string) => {
    if (aiBusy) return
    setAiBusy(true)
    try {
      await onRevise({ action, instruction: aiInstruction.trim(), slot: regionSlot })
      setAiInstruction('')
    } finally {
      setAiBusy(false)
    }
  }

  const beginDrag = (event: ReactPointerEvent, mode: DragMode, handle?: Handle, forcedSelection?: string[]) => {
    event.preventDefault()
    event.stopPropagation()
    const ids = forcedSelection || selected
    let originals = elements.filter((element) => ids.includes(element.id) && !element.locked)
    if (originals.length === 0) return
    const start = point(event)
    pushHistory()
    // Alt-dragging leaves the original where it was and drags a copy, which is
    // how a row of identical boxes gets made anywhere else.
    if (event.altKey && mode === 'move') {
      const copies = originals.map((element) => ({ ...element, id: nextID(), groupId: undefined, locked: false }))
      onChange([...elementsRef.current, ...copies])
      setSelected(copies.map((element) => element.id))
      originals = copies
    }
    drag.current = { mode, handle, pointerId: event.pointerId, startX: start.x, startY: start.y, bounds: boundsOf(originals), originals: clone(originals) }
    page.current?.setPointerCapture(event.pointerId)
  }

  const onPointerMove = (event: ReactPointerEvent) => {
    const operation = drag.current
    if (!operation) return
    const current = point(event)
    const dx = snapped(current.x - operation.startX)
    const dy = snapped(current.y - operation.startY)
    let replacement = operation.originals
    if (operation.mode === 'move') {
      const group = operation.bounds
      // Shift locks the drag to the axis it has travelled furthest along, the way
      // every drawing tool does: a row of boxes stays a row.
      const lockedX = event.shiftKey && Math.abs(dx) < Math.abs(dy) ? 0 : dx
      const lockedY = event.shiftKey && Math.abs(dy) <= Math.abs(dx) ? 0 : dy
      let boundedDX = clamp(lockedX, -group.x, 100 - group.x - group.width)
      let boundedDY = clamp(lockedY, -group.y, 100 - group.y - group.height)
      const aligned = snapToGuides({ ...group, x: group.x + boundedDX, y: group.y + boundedDY },
        new Set(operation.originals.map((element) => element.id)), '')
      setGuides(aligned.guides)
      boundedDX += aligned.dx
      boundedDY += aligned.dy
      replacement = operation.originals.map((element) => ({ ...element, x: snapped(element.x + boundedDX), y: snapped(element.y + boundedDY) }))
    } else if (operation.mode === 'resize') {
      const handle = operation.handle || 'se'
      let left = operation.bounds.x
      let top = operation.bounds.y
      let right = left + operation.bounds.width
      let bottom = top + operation.bounds.height
      if (handle.includes('w')) left = clamp(snapped(left + dx), 0, right - 1)
      if (handle.includes('e')) right = clamp(snapped(right + dx), left + 1, 100)
      if (handle.includes('n')) top = clamp(snapped(top + dy), 0, bottom - 1)
      if (handle.includes('s')) bottom = clamp(snapped(bottom + dy), top + 1, 100)
      const next = { x: left, y: top, width: right - left, height: bottom - top }
      // Shift keeps the proportions, so a picture resized by its corner is not
      // quietly squashed.
      if (event.shiftKey && operation.bounds.width > 0 && operation.bounds.height > 0) {
        const ratio = operation.bounds.width / operation.bounds.height
        const byWidth = next.width / operation.bounds.width >= next.height / operation.bounds.height
        if (byWidth) next.height = next.width / ratio
        else next.width = next.height * ratio
        if (handle.includes('n')) next.y = bottom - next.height
        if (handle.includes('w')) next.x = right - next.width
      }
      const sx = next.width / Math.max(operation.bounds.width, .1)
      const sy = next.height / Math.max(operation.bounds.height, .1)
      replacement = operation.originals.map((element) => ({
        ...element,
        x: snapped(next.x + (element.x - operation.bounds.x) * sx),
        y: snapped(next.y + (element.y - operation.bounds.y) * sy),
        width: Math.max(.5, snapped(element.width * sx)),
        height: Math.max(.5, snapped(element.height * sy)),
      }))
    } else {
      const centerX = operation.bounds.x + operation.bounds.width / 2
      const centerY = operation.bounds.y + operation.bounds.height / 2
      const startAngle = Math.atan2(operation.startY - centerY, operation.startX - centerX)
      const currentAngle = Math.atan2(current.y - centerY, current.x - centerX)
      let delta = (currentAngle - startAngle) * 180 / Math.PI
      // Shift turns in fifteen-degree steps.
      if (event.shiftKey) delta = Math.round(delta / 15) * 15
      const radians = delta * Math.PI / 180
      replacement = operation.originals.map((element) => {
        const elementCenterX = element.x + element.width / 2
        const elementCenterY = element.y + element.height / 2
        const relativeX = elementCenterX - centerX
        const relativeY = elementCenterY - centerY
        const rotatedX = centerX + relativeX * Math.cos(radians) - relativeY * Math.sin(radians)
        const rotatedY = centerY + relativeX * Math.sin(radians) + relativeY * Math.cos(radians)
        return {
          ...element,
          x: rotatedX - element.width / 2,
          y: rotatedY - element.height / 2,
          rotation: Math.round(((element.rotation || 0) + delta) * 10) / 10,
        }
      })
    }
    const nextByID = new Map(replacement.map((element) => [element.id, element]))
    onChange(elementsRef.current.map((element) => nextByID.get(element.id) || element))
  }

  const onMarqueeMove = (event: ReactPointerEvent) => {
    const start = marqueeStart.current
    if (!start || drag.current || regionDrag.current) return
    const current = point(event)
    setMarquee({
      x: Math.min(start.x, current.x), y: Math.min(start.y, current.y),
      width: Math.abs(current.x - start.x), height: Math.abs(current.y - start.y),
    })
  }

  const endMarquee = () => {
    const band = marquee
    marqueeStart.current = null
    setMarquee(null)
    if (!band || band.width < 1 || band.height < 1) return
    const caught = elements.filter((element) => !element.hidden && !element.locked
      && element.x < band.x + band.width && element.x + element.width > band.x
      && element.y < band.y + band.height && element.y + element.height > band.y)
    if (caught.length > 0) setSelected(caught.map((element) => element.id))
  }

  const endDrag = (event: ReactPointerEvent) => {
    setGuides([])
    if (!drag.current) return
    if (page.current?.hasPointerCapture(event.pointerId)) page.current.releasePointerCapture(event.pointerId)
    drag.current = null
  }

  const add = (kind: SlideElement['kind'], requestedShape?: string) => {
    const highest = Math.max(0, ...elements.map((element) => element.zIndex || 0))
    const defaults: SlideElement = {
      id: nextID(), kind, x: kind === 'table' ? 22 : 30, y: kind === 'table' ? 28 : 32,
      width: kind === 'table' ? 56 : kind === 'line' ? 30 : 26,
      height: kind === 'table' ? 34 : kind === 'line' ? 8 : kind === 'text' ? 12 : 22,
      zIndex: highest + 1, rotation: 0, opacity: 100,
      /* A new text box starts empty. It used to start with the prompt as its
         content, so a box the author clicked away from without typing shipped
         "텍스트를 입력하세요" on the slide — in the preview and in the file. */
      ...(kind === 'text' ? { text: '', fontSize: 24, fontFamily: 'Aptos', textColor: '20242D', align: 'left', verticalAlign: 'middle' } : {}),
      ...(kind === 'shape' ? { shape: requestedShape || shape, fill: '725BD6', stroke: '4C3AA0', strokeWidth: 1 } : {}),
      ...(kind === 'line' ? { shape: 'line', stroke: '4C3AA0', strokeWidth: 2 } : {}),
      ...(kind === 'table' ? {
        cells: [['항목', '담당', '상태'], ['첫 번째', '김담당', '진행 중'], ['두 번째', '이담당', '대기']],
        headerRows: 1, headerColumns: 0, fontSize: 14, fontFamily: 'Aptos', textColor: '20242D',
        fill: '725BD6', stroke: 'D9D6E1', strokeWidth: 1,
      } : {}),
    }
    commit([...elements, defaults])
    setSelected([defaults.id])
    if (kind === 'text') window.setTimeout(() => setEditing(defaults.id), 0)
  }

  // A text box nobody typed into is nothing, so leaving it empty removes it
  // rather than leaving an invisible box on the slide for someone to find later
  // by clicking on it.
  const finishEditing = (element: SlideElement) => {
    setEditing('')
    if (element.kind !== 'text' || (element.text || '').trim() !== '') return
    commit(elements.filter((candidate) => candidate.id !== element.id))
    setSelected([])
  }

  const patchSelected = (patch: Partial<SlideElement>, record = true) => {
    if (selected.length === 0) return
    const mayUnlock = Object.prototype.hasOwnProperty.call(patch, 'locked')
    commit(elements.map((element) => selected.includes(element.id) && (mayUnlock || !element.locked) ? { ...element, ...patch } : element), record)
  }

  const patchTableCells = (cells: string[][], record = true) => patchSelected({ cells }, record)
  const addTableRow = () => {
    if (!primary || primary.kind !== 'table') return
    const cells = (primary.cells || [['']]).map((row) => [...row])
    if (cells.length >= 50) return
    cells.push(Array.from({ length: cells[0]?.length || 1 }, () => ''))
    patchTableCells(cells)
  }
  const removeTableRow = () => {
    if (!primary || primary.kind !== 'table' || (primary.cells?.length || 0) <= 1) return
    const cells = primary.cells!.slice(0, -1).map((row) => [...row])
    patchSelected({ cells, headerRows: Math.min(primary.headerRows || 0, cells.length) })
  }
  const addTableColumn = () => {
    if (!primary || primary.kind !== 'table') return
    const cells = (primary.cells || [['']]).map((row) => row.length >= 20 ? [...row] : [...row, ''])
    if ((primary.cells?.[0]?.length || 0) >= 20) return
    patchTableCells(cells)
  }
  const removeTableColumn = () => {
    if (!primary || primary.kind !== 'table' || (primary.cells?.[0]?.length || 0) <= 1) return
    const cells = primary.cells!.map((row) => row.slice(0, -1))
    patchSelected({ cells, headerColumns: Math.min(primary.headerColumns || 0, cells[0].length) })
  }

  const patchOne = (id: string, patch: Partial<SlideElement>) => {
    pushHistory()
    onChange(elements.map((element) => element.id === id ? { ...element, ...patch } : element))
  }

  const removeSelected = () => {
    if (selected.length === 0) return
    commit(elements.filter((element) => !selected.includes(element.id) || element.locked))
    setSelected((current) => current.filter((id) => elements.find((element) => element.id === id)?.locked))
  }

  const duplicateSelected = () => {
    if (selectedElements.length === 0) return
    const groupMap = new Map<string, string>()
    const copies = selectedElements.map((element) => {
      let groupId = element.groupId
      if (groupId) {
        if (!groupMap.has(groupId)) groupMap.set(groupId, `group-${crypto.randomUUID()}`)
        groupId = groupMap.get(groupId)
      }
      return { ...element, id: nextID(), x: clamp(element.x + 2, 0, 100 - element.width), y: clamp(element.y + 2, 0, 100 - element.height), groupId, locked: false }
    })
    commit([...elements, ...copies])
    setSelected(copies.map((element) => element.id))
  }

  const undo = () => { setSelected([]); setRegionEditing(''); onUndo() }
  const redo = () => { setSelected([]); setRegionEditing(''); onRedo() }

  const group = () => {
    if (selectedElements.length < 2) return
    patchSelected({ groupId: `group-${crypto.randomUUID()}` })
  }
  const ungroup = () => patchSelected({ groupId: undefined })

  const align = (mode: 'left' | 'center' | 'right' | 'top' | 'middle' | 'bottom') => {
    if (selectedElements.length < 2) return
    const bounds = boundsOf(selectedElements)
    pushHistory()
    onChange(elements.map((element) => {
      if (!selected.includes(element.id) || element.locked) return element
      switch (mode) {
        case 'left': return { ...element, x: bounds.x }
        case 'center': return { ...element, x: bounds.x + (bounds.width - element.width) / 2 }
        case 'right': return { ...element, x: bounds.x + bounds.width - element.width }
        case 'top': return { ...element, y: bounds.y }
        case 'middle': return { ...element, y: bounds.y + (bounds.height - element.height) / 2 }
        case 'bottom': return { ...element, y: bounds.y + bounds.height - element.height }
      }
    }))
  }

  const distribute = (axis: 'horizontal' | 'vertical') => {
    if (selectedElements.length < 3) return
    const ordered = [...selectedElements].sort((a, b) => axis === 'horizontal' ? a.x - b.x : a.y - b.y)
    const start = axis === 'horizontal' ? ordered[0].x : ordered[0].y
    const endItem = ordered[ordered.length - 1]
    const end = axis === 'horizontal' ? endItem.x + endItem.width : endItem.y + endItem.height
    const size = ordered.reduce((sum, element) => sum + (axis === 'horizontal' ? element.width : element.height), 0)
    const gap = (end - start - size) / (ordered.length - 1)
    const positions = new Map<string, number>()
    let cursor = start
    for (const element of ordered) {
      positions.set(element.id, cursor)
      cursor += (axis === 'horizontal' ? element.width : element.height) + gap
    }
    pushHistory()
    onChange(elements.map((element) => !positions.has(element.id) || element.locked ? element : { ...element, [axis === 'horizontal' ? 'x' : 'y']: positions.get(element.id)! }))
  }

  const layer = (front: boolean) => {
    if (selected.length === 0) return
    const ordered = [...elements].sort((a, b) => (a.zIndex || 0) - (b.zIndex || 0))
    const moving = ordered.filter((element) => selected.includes(element.id) && !element.locked)
    const stationary = ordered.filter((element) => !selected.includes(element.id) || element.locked)
    const layered = front ? [...stationary, ...moving] : [...moving, ...stationary]
    const zIndexes = new Map(layered.map((element, index) => [element.id, index]))
    pushHistory()
    onChange(elements.map((element) => ({ ...element, zIndex: zIndexes.get(element.id) ?? element.zIndex })))
  }

  const copySelected = () => { clipboard.current = clone(selectedElements) }
  const paste = () => {
    if (clipboard.current.length === 0) return
    const copies = clipboard.current.map((element) => ({ ...element, id: nextID(), x: clamp(element.x + 2, 0, 100 - element.width), y: clamp(element.y + 2, 0, 100 - element.height), groupId: undefined, locked: false }))
    commit([...elements, ...copies])
    clipboard.current = clone(copies)
    setSelected(copies.map((element) => element.id))
  }

  const onKeyDown = (event: ReactKeyboardEvent<HTMLDivElement>) => {
    const target = event.target as HTMLElement
    if (target.matches('input, textarea, select, [contenteditable="true"]')) return
    const control = event.ctrlKey || event.metaKey
    if (control && event.key.toLowerCase() === 'z') { event.preventDefault(); event.shiftKey ? redo() : undo(); return }
    if (control && event.key.toLowerCase() === 'y') { event.preventDefault(); redo(); return }
    if (control && event.key.toLowerCase() === 'a') { event.preventDefault(); setSelected(elements.filter((element) => !element.hidden).map((element) => element.id)); return }
    if (control && event.key.toLowerCase() === 'c') { event.preventDefault(); copySelected(); return }
    if (control && event.key.toLowerCase() === 'x') { event.preventDefault(); copySelected(); removeSelected(); return }
    if (control && event.key.toLowerCase() === 'd') { event.preventDefault(); duplicateSelected(); return }
    if (control && event.key.toLowerCase() === 'g') { event.preventDefault(); event.shiftKey ? ungroup() : group(); return }
    if (control && event.key === ']') { event.preventDefault(); layer(true); return }
    if (control && event.key === '[') { event.preventDefault(); layer(false); return }
    if (event.key === 'Escape') { setEditing(''); setSelected([]); setRegionEditing(''); setRegionSlot(''); return }
    // A selected region answers the same keys an object does, except Delete:
    // emptying a template region is done deliberately, from its panel.
    if (region && selected.length === 0) {
      if (event.key === 'Enter' || event.key === 'F2') { event.preventDefault(); startRegionEdit(); return }
      if (['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'].includes(event.key) && regionDrawn) {
        event.preventDefault()
        const distance = event.shiftKey ? 2 : .25
        const current = regionFrame(region)
        const moved = {
          ...current,
          x: current.x + (event.key === 'ArrowLeft' ? -distance : event.key === 'ArrowRight' ? distance : 0),
          y: current.y + (event.key === 'ArrowUp' ? -distance : event.key === 'ArrowDown' ? distance : 0),
        }
        onRegionFrames({ ...frames, [region.slot]: moved })
        return
      }
    }
    if (event.key === 'Delete' || event.key === 'Backspace') { event.preventDefault(); removeSelected(); return }
    if (['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'].includes(event.key) && selected.length > 0) {
      event.preventDefault()
      const distance = event.shiftKey ? 2 : .25
      const patchX = event.key === 'ArrowLeft' ? -distance : event.key === 'ArrowRight' ? distance : 0
      const patchY = event.key === 'ArrowUp' ? -distance : event.key === 'ArrowDown' ? distance : 0
      pushHistory()
      onChange(elements.map((element) => selected.includes(element.id) && !element.locked ? {
        ...element, x: clamp(element.x + patchX, 0, 100 - element.width), y: clamp(element.y + patchY, 0, 100 - element.height),
      } : element))
    }
  }

  const renderElement = (element: SlideElement) => {
    if (element.hidden) return null
    const isSelected = selected.includes(element.id)
    const style = {
      left: `${element.x}%`, top: `${element.y}%`, width: `${element.width}%`, height: `${element.height}%`,
      transform: `rotate(${element.rotation || 0}deg)`, zIndex: 10 + (element.zIndex || 0), opacity: (element.opacity || 100) / 100,
    } as CSSProperties
    const textStyle = {
      color: `#${cleanColor(element.textColor)}`, fontFamily: `${element.fontFamily || 'Aptos'}, 'Noto Sans KR', sans-serif`,
      fontSize: `${Math.max(.7, (element.fontSize || 18) * .12)}cqw`, fontWeight: element.bold ? 700 : 400,
      fontStyle: element.italic ? 'italic' : 'normal', textDecoration: element.underline ? 'underline' : 'none',
      textAlign: (element.align || 'left') as CSSProperties['textAlign'],
      justifyContent: element.verticalAlign === 'bottom' ? 'flex-end' : element.verticalAlign === 'middle' ? 'center' : 'flex-start',
    } as CSSProperties
    const lineID = element.id.replace(/[^a-zA-Z0-9_-]/g, '') || 'freeform-line'
    const lineColor = `#${cleanColor(element.stroke, '4C3AA0')}`
    const dashArray = element.dash === 'dash' ? '8 5' : element.dash === 'dot' ? '2 4' : element.dash === 'dashDot' ? '8 4 2 4' : undefined
    return <div
      key={element.id}
      className={`freeform-element kind-${element.kind} ${isSelected ? 'selected' : ''} ${element.locked ? 'locked' : ''}`}
      style={style}
      title={`${elementLabel(element)}${element.locked ? ' · 잠김' : ''}`}
      onPointerDown={(event) => {
        const ids = resolveSelection(element, event.shiftKey)
        setSelected(ids)
        setRegionSlot('')
        if (editing && editing !== element.id) setEditing('')
        // A right-click selects and then lets the context menu through. Starting a
        // drag would capture the pointer, and a captured pointer sends the menu
        // event to the page instead of to the object under the cursor.
        if (event.button !== 0) { event.stopPropagation(); return }
        if (!event.altKey && doublePress(`element:${element.id}`, event.timeStamp) && !element.locked
          && (element.kind === 'text' || element.kind === 'table' || element.text !== undefined)) {
          // Cancel the press: the browser's focus fix-up would otherwise run after
          // the box has opened and pull focus back out of it, blurring the
          // textarea and closing it again inside the same gesture.
          event.preventDefault()
          event.stopPropagation()
          pushHistory()
          setEditing(element.id)
          return
        }
        if (!event.shiftKey) beginDrag(event, 'move', undefined, ids)
        else { event.preventDefault(); event.stopPropagation() }
      }}
      onDoubleClick={(event) => {
        event.stopPropagation()
        if ((element.kind === 'text' || element.kind === 'table' || element.text !== undefined) && !element.locked) {
          pushHistory()
          setEditing(element.id)
        }
      }}
      onContextMenu={(event) => {
        event.preventDefault()
        event.stopPropagation()
        if (!selected.includes(element.id)) setSelected([element.id])
        setRegionSlot('')
        setMenu({ x: event.clientX, y: event.clientY, target: 'element' })
      }}
    >
      {element.kind === 'line'
        ? <svg viewBox="0 0 100 100" preserveAspectRatio="none">
            <defs><LineMarker id={`${lineID}-start`} kind={element.startArrow} color={lineColor} /><LineMarker id={`${lineID}-end`} kind={element.endArrow} color={lineColor} /></defs>
            <line x1="0" y1="0" x2="100" y2="100" stroke={lineColor} strokeWidth={Math.max(1, element.strokeWidth || 2)}
              strokeDasharray={dashArray} markerStart={element.startArrow && element.startArrow !== 'none' ? `url(#${lineID}-start)` : undefined}
              markerEnd={element.endArrow && element.endArrow !== 'none' ? `url(#${lineID}-end)` : undefined} vectorEffect="non-scaling-stroke" />
          </svg>
        : element.kind === 'table'
          ? <div className={`freeform-table ${editing === element.id ? 'editing' : ''}`} style={{ fontFamily: textStyle.fontFamily, fontSize: textStyle.fontSize }}>
              <table><tbody>{(element.cells || [['']]).map((row, rowIndex) => <tr key={rowIndex}>{row.map((cell, columnIndex) => {
                const header = rowIndex < (element.headerRows || 0) || columnIndex < (element.headerColumns || 0)
                return <td key={columnIndex} style={{
                  color: header ? '#fff' : `#${cleanColor(element.textColor)}`,
                  background: header ? `#${cleanColor(element.fill, '725BD6')}` : '#fff',
                  borderColor: `#${cleanColor(element.stroke, 'D9D6E1')}`,
                  fontWeight: header ? 700 : element.bold ? 700 : 400,
                }}>{editing === element.id
                  ? <textarea rows={1} value={cell} aria-label={`${rowIndex + 1}행 ${columnIndex + 1}열`}
                      onPointerDown={(event) => event.stopPropagation()}
                      onKeyDown={(event) => { if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); setEditing('') } }}
                      onChange={(event) => {
                      const cells = (element.cells || [['']]).map((currentRow) => [...currentRow])
                      cells[rowIndex][columnIndex] = event.target.value
                      onChange(elementsRef.current.map((candidate) => candidate.id === element.id ? { ...candidate, cells } : candidate))
                    }} />
                  : cell}</td>
              })}</tr>)}</tbody></table>
            </div>
        : element.kind === 'image'
          ? assetURLs[element.assetId || ''] ? <img src={assetURLs[element.assetId || '']} alt={element.caption || element.name || '배치된 이미지'} style={{ objectFit: (element.fit === 'fill' ? 'fill' : element.fit === 'contain' ? 'contain' : 'cover') as CSSProperties['objectFit'] }} /> : <span className="freeform-image-empty">이미지를 불러올 수 없음</span>
          : <div className="freeform-element-body" style={element.kind === 'shape' ? shapeStyle(element) : undefined}>
              {(element.kind === 'text' || element.text) && (editing === element.id
                ? <textarea autoFocus value={element.text || ''} style={textStyle}
                    onPointerDown={(event) => event.stopPropagation()}
                    onBlur={() => finishEditing(element)}
                    onKeyDown={(event) => {
                      /* The editor's own key handling stops at a textarea, so without
                         this the only way out of a text box was to click elsewhere. */
                      if (event.key === 'Escape' || (event.key === 'Enter' && (event.ctrlKey || event.metaKey))) {
                        event.preventDefault()
                        event.stopPropagation()
                        finishEditing(element)
                      }
                    }}
                    onChange={(event) => patchSelected({ text: event.target.value }, false)} />
                : <div className="freeform-text" style={textStyle}>{element.text}</div>)}
            </div>}
      {element.locked && <Lock className="freeform-lock-mark" size={10} />}
    </div>
  }

  const handles: Handle[] = ['nw', 'n', 'ne', 'e', 'se', 's', 'sw', 'w']

  return <div className="freeform-editor" ref={editorRoot} tabIndex={0} onKeyDown={onKeyDown}>
    <div className="freeform-toolbar" role="toolbar" aria-label="슬라이드 편집 도구">
      <div className="freeform-tool-group">
        <button type="button" className="active" title="선택 도구 (Esc)"><MousePointer2 size={15} /></button>
        <button type="button" onClick={() => add('text')} title="텍스트 상자 추가"><Type size={15} /><span>텍스트</span></button>
        <button type="button" onClick={() => add('shape', shape)} title="도형 추가"><Square size={15} /><span>도형</span></button>
        <select value={shape} onChange={(event) => setShape(event.target.value)} aria-label="추가할 도형">
          {Object.entries(shapeNames).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
        </select>
        <button type="button" onClick={() => add('line')} title="선 추가"><Minus size={15} /><span>선</span></button>
        <button type="button" onClick={() => add('table')} title="표 추가"><Table2 size={15} /><span>표</span></button>
      </div>
      <div className="freeform-tool-group">
        <button type="button" disabled={!canUndo} onClick={undo} title="실행 취소 (Ctrl+Z) — 영역·컴포넌트·개체 모두"><Undo2 size={15} /></button>
        <button type="button" disabled={!canRedo} onClick={redo} title="다시 실행 (Ctrl+Y)"><Redo2 size={15} /></button>
        <button type="button" disabled={!primary} onClick={duplicateSelected} title="복제 (Ctrl+D)"><Copy size={15} /></button>
        <button type="button" disabled={!primary} onClick={removeSelected} title="삭제 (Delete)"><Trash2 size={15} /></button>
      </div>
      <div className="freeform-tool-group compact">
        <button type="button" disabled={selectedElements.length < 2} onClick={() => align('left')} title="왼쪽 맞춤"><AlignStartVertical size={14} /></button>
        <button type="button" disabled={selectedElements.length < 2} onClick={() => align('center')} title="가로 가운데"><AlignHorizontalJustifyCenter size={14} /></button>
        <button type="button" disabled={selectedElements.length < 2} onClick={() => align('right')} title="오른쪽 맞춤"><AlignEndVertical size={14} /></button>
        <button type="button" disabled={selectedElements.length < 2} onClick={() => align('top')} title="위쪽 맞춤"><AlignStartHorizontal size={14} /></button>
        <button type="button" disabled={selectedElements.length < 2} onClick={() => align('middle')} title="세로 가운데"><AlignVerticalJustifyCenter size={14} /></button>
        <button type="button" disabled={selectedElements.length < 2} onClick={() => align('bottom')} title="아래쪽 맞춤"><AlignEndHorizontal size={14} /></button>
        <button type="button" disabled={selectedElements.length < 3} onClick={() => distribute('horizontal')} title="가로 균등 배치">H</button>
        <button type="button" disabled={selectedElements.length < 3} onClick={() => distribute('vertical')} title="세로 균등 배치">V</button>
      </div>
      <div className="freeform-tool-group compact">
        <button type="button" disabled={!primary} onClick={() => layer(true)} title="맨 앞으로 (Ctrl+])"><BringToFront size={14} /></button>
        <button type="button" disabled={!primary} onClick={() => layer(false)} title="맨 뒤로 (Ctrl+[)"><SendToBack size={14} /></button>
        <button type="button" disabled={selectedElements.length < 2} onClick={group} title="그룹 (Ctrl+G)"><Group size={14} /></button>
        <button type="button" disabled={!selectedElements.some((element) => element.groupId)} onClick={ungroup} title="그룹 해제 (Ctrl+Shift+G)"><Ungroup size={14} /></button>
        <button type="button" disabled={!primary} onClick={() => patchSelected({ locked: !selectedElements.every((element) => element.locked) })} title="선택 개체 잠금/해제">{selectedElements.every((element) => element.locked) ? <Unlock size={14} /> : <Lock size={14} />}</button>
      </div>
    </div>

    {/* The properties bar is always here, even with nothing selected. It used to
        appear on the first click, and the 36px it added pushed the slide down
        under the pointer — so the second click of a double-click landed on the
        object's own resize handle instead of opening it for typing. */}
    {!primary && !region && <div className="freeform-properties region-properties empty" aria-label="선택 없음">
      <span className="region-hint"><MousePointer2 size={12} /> 슬라이드의 글·컴포넌트나 얹은 개체를 클릭하면 여기에서 고칩니다. 한 번 더 누르면 그 자리에서 타이핑합니다.</span>
    </div>}
    {!primary && region && <div className="freeform-properties region-properties" aria-label="선택 영역">
      <span className="region-chip"><LayoutTemplate size={12} /> {regionLabel(region)}</span>
      {region.kind !== 'component' && region.kind !== 'picture' && region.acceptsText && (
        <button type="button" className={regionEditing === region.slot ? 'active' : ''}
          onClick={() => regionEditing === region.slot ? commitRegionEdit() : startRegionEdit()}>
          <Type size={13} /> {regionEditing === region.slot ? '편집 완료' : '텍스트 편집'}
        </button>
      )}
      {/* An empty region is an offer, not a hole: text, a component or a picture.
          A component is only offered where one fits — four lines of the region's
          own type — because putting one in a one-line strip only produces a
          defect to clean up afterwards. */}
      {region.kind === 'empty' && <>
        {region.frame.height >= (region.fontSize || 18) * 4.8 / slideHeightPoints * 100
          ? <select value="" aria-label="이 영역에 넣을 컴포넌트"
              onChange={(event) => { if (event.target.value) { setRegionDirty(true); onRegionBlock(region.slot, starterBlock(event.target.value)) } }}>
              <option value="">컴포넌트 넣기…</option>
              {switchableBlocks.map((kind) => <option key={kind} value={kind}>{blockNames[kind] || kind}</option>)}
            </select>
          : <span className="region-note">한 줄짜리 영역입니다 · 컴포넌트를 넣으려면 아래로 늘리세요</span>}
        <button type="button" onClick={() => onPickImage(region.slot)}><ImagePlus size={13} /> 이미지</button>
      </>}
      {region.kind === 'picture' && <button type="button" onClick={() => onPickImage(region.slot)}><ImagePlus size={13} /> 이미지 바꾸기</button>}
      {/* Type the slide sets for this region. Nothing is written unless it is
          changed, so a region left alone keeps the template's own styling. */}
      {/* Type is set for the words in a region. A component draws its own, from
          the template's design system, so it is not offered here. */}
      {region.acceptsText && (region.kind === 'text' || region.kind === 'empty') && <>
        <span className="region-divider" />
        <label>크기
          <button type="button" onClick={() => patchStyle({ scale: Math.max(0.4, Math.round(((style.scale || 1) - 0.1) * 10) / 10) })} title="작게">−</button>
          <span className="region-scale">{Math.round((style.scale || 1) * 100)}%</span>
          <button type="button" onClick={() => patchStyle({ scale: Math.min(3, Math.round(((style.scale || 1) + 0.1) * 10) / 10) })} title="크게">+</button>
        </label>
        <button type="button" className={style.bold ?? region.bold ? 'active' : ''}
          onClick={() => patchStyle({ bold: !(style.bold ?? region.bold) })} title="굵게"><Bold size={13} /></button>
        <button type="button" className={style.italic ?? region.italic ? 'active' : ''}
          onClick={() => patchStyle({ italic: !(style.italic ?? region.italic) })} title="기울임"><Italic size={13} /></button>
        <label>글자 <input type="color" value={`#${cleanColor(style.color || region.color, '20242D')}`}
          onChange={(event) => patchStyle({ color: cleanColor(event.target.value) })} aria-label="글자 색" /></label>
        <select value={style.align || ''} onChange={(event) => patchStyle({ align: (event.target.value || undefined) as SlotStyle['align'] })} aria-label="정렬">
          <option value="">템플릿 정렬</option>
          <option value="left">왼쪽</option>
          <option value="center">가운데</option>
          <option value="right">오른쪽</option>
          <option value="justify">양쪽</option>
        </select>
        {Object.keys(style).length > 0 && <button type="button" onClick={() => patchStyle(null)} title="이 영역의 서식을 템플릿으로 되돌립니다"><CornerUpLeft size={13} /> 서식 초기화</button>}
      </>}
      {region.moved || regionBox ? <button type="button" onClick={resetRegionFrame}><CornerUpLeft size={13} /> 원래 자리로</button> : null}
      {region.kind !== 'empty' && <button type="button" className="danger-hover" onClick={() => { setRegionDirty(true); onRegionClear(region.slot) }}>
        <Eraser size={13} /> 내용 비우기
      </button>}
      {aiEnabled && <>
        <span className="region-divider" />
        <input className="region-instruction" value={aiInstruction} placeholder="AI에게 시킬 내용 (예: 숫자를 앞에 세워 줘)"
          onChange={(event) => setAiInstruction(event.target.value)}
          onKeyDown={(event) => { if (event.key === 'Enter') { event.preventDefault(); void runRevision('rewrite') } }} />
        <button type="button" disabled={aiBusy} onClick={() => void runRevision('rewrite')}><WandSparkles size={13} /> {aiBusy ? '고쳐 쓰는 중…' : '다시 쓰기'}</button>
        <button type="button" disabled={aiBusy} onClick={() => void runRevision('shorten')}>짧게</button>
        <button type="button" disabled={aiBusy} onClick={() => void runRevision('expand')}>자세히</button>
        <button type="button" disabled={aiBusy} onClick={() => void runRevision('component')}>컴포넌트로</button>
        <button type="button" disabled={aiBusy} onClick={() => void runRevision('notes')}>발표 노트</button>
        {canUndoRevise && <button type="button" onClick={onUndoRevise}><Undo2 size={13} /> AI 이전으로</button>}
      </>}
    </div>}
    {primary && <div className="freeform-properties" aria-label="선택 개체 속성">
      {(primary.kind === 'text' || primary.kind === 'table' || primary.text !== undefined) && <>
        <select value={primary.fontFamily || 'Aptos'} onChange={(event) => patchSelected({ fontFamily: event.target.value })} aria-label="글꼴">
          {['Aptos', 'Arial', 'Noto Sans KR', 'Malgun Gothic', 'Georgia', 'Courier New'].map((font) => <option key={font}>{font}</option>)}
        </select>
        <label>크기 <input type="number" min="6" max="400" value={primary.fontSize || 18} onChange={(event) => patchSelected({ fontSize: Number(event.target.value) })} /></label>
        <button type="button" className={primary.bold ? 'active' : ''} onClick={() => patchSelected({ bold: !primary.bold })} title="굵게"><Bold size={13} /></button>
        <button type="button" className={primary.italic ? 'active' : ''} onClick={() => patchSelected({ italic: !primary.italic })} title="기울임"><Italic size={13} /></button>
        <button type="button" className={primary.underline ? 'active' : ''} onClick={() => patchSelected({ underline: !primary.underline })} title="밑줄"><Underline size={13} /></button>
        <label>글자 <input type="color" value={`#${cleanColor(primary.textColor)}`} onChange={(event) => patchSelected({ textColor: cleanColor(event.target.value) })} /></label>
        <select value={primary.align || 'left'} onChange={(event) => patchSelected({ align: event.target.value })} aria-label="텍스트 정렬"><option value="left">왼쪽</option><option value="center">가운데</option><option value="right">오른쪽</option><option value="justify">양쪽</option></select>
      </>}
      {(primary.kind === 'shape' || primary.kind === 'table') && <label>{primary.kind === 'table' ? '머리글' : '채우기'} <input type="color" value={`#${cleanColor(primary.fill, '725BD6')}`} onChange={(event) => patchSelected({ fill: cleanColor(event.target.value) })} /></label>}
      {(primary.kind === 'shape' || primary.kind === 'line' || primary.kind === 'table') && <>
        <label>선 <input type="color" value={`#${cleanColor(primary.stroke, '4C3AA0')}`} onChange={(event) => patchSelected({ stroke: cleanColor(event.target.value) })} /></label>
        <label>두께 <input type="number" min="0" max="50" step=".5" value={primary.strokeWidth || 1} onChange={(event) => patchSelected({ strokeWidth: Number(event.target.value) })} /></label>
      </>}
      {primary.kind === 'line' && <>
        <select value={primary.startArrow || 'none'} onChange={(event) => patchSelected({ startArrow: event.target.value })} aria-label="시작 화살표"><option value="none">시작 없음</option><option value="triangle">시작 화살표</option><option value="stealth">시작 스텔스</option><option value="diamond">시작 다이아</option><option value="oval">시작 원</option></select>
        <ArrowRight size={13} />
        <select value={primary.endArrow || 'none'} onChange={(event) => patchSelected({ endArrow: event.target.value })} aria-label="끝 화살표"><option value="none">끝 없음</option><option value="triangle">끝 화살표</option><option value="stealth">끝 스텔스</option><option value="diamond">끝 다이아</option><option value="oval">끝 원</option></select>
        <select value={primary.dash || 'solid'} onChange={(event) => patchSelected({ dash: event.target.value })} aria-label="선 스타일"><option value="solid">실선</option><option value="dash">파선</option><option value="dot">점선</option><option value="dashDot">일점쇄선</option></select>
      </>}
      {primary.kind === 'table' && <>
        <button type="button" className={editing === primary.id ? 'active' : ''} onClick={() => { if (editing !== primary.id) pushHistory(); setEditing(editing === primary.id ? '' : primary.id) }}><Table2 size={13} /> {editing === primary.id ? '셀 편집 완료' : '셀 편집'}</button>
        <button type="button" onClick={addTableRow}>+ 행</button><button type="button" onClick={removeTableRow} disabled={(primary.cells?.length || 0) <= 1}>− 행</button>
        <button type="button" onClick={addTableColumn}>+ 열</button><button type="button" onClick={removeTableColumn} disabled={(primary.cells?.[0]?.length || 0) <= 1}>− 열</button>
        <label>머리행 <input type="number" min="0" max={primary.cells?.length || 1} value={primary.headerRows || 0} onChange={(event) => patchSelected({ headerRows: Number(event.target.value) })} /></label>
        <label>머리열 <input type="number" min="0" max={primary.cells?.[0]?.length || 1} value={primary.headerColumns || 0} onChange={(event) => patchSelected({ headerColumns: Number(event.target.value) })} /></label>
      </>}
      {primary.kind === 'image' && <select value={primary.fit || 'cover'} onChange={(event) => patchSelected({ fit: event.target.value })} aria-label="이미지 맞춤"><option value="cover">프레임 채우기</option><option value="contain">전체 보기</option><option value="fill">늘여 맞추기</option></select>}
      <label className="opacity-property">불투명도 <input type="range" min="1" max="100" value={primary.opacity || 100} onChange={(event) => patchSelected({ opacity: Number(event.target.value) }, false)} onPointerDown={() => pushHistory()} /><span>{primary.opacity || 100}%</span></label>
      <label>회전 <input type="number" min="-3600" max="3600" value={Math.round(primary.rotation || 0)} onChange={(event) => patchSelected({ rotation: Number(event.target.value) })} />°</label>
    </div>}

    <div className="freeform-workarea">
      <div className="freeform-scroll">
        <div
          ref={page}
          className={`freeform-page ${showGrid ? 'show-grid' : ''}`}
          style={{ width: `${zoom}%` }}
          onPointerDown={(event) => {
            setSelected([]); setEditing(''); commitRegionEdit(); setRegionSlot(''); setMenu(null)
            if (event.button !== 0) return
            // Dragging across empty page draws a selection band.
            const start = point(event)
            marqueeStart.current = start
            setMarquee({ x: start.x, y: start.y, width: 0, height: 0 })
            page.current?.setPointerCapture(event.pointerId)
          }}
          onPointerMove={(event) => { onPointerMove(event); onRegionPointerMove(event); onMarqueeMove(event) }}
          onPointerUp={(event) => { endDrag(event); endRegionDrag(event); endMarquee() }}
          onPointerCancel={(event) => { endDrag(event); endRegionDrag(event); endMarquee() }}
          onContextMenu={(event) => { event.preventDefault(); setMenu({ x: event.clientX, y: event.clientY, target: 'page' }) }}
          onDragOver={onImageFiles ? (event) => {
            if (!Array.from(event.dataTransfer.types).includes('Files')) return
            event.preventDefault()
            event.dataTransfer.dropEffect = 'copy'
            setDropping(true)
          } : undefined}
          onDragLeave={onImageFiles ? (event) => { if (event.currentTarget === event.target) setDropping(false) } : undefined}
          onDrop={onImageFiles ? (event) => {
            const files = imageFiles(event.dataTransfer.files)
            setDropping(false)
            if (files.length === 0) return
            event.preventDefault()
            onImageFiles(files, point(event))
          } : undefined}
        >
          {dropping && <div className="freeform-drop-hint" aria-hidden="true"><ImagePlus size={22} /> 놓으면 이 자리에 이미지가 들어갑니다</div>}
          <SlidePreview
            className="freeform-base"
            cacheKey={`${slideId}-${baseVersion}-${lifted}`}
            alt="템플릿 기반 슬라이드"
            load={() => api.slidePreview(presentationId, position, 1400, false, lifted ? { exclude: [lifted] } : {})}
          />
          {/* The lifted region, drawn by the same renderer as the page it came
              from, so dragging it moves the drawing itself. */}
          {lifted && region && shown && spriteAt && spriteURL && regionEditing !== lifted
            && !(regionDirty && region.kind === 'text') && (
            <img
              className={`canvas-region-sprite ${regionDirty ? 'pending' : ''}`}
              src={spriteURL}
              alt=""
              style={{
                transform: `translate(${shown.x - spriteAt.x}%, ${shown.y - spriteAt.y}%) scale(${(shown.width / spriteAt.width).toFixed(4)}, ${(shown.height / spriteAt.height).toFixed(4)})`,
                transformOrigin: `${spriteAt.x}% ${spriteAt.y}%`,
              }}
            />
          )}
          {showRegions && regions.map((candidate) => {
            const frame = candidate.slot === regionSlot && regionBox ? regionBox : regionFrame(candidate)
            const empty = candidate.kind === 'empty'
            if (empty && !candidate.acceptsText) return null
            if (candidate.spannedBy) return null
            const active = candidate.slot === regionSlot
            return <div
              key={candidate.slot}
              className={`canvas-region kind-${candidate.kind} ${active ? 'active' : ''} ${empty ? 'empty' : ''}`}
              style={{ left: `${frame.x}%`, top: `${frame.y}%`, width: `${frame.width}%`, height: `${frame.height}%` }}
              title={`${regionLabel(candidate)} · 두 번 클릭해 편집`}
              onPointerDown={(event) => {
                event.stopPropagation()
                if (regionEditing && regionEditing !== candidate.slot) commitRegionEdit()
                if (event.button !== 0) { if (!active) selectRegion(candidate.slot); return }
                // A second click on a region opens it for typing. Reading that from
                // pointerdown rather than waiting for dblclick keeps it working when
                // the same press also begins a drag.
                const typing = candidate.acceptsText && candidate.kind !== 'component' && candidate.kind !== 'picture'
                if (doublePress(`region:${candidate.slot}`, event.timeStamp) && typing) {
                  event.preventDefault()
                  setRegionDraft(candidate.text || '')
                  setRegionEditing(candidate.slot)
                  return
                }
                if (!active) { selectRegion(candidate.slot); return }
                if (candidate.kind !== 'empty') beginRegionDrag(event, 'move')
              }}
              onContextMenu={(event) => {
                event.preventDefault()
                event.stopPropagation()
                if (!active) selectRegion(candidate.slot)
                setMenu({ x: event.clientX, y: event.clientY, target: 'region' })
              }}
              onDoubleClick={(event) => {
                event.stopPropagation()
                if (!active) selectRegion(candidate.slot)
                if (candidate.acceptsText && candidate.kind !== 'component' && candidate.kind !== 'picture') {
                  setRegionDraft(candidate.text || '')
                  setRegionEditing(candidate.slot)
                }
              }}
            >
              <span className="canvas-region-tag">{regionLabel(candidate)}{empty ? ' · 비어 있음' : ''}</span>
              {active && regionEditing === candidate.slot && <textarea
                autoFocus
                className="canvas-region-input"
                value={regionDraft}
                style={regionTextStyle(candidate)}
                onPointerDown={(event) => event.stopPropagation()}
                onChange={(event) => setRegionDraft(event.target.value)}
                onBlur={commitRegionEdit}
                onKeyDown={(event) => {
                  // Escape ends the edit and keeps what was typed. Every editor
                  // people come from behaves that way, and throwing away a
                  // sentence someone just wrote — silently — is not a shortcut.
                  // Ctrl+Z is how a change is taken back.
                  if (event.key === 'Escape') { event.preventDefault(); commitRegionEdit(); return }
                  if (event.key === 'Enter' && (event.ctrlKey || event.metaKey)) { event.preventDefault(); commitRegionEdit() }
                }}
              />}
              {active && regionDirty && regionEditing !== candidate.slot && candidate.kind !== 'component' && candidate.kind !== 'picture' && (
                <div className="canvas-region-draft" style={regionTextStyle(candidate)}>{regionDraft}</div>
              )}
            </div>
          })}
          {elements.map(renderElement)}
          {marquee && marquee.width > 0.4 && <div className="canvas-marquee" style={{
            left: `${marquee.x}%`, top: `${marquee.y}%`, width: `${marquee.width}%`, height: `${marquee.height}%`,
          }} />}
          {guides.map((guide) => <div key={`${guide.axis}-${guide.at}`} className={`canvas-guide ${guide.axis}`}
            style={guide.axis === 'x' ? { left: `${guide.at}%` } : { top: `${guide.at}%` }} />)}
          {region && shown && selectedElements.length === 0 && <div className="freeform-selection region" style={{ left: `${shown.x}%`, top: `${shown.y}%`, width: `${shown.width}%`, height: `${shown.height}%` }}>
            {regionDrawn && !regionEditing && handles.map((handle) => <button type="button" key={handle}
              className={`resize-handle handle-${handle}`} onPointerDown={(event) => beginRegionDrag(event, 'resize', handle)}
              aria-label={`${handle} 방향 크기 조절`} />)}
          </div>}
          {selectedElements.length > 0 && <div className="freeform-selection" style={{ left: `${selectedBounds.x}%`, top: `${selectedBounds.y}%`, width: `${selectedBounds.width}%`, height: `${selectedBounds.height}%` }}>
            {!selectedElements.every((element) => element.locked) && <>
              <button type="button" className="rotate-handle" onPointerDown={(event) => beginDrag(event, 'rotate')} title="회전"><RotateCw size={11} /></button>
              {handles.map((handle) => <button type="button" key={handle} className={`resize-handle handle-${handle}`} onPointerDown={(event) => beginDrag(event, 'resize', handle)} aria-label={`${handle} 방향 크기 조절`} />)}
            </>}
          </div>}
        </div>
      </div>
      {region?.kind === 'component' && region.block && <aside className="region-inspector" aria-label="컴포넌트 편집">
        <header><strong>{blockNames[String(region.block.kind)] || '컴포넌트'} 편집</strong><span>{region.slot}</span></header>
        <label>형식
          <select value={String(region.block.kind)} onChange={(event) => patchBlock({ kind: event.target.value })}>
            {switchableBlocks.map((kind) => <option key={kind} value={kind}>{blockNames[kind] || kind}</option>)}
            {!switchableBlocks.includes(String(region.block.kind)) && <option value={String(region.block.kind)}>{blockNames[String(region.block.kind)] || String(region.block.kind)}</option>}
          </select>
        </label>
        <label>제목 <input value={String(region.block.heading || '')} placeholder="없음" onChange={(event) => patchBlock({ heading: event.target.value })} /></label>
        <label>캡션 <input value={String(region.block.caption || '')} placeholder="없음" onChange={(event) => patchBlock({ caption: event.target.value })} /></label>
        {(region.block.kind === 'quote' || region.block.kind === 'callout') && (
          <label>문장 <textarea rows={3} value={String(region.block.text || '')} onChange={(event) => patchBlock({ text: event.target.value })} /></label>
        )}
        {Array.isArray(region.block.rows) && region.block.rows.length > 0 ? <>
          <p className="inspector-hint">표의 칸을 직접 고칩니다. 첫 줄은 머리글입니다.</p>
          <div className="inspector-rows">
            {(region.block.rows as string[][]).map((row, rowIndex) => <div key={rowIndex} className="inspector-row">
              {row.map((cell, columnIndex) => <input key={columnIndex} value={cell} aria-label={`${rowIndex + 1}행 ${columnIndex + 1}열`}
                onChange={(event) => {
                  const rows = (region.block!.rows as string[][]).map((current) => [...current])
                  rows[rowIndex][columnIndex] = event.target.value
                  patchBlock({ rows })
                }} />)}
            </div>)}
          </div>
        </> : <>
          <p className="inspector-hint">항목을 고치면 슬라이드가 그대로 다시 그려집니다.</p>
          {blockItems(region.block).map((item, index) => <div key={index} className="inspector-item">
            <div className="inspector-item-head"><span>{index + 1}</span>
              <button type="button" aria-label={`${index + 1}번째 항목 삭제`} onClick={() => patchBlockItems(blockItems(region.block).filter((_, position) => position !== index))}><Trash2 size={11} /></button>
            </div>
            <input value={item.label} placeholder="이름" onChange={(event) => patchBlockItems(blockItems(region.block).map((current, position) => position === index ? { ...current, label: event.target.value } : current))} />
            <input value={item.value} placeholder="값" onChange={(event) => patchBlockItems(blockItems(region.block).map((current, position) => position === index ? { ...current, value: event.target.value } : current))} />
            <input value={item.detail} placeholder="설명 (선택)" onChange={(event) => patchBlockItems(blockItems(region.block).map((current, position) => position === index ? { ...current, detail: event.target.value } : current))} />
          </div>)}
          <button type="button" className="inspector-add" disabled={blockItems(region.block).length >= 8}
            onClick={() => patchBlockItems([...blockItems(region.block), { label: '새 항목', value: '', detail: '' }])}>
            <Plus size={12} /> 항목 추가
          </button>
        </>}
      </aside>}
      {layersOpen && <aside className="freeform-layers" aria-label="개체 레이어">
        <header><strong><Layers3 size={13} /> 개체 레이어</strong><span>{elements.length}</span></header>
        {regions.some((candidate) => candidate.kind !== 'empty') && <>
          <p className="layer-group">템플릿 영역</p>
          <ul>{regions.filter((candidate) => candidate.kind !== 'empty' && !candidate.spannedBy).map((candidate) => <li key={candidate.slot} className={candidate.slot === regionSlot ? 'active' : ''}>
            <button type="button" className="layer-select" onClick={() => selectRegion(candidate.slot)} title={candidate.text || regionLabel(candidate)}>
              <LayoutTemplate size={12} />
              <span>{regionLabel(candidate)}{candidate.moved ? ' · 옮김' : ''}</span>
            </button>
          </li>)}</ul>
          <p className="layer-group">자유 배치 개체</p>
        </>}
        {layerElements.length === 0 ? <p>아직 개체가 없습니다.</p> : <ul>{layerElements.map((element) => <li key={element.id} className={selected.includes(element.id) ? 'active' : ''}>
          <button type="button" className="layer-select" onClick={() => { setSelected([element.id]); setEditing('') }} title={elementLabel(element)}>
            {element.kind === 'table' ? <Table2 size={12} /> : element.kind === 'text' ? <Type size={12} /> : element.kind === 'line' ? <Minus size={12} /> : element.kind === 'image' ? <Square size={12} /> : <Square size={12} />}
            <span>{elementLabel(element)}</span>
          </button>
          <button type="button" onClick={() => patchOne(element.id, { hidden: !element.hidden })} title={element.hidden ? '표시' : '숨기기'}>{element.hidden ? <EyeOff size={12} /> : <Eye size={12} />}</button>
          <button type="button" onClick={() => patchOne(element.id, { locked: !element.locked })} title={element.locked ? '잠금 해제' : '잠금'}>{element.locked ? <Lock size={12} /> : <Unlock size={12} />}</button>
        </li>)}</ul>}
      </aside>}
    </div>

    {menu && <>
      <div className="canvas-menu-shield" onPointerDown={() => setMenu(null)} onContextMenu={(event) => { event.preventDefault(); setMenu(null) }} />
      <div className="canvas-menu" style={{ left: menu.x, top: menu.y }} role="menu">
        {menu.target === 'element' && <>
          <button type="button" onClick={() => { copySelected(); setMenu(null) }}>복사 <kbd>Ctrl C</kbd></button>
          <button type="button" onClick={() => { duplicateSelected(); setMenu(null) }}>복제 <kbd>Ctrl D</kbd></button>
          <button type="button" onClick={() => { layer(true); setMenu(null) }}>맨 앞으로</button>
          <button type="button" onClick={() => { layer(false); setMenu(null) }}>맨 뒤로</button>
          <button type="button" onClick={() => { patchSelected({ locked: !selectedElements.every((element) => element.locked) }); setMenu(null) }}>
            {selectedElements.every((element) => element.locked) ? '잠금 해제' : '잠금'}
          </button>
          <span className="canvas-menu-line" />
          <button type="button" className="danger" onClick={() => { removeSelected(); setMenu(null) }}>삭제 <kbd>Del</kbd></button>
        </>}
        {menu.target === 'region' && region && <>
          {region.acceptsText && region.kind !== 'component' && region.kind !== 'picture' &&
            <button type="button" onClick={() => { startRegionEdit(); setMenu(null) }}>텍스트 편집</button>}
          {(region.kind === 'empty' || region.kind === 'picture') &&
            <button type="button" onClick={() => { onPickImage(region.slot); setMenu(null) }}>이미지 넣기</button>}
          {(region.moved || regionBox) && <button type="button" onClick={() => { resetRegionFrame(); setMenu(null) }}>원래 자리로</button>}
          {Object.keys(style).length > 0 && <button type="button" onClick={() => { patchStyle(null); setMenu(null) }}>서식 초기화</button>}
          {aiEnabled && <button type="button" onClick={() => { void runRevision('rewrite'); setMenu(null) }}>AI로 다시 쓰기</button>}
          {region.kind !== 'empty' && <>
            <span className="canvas-menu-line" />
            <button type="button" className="danger" onClick={() => { setRegionDirty(true); onRegionClear(region.slot); setMenu(null) }}>내용 비우기</button>
          </>}
        </>}
        {menu.target === 'page' && <>
          <button type="button" onClick={() => { add('text'); setMenu(null) }}>텍스트 상자 추가</button>
          <button type="button" onClick={() => { add('shape', shape); setMenu(null) }}>도형 추가</button>
          <button type="button" onClick={() => { add('table'); setMenu(null) }}>표 추가</button>
          <span className="canvas-menu-line" />
          <button type="button" disabled={clipboard.current.length === 0} onClick={() => { paste(); setMenu(null) }}>붙여넣기 <kbd>Ctrl V</kbd></button>
        </>}
      </div>
    </>}

    <div className="freeform-footer">
      <span>
        {regions.filter((candidate) => candidate.kind !== 'empty').length}개 템플릿 영역 · {elements.length}개 개체 ·{' '}
        {region ? `${regionLabel(region)} 선택` : selected.length > 0 ? `${selected.length}개 선택` : '슬라이드의 글이나 도형을 클릭하세요'}
      </span>
      <div>
        <button type="button" className={showRegions ? 'active' : ''} onClick={() => { setShowRegions((value) => !value); setRegionSlot('') }} title="AI가 만든 영역 선택 가능"><LayoutTemplate size={13} /></button>
        <button type="button" className={layersOpen ? 'active' : ''} onClick={() => setLayersOpen((value) => !value)} title="개체 레이어"><Layers3 size={13} /></button>
        <label><input type="checkbox" checked={snap} onChange={(event) => setSnap(event.target.checked)} /> 맞춤</label>
        <button type="button" className={showGrid ? 'active' : ''} onClick={() => setShowGrid((value) => !value)} title="격자 표시"><Grid3X3 size={13} /></button>
        <button type="button" onClick={() => setZoom((value) => Math.max(50, value - 10))} disabled={zoom <= 50} title="축소"><ZoomOut size={13} /></button>
        <span>{zoom}%</span>
        <button type="button" onClick={() => setZoom((value) => Math.min(200, value + 10))} disabled={zoom >= 200} title="확대"><ZoomIn size={13} /></button>
      </div>
    </div>
  </div>
}
