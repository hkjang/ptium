import { describe, expect, it } from 'vitest'
import { howBig, nothingWord } from './AdminTidyPage'

describe('what has piled up, said plainly', () => {
  it('says a size the way an operator reads it', () => {
    expect(howBig(0)).toBe('')
    expect(howBig(undefined)).toBe('')
    expect(howBig(900)).toBe('1KB')
    expect(howBig(58_600_000)).toBe('55.9MB')
    expect(howBig(3_221_225_472)).toBe('3.00GB')
  })

  it('says nothing is there rather than showing a zero', () => {
    expect(nothingWord({ kind: 'trashed', count: 0 })).toBe('없습니다')
    expect(nothingWord({ kind: 'trashed', count: 3 })).toBe('')
  })
})
