import { describe, expect, it } from 'vitest'
import { linkState, whoseLink } from './AdminSharesPage'

describe('the links a deployment has handed out', () => {
  it('names who made one with whatever the account has', () => {
    expect(whoseLink({ ownerEmail: 'a@b.com', ownerName: '홍길동' })).toBe('a@b.com')
    // An account signed in through a proxy can have no address at all, and a
    // list whose owner column is blank names nobody.
    expect(whoseLink({ ownerName: '홍길동' })).toBe('홍길동')
    expect(whoseLink({})).toBe('알 수 없는 사용자')
  })

  it('says what a link does now', () => {
    expect(linkState({ state: 'revoked' }).text).toBe('회수됨')
    expect(linkState({ state: 'expired' }).text).toBe('기한 지남')
    // A link with no day on it is the one to look at: it stays open until
    // somebody takes it back.
    expect(linkState({ state: 'open' })).toEqual({ text: '직접 회수할 때까지', tone: 'warning' })
    expect(linkState({ state: 'open', expiresAt: '2026-09-01T00:00:00Z' }).text).toContain('까지')
  })
})
