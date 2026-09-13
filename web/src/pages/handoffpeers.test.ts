import { describe, expect, it } from 'vitest'
import { handoffProblem, handoffSummary, parsePeerLine } from './handoffpeers'

describe('the handoff peer list refuses what the server would', () => {
  it('reads name=origin, lowered, and nothing looser', () => {
    expect(parsePeerLine(' Weekly = HTTPS://Weekly.Intra/ ')).toEqual({ name: 'weekly', origin: 'https://weekly.intra' })
    expect(parsePeerLine('umm=http://umm.intra:8080')).toEqual({ name: 'umm', origin: 'http://umm.intra:8080' })
    for (const bad of ['https://weekly.intra', 'weekly=', 'weekly=weekly.intra', 'weekly=ftp://weekly.intra',
      'weekly=https://weekly.intra/handoff', 'weekly=https://weekly.intra?x=1', 'weekly=https://u@weekly.intra', 'we ekly=https://weekly.intra']) {
      expect(parsePeerLine(bad), bad).toHaveProperty('problem')
    }
  })

  it('lets an empty list and a sound one save, and names the first bad line', () => {
    expect(handoffProblem([])).toBe('')
    expect(handoffProblem(['umm=https://umm.intra', 'weekly=https://weekly.intra'])).toBe('')
    expect(handoffProblem(['umm=https://umm.intra', 'nonsense'])).toContain('nonsense')
    expect(handoffProblem(['umm=https://a.intra', 'muni=https://a.intra'])).toContain('두 번')
    expect(handoffProblem(Array.from({ length: 51 }, (_, i) => `s${i}=https://s${i}.intra`))).toContain('50')
  })

  it('says what the list would do', () => {
    expect(handoffSummary([])).toContain('비어')
    expect(handoffSummary(['umm=https://umm.intra', 'muni=https://muni.intra'])).toContain('없음')
    expect(handoffSummary(['umm=https://umm.intra', 'weekly=https://weekly.intra'])).toContain('weekly')
  })
})
