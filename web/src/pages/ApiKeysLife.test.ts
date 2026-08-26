import { describe, expect, it } from 'vitest'
import { stopsWorking } from './ApiKeysPage'

const day = 86400000
const tomorrow = new Date(Date.now() + day).toISOString()
const nextWeek = new Date(Date.now() + 7 * day).toISOString()

describe('when a key stops working', () => {
  it('says nothing stops an active key with no expiry', () => {
    expect(stopsWorking({ status: 'active' })).toEqual({ reason: 'none' })
  })

  it('says the expiry of a key that has one', () => {
    expect(stopsWorking({ status: 'active', expiresAt: nextWeek })).toEqual({ at: nextWeek, reason: 'expiry' })
  })

  // The one that was wrong: a key rotated away has no expiry of its own, so the
  // column said "만료 없음" over the key that dies tomorrow.
  it('says the grace end of a key that was rotated away', () => {
    expect(stopsWorking({ status: 'rotating', graceUntil: tomorrow })).toEqual({ at: tomorrow, reason: 'grace' })
  })

  it('says whichever comes first when a rotated key also has an expiry', () => {
    expect(stopsWorking({ status: 'rotating', graceUntil: tomorrow, expiresAt: nextWeek }))
      .toEqual({ at: tomorrow, reason: 'grace' })
    expect(stopsWorking({ status: 'rotating', graceUntil: nextWeek, expiresAt: tomorrow }))
      .toEqual({ at: tomorrow, reason: 'expiry' })
  })

  it('ignores a grace that belongs to a key already over', () => {
    // Once the grace has passed the key reads as revoked, and a revoked key is
    // not waiting for anything.
    expect(stopsWorking({ status: 'revoked', graceUntil: tomorrow })).toEqual({ reason: 'none' })
  })
})
