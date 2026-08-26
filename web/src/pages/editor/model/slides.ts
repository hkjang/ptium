/**
 * The shape of a slide as the editor holds it.
 *
 * A slide arrives from the API as fields bound to template slots and leaves as
 * the same thing; in between the editor needs to know which slot holds the
 * prose, what a slide is holding, and how to write text back into the slot it
 * came from. None of that needs React, so none of it lives in the page.
 */

import { bodySlots, primaryBodySlot, textToParagraphs } from '../../../api/client'
import type { Slide, SlideParagraph, TemplateLayout } from '../../../types'

export const MAX_SLIDES = 50
export const defaultSlide = (order: number, layoutId?: string): Slide => ({
  id: `new-${crypto.randomUUID()}`, order, layout: 'content', layoutId,
  title: '새로운 슬라이드', body: '핵심 메시지를 입력하세요.', bullets: ['핵심 메시지를 입력하세요.'],
  fields: { title: [{ text: '새로운 슬라이드' }], body: [{ text: '핵심 메시지를 입력하세요.' }] },
  elements: [],
})

export const slideBody = (slide?: Slide) => slide?.body || slide?.bullets?.join('\n') || ''
/** What a slide holds besides prose, named in the workspace's language. */
interface SlideHolding { slot: string; kind: 'block' | 'image' | 'element'; label: string; detail: string }

export function slideHoldings(slide?: Slide): SlideHolding[] {
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
export function blockLabel(kind: string) {
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
export const slideBodyLines = (slide?: Slide) => slideBody(slide).split(/\r?\n/).map((line) => line.trim()).filter(Boolean)

/** The slots a component or an image occupies. A slot holds one thing. */
export function drawnSlots(slide: Slide) {
  return new Set([...Object.keys(slide.blocks || {}), ...Object.keys(slide.images || {})])
}

/**
 * proseSlot is the slot the body textarea writes to: the first body slot no
 * drawing occupies. Writing prose into a component's slot would put two things in
 * one place, and the server keeps whichever it decides — silently losing one.
 */
export function proseSlot(slide: Slide, layout?: TemplateLayout) {
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
export function slideFields(slide: Slide, layout?: TemplateLayout) {
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
export function bodyFromFields(fields: Record<string, SlideParagraph[]>, slot: string) {
  const bullets = (fields[slot] || []).map((paragraph) => `${'  '.repeat(paragraph.level || 0)}${paragraph.text}`)
  return { body: bullets.join('\n'), bullets }
}

/** The same, from text the canvas just typed into the prose slot. */
export function bodyFromText(text: string) {
  return { body: text, bullets: text.split(/\r?\n/).map((line) => line.trim()).filter(Boolean) }
}

/**
 * The text regions of a slide, in the order the layout lists them.
 *
 * A comparison slide has four: a heading above each column and a list under
 * each. The panel used to show one — the first free one — so half of such a
 * slide could not be read, let alone edited, without going to the canvas or the
 * source. Regions a component or an image occupies are not text regions.
 */
export function textRegions(slide?: Slide, layout?: TemplateLayout) {
  if (!slide) return []
  const drawn = drawnSlots(slide)
  const skip = new Set(['title', 'subtitle'])
  const order: string[] = []
  for (const placeholder of layout?.placeholders || []) {
    if (placeholder.kind !== 'text' || skip.has(placeholder.slot) || drawn.has(placeholder.slot)) continue
    if (!order.includes(placeholder.slot)) order.push(placeholder.slot)
  }
  for (const slot of bodySlots(slide.fields)) {
    if (!skip.has(slot) && !drawn.has(slot) && !order.includes(slot)) order.push(slot)
  }
  return order.map((slot) => ({
    slot,
    label: regionLabel(slot, (layout?.placeholders || []).find((placeholder) => placeholder.slot === slot)?.region),
    text: bodyFromFields(slide.fields || {}, slot).body,
  }))
}

/** regionLabel names a region the way someone looking at the slide would. */
export function regionLabel(slot: string, region?: string) {
  const words: Record<string, string> = {
    left: '왼쪽', right: '오른쪽', top: '위', middle: '가운데', bottom: '아래', full: '전체', centre: '가운데', center: '가운데',
  }
  const named = (region || '').split('-').map((part) => words[part]).filter(Boolean)
  if (named.length > 0) return `${named.join(' ')} 영역`
  const fallback: Record<string, string> = { body: '본문', body1: '본문', body2: '본문 2', body3: '본문 3', body4: '본문 4' }
  return fallback[slot] || slot
}

export function toApiSlides(slides: Slide[], layouts: TemplateLayout[]) {
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
        // Kept out of the talk, kept in the deck. The server reads it from the
        // same place, which is what carries it through a duplicate, a restore
        // and an export.
        skipped: slide.skipped || undefined,
        // A slide that gives its points out one at a time while presenting.
        built: slide.built || undefined,
        accent: slide.accent,
      },
    }
  })
}

