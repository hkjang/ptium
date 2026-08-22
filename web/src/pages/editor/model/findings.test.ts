import { describe, expect, it } from 'vitest'
import { findingDetail, findingLabel, revisionReason, scoreDimensionLabel } from './findings'

describe('a measurement in the reader’s words', () => {
  it('writes each finding as a sentence about their slide', () => {
    const cases: [string, string][] = [
      ['title region extends 1.2cm past the slide edge', '제목 영역이 슬라이드 밖으로 1.2cm 나갔습니다'],
      ["7 lines of text in room for 5; it must shrink to 82% of the template's size",
        '5줄 자리에 7줄이 들어가 템플릿 크기의 82%로 줄여야 합니다'],
      ['body overlaps title by 18%', '본문이 제목 영역과 18% 겹칩니다'],
      ['text 1f2937 on ffffff is 3.1:1, below 4.5:1',
        '글자색 #1f2937과 배경 #ffffff의 대비가 3.1:1로, 기준 4.5:1에 못 미칩니다'],
      ['9 points on one slide; past 6 an audience reads instead of listening',
        '한 장에 요점이 9개입니다. 6개를 넘으면 듣지 않고 읽습니다'],
      ['no speaker notes: nothing is written down to say over this slide',
        '발표 노트가 없습니다. 이 슬라이드에서 무엇을 말할지 적혀 있지 않습니다'],
      ['kpi had too little room to draw anything', '핵심 지표을 그릴 자리가 없었습니다'],
      ['two lines of the quote overlap', '인용의 두 줄이 서로 겹칩니다'],
    ]
    for (const [measured, written] of cases) {
      expect(findingDetail(measured), measured).toBe(written)
    }
    // Nothing a rule produced may still be in the measurement's own language:
    // a half-translated sentence reads as a bug in the product.
    for (const [measured] of cases) {
      // A colour is a colour in every language; everything else a rule wrote
      // must be in the reader's, or the sentence reads as a bug.
      const written = findingDetail(measured).replace(/#[0-9a-f]{3,8}/g, '')
      expect(written, measured).not.toMatch(/[a-z]{3,}/)
    }
  })

  it('leaves a sentence it has no rule for exactly as measured', () => {
    // Half-translating would be worse than not translating: the reader would
    // not know whether the words are theirs or the measurement's.
    expect(findingDetail('something nobody wrote a rule for')).toBe('something nobody wrote a rule for')
  })

  it('names every kind the measurement can report', () => {
    const kinds = ['overflow', 'outside', 'collision', 'contrast', 'orphan', 'density', 'notes', 'repeat', 'source']
    for (const kind of kinds) {
      expect(findingLabel(kind), kind).not.toBe(kind)
    }
    expect(findingLabel('kind-from-a-newer-server')).toBe('kind-from-a-newer-server')
  })

  it('names every axis of the score and every checkpoint reason', () => {
    for (const key of ['readability', 'structure', 'visual', 'accessibility', 'evidence']) {
      expect(scoreDimensionLabel(key), key).not.toBe(key)
    }
    for (const reason of ['edit', 'source', 'generation', 'restore']) {
      expect(revisionReason(reason), reason).not.toBe(reason)
    }
  })
})
