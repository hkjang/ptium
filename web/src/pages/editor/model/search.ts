/**
 * Find and replace, across every word a deck holds.
 *
 * A deck says a product's name on nine slides: in a title, in a bullet, inside
 * a KPI card, in a table cell, on a text box somebody dropped on the canvas,
 * and in the speaker notes nobody sees until they are presenting. Renaming it
 * meant opening all nine and hunting. Every editor people come here from has
 * this, and it is the one thing a deck of fifty slides makes unbearable to do
 * by hand.
 *
 * The work is knowing where text lives. A slide keeps it in six places and a
 * replacement that reaches five of them is worse than none: the deck reads as
 * renamed until the one that was missed is on the screen in front of a room.
 */

import type { Slide, SlideBlock, SlideElement, SlideParagraph } from '../../../types'

/** Where in a slide a hit was found, in the reader's language. */
export type MatchWhere = 'title' | 'subtitle' | 'body' | 'component' | 'object' | 'notes'

export interface Match {
  /** 1-based, the way slides are numbered everywhere else. */
  slide: number
  slideId: string
  where: MatchWhere
  /** What the region is called, for the list: "본문", "핵심 지표", "표"... */
  label: string
  /** The line the hit sits on, and where in it, so the list can show context. */
  text: string
  start: number
  end: number
}

export interface SearchOptions {
  /** Off by default: people type what they remember, not what they typed. */
  matchCase?: boolean
  /** On for "2026" not matching "12026", off for finding a word inside another. */
  wholeWord?: boolean
}

const whereLabels: Record<MatchWhere, string> = {
  title: '제목', subtitle: '리드 문장', body: '본문',
  component: '컴포넌트', object: '캔버스 개체', notes: '발표자 노트',
}

export function whereLabel(where: MatchWhere) {
  return whereLabels[where] || where
}

/** occurrences lists where a needle sits in one string. */
function occurrences(text: string, needle: string, options: SearchOptions): [number, number][] {
  if (!needle) return []
  const hay = options.matchCase ? text : text.toLowerCase()
  const find = options.matchCase ? needle : needle.toLowerCase()
  const found: [number, number][] = []
  let at = hay.indexOf(find)
  while (at !== -1) {
    const end = at + find.length
    if (!options.wholeWord || isWholeWord(text, at, end)) found.push([at, end])
    // Overlapping matches are one match: replacing "aa" in "aaa" twice would
    // eat the replacement's own text.
    at = hay.indexOf(find, end)
  }
  return found
}

/**
 * A word boundary that works for Korean as well as Latin.
 *
 * \b is defined over [A-Za-z0-9_], so in "매출액" every position is a boundary
 * and "매출" would count as a whole word — which makes the option a lie in the
 * language most of these decks are written in. A letter next to a letter is
 * inside a word, whatever alphabet both belong to.
 */
function isWholeWord(text: string, start: number, end: number) {
  const letter = /[\p{L}\p{N}_]/u
  const before = start > 0 ? text[start - 1] : ''
  const after = end < text.length ? text[end] : ''
  return !(before && letter.test(before)) && !(after && letter.test(after))
}

/** A place in a slide that holds text, with the way to write it back. */
interface Region {
  where: MatchWhere
  label: string
  read: () => string
  write: (value: string) => void
}

/**
 * regionsOf walks one slide and hands back everywhere text lives.
 *
 * The writers mutate the copy they are given, so a caller clones first. The
 * list is the whole contract of this file: a place missing here is a place
 * find-and-replace does not reach.
 */
function regionsOf(slide: Slide, blockLabel: (kind: string) => string): Region[] {
  const regions: Region[] = []
  regions.push({ where: 'title', label: whereLabels.title, read: () => slide.title || '', write: (value) => { slide.title = value } })
  if (slide.subtitle !== undefined) {
    regions.push({ where: 'subtitle', label: whereLabels.subtitle, read: () => slide.subtitle || '', write: (value) => { slide.subtitle = value } })
  }
  // The prose of a slide is held twice — as text and as the paragraphs bound to
  // template slots — and both are what the server reads back.
  if (slide.body !== undefined) {
    regions.push({ where: 'body', label: whereLabels.body, read: () => slide.body || '', write: (value) => { slide.body = value } })
  }
  ;(slide.bullets || []).forEach((line, index) => {
    regions.push({
      where: 'body', label: whereLabels.body,
      read: () => slide.bullets![index] || '',
      write: (value) => { slide.bullets![index] = value },
    })
  })
  for (const [slot, paragraphs] of Object.entries(slide.fields || {})) {
    paragraphs.forEach((paragraph: SlideParagraph, index) => {
      regions.push({
        where: slot === 'title' ? 'title' : slot === 'subtitle' ? 'subtitle' : 'body',
        label: slot === 'title' ? whereLabels.title : slot === 'subtitle' ? whereLabels.subtitle : whereLabels.body,
        read: () => slide.fields![slot][index].text || '',
        write: (value) => { slide.fields![slot][index] = { ...slide.fields![slot][index], text: value } },
      })
    })
  }
  for (const [slot, block] of Object.entries(slide.blocks || {})) {
    regions.push(...blockRegions(slide, slot, block, blockLabel))
  }
  ;(slide.elements || []).forEach((element: SlideElement, index) => {
    if (element.text !== undefined) {
      regions.push({
        where: 'object', label: whereLabels.object,
        read: () => slide.elements![index].text || '',
        write: (value) => { slide.elements![index] = { ...slide.elements![index], text: value } },
      })
    }
    ;(element.cells || []).forEach((row, rowIndex) => {
      row.forEach((_, cellIndex) => {
        regions.push({
          where: 'object', label: '표',
          read: () => slide.elements![index].cells![rowIndex][cellIndex] || '',
          write: (value) => {
            const cells = slide.elements![index].cells!.map((line) => [...line])
            cells[rowIndex][cellIndex] = value
            slide.elements![index] = { ...slide.elements![index], cells }
          },
        })
      })
    })
  })
  if (slide.speakerNotes !== undefined) {
    regions.push({
      where: 'notes', label: whereLabels.notes,
      read: () => slide.speakerNotes || '',
      write: (value) => { slide.speakerNotes = value },
    })
  }
  return regions
}

