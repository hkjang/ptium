import type { ServerError } from '../types'

/**
 * Which build a recorded fault belongs to.
 *
 * An operator reading a critical incident months later has one question the
 * record could not answer: is this still my deployment's bug? The occurrence
 * count and the timestamps say how often and when, never on what. A panic last
 * seen on a build this site has since left reads very differently from one that
 * survived the upgrade, and the difference decides whether anybody has to act.
 *
 * Nothing here claims a fault is fixed — the product cannot know that. It says
 * which build saw it and which build is running, and it only adds "not recorded
 * on this build" for a group that is still open, because that is the only case
 * where a recurrence would have been folded into this same record. A resolved
 * group starts a new one when it comes back, so silence there means nothing.
 */
export type BuildStanding = 'live' | 'spanning' | 'elsewhere' | 'unknown'

export interface BuildNote {
  standing: BuildStanding
  /** What to show, or empty when the record cannot say anything. */
  text: string
}

export function buildNote(incident: Pick<ServerError, 'firstSeenVersion' | 'lastSeenVersion' | 'status'>, running: string): BuildNote {
  const first = (incident.firstSeenVersion || '').trim()
  const last = (incident.lastSeenVersion || '').trim()
  const now = (running || '').trim()
  if (!last) {
    // Written before the product kept the build, so it stays silent rather than
    // guessing that the record belongs to whatever is running today.
    return { standing: 'unknown', text: '' }
  }
  if (!now) return { standing: 'unknown', text: `마지막 발생 버전 ${last}` }
  if (last === now) {
    return first && first !== last
      ? { standing: 'spanning', text: `${first}에서 처음 기록됐고 지금 실행 중인 ${now}에서도 발생했습니다.` }
      : { standing: 'live', text: `지금 실행 중인 ${now}에서 발생했습니다.` }
  }
  const open = incident.status === 'open' || incident.status === 'investigating'
  const since = open ? ` 이 그룹은 ${now}에서는 아직 기록되지 않았습니다.` : ''
  return {
    standing: first && first !== last ? 'spanning' : 'elsewhere',
    text: `마지막 발생은 ${last}이고, 지금 실행 중인 버전은 ${now}입니다.${since}`,
  }
}
