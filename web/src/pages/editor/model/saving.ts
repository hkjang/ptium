/**
 * What the editor's save asks the server with.
 *
 * A save that sends an older version than the server holds is refused with a
 * conflict, and the editor tells the author it could not save an edit that was
 * perfectly fine. The number is easy to send by accident: a handler that writes
 * `{ ...presentation, title }` copies whatever version was current when that
 * render happened, which may be two saves ago.
 *
 * So the save asks with the highest version it has ever seen rather than the
 * one attached to whichever object it happens to be holding.
 */
export function versionToSend(snapshotVersion: number, newestSeen: number) {
  return Math.max(snapshotVersion || 0, newestSeen || 0)
}