/** Every string a component draws: its heading, its caption, and its data. */
function blockRegions(slide: Slide, slot: string, block: SlideBlock, blockLabel: (kind: string) => string): Region[] {
  const label = blockLabel(String(block.kind || ''))
  const at = () => slide.blocks![slot]
  const patch = (change: Partial<SlideBlock>) => { slide.blocks![slot] = { ...at(), ...change } }
  const regions: Region[] = []
  for (const key of ['heading', 'caption', 'text', 'unit', 'attribute'] as const) {
    if (typeof (block as Record<string, unknown>)[key] === 'string') {
      regions.push({
        where: 'component', label,
        read: () => String((at() as Record<string, unknown>)[key] ?? ''),
        write: (value) => patch({ [key]: value } as Partial<SlideBlock>),
      })
    }
  }
  ;(block.items || []).forEach((item, index) => {
    for (const key of Object.keys(item)) {
      if (typeof item[key] !== 'string') continue
      regions.push({
        where: 'component', label,
        read: () => String((at().items?.[index] as Record<string, unknown>)?.[key] ?? ''),
        write: (value) => {
          const items = (at().items || []).map((one) => ({ ...one }))
          items[index] = { ...items[index], [key]: value }
          patch({ items })
        },
      })
    }
  })
  ;(block.rows || []).forEach((row, rowIndex) => {
    row.forEach((_, cellIndex) => {
      regions.push({
        where: 'component', label,
        read: () => at().rows?.[rowIndex]?.[cellIndex] ?? '',
        write: (value) => {
          const rows = (at().rows || []).map((line) => [...line])
          rows[rowIndex][cellIndex] = value
          patch({ rows })
        },
      })
    })
  })
  const columns = Array.isArray(block.columns) ? (block.columns as string[]) : []
  columns.forEach((_, index) => {
    regions.push({
      where: 'component', label,
      read: () => (at().columns as string[] | undefined)?.[index] ?? '',
      write: (value) => {
        const next = [...((at().columns as string[] | undefined) || [])]
        next[index] = value
        patch({ columns: next })
      },
    })
  })
  return regions
}

/**
 * findInDeck lists every hit, in the order a reader would meet them.
 *
 * A slide holds its prose three times over — as text, as the lines of that
 * text, and as the paragraphs bound to template slots — because that is what
 * the server reads back. Replacing has to reach all three; a person looking at
 * a list does not want to be told the same sentence three times, and a count
 * that says thirteen where they can see seven reads as a bug. So the same line
 * in the same region of the same slide is one place, and `everywhere` asks for
 * the raw list that replacing works from.
 */
export function findInDeck(slides: Slide[], query: string, options: SearchOptions,
                           blockLabel: (kind: string) => string,
                           everywhere = false): Match[] {
  const matches: Match[] = []
  if (!query) return matches
  const seen = new Set<string>()
  slides.forEach((slide, index) => {
    for (const region of regionsOf(slide, blockLabel)) {
      const text = region.read()
      for (const [start, end] of occurrences(text, query, options)) {
        // The line the hit sits on, so a hit in a bullet and the same hit
        // inside the joined body are recognisably the same place.
        const from = text.lastIndexOf('\n', start - 1) + 1
        const to = text.indexOf('\n', end)
        const line = text.slice(from, to === -1 ? undefined : to)
        const key = `${index}|${region.where}|${region.label}|${line}|${start - from}`
        if (!everywhere && seen.has(key)) continue
        seen.add(key)
        matches.push({
          slide: index + 1, slideId: slide.id, where: region.where, label: region.label,
          text: everywhere ? text : line,
          start: everywhere ? start : start - from,
          end: everywhere ? end : end - from,
        })
      }
    }
  })
  return matches
}

/**
 * replaceInDeck writes the replacement everywhere it was found.
 *
 * Returns fresh slides and how many words changed; the slides given are not
 * touched, because the editor's undo stack keeps the ones it was holding.
 */
export function replaceInDeck(slides: Slide[], query: string, replacement: string, options: SearchOptions,
                              blockLabel: (kind: string) => string,
                              only?: { slideId?: string }): { slides: Slide[]; replaced: number; places: number } {
  if (!query) return { slides, replaced: 0, places: 0 }
  // What to tell the person afterwards: the places they could see, not the
  // regions the model wrote to.
  const places = findInDeck(only?.slideId ? slides.filter((slide) => slide.id === only.slideId) : slides,
    query, options, blockLabel).length
  let replaced = 0
  const next = slides.map((slide) => {
    if (only?.slideId && slide.id !== only.slideId) return slide
    const copy: Slide = structuredClone(slide)
    for (const region of regionsOf(copy, blockLabel)) {
      const text = region.read()
      const hits = occurrences(text, query, options)
      if (hits.length === 0) continue
      let rebuilt = ''
      let cursor = 0
      for (const [start, end] of hits) {
        rebuilt += text.slice(cursor, start) + replacement
        cursor = end
        replaced++
      }
      region.write(rebuilt + text.slice(cursor))
    }
    return copy
  })
  return { slides: replaced > 0 ? next : slides, replaced, places }
}
