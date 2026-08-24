import { describe, expect, it } from 'vitest'
import {
  findingDetail, findingLabel, objectParticle, trimmedCounts, revisionReason, scoreDimensionLabel,
  subjectParticle, toParticle, warningText,
} from './findings'

describe('a measurement in the reader’s words', () => {
  it('writes each finding as a sentence about their slide', () => {
    const cases: [string, string][] = [
      ['title region extends 1.2cm past the slide edge', '제목 영역이 슬라이드 밖으로 1.2cm 나갔습니다'],
      ["7 lines of text in room for 5; it must shrink to 82% of the template's size",
        '5줄 자리에 7줄이 들어가 템플릿 크기의 82%로 줄여야 합니다'],
      ['body overlaps title by 18%', '본문이 제목 영역과 18% 겹칩니다'],
      ['text 1f2937 on ffffff is 3.1:1, below 4.5:1',
        '글자색 #1f2937과 배경 #ffffff의 대비가 3.1:1로, 기준 4.5:1에 못 미칩니다'],
      ['9 points in one region; past 6 an audience reads instead of listening',
        '한 영역에 요점이 9개입니다. 6개를 넘으면 듣지 않고 읽습니다'],
      ['no speaker notes: nothing is written down to say over this slide',
        '발표 노트가 없습니다. 이 슬라이드에서 무엇을 말할지 적혀 있지 않습니다'],
      ['kpi had too little room to draw anything', '핵심 지표를 그릴 자리가 없었습니다'],
      ['two lines of the quote overlap', '인용의 두 줄이 서로 겹칩니다'],
      ["2 of this slide's 4 points were already made on slide 3",
        '이 장의 요점 4개 중 2개를 3번 슬라이드에서 이미 말했습니다'],
      ['meter draws "이관 일정과 담당" 2.92cm wider than the room it reserved',
        '달성률의 "이관 일정과 담당"이 확보한 자리보다 옆으로 2.92cm 넘칩니다'],
      ['figures with no source: 28.5%, 1,200건',
        '브리프에 없는 숫자에 출처가 없습니다: 28.5%, 1,200건. !source 로 어디서 온 숫자인지 적어 두면 발표자 노트에 함께 나갑니다'],
    ]
    for (const [measured, written] of cases) {
      expect(findingDetail(measured), measured).toBe(written)
    }
    // Nothing a rule produced may still be in the measurement's own language:
    // a half-translated sentence reads as a bug in the product.
    for (const [measured] of cases) {
      // A colour is a colour in every language, and so is a directive someone
      // types (!source, ::kpi). Everything else a rule wrote must be in the
      // reader's language, or the sentence reads as a bug.
      const written = findingDetail(measured).replace(/#[0-9a-f]{3,8}|[!:]{1,2}[a-z]+/g, '')
      expect(written, measured).not.toMatch(/[a-z]{3,}/)
    }
  })

  it('leaves a sentence it has no rule for exactly as measured', () => {
    // Half-translating would be worse than not translating: the reader would
    // not know whether the words are theirs or the measurement's.
    expect(findingDetail('something nobody wrote a rule for')).toBe('something nobody wrote a rule for')
  })

  it('has a sentence for every kind the server measures', () => {
    // The kinds and one real detail line from each, copied from the server. A
    // kind with a name but no rule is how "figures with no source: nothing on
    // this slide…" reached a Korean workspace in v0.46.
    const measured: Record<string, string> = {
      overflow: "7 lines of text in room for 5; it must shrink to 82% of the template's size",
      outside: 'title region extends 1.2cm past the slide edge',
      collision: 'body overlaps title by 18%',
      contrast: 'text 1f2937 on ffffff is 3.1:1, below 4.5:1',
      orphan: 'the last line holds 20% of a line; shortening or rewording the text avoids the stray ending',
      density: '9 points in one region; past 6 an audience reads instead of listening',
      notes: 'no speaker notes: nothing is written down to say over this slide',
      repeat: 'the same point twice: "매출이 늘었다" and "매출 증가"',
      source: 'figures with no source: 28.5%, 1,200건',
      echo: "2 of this slide's 4 points were already made on slide 3",
      trimmed: 'steps draws 5 of its 6 entries; the rest are on no slide',
      link: '"www.example.com" is not a link the deck can follow, so the line draws its markup; a link is https://…, mailto:… or #3 for another slide',
    }
    for (const [kind, detail] of Object.entries(measured)) {
      // A colour, a directive, a scheme and the author's own address are the
      // same in every language; what a rule wrote around them must not be.
      const written = findingDetail(detail)
        .replace(/"[^"]*"|https?:\/\/|mailto:|#[0-9a-f]{3,8}|[!:]{1,2}[a-z]+/g, '')
      expect(written, kind).not.toMatch(/[a-z]{3,}/)
    }
  })

  it('names every kind the measurement can report', () => {
    const kinds = ['overflow', 'outside', 'collision', 'contrast', 'orphan', 'density', 'notes', 'repeat', 'source', 'echo', 'trimmed', 'link']
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

describe('what compiling adjusted, in the reader’s words', () => {
  it('keeps the place and rewrites the sentence', () => {
    expect(warningText('line 46 (slide 8): layout "마무리" has no free body region, so its points were kept as plain text'))
      .toBe('line 46 (slide 8): "마무리" 레이아웃에는 본문 영역이 없어 요점을 제목 아래 줄로 적었습니다')
  })

  it('names the component the way the rest of the editor does', () => {
    expect(warningText('line 12: lineChart has no free region in layout "제목만" and was written as text'))
      .toBe('line 12: "제목만" 레이아웃에 추이 차트를 그릴 자리가 없어 글로 적었습니다')
    expect(warningText('slide 3: columnChart had no numeric values and was drawn as table'))
      .toBe('slide 3: 세로 막대 차트에 숫자가 없어 표로 그렸습니다')
  })

  it('says what a source-language mistake was', () => {
    expect(warningText('line 7: unknown component "flowchart"')).toBe('line 7: "flowchart"은 없는 컴포넌트입니다')
    expect(warningText('line 9: @layout needs a layout id')).toBe('line 9: @layout 에는 레이아웃 id가 필요합니다')
  })

  it('picks the particle the word actually takes', () => {
    // 표를 · 차트를, 표로 · 격자로 — the final consonant decides, and ㄹ takes 로.
    expect(objectParticle('표')).toBe('를')
    expect(objectParticle('격자')).toBe('를')
    expect(objectParticle('인용')).toBe('을')
    expect(toParticle('표')).toBe('로')
    expect(toParticle('인용')).toBe('으로')
    expect(toParticle('서울')).toBe('로')
    // A Latin or numeric ending is read as ending open.
    expect(objectParticle('KPI')).toBe('를')
    expect(subjectParticle('표')).toBe('가')
    expect(subjectParticle('인용')).toBe('이')
  })

  it('leaves a sentence it does not know alone rather than mangling it', () => {
    expect(warningText('something nobody has written a rule for')).toBe('something nobody has written a rule for')
  })
})

describe('what a trimmed component counted', () => {
  it('reads the numbers back out of the sentence the server wrote', () => {
    expect(trimmedCounts('steps draws 5 of its 6 entries; the rest are on no slide'))
      .toEqual({ kind: 'steps', drawn: 5, held: 6 })
    expect(trimmedCounts('comparison draws 3 of its 10 entries; the rest are on no slide'))
      .toEqual({ kind: 'comparison', drawn: 3, held: 10 })
  })

  it('refuses a sentence it does not understand rather than guessing', () => {
    for (const detail of ['9 points in one region; past 6 an audience reads instead of listening',
      'steps draws 5 of its 5 entries; the rest are on no slide', 'steps draws many of its entries']) {
      expect(trimmedCounts(detail), detail).toBeNull()
    }
  })
})
