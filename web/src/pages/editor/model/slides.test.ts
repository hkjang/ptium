import { describe, expect, it } from 'vitest'
import { bodyFromFields, carryTrimmedEntries, bodyFromText, drawnSlots, proseSlot, regionLabel, slideBody, slideFields, slideHoldings, textRegions, toApiSlides } from './slides'
import type { Slide, TemplateLayout } from '../../../types'

const layout: TemplateLayout = {
  id: 'content', name: '제목 및 내용', role: 'content',
  placeholders: [
    { slot: 'title', kind: 'text', maxChars: 60, maxLines: 2 },
    { slot: 'body', kind: 'text', maxChars: 300, maxLines: 8 },
    { slot: 'body2', kind: 'text', maxChars: 300, maxLines: 8 },
  ],
}

const slide = (overrides: Partial<Slide> = {}): Slide => ({
  id: 's1', order: 1, layout: 'content', layoutId: 'content',
  title: '실적', body: '매출 1,240억\n이익률 9.8%',
  bullets: ['매출 1,240억', '이익률 9.8%'],
  fields: { title: [{ text: '실적' }], body: [{ text: '매출 1,240억' }, { text: '이익률 9.8%' }] },
  ...overrides,
})

describe('a slide as the editor holds it', () => {
  it('reads its prose from the slot the template put it in', () => {
    expect(proseSlot(slide(), layout)).toBe('body')
    expect(slideBody(slide())).toContain('매출 1,240억')
    expect(bodyFromFields(slideFields(slide(), layout), 'body').body).toBe('매출 1,240억\n이익률 9.8%')
  })

  it('says what a slide is holding, and in which region', () => {
    const withBlock = slide({ blocks: { body: { kind: 'kpi', items: [{ label: '매출', value: '1,240억' }] } } } as Partial<Slide>)
    const holdings = slideHoldings(withBlock)
    expect(holdings.length).toBeGreaterThan(0)
    expect(holdings.some((holding) => holding.slot === 'body')).toBe(true)
    // A region holding a component is drawn, not typed into.
    expect(drawnSlots(withBlock)).toContain('body')
    expect(drawnSlots(slide())).not.toContain('body')
  })

  it('writes typed text back into paragraphs, and empty text into none', () => {
    expect(bodyFromText('첫 줄\n둘째 줄').bullets).toEqual(['첫 줄', '둘째 줄'])
    expect(bodyFromText('   ').bullets).toEqual([])
  })

  it('sends a slide back the way the API takes it', () => {
    const sent = toApiSlides([slide()], [layout])
    expect(sent).toHaveLength(1)
    expect(sent[0].title).toBe('실적')
    expect(sent[0].content.fields?.body?.map((paragraph) => paragraph.text)).toEqual(['매출 1,240억', '이익률 9.8%'])
  })
})

describe('textRegions', () => {
  const comparison = {
    id: 'c', name: '비교', role: 'comparison',
    placeholders: [
      { slot: 'title', kind: 'text', region: 'full-top', maxChars: 56, maxLines: 2 },
      { slot: 'body', kind: 'text', region: 'left-top', maxChars: 30, maxLines: 1 },
      { slot: 'body2', kind: 'text', region: 'right-top', maxChars: 30, maxLines: 1 },
      { slot: 'body3', kind: 'text', region: 'left-middle', maxChars: 300, maxLines: 13 },
      { slot: 'body4', kind: 'text', region: 'right-middle', maxChars: 300, maxLines: 13 },
    ],
  }
  const slide = () => ({
    id: 's', order: 1, layout: 'comparison', layoutId: 'c', title: '자동화 도입 전후 비교',
    fields: {
      title: [{ text: '자동화 도입 전후 비교' }],
      body: [{ text: '현재' }], body2: [{ text: '자동화' }],
      body3: [{ text: '0.8% 오배송' }], body4: [{ text: '0.1% 목표' }, { text: '2배 향상' }],
    },
    elements: [],
  })

  it('lists every text region of a two-column slide, not just the first', () => {
    const regions = textRegions(slide() as never, comparison as never)
    expect(regions.map((region) => region.slot)).toEqual(['body', 'body2', 'body3', 'body4'])
    expect(regions[1].text).toBe('자동화')
    expect(regions[3].text).toBe('0.1% 목표\n2배 향상')
  })

  it('names a region by where it is on the slide', () => {
    const regions = textRegions(slide() as never, comparison as never)
    expect(regions[0].label).toBe('왼쪽 위 영역')
    expect(regions[3].label).toBe('오른쪽 가운데 영역')
    expect(regionLabel('body2')).toBe('본문 2')
  })

  it('leaves out a region a component occupies', () => {
    const withBlock = { ...slide(), blocks: { body3: { kind: 'kpi', items: [] } } }
    const regions = textRegions(withBlock as never, comparison as never)
    expect(regions.map((region) => region.slot)).toEqual(['body', 'body2', 'body4'])
  })
})

describe('a component that draws less than it holds', () => {
  const stepped = (count: number): Slide => ({
    id: 'a', order: 3, layout: 'content', title: '이행 순서', speakerNotes: '순서대로 말합니다',
    blocks: { body: { kind: 'steps', items: Array.from({ length: count }, (_, index) => ({ label: `${index + 1}`, value: `단계 ${index + 1}` })) } },
    elements: [{ id: 'e1', kind: 'text', x: 1, y: 1, width: 10, height: 5, text: '메모' }],
  })

  it('carries the entries nobody would see onto a second slide', () => {
    const split = carryTrimmedEntries(stepped(6), 'body', 5, 'new-1')
    expect(split).not.toBeNull()
    expect(split!.kept.blocks!.body.items).toHaveLength(5)
    expect(split!.rest.blocks!.body.items).toHaveLength(1)
    // Nothing is lost and nothing is duplicated.
    const all = [...split!.kept.blocks!.body.items!, ...split!.rest.blocks!.body.items!]
    expect(all.map((item) => (item as { value: string }).value)).toEqual(
      Array.from({ length: 6 }, (_, index) => `단계 ${index + 1}`))
    expect(split!.rest.title).toBe('이행 순서 (계속)')
    expect(split!.rest.id).toBe('new-1')
    // Objects placed against the first drawing do not follow onto the second.
    expect(split!.rest.elements).toEqual([])
  })

  it('does nothing when the slide draws everything it holds', () => {
    expect(carryTrimmedEntries(stepped(5), 'body', 5, 'new-1')).toBeNull()
    expect(carryTrimmedEntries(stepped(6), 'nosuchslot', 5, 'new-1')).toBeNull()
  })
})
