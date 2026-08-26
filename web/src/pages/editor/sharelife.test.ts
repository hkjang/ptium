import { describe, expect, it } from 'vitest'
import { shareLife, shareState } from './sharelife'

const day = 86400000
const past = new Date(Date.now() - 3 * day).toISOString()
const future = new Date(Date.now() + 3 * day).toISOString()

describe('whether a share link still opens the deck', () => {
  it('is open until it is revoked when it has no day', () => {
    expect(shareState({})).toBe('open')
    expect(shareLife({})).toBe('직접 회수할 때까지')
  })

  it('is open while its day is ahead, and says until when', () => {
    expect(shareState({ expiresAt: future })).toBe('open')
    expect(shareLife({ expiresAt: future })).toContain('까지')
  })

  // The one that was wrong: a link whose day has passed sat among the open ones
  // described as "3일 전까지", with a 회수 button beside it.
  it('is over once its day has passed, and says so', () => {
    expect(shareState({ expiresAt: past })).toBe('expired')
    expect(shareLife({ expiresAt: past })).toContain('만료됨')
    expect(shareLife({ expiresAt: past })).not.toBe('3일 전까지')
  })

  it('says revoked whatever its day was', () => {
    for (const share of [{ revokedAt: past }, { revokedAt: past, expiresAt: future }]) {
      expect(shareState(share)).toBe('revoked')
      expect(shareLife(share)).toBe('회수됨')
    }
  })
})
