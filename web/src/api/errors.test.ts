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


  it('says a file that is not an image is not an image', () => {
    // The bytes decide, and the person is told what to upload instead.
    expect(errorText('unsupported_image', 'This file is not an image Ptium can place: PNG, JPEG, GIF and SVG'))
      .toContain('PNG')
    expect(errorText('unsupported_image', 'This file is not an image Ptium can place: PNG, JPEG, GIF and SVG'))
      .toContain('확장자만 바뀐')
    // Even when the server words it some other way, the code carries it.
    expect(errorText('unsupported_image', 'something new about images')).toContain('이미지 파일이 아닙니다')
  })


  it('keeps the field and the limit the server named', () => {
    // These arrived as "입력한 값이 올바르지 않습니다" — the code's fallback —
    // because no exact string matches a message with a number in it. The
    // server said which field and what the limit is.
    expect(errorText('validation_error', 'title is required and must not exceed 200 characters'))
      .toBe('제목은 비워 둘 수 없고 200자를 넘을 수 없습니다.')
    expect(errorText('validation_error', 'requestedSlideCount must be between 1 and 50'))
      .toBe('슬라이드 수는 1에서 50 사이여야 합니다.')
    expect(errorText('validation_error', 'slide 7 does not exist'))
      .toContain('7번 슬라이드')
    expect(errorText('validation_error', 'invalid input: the file is empty'))
      .toBe('올린 파일이 비어 있습니다.')
    expect(errorText('validation_error', 'AI provider must be fallback, openai, or openai-compatible'))
      .toBe('AI 공급자는 fallback · openai · openai-compatible 중 하나여야 합니다.')
    // The subject particle is chosen the way Korean chooses it.
    expect(errorText('validation_error', 'name must not exceed 60 characters')).toContain('이름은')
    // And anything with no rule is still shown as the server wrote it.
    expect(errorText('', 'something nobody has written a rule for')).toBe('something nobody has written a rule for')
  })

  it('says how many slides are allowed, not just that there are too many', () => {
    // The author has one of the two numbers and needs the other.
    const said = errorText('too_many_slides', 'The deck source produced 62 slides; this deployment allows 50')
    expect(said).toContain('62장')
    expect(said).toContain('50장')
    expect(said).toContain('12장')
  })
})