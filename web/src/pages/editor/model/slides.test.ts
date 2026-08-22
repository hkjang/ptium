import { describe, expect, it } from 'vitest'
import { bodyFromFields, bodyFromText, drawnSlots, proseSlot, slideBody, slideFields, slideHoldings, toApiSlides } from './slides'
import type { Slide, TemplateLayout } from '../../../types'

const layout: TemplateLayout = {
  id: 'content', name: '제목 및 내용', role: 'content',
  placeholders: [
    { slot: 'title', name: 'Title', type: 'title', kind: 'text' },
    { slot: 'body', name: 'Body', type: 'body', kind: 'text' },
    { slot: 'body2', name: 'Body 2', type: 'body', kind: 'text' },
  ],
} as TemplateLayout

const slide = (overrides: Partial<Slide> = {}): Slide => ({
  id: 's1', order: 1, layout: 'content', layoutId: 'content',
  title: '실적', body: '매출 1,240억\n이익률 9.8%',
  bullets: ['매출 1,240억', '이익률 9.8%'],
  fields: { title: [{ text: '실적' }], body: [{ text: '매출 1,240억' }, { text: '이익률 9.8%' }] },
  ...overrides,
} as Slide)

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
