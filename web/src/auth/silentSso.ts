import type { AuthConfig } from '../types'

// Whether to ask the identity provider for a session it already holds before
// showing anyone a login screen. The asking is prompt=none: the provider never
// draws a page — it answers with a code, or comes straight back with
// error=login_required. That second answer is ordinary, not a failure, and the
// whole of this module is making sure it is answered once and not again, or
// the browser bounces between the provider and this app for as long as the
// person watches it flicker.

// sessionStorage, not localStorage: a new tab tries again, a reload after a
// refusal does not. That is the shape of "once".
const ATTEMPTED_KEY = 'ptium.sso.attempted'
const SIGNED_OUT_KEY = 'ptium.sso.signed_out'

// The marker the callback leaves in the address when the provider had no
// session: /login?sso=none. Storage can be cleared in between; the address
// cannot, so this is the guard that holds when the other one is gone.
export const SSO_MARKER = 'sso'

type Flags = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>
type Place = { pathname: string; search: string }

// Storage that answers every question with an exception, which readFlag reads
// as "already tried". Some browsers throw on touching sessionStorage at all,
// not only on reading it, and that has to fail the same way.
const closed: Flags = {
  getItem: () => { throw new Error('storage is closed') },
  setItem: () => { throw new Error('storage is closed') },
  removeItem: () => { throw new Error('storage is closed') },
}

function flags(): Flags {
  try {
    return window.sessionStorage ?? closed
  } catch {
    return closed
  }
}
function here(): Place { return window.location }

function readFlag(key: string, storage: Flags): boolean {
  try {
    return storage.getItem(key) === 'true'
  } catch {
    // A private window or blocked site data throws here. Reading that as
    // "not yet tried" is exactly the loop: nothing could ever record the try.
    // So it reads as "already tried" — the side that fails still.
    return true
  }
}

function writeFlag(key: string, value: boolean, storage: Flags) {
  try {
    if (value) storage.setItem(key, 'true')
    else storage.removeItem(key)
  } catch {
    // Nothing to do: readFlag already answers "tried" when storage is closed.
  }
}

/** The person signed out on purpose; signing them straight back in would make sign-out look broken. */
export function markSignedOut(storage: Flags = flags()) {
  writeFlag(SIGNED_OUT_KEY, true, storage)
  writeFlag(ATTEMPTED_KEY, true, storage)
}

/** A session exists again, so the next tab may try silently once more. */
export function clearSilentSsoState(storage: Flags = flags()) {
  writeFlag(SIGNED_OUT_KEY, false, storage)
  writeFlag(ATTEMPTED_KEY, false, storage)
}

/** Records that this tab has had its one silent try. */
export function markSilentSsoAttempted(storage: Flags = flags()) {
  writeFlag(ATTEMPTED_KEY, true, storage)
}

// Where a silent try is never started from. The callback and the login page
// are where a refused try lands, so trying again from them is the loop itself;
// the rest are not pages a person is looking at.
const NEVER_FROM = ['/auth/', '/login', '/view/', '/api/', '/mcp', '/healthz', '/readyz']

export function silentSsoAllowedAt(pathname: string): boolean {
  return !NEVER_FROM.some((prefix) => pathname === prefix.replace(/\/$/, '') || pathname.startsWith(prefix))
}

/**
 * Whether this page load may try to sign in without a login screen.
 *
 * Every answer of "no" here is a layer of the same guard: the administrator
 * has not turned it on; this tab already tried; the person signed out; the
 * address says the provider already refused. Any one of them alone stops the
 * loop, and they are kept separate because each survives something the
 * others do not.
 */
export function shouldAttemptSilentSso(config: AuthConfig | null | undefined, place: Place = here(), storage: Flags = flags()): boolean {
  if (!config?.oidcEnabled || !config.autoLogin) return false
  if (!silentSsoAllowedAt(place.pathname)) return false
  if (new URLSearchParams(place.search).has(SSO_MARKER)) return false
  if (readFlag(SIGNED_OUT_KEY, storage)) return false
  if (readFlag(ATTEMPTED_KEY, storage)) return false
  return true
}

/**
 * The place to come back to after signing in, or the home page when the
 * given one could carry someone off this site. A path that begins "//" is a
 * host to the browser, and a backslash is a slash to it too.
 */
export function safeReturnTo(value: string | null | undefined, fallback = '/dashboard'): string {
  const candidate = (value ?? '').trim()
  if (!candidate.startsWith('/') || candidate.startsWith('//') || /[\\\r\n]/.test(candidate)) return fallback
  if (candidate.startsWith('/login') || candidate.startsWith('/auth/')) return fallback
  return candidate
}

/** The address this tab is on, as the return_to a silent sign-in carries. */
export function returnToHere(place: Place = here()): string {
  return safeReturnTo(`${place.pathname}${place.search}`)
}
