import { describe, expect, it } from 'vitest'
import { changeSummary, changeLabel } from './changes'
import type { SlideChange } from '../../../types'

const change = (kind: SlideChange['kind'], position: number): SlideChange =>
  ({ kind, position, title: `슬라이드 ${position}` })

describe('what changed since a version, in one line', () => {
  it('counts each kind of change', () => {
    expect(changeSummary([change('changed', 2), change('added', 3), change('added', 4), change('moved', 5)]))
      .toBe('1장 수정 · 2장 추가 · 1장 이동')
  })

  it('says so plainly when nothing changed', () => {
    expect(changeSummary([])).toBe('이 버전 이후 바뀐 것이 없습니다')
  })

  it('names each kind in the reader’s words', () => {
    expect(changeLabel('changed')).toBe('수정')
    expect(changeLabel('removed')).toBe('삭제')
    expect(changeLabel('something-new')).toBe('something-new')
  })
})
