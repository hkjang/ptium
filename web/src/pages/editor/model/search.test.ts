import { describe, expect, it } from 'vitest'
import { findInDeck, replaceInDeck } from './search'
import { blockLabel } from './slides'
import type { Slide } from '../../../types'

// A deck says the product's name in every place a slide can hold text. A
// replacement that reaches five of the six is worse than none: the deck reads
// as renamed right up until the slide it missed is on the screen.
const deck = (): Slide[] => [
  {
    id: 'a', order: 1, layout: 'cover', title: '아틀라스 도입 계획',
    subtitle: '아틀라스로 무엇이 달라지는가',
    fields: { title: [{ text: '아틀라스 도입 계획' }], subtitle: [{ text: '아틀라스로 무엇이 달라지는가' }] },
    speakerNotes: '아틀라스는 올해 안에 들어옵니다.',
    elements: [],
  },
  {
    id: 'b', order: 2, layout: 'content', title: '효과',
    body: '아틀라스 도입 후 처리량\n아틀라스 운영 비용',
    bullets: ['아틀라스 도입 후 처리량', '아틀라스 운영 비용'],
    fields: { body: [{ text: '아틀라스 도입 후 처리량' }, { text: '아틀라스 운영 비용' }] },
    blocks: {
      body2: {
        kind: 'kpi', heading: '아틀라스 지표', caption: '분기 기준',
        items: [{ label: '아틀라스 처리량', value: '1,200건' }],
        rows: [['항목', '아틀라스'], ['비용', '8천만']],
      },
    },
    elements: [
      { id: 'e1', kind: 'text', x: 1, y: 1, width: 10, height: 5, text: '아틀라스 로고 자리' },
      { id: 'e2', kind: 'table', x: 1, y: 20, width: 30, height: 10, cells: [['도구', '아틀라스'], ['담당', '운영팀']] },
    ],
  },
]

describe('finding a word across a deck', () => {
  it('finds it everywhere a slide keeps text', () => {
    const matches = findInDeck(deck(), '아틀라스', {}, blockLabel)
    const where = new Set(matches.map((match) => match.where))
    for (const place of ['title', 'subtitle', 'body', 'component', 'object', 'notes']) {
      expect(where.has(place as never), place).toBe(true)
    }
    expect(matches.every((match) => match.text.slice(match.start, match.end) === '아틀라스')).toBe(true)
    expect(matches[0].slide).toBe(1)
  })

  it('ignores case unless asked, and can hold a word whole', () => {
    const slides: Slide[] = [{ id: 'a', order: 1, layout: 'content', title: 'Atlas and ATLASES', elements: [] }]
    expect(findInDeck(slides, 'atlas', {}, blockLabel)).toHaveLength(2)
    expect(findInDeck(slides, 'atlas', { matchCase: true }, blockLabel)).toHaveLength(0)
    expect(findInDeck(slides, 'Atlas', { matchCase: true, wholeWord: true }, blockLabel)).toHaveLength(1)
  })

  // \b is defined over Latin letters, so a naive whole-word test calls every
  // position in 매출액 a boundary and matches 매출 inside it.
  it('knows where a Korean word ends', () => {
    const slides: Slide[] = [{ id: 'a', order: 1, layout: 'content', title: '매출액과 매출 비교', elements: [] }]
    expect(findInDeck(slides, '매출', { wholeWord: true }, blockLabel)).toHaveLength(1)
    expect(findInDeck(slides, '매출', {}, blockLabel)).toHaveLength(2)
  })
})

describe('replacing it', () => {
  it('changes every one and leaves the deck it was given alone', () => {
    const before = deck()
    const { slides, replaced } = replaceInDeck(before, '아틀라스', '오리온', {}, blockLabel)
    // Every copy, including the ones the list folds together for the reader.
    expect(replaced).toBe(findInDeck(before, '아틀라스', {}, blockLabel, true).length)
    expect(findInDeck(slides, '아틀라스', {}, blockLabel, true)).toHaveLength(0)
    expect(findInDeck(slides, '오리온', {}, blockLabel, true)).toHaveLength(replaced)
    // The slides handed in are what undo is holding.
    expect(findInDeck(before, '아틀라스', {}, blockLabel).length).toBeGreaterThan(0)
    // And the words around it are untouched.
    expect(slides[1].blocks!.body2.items![0]).toMatchObject({ label: '오리온 처리량', value: '1,200건' })
    expect(slides[1].elements![1].cells).toEqual([['도구', '오리온'], ['담당', '운영팀']])
    expect(slides[0].speakerNotes).toBe('오리온는 올해 안에 들어옵니다.')
  })

  it('can be held to one slide', () => {
    const { slides, replaced } = replaceInDeck(deck(), '아틀라스', '오리온', {}, blockLabel, { slideId: 'b' })
    expect(replaced).toBeGreaterThan(0)
    expect(findInDeck([slides[0]], '아틀라스', {}, blockLabel).length).toBeGreaterThan(0)
    expect(findInDeck([slides[1]], '아틀라스', {}, blockLabel)).toHaveLength(0)
  })

  it('does not eat its own replacement', () => {
    const slides: Slide[] = [{ id: 'a', order: 1, layout: 'content', title: 'aaa', elements: [] }]
    const { slides: next, replaced } = replaceInDeck(slides, 'aa', 'aaa', {}, blockLabel)
    expect(replaced).toBe(1)
    expect(next[0].title).toBe('aaaa')
  })

  it('says nothing changed when nothing matched', () => {
    const before = deck()
    const { slides, replaced } = replaceInDeck(before, '없는말', '있는말', {}, blockLabel)
    expect(replaced).toBe(0)
    expect(slides).toBe(before)
  })
})

// The editor holds a slide's prose three times — as text, as its lines, and as
// the paragraphs bound to template slots — so a list built from the raw regions
// says the same sentence three times and counts thirteen where a reader counts
// seven. Replacing still has to reach all three.
describe('what the reader is shown', () => {
  it('lists a line once however many places hold it', () => {
    const shown = findInDeck(deck(), '아틀라스', {}, blockLabel)
    const everywhere = findInDeck(deck(), '아틀라스', {}, blockLabel, true)
    expect(shown.length).toBeLessThan(everywhere.length)
    const keys = shown.map((match) => `${match.slide}|${match.label}|${match.text}|${match.start}`)
    expect(new Set(keys).size).toBe(keys.length)
    expect(shown.every((match) => !match.text.includes('\n'))).toBe(true)
    expect(shown.every((match) => match.text.slice(match.start, match.end) === '아틀라스')).toBe(true)
  })

  it('reports the places a reader can count, and still replaces every copy', () => {
    const before = deck()
    const shown = findInDeck(before, '아틀라스', {}, blockLabel).length
    const result = replaceInDeck(before, '아틀라스', '오리온', {}, blockLabel)
    expect(result.places).toBe(shown)
    expect(result.replaced).toBeGreaterThan(result.places)
    expect(findInDeck(result.slides, '아틀라스', {}, blockLabel, true)).toHaveLength(0)
  })
})
