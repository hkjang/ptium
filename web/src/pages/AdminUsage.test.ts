import { describe, expect, it } from 'vitest'
import { barHeight, failedShare, howLong } from './AdminUsagePage'

describe('what a deployment has been doing, said in numbers', () => {
  it('says a duration the way its size deserves', () => {
    // The built-in writer answers in hundredths of a second: rounding that to
    // whole seconds told an operator their decks take "0초".
    expect(howLong(0.01)).toBe('10밀리초')
    expect(howLong(2.4)).toBe('2.4초')
    expect(howLong(45)).toBe('45초')
    expect(howLong(305.3)).toBe('5분 5초')
    expect(howLong(0)).toBe('—')
  })

  it('draws a day against the busiest day, and nothing for an empty one', () => {
    const days = [
      { day: '2026-08-24', generated: 100, failed: 0, medianSeconds: 0, slowestSeconds: 0 },
      { day: '2026-08-25', generated: 50, failed: 0, medianSeconds: 0, slowestSeconds: 0 },
      { day: '2026-08-26', generated: 0, failed: 0, medianSeconds: 0, slowestSeconds: 0 },
    ]
    expect(barHeight(days[0], days)).toBe(100)
    expect(barHeight(days[1], days)).toBe(50)
    expect(barHeight(days[2], days)).toBe(0)
    // A day with one deck is still visible against a day with a thousand.
    const busy = [{ day: 'a', generated: 1000, failed: 0, medianSeconds: 0, slowestSeconds: 0 },
      { day: 'b', generated: 1, failed: 0, medianSeconds: 0, slowestSeconds: 0 }]
    expect(barHeight(busy[1], busy)).toBe(4)
  })

  it('says what share failed, and nothing at all when nothing was made', () => {
    expect(failedShare({ generated: 200, failed: 5 })).toBe(2.5)
    expect(failedShare({ generated: 0, failed: 0 })).toBe(0)
  })
})
