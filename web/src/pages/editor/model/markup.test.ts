import { describe, expect, it } from 'vitest'
import { linkSelection, markupFor, wrapSelection } from './markup'

describe('marking a word', () => {
  it('wraps what is selected', () => {
    const line = '이번 분기 매출이 늘었습니다'
    const marked = wrapSelection(line, 6, 9, '**', '굵게')
    expect(marked.text).toBe('이번 분기 **매출이** 늘었습니다')
    expect(marked.text.slice(marked.start, marked.end)).toBe('매출이')
  })

  it('takes the marks off when the same word is marked again', () => {
    const line = '이번 분기 **매출이** 늘었습니다'
    // The selection people actually make: the word, not the marks around it.
    const inside = wrapSelection(line, 8, 11, '**', '굵게')
    expect(inside.text).toBe('이번 분기 매출이 늘었습니다')
    expect(inside.text.slice(inside.start, inside.end)).toBe('매출이')
    // And the selection that takes the marks in.
    const around = wrapSelection(line, 6, 13, '**', '굵게')
    expect(around.text).toBe('이번 분기 매출이 늘었습니다')
  })

  it('leaves something to type when nothing is selected', () => {
    const marked = wrapSelection('', 0, 0, '*', '기울임')
    expect(marked.text).toBe('*기울임*')
    expect(marked.text.slice(marked.start, marked.end)).toBe('기울임')
  })

  it('does not mistake bold for italic', () => {
    const marked = wrapSelection('**굵게**', 2, 4, '*', '기울임')
    expect(marked.text).toBe('***굵게***')
  })
})

describe('making a link', () => {
  it('keeps the words and puts the cursor where the address goes', () => {
    const marked = linkSelection('안내 문서를 보십시오', 0, 5)
    expect(marked.text).toBe('[안내 문서](https://)를 보십시오')
    expect(marked.start).toBe(marked.end)
    expect(marked.text.slice(0, marked.start).endsWith('https://')).toBe(true)
  })

  it('writes both halves when nothing is selected', () => {
    const marked = linkSelection('', 0, 0)
    expect(marked.text).toBe('[보이는 말](https://)')
  })
})

describe('which key does what', () => {
  it('answers only for the three gestures, and only with a modifier', () => {
    expect(markupFor({ key: 'b', ctrlKey: true, metaKey: false, altKey: false })).toBeTruthy()
    expect(markupFor({ key: 'I', ctrlKey: false, metaKey: true, altKey: false })).toBeTruthy()
    expect(markupFor({ key: 'k', ctrlKey: true, metaKey: false, altKey: false })).toBeTruthy()
    expect(markupFor({ key: 'b', ctrlKey: false, metaKey: false, altKey: false })).toBeNull()
    // Alt+Ctrl+B is somebody's own shortcut, not ours.
    expect(markupFor({ key: 'b', ctrlKey: true, metaKey: false, altKey: true })).toBeNull()
    expect(markupFor({ key: 's', ctrlKey: true, metaKey: false, altKey: false })).toBeNull()
  })
})
