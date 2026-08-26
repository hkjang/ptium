import { describe, expect, it } from 'vitest'
import { elapsedMeans, troubled } from './queuehealth'

describe('whether a deck in the queue is in trouble', () => {
  it('leaves a long generation alone while its worker is saying it is alive', () => {
    // Half an hour of writing, ten seconds since the last heartbeat.
    expect(troubled({ status: 'generating', waitingSeconds: 1800, quietSeconds: 10 })).toBe(false)
  })

  it('calls a generation in trouble when nobody is saying anything', () => {
    expect(troubled({ status: 'generating', waitingSeconds: 240, quietSeconds: 200 })).toBe(true)
  })

  it('still calls a deck nothing has picked up in fifteen minutes stuck', () => {
    expect(troubled({ status: 'queued', waitingSeconds: 1000 })).toBe(true)
    expect(troubled({ status: 'queued', waitingSeconds: 300 })).toBe(false)
  })

  it('says nothing about a deck that already failed', () => {
    expect(troubled({ status: 'failed', waitingSeconds: 99999 })).toBe(false)
  })

  it('treats a generating deck with no heartbeat yet as freshly claimed', () => {
    // An older server does not answer with quietSeconds at all.
    expect(troubled({ status: 'generating', waitingSeconds: 4000 })).toBe(false)
  })

  it('knows what the elapsed column is counting', () => {
    expect(elapsedMeans({ status: 'generating', waitingSeconds: 60 })).toBe('writing')
    expect(elapsedMeans({ status: 'queued', waitingSeconds: 60 })).toBe('waiting')
  })
})
