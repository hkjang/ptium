import { describe, expect, it } from 'vitest'
import { incidentsHere } from './AdminOverviewPage'

describe('how much of the open-error count belongs to this build', () => {
  it('says nothing new when there is nothing open', () => {
    expect(incidentsHere({ openIncidents: 0 })).toBe('확인 또는 해결 필요')
  })

  // The one that matters: a site that upgrades into this release carries open
  // groups recorded before the product kept the build. Blank means nobody
  // knows which build saw them, and "모두 이전 버전에서 발생" would be a claim
  // made from no evidence — the kind of sentence that gets an operator to
  // ignore a fault that is still live.
  it('does not call an unrecorded build an earlier one', () => {
    const line = incidentsHere({ openIncidents: 8, openIncidentsThisBuild: 0, openIncidentsOtherBuild: 0 })
    expect(line).toBe('확인 또는 해결 필요')
    expect(line).not.toContain('버전')
  })

  it('counts what this build has seen', () => {
    expect(incidentsHere({ openIncidents: 5, openIncidentsThisBuild: 2, openIncidentsOtherBuild: 3 })).toContain('2')
    expect(incidentsHere({ openIncidents: 3, openIncidentsThisBuild: 3, openIncidentsOtherBuild: 0 })).toBe('모두 현재 버전에서 발생')
  })

  it('only says all of them are elsewhere when all of them are accounted for', () => {
    expect(incidentsHere({ openIncidents: 4, openIncidentsThisBuild: 0, openIncidentsOtherBuild: 4 })).toBe('모두 다른 버전에서 발생')
    expect(incidentsHere({ openIncidents: 4, openIncidentsThisBuild: 0, openIncidentsOtherBuild: 1 })).toBe('다른 버전에서 발생 1')
  })
})
