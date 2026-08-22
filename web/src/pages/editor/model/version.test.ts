import { describe, expect, it } from 'vitest'
import { versionToSend } from './saving'

describe('the version a save asks with', () => {
  it('is the newest the server has reported', () => {
    expect(versionToSend(4, 6)).toBe(6)
  })

  it('is the snapshot when nothing newer has arrived', () => {
    expect(versionToSend(6, 6)).toBe(6)
    expect(versionToSend(7, 6)).toBe(7)
  })

  it('never goes backwards while typing over an in-flight save', () => {
    // A render at version 4; a save completes and the server says 5; a keystroke
    // then writes { ...presentation } from the older render.
    let newest = 0
    const seen = (version: number) => { if (version > newest) newest = version }
    seen(4)
    seen(5)
    const staleCopy = { version: 4 }
    expect(versionToSend(staleCopy.version, newest)).toBe(5)
  })
})
