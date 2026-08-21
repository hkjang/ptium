import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { CSSProperties, KeyboardEvent as ReactKeyboardEvent, PointerEvent as ReactPointerEvent } from 'react'
import {
  AlignEndHorizontal, AlignEndVertical, AlignHorizontalJustifyCenter, AlignStartHorizontal, AlignStartVertical,
  AlignVerticalJustifyCenter, ArrowRight, Bold, BringToFront, Copy, Eye, EyeOff, Grid3X3, Group, Italic,
  Layers3, Lock, Minus, MousePointer2, Redo2, RotateCw, SendToBack, Square, Table2, Trash2, Type, Underline,
  Ungroup, Unlock, Undo2, ZoomIn, ZoomOut,
} from 'lucide-react'
import { api } from '../api/client'
import type { SlideElement } from '../types'
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
  slideId, elements, loadBase, cacheKey, onChange,
}: {
  slideId: string
  elements: SlideElement[]
  loadBase: () => Promise<string>
  cacheKey: string | number
  onChange: (elements: SlideElement[]) => void
}) {
  const [selected, setSelected] = useState<string[]>([])
  const [editing, setEditing] = useState('')
  const [zoom, setZoom] = useState(100)
  const [showGrid, setShowGrid] = useState(true)
  const [snap, setSnap] = useState(true)
  const [shape, setShape] = useState('rect')
  const [layersOpen, setLayersOpen] = useState(false)
  const [assetURLs, setAssetURLs] = useState<Record<string, string>>({})
  const page = useRef<HTMLDivElement>(null)
  const history = useRef<SlideElement[][]>([])
  const future = useRef<SlideElement[][]>([])
  const clipboard = useRef<SlideElement[]>([])
  const drag = useRef<DragState | null>(null)
  const elementsRef = useRef(elements)
  elementsRef.current = elements

  useEffect(() => {
    setSelected([])
    setEditing('')
    history.current = []
    future.current = []
  }, [slideId])

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

  const pushHistory = useCallback((snapshot = elementsRef.current) => {
    history.current = [...history.current.slice(-99), clone(snapshot)]
    future.current = []
  }, [])

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

  const point = (event: { clientX: number; clientY: number }) => {
    const rect = page.current?.getBoundingClientRect()
    if (!rect) return { x: 0, y: 0 }
    return { x: (event.clientX - rect.left) / rect.width * 100, y: (event.clientY - rect.top) / rect.height * 100 }
  }
  const snapped = (value: number) => snap ? Math.round(value * 2) / 2 : value

  const beginDrag = (event: ReactPointerEvent, mode: DragMode, handle?: Handle, forcedSelection?: string[]) => {
    event.preventDefault()
    event.stopPropagation()
    const ids = forcedSelection || selected
    const originals = elements.filter((element) => ids.includes(element.id) && !element.locked)
    if (originals.length === 0) return
    const start = point(event)
    pushHistory()
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
      const boundedDX = clamp(dx, -group.x, 100 - group.x - group.width)
      const boundedDY = clamp(dy, -group.y, 100 - group.y - group.height)
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
      const delta = (currentAngle - startAngle) * 180 / Math.PI
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

  const endDrag = (event: ReactPointerEvent) => {
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
      ...(kind === 'text' ? { text: '텍스트를 입력하세요', fontSize: 24, fontFamily: 'Aptos', textColor: '20242D', align: 'left', verticalAlign: 'middle' } : {}),
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

  const undo = () => {
    const previous = history.current.pop()
    if (!previous) return
    future.current.push(clone(elements))
    onChange(previous)
    setSelected([])
  }
  const redo = () => {
    const next = future.current.pop()
    if (!next) return
    history.current.push(clone(elements))
    onChange(next)
    setSelected([])
  }

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
    if (control && event.key.toLowerCase() === 'v') { event.preventDefault(); paste(); return }
    if (control && event.key.toLowerCase() === 'd') { event.preventDefault(); duplicateSelected(); return }
    if (control && event.key.toLowerCase() === 'g') { event.preventDefault(); event.shiftKey ? ungroup() : group(); return }
    if (control && event.key === ']') { event.preventDefault(); layer(true); return }
    if (control && event.key === '[') { event.preventDefault(); layer(false); return }
    if (event.key === 'Delete' || event.key === 'Backspace') { event.preventDefault(); removeSelected(); return }
    if (event.key === 'Escape') { setEditing(''); setSelected([]); return }
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
        if (editing && editing !== element.id) setEditing('')
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
                  ? <textarea rows={1} value={cell} aria-label={`${rowIndex + 1}행 ${columnIndex + 1}열`} onPointerDown={(event) => event.stopPropagation()} onChange={(event) => {
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
                ? <textarea autoFocus value={element.text || ''} style={textStyle} onPointerDown={(event) => event.stopPropagation()} onBlur={() => setEditing('')} onChange={(event) => patchSelected({ text: event.target.value }, false)} />
                : <div className="freeform-text" style={textStyle}>{element.text}</div>)}
            </div>}
      {element.locked && <Lock className="freeform-lock-mark" size={10} />}
    </div>
  }

  const handles: Handle[] = ['nw', 'n', 'ne', 'e', 'se', 's', 'sw', 'w']

  return <div className="freeform-editor" tabIndex={0} onKeyDown={onKeyDown}>
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
        <button type="button" disabled={history.current.length === 0} onClick={undo} title="실행 취소 (Ctrl+Z)"><Undo2 size={15} /></button>
        <button type="button" disabled={future.current.length === 0} onClick={redo} title="다시 실행 (Ctrl+Y)"><Redo2 size={15} /></button>
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
          onPointerDown={() => { setSelected([]); setEditing('') }}
          onPointerMove={onPointerMove}
          onPointerUp={endDrag}
          onPointerCancel={endDrag}
        >
          <SlidePreview className="freeform-base" cacheKey={cacheKey} alt="템플릿 기반 슬라이드" load={loadBase} />
          {elements.map(renderElement)}
          {selectedElements.length > 0 && <div className="freeform-selection" style={{ left: `${selectedBounds.x}%`, top: `${selectedBounds.y}%`, width: `${selectedBounds.width}%`, height: `${selectedBounds.height}%` }}>
            {!selectedElements.every((element) => element.locked) && <>
              <button type="button" className="rotate-handle" onPointerDown={(event) => beginDrag(event, 'rotate')} title="회전"><RotateCw size={11} /></button>
              {handles.map((handle) => <button type="button" key={handle} className={`resize-handle handle-${handle}`} onPointerDown={(event) => beginDrag(event, 'resize', handle)} aria-label={`${handle} 방향 크기 조절`} />)}
            </>}
          </div>}
        </div>
      </div>
      {layersOpen && <aside className="freeform-layers" aria-label="개체 레이어">
        <header><strong><Layers3 size={13} /> 개체 레이어</strong><span>{elements.length}</span></header>
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

    <div className="freeform-footer">
      <span>{elements.length}개 개체 · {selected.length > 0 ? `${selected.length}개 선택` : '개체를 클릭해 선택'}</span>
      <div>
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
