import { afterEach, describe, expect, it } from 'vitest'
import { markSignedOut, shouldAttemptSilentSso } from './silentSso'

// This one runs with a document, because what it checks is the module's own
// reach for window.sessionStorage — the defaults the app calls it with — and
// not a storage a test handed in.
const config = { enabled: true, oidcEnabled: true, devAuthEnabled: false, autoLogin: true }
const original = Object.getOwnPropertyDescriptor(window, 'sessionStorage')

afterEach(() => {
  if (original) Object.defineProperty(window, 'sessionStorage', original)
  window.history.replaceState(null, '', '/')
})

describe('the storage the app hands over', () => {
  it('is this tab\'s session storage, read once per tab', () => {
    window.history.replaceState(null, '', '/dashboard')
    window.sessionStorage.clear()
    expect(shouldAttemptSilentSso(config)).toBe(true)
    markSignedOut()
    expect(shouldAttemptSilentSso(config)).toBe(false)
  })

  it('fails towards the login screen when the browser will not even hand it over', () => {
    // A browser with site data blocked throws on the property itself, before
    // anything is read from it. That is not "not yet tried".
    window.history.replaceState(null, '', '/dashboard')
    Object.defineProperty(window, 'sessionStorage', {
      configurable: true,
      get() { throw new DOMException('The operation is insecure.', 'SecurityError') },
    })
    expect(shouldAttemptSilentSso(config)).toBe(false)
    expect(() => markSignedOut()).not.toThrow()
  })
})
