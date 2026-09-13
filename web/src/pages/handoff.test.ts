import { describe, expect, it } from 'vitest'
import { handoffReceiveUrl, loginPathKeeping, parseHandoffQuery, sendableTargets, targetLabel } from './handoff'

describe('sending a deck to another service', () => {
  it('opens the receiving side at /handoff with the source and the claim', () => {
    expect(handoffReceiveUrl('https://weekly.intra/', 'https://slides.intra', 'abc_DEF-0123456789'))
      .toBe('https://weekly.intra/handoff?source=https%3A%2F%2Fslides.intra&claim=abc_DEF-0123456789')
  })

  it('shows only destinations that take a presentation', () => {
    expect(sendableTargets([
      { name: 'weekly', origin: 'https://weekly.intra', formats: ['pptx'] },
      { name: 'muni', origin: 'https://muni.intra', formats: ['markdown'] },
      { name: 'odd', origin: 'javascript:alert(1)', formats: ['pptx'] },
    ]).map((target) => target.name)).toEqual(['weekly'])
    expect(sendableTargets([])).toEqual([])
  })

  it('labels a destination by its name and host', () => {
    expect(targetLabel({ name: 'weekly', origin: 'https://weekly.intra', formats: ['pptx'] })).toBe('weekly (weekly.intra)')
    expect(targetLabel({ name: '', origin: 'https://weekly.intra:8443', formats: ['pptx'] })).toBe('weekly.intra:8443')
  })
})

describe('receiving a document another service offers', () => {
  it('reads the source and the claim a browser brought', () => {
    expect(parseHandoffQuery('?source=https%3A%2F%2Fumm.intra&claim=abcdefghijklmnopqrstuvwxyz012345'))
      .toEqual({ source: 'https://umm.intra', claim: 'abcdefghijklmnopqrstuvwxyz012345' })
  })

  it('says in words when the address is not a handoff', () => {
    for (const search of ['', '?source=https://umm.intra', '?claim=abcdefghijklmnopqrstuvwxyz012345']) {
      expect(parseHandoffQuery(search)).toHaveProperty('problem')
    }
    expect(parseHandoffQuery('?source=https://umm.intra/docs/1&claim=abcdefghijklmnopqrstuvwxyz012345')).toHaveProperty('problem')
    expect(parseHandoffQuery('?source=umm.intra&claim=abcdefghijklmnopqrstuvwxyz012345')).toHaveProperty('problem')
    expect(parseHandoffQuery('?source=https://umm.intra&claim=../x')).toHaveProperty('problem')
    expect(parseHandoffQuery('?source=https://umm.intra&claim=short')).toHaveProperty('problem')
  })

  it('keeps /handoff through a login, and nothing else it need not', () => {
    expect(loginPathKeeping('/handoff', '?source=https%3A%2F%2Fumm.intra&claim=abcdefghijklmnop'))
      .toBe('/login?return_to=%2Fhandoff%3Fsource%3Dhttps%253A%252F%252Fumm.intra%26claim%3Dabcdefghijklmnop')
    expect(loginPathKeeping('/presentations/abc/editor', '')).toBe('/login?return_to=%2Fpresentations%2Fabc%2Feditor')
    expect(loginPathKeeping('/', '')).toBe('/login')
    expect(loginPathKeeping('/dashboard', '')).toBe('/login')
    expect(loginPathKeeping('/login', '?sso=none')).toBe('/login')
  })
})
