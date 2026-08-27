import { describe, expect, it } from 'vitest'
import { pacing, pacingSeconds } from './Presentation'

// A talk has a length somebody agreed to, and the speaker cannot divide while
// speaking: knowing the clock is not the same as knowing whether they are
// behind. The number is measured against where the deck should be by now.
describe('pacing', () => {
  it('says nothing is wrong when nothing is', () => {
    expect(pacing(0).tone).toBe('on')
    expect(pacing(44).tone).toBe('on')
    expect(pacing(-44).tone).toBe('on')
    expect(pacing(20).text).toBe('예정대로')
  })

  it('says how far behind, in minutes a person can act on', () => {
    expect(pacing(180)).toEqual({ tone: 'late', text: '3분 늦음' })
    expect(pacing(90).text).toBe('2분 늦음')
  })

  it('says ahead as clearly as behind', () => {
    expect(pacing(-180)).toEqual({ tone: 'early', text: '3분 이름' })
  })
})

// The one that was wrong. A slide is a stretch of time, not a moment, and the
// schedule was read from the start of the one the speaker is on — so somebody
// keeping perfect time was told they were falling further behind all the way
// through every slide, and then that they were on time again the instant they
// pressed the arrow. Nothing about their pace had changed.
describe('where the deck should be by now', () => {
  // Ten slides, twenty minutes, exactly two minutes each.
  const perfect = (minutes: number) => {
    const spent = minutes * 60
    const index = Math.min(9, Math.floor(spent / 120))
    return pacingSeconds(spent, index, 10, 20)
  }

  it('reads zero halfway through the slide a perfect talk is on', () => {
    expect(perfect(1)).toBe(0)     // one minute in, halfway through slide 1
    expect(perfect(7)).toBe(0)     // seven minutes in, halfway through slide 4
    expect(perfect(19)).toBe(0)    // nineteen minutes in, halfway through slide 10
  })

  it('never calls a perfectly kept talk two minutes late', () => {
    for (let second = 0; second < 20 * 60; second++) {
      const index = Math.min(9, Math.floor(second / 120))
      const reading = pacingSeconds(second, index, 10, 20)!
      expect(Math.abs(reading)).toBeLessThanOrEqual(60)
    }
  })

  it('is symmetric across a slide: early at its start, late at its end', () => {
    expect(pacingSeconds(6 * 60, 3, 10, 20)).toBe(-60)        // arriving at slide 4
    expect(pacingSeconds(8 * 60 - 1, 3, 10, 20)).toBe(59)     // leaving slide 4
  })

  it('still says late when the talk really is late, and early when it is early', () => {
    expect(pacingSeconds(10 * 60, 1, 10, 20)).toBe(420)       // still on slide 2 at ten minutes
    expect(pacingSeconds(2 * 60, 7, 10, 20)).toBe(-780)       // on slide 8 at two minutes
  })

  it('says nothing without a target or a deck', () => {
    expect(pacingSeconds(600, 3, 10, 0)).toBeNull()
    expect(pacingSeconds(600, 3, 0, 20)).toBeNull()
  })
})
