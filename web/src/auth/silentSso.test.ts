import { describe, expect, it } from 'vitest'
import type { AuthConfig } from '../types'
import {
  clearSilentSsoState, markSignedOut, markSilentSsoAttempted, returnToHere, safeReturnTo,
  shouldAttemptSilentSso, silentSsoAllowedAt,
} from './silentSso'

// A tab's storage, or one that refuses to be read — a private window, or a
// browser with site data blocked.
function tabStorage(): Storage {
  const held = new Map<string, string>()
  return {
    getItem: (key) => held.get(key) ?? null,
    setItem: (key, value) => { held.set(key, value) },
    removeItem: (key) => { held.delete(key) },
    clear: () => held.clear(),
    key: () => null,
    length: 0,
  }
}
function closedStorage(): Storage {
  const refuse = () => { throw new DOMException('The operation is insecure.', 'SecurityError') }
  return { getItem: refuse, setItem: refuse, removeItem: refuse, clear: refuse, key: refuse, length: 0 }
}

const on: AuthConfig = { enabled: true, oidcEnabled: true, devAuthEnabled: false, autoLogin: true }
const off: AuthConfig = { ...on, autoLogin: false }
const at = (pathname: string, search = '') => ({ pathname, search })

// prompt=none answers without drawing a page: a code when the provider holds a
// session, error=login_required when it does not. Asked again after the second
// answer, the browser bounces between the provider and this app until the
// person closes it. Every "no" below is one layer of what stops that.
describe('whether to sign in without a login screen', () => {
  it('asks once when the administrator turned it on', () => {
    const storage = tabStorage()
    expect(shouldAttemptSilentSso(on, at('/dashboard'), storage, tabStorage())).toBe(true)
    markSilentSsoAttempted(storage)
    expect(shouldAttemptSilentSso(on, at('/dashboard'), storage, tabStorage())).toBe(false)
  })

  it('never asks while the administrator has it off, whatever the browser holds', () => {
    expect(shouldAttemptSilentSso(off, at('/dashboard'), tabStorage(), tabStorage())).toBe(false)
    expect(shouldAttemptSilentSso({ ...on, oidcEnabled: false }, at('/dashboard'), tabStorage(), tabStorage())).toBe(false)
    expect(shouldAttemptSilentSso(null, at('/dashboard'), tabStorage(), tabStorage())).toBe(false)
  })

  it('reads the refusal the callback left in the address, even with storage wiped', () => {
    expect(shouldAttemptSilentSso(on, at('/login', '?sso=none'), tabStorage(), tabStorage())).toBe(false)
    expect(shouldAttemptSilentSso(on, at('/login', '?sso=error'), tabStorage(), tabStorage())).toBe(false)
    expect(shouldAttemptSilentSso(on, at('/dashboard', '?sso=none'), tabStorage(), tabStorage())).toBe(false)
  })

  it('does not sign back in somebody who just signed out, until they sign in again', () => {
    const storage = tabStorage()
    const browser = tabStorage()
    markSignedOut(storage, browser)
    expect(shouldAttemptSilentSso(on, at('/dashboard'), storage, browser)).toBe(false)
    clearSilentSsoState(storage, browser)
    expect(shouldAttemptSilentSso(on, at('/dashboard'), storage, browser)).toBe(true)
  })

  // Measured against a real Keycloak: sign out, open a new tab, and the new
  // tab signed straight back in. The sign-out was written to the tab's own
  // storage, which a new tab does not share.
  it('does not sign somebody back in from a new tab after they signed out', () => {
    const browser = tabStorage()
    markSignedOut(tabStorage(), browser)
    const newTab = tabStorage()
    expect(shouldAttemptSilentSso(on, at('/dashboard'), newTab, browser)).toBe(false)
  })

  it('treats storage it cannot read as already tried', () => {
    // Reading a thrown exception as "not yet" is the loop: nothing could ever
    // record the try. Failing towards the login screen is the safe side.
    expect(shouldAttemptSilentSso(on, at('/dashboard'), closedStorage(), tabStorage())).toBe(false)
    expect(shouldAttemptSilentSso(on, at('/dashboard'), tabStorage(), closedStorage())).toBe(false)
    expect(() => markSignedOut(closedStorage(), closedStorage())).not.toThrow()
    expect(() => clearSilentSsoState(closedStorage(), closedStorage())).not.toThrow()
  })

  it('never starts from the callback, the login page, or anything that is not a page', () => {
    for (const path of ['/auth/callback', '/login', '/view/abc', '/api/v1/auth/config', '/mcp', '/healthz', '/readyz']) {
      expect(silentSsoAllowedAt(path), path).toBe(false)
      expect(shouldAttemptSilentSso(on, at(path), tabStorage(), tabStorage()), path).toBe(false)
    }
    for (const path of ['/', '/dashboard', '/presentations/abc/editor', '/admin/settings']) {
      expect(silentSsoAllowedAt(path), path).toBe(true)
    }
  })
})

// The person who arrived on a deep link goes back to it after signing in —
// and only to it. A return address that names a host would make this login a
// stepping stone off the site.
describe('where a silent sign-in comes back to', () => {
  it('keeps a path on this site, with its query', () => {
    expect(safeReturnTo('/presentations/abc/editor?slide=3')).toBe('/presentations/abc/editor?slide=3')
    expect(returnToHere(at('/presentations/abc/editor', '?slide=3'))).toBe('/presentations/abc/editor?slide=3')
  })

  it('refuses anything that could leave the site', () => {
    for (const bad of ['//evil.example/x', 'https://evil.example', '/\\evil.example', 'evil', '', '/x\r\nLocation: y', null, undefined]) {
      expect(safeReturnTo(bad), String(bad)).toBe('/dashboard')
    }
  })

  it('does not come back to the login page or the callback', () => {
    expect(safeReturnTo('/login?sso=none')).toBe('/dashboard')
    expect(safeReturnTo('/auth/callback?code=x')).toBe('/dashboard')
  })
})