// findingLabel names a measured defect in the workspace's language.
/** The axes of the score, in the words the measurement uses. */

/**
 * A component that draws fewer entries than it holds, split across two slides.
 *
 * A steps component draws five entries because six across a slide would be
 * unreadable, and the sixth was on no slide at all. Which five were drawn is
 * arithmetic — the first five — so carrying the rest onto a second slide keeps
 * every word without anybody rewriting anything.
 *
 * Returns the slide as it should now be and the slide that carries the rest, or
 * null when the slide does not hold what was measured.
 */
export function carryTrimmedEntries(slide: Slide, slot: string, drawn: number, newId: string) {
  const block = slide.blocks?.[slot]
  const items = Array.isArray(block?.items) ? block.items : []
  if (!block || drawn < 1 || items.length <= drawn) return null
  const kept: Slide = { ...slide, blocks: { ...slide.blocks, [slot]: { ...block, items: items.slice(0, drawn) } } }
  const rest: Slide = {
    ...slide,
    id: newId,
    order: slide.order + 1,
    title: `${slide.title} (계속)`.slice(0, 200),
    blocks: { ...slide.blocks, [slot]: { ...block, items: items.slice(drawn) } },
    // Freeform objects were placed against the first slide's drawing; repeating
    // them over the continuation would put a callout on the wrong entries.
    elements: [],
    speakerNotes: slide.speakerNotes ? `${slide.speakerNotes} (계속)` : undefined,
  }
  return { kept, rest }
}

/**
 * Where the editor should stand after the deck was replaced under it.
 *
 * A rewrite hands back a deck of new slides — same shape, every id different —
 * and matching by id lands the author back on slide one. They were working on
 * slide seven; the rewrite was for slide seven; being thrown to the top of a
 * forty-slide deck is the product losing their place, not a redraw.
 *
 * The id wins when it survived. Otherwise the position does: the slide that
 * took the place of the one being looked at is the one to look at.
 */
export function keepPlace(currentId: string, before: Slide[], after: Slide[]): string {
  if (after.length === 0) return ''
  if (currentId && after.some((slide) => slide.id === currentId)) return currentId
  const was = before.findIndex((slide) => slide.id === currentId)
  if (was < 0) return after[0].id
  return after[Math.min(was, after.length - 1)].id
}

/**
 * A slide moved to a new place in the deck.
 *
 * Dragging a thumbnail says "put this one here", where "here" is a gap between
 * two slides — so `to` is a gap index from 0 to slides.length, not the index of
 * a slide. Taking the slide out first shifts every gap after it down by one,
 * which is the off-by-one that makes a dragged slide land one place too far.
 */
export function moveSlideTo(slides: Slide[], from: number, to: number): Slide[] {
  if (from < 0 || from >= slides.length) return slides
  const gap = Math.max(0, Math.min(to, slides.length))
  const landing = gap > from ? gap - 1 : gap
  if (landing === from) return slides
  const next = [...slides]
  const [moved] = next.splice(from, 1)
  next.splice(landing, 0, moved)
  return next.map((slide, index) => ({ ...slide, order: index + 1 }))
}

/**
 * Presenting walks past the slides marked skipped.
 *
 * They stay in the deck, in the rail and in the exported file — where
 * PowerPoint reads the same flag — so this is the only place that leaves them
 * out. A deck where every slide is skipped still presents: an empty show is
 * never what the person pressing F5 meant.
 */
export function slidesToPresent(slides: Slide[]): Slide[] {
  const shown = slides.filter((slide) => !slide.skipped)
  return shown.length > 0 ? shown : slides
}

/**
 * Where a slide sits in the show, given where it sits in the deck.
 *
 * Presenting from a slide that is skipped starts at the next one that is not,
 * because there is nowhere else for it to start.
 */
export function presentIndexOf(slides: Slide[], deckIndex: number): number {
  const shown = slidesToPresent(slides)
  for (let index = deckIndex; index < slides.length; index++) {
    const at = shown.indexOf(slides[index])
    if (at !== -1) return at
  }
  return Math.max(0, shown.length - 1)
}
