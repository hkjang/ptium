import { describe, expect, it } from 'vitest'
import { beingWritten, normalizeStatus } from './client'

describe('where a deck is', () => {
  it('keeps waiting for a worker apart from being written', () => {
    // Folding these together told an author slides were being written for them
    // while their deck sat in a queue — or while no worker was running at all.
    expect(normalizeStatus('queued')).toBe('queued')
    expect(normalizeStatus('generating')).toBe('generating')
  })

  it('reads the words other servers use for being written', () => {
    expect(normalizeStatus('processing')).toBe('generating')
    expect(normalizeStatus('running')).toBe('generating')
  })

  it('calls a finished deck ready and keeps the rest as they are', () => {
    expect(normalizeStatus('completed')).toBe('ready')
    expect(normalizeStatus('draft')).toBe('draft')
    expect(normalizeStatus('failed')).toBe('failed')
    expect(normalizeStatus('something-a-newer-server-says')).toBe('draft')
  })

  it('counts both as on their way', () => {
    expect(beingWritten('queued')).toBe(true)
    expect(beingWritten('generating')).toBe(true)
    for (const status of ['draft', 'ready', 'failed'] as const) {
      expect(beingWritten(status), status).toBe(false)
    }
  })
})
