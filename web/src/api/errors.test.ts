import { describe, expect, it } from 'vitest'
import { errorText } from './errors'

describe('what the server refused, in the reader’s words', () => {
  it('writes the message the server sent', () => {
    expect(errorText('version_conflict', 'The presentation changed in another session'))
      .toBe('다른 곳에서 이 덱이 먼저 바뀌었습니다. 새로고침한 뒤 다시 시도해 주세요.')
    expect(errorText('presentation_has_no_slides', 'Generate or add slides before exporting'))
      .toBe('내보내려면 먼저 슬라이드를 만들어 주세요.')
  })

  // A deck whose every slide is skipped is a deck the person can fix, and the
  // toast has to say which thing to do.
  it('says what to do about a deck with nothing to print', () => {
    const said = errorText('presentation_has_no_printable_slides',
      'Every slide is marked skipped, so the PDF would have no pages')
    expect(said).toContain('건너뛰기')
    expect(said).not.toMatch(/[a-z]{4,}/)
  })

  it('falls back to the code when the message is one nobody listed', () => {
    expect(errorText('validation_error', 'some new rule nobody has written yet'))
      .toBe('입력한 값이 올바르지 않습니다.')
    expect(errorText('not_found', 'A thing that is new')).toBe('요청한 항목을 찾을 수 없습니다.')
  })

  it('leaves an unknown message alone rather than mangling it', () => {
    expect(errorText('brand_new_code', 'Something specific the server wanted to say'))
      .toBe('Something specific the server wanted to say')
  })

  it('says nothing when the server said nothing', () => {
    expect(errorText('', '')).toBe('')
  })
})
