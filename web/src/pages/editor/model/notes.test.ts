import { describe, expect, it } from 'vitest'

import { noteDraft } from './notes'
import type { Slide } from '../../../types'

const slide = (over: Partial<Slide>): Slide => ({
  id: 'id', order: 1, layout: 'content', title: '제목', ...over,
})

describe('발표 노트 초안', () => {
  it('슬라이드에 있는 말을 되풀이하지 않는다', () => {
    // What the draft used to be: `${title}: ${first point}`, which is the
    // slide read back to the person standing next to it.
    const deck = [slide({ title: '재고 회전율 개선 근거', body: '투입과 회수를 같은 기준으로 놓고 봅니다.\n회수 시점' })]
    const draft = noteDraft(deck, 0)
    const onTheSlide = (deck[0].title + (deck[0].body || '')).replace(/\s/g, '')
    expect(onTheSlide).not.toContain(draft.replace(/\s/g, ''))
    expect(draft).not.toContain('재고 회전율 개선 근거')
  })

  it('무엇을 들고 있는 장인지에 따라 다른 것을 말한다', () => {
    const deck = [
      slide({ title: '표지' }),
      slide({ title: '단계', order: 2, blocks: { body: { kind: 'steps', items: ['하나', '둘'] } } }),
      slide({ title: '지표', order: 3, blocks: { body: { kind: 'kpi', items: ['하나'] } } }),
      slide({ title: '비교', order: 4, blocks: { body: { kind: 'comparison', items: ['하나'] } } }),
      slide({ title: '추이', order: 5, blocks: { body: { kind: 'lineChart', items: ['하나'] } } }),
      slide({ title: '다음 단계', order: 6 }),
    ]
    const drafts = deck.map((_, index) => noteDraft(deck, index))
    expect(new Set(drafts).size).toBe(drafts.length)
    expect(drafts[0]).toContain('왜 지금')
    expect(drafts[1]).toContain('완료 조건')
    expect(drafts[2]).toContain('숫자')
    expect(drafts[3]).toContain('권고안')
    expect(drafts[4]).toContain('차트')
    expect(drafts[5]).toContain('결정')
  })

  it('다음 장으로 넘기는 말을 붙이고, 조사를 맞춘다', () => {
    const deck = [slide({ title: '현황' }), slide({ title: '비용과 효과', order: 2 })]
    expect(noteDraft(deck, 0)).toContain('그다음 비용과 효과로 넘어갑니다.')
    const consonant = [slide({ title: '현황' }), slide({ title: '실행 계획', order: 2 })]
    expect(noteDraft(consonant, 0)).toContain('그다음 실행 계획으로 넘어갑니다.')
  })

  it('마지막 장에는 넘길 곳이 없으므로 붙이지 않는다', () => {
    const deck = [slide({ title: '현황' }), slide({ title: '다음 단계', order: 2 })]
    expect(noteDraft(deck, 1)).not.toContain('넘어갑니다')
  })

  it('제목뿐인 장에도 할 말을 준다', () => {
    // The old draft gave this slide its own title, twice.
    const draft = noteDraft([slide({ title: '남은 질문' })], 0)
    expect(draft.length).toBeGreaterThan(10)
    expect(draft).not.toContain('남은 질문')
  })
})
