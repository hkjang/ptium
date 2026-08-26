import type { Share } from '../../types'
import { relativeDate } from '../../utils'

/**
 * Whether a link still opens the deck.
 *
 * The list asked only whether a link had been revoked, so a link whose day had
 * passed sat among the open ones with a 회수 button beside it, described as
 * "3일 전까지" — a sentence that reads as recency rather than as death. Somebody
 * looking at that list to see what is still out there would have handed it on.
 */
export function shareState(share: Pick<Share, 'revokedAt' | 'expiresAt'>): 'revoked' | 'expired' | 'open' {
  if (share.revokedAt) return 'revoked'
  if (share.expiresAt && Date.parse(share.expiresAt) <= Date.now()) return 'expired'
  return 'open'
}

/** What the row says about how long this link has. */
export function shareLife(share: Pick<Share, 'revokedAt' | 'expiresAt'>) {
  switch (shareState(share)) {
    case 'revoked':
      return '회수됨'
    case 'expired':
      return `만료됨 · ${relativeDate(share.expiresAt!)}`
    default:
      return share.expiresAt ? `${relativeDate(share.expiresAt)}까지` : '직접 회수할 때까지'
  }
}
