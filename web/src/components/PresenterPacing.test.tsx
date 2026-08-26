import { describe, expect, it } from 'vitest'
import { pacing } from './Presentation'

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
