/**
 * Whether a deck in the queue is in trouble.
 *
 * The screen used to call anything older than fifteen minutes stuck. That was a
 * statement about how long generation takes, and this product now leaves a slow
 * generation alone however long it runs — a self-hosted model takes minutes per
 * call and a deployment may ask for ten repair passes on top, so half an hour of
 * writing can be a deck going perfectly well. A worker says it is alive every
 * thirty seconds while it writes; what separates trouble from patience is
 * whether anybody is still saying so.
 */
export type QueueRow = {
  status: string
  waitingSeconds: number
  quietSeconds?: number
}

/** A deck being written has said nothing for this long: nobody is holding it. */
export const quietTooLong = 180

/** A deck still waiting to be picked up after this long: nothing is claiming it. */
export const waitedTooLong = 900

export function troubled(deck: QueueRow) {
  if (deck.status === 'failed') return false
  if (deck.status === 'generating') return (deck.quietSeconds ?? 0) >= quietTooLong
  return deck.waitingSeconds >= waitedTooLong
}

/** What the elapsed column is counting for this row. */
export function elapsedMeans(deck: QueueRow): 'writing' | 'waiting' {
  return deck.status === 'generating' ? 'writing' : 'waiting'
}
