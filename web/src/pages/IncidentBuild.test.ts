import { describe, expect, it } from 'vitest'
import { buildNote } from './incidentbuild'

const record = (over: Partial<Parameters<typeof buildNote>[0]> = {}) =>
  ({ status: 'open' as const, firstSeenVersion: '', lastSeenVersion: '', ...over })

describe('which build a fault belongs to', () => {
  it('says nothing about a record written before the product kept the build', () => {
    const note = buildNote(record(), '1.39.0')
    expect(note.standing).toBe('unknown')
    expect(note.text).toBe('')
  })

  it('names the running build when the fault happened on it', () => {
    const note = buildNote(record({ firstSeenVersion: '1.39.0', lastSeenVersion: '1.39.0' }), '1.39.0')
    expect(note.standing).toBe('live')
    expect(note.text).toContain('1.39.0')
  })

  it('separates a fault that survived an upgrade from one that stopped', () => {
    const survived = buildNote(record({ firstSeenVersion: '1.30.0', lastSeenVersion: '1.39.0' }), '1.39.0')
    expect(survived.standing).toBe('spanning')
    const stopped = buildNote(record({ firstSeenVersion: '1.13.9', lastSeenVersion: '1.13.9' }), '1.39.0')
    expect(stopped.standing).toBe('elsewhere')
    expect(stopped.text).toContain('1.13.9')
    expect(stopped.text).toContain('1.39.0')
  })

  // The product must not imply a fault stopped happening when a recurrence
  // would have opened a record of its own rather than touching this one.
  it('only claims silence on this build for a group a recurrence would reopen', () => {
    const open = buildNote(record({ status: 'open', lastSeenVersion: '1.13.9' }), '1.39.0')
    expect(open.text).toContain('아직 기록되지 않았습니다')
    const resolved = buildNote(record({ status: 'resolved', lastSeenVersion: '1.13.9' }), '1.39.0')
    expect(resolved.text).not.toContain('아직 기록되지 않았습니다')
  })

  it('still names the build when the server has not said which one it runs', () => {
    const note = buildNote(record({ lastSeenVersion: '1.13.9' }), '')
    expect(note.text).toContain('1.13.9')
  })
})
