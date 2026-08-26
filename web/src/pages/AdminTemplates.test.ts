import { describe, expect, it } from 'vitest'
import { useWord, whoMayUse } from './AdminTemplatesPage'

describe('the designs a deployment writes decks in', () => {
  it('says who may write a deck in one', () => {
    expect(whoMayUse({ kind: 'builtin', scope: 'shared' })).toContain('모두')
    expect(whoMayUse({ kind: 'uploaded', scope: 'shared' })).toContain('모두')
    // An upload nobody opened up belongs to whoever uploaded it, and the screen
    // says whose it is rather than leaving an operator to guess.
    expect(whoMayUse({ kind: 'uploaded', scope: 'private', ownerEmail: 'a@b.com' })).toBe('a@b.com만')
    expect(whoMayUse({ kind: 'uploaded', scope: 'private', ownerName: '홍길동' })).toBe('홍길동만')
    expect(whoMayUse({ kind: 'uploaded', scope: 'private' })).toBe('올린 사람만')
  })

  it('tells a design nobody uses from one nobody used lately', () => {
    expect(useWord({ decks: 0, recent: 0 })).toBe('아직 쓰이지 않았습니다')
    expect(useWord({ decks: 120, recent: 0 })).toContain('최근 30일은 없음')
    expect(useWord({ decks: 120, recent: 8 })).toContain('최근 30일 8개')
  })
})
