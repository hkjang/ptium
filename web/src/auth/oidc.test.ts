import { describe, expect, it } from 'vitest'
import type { AuthConfig } from '../types'
import { authorizationUrl, silentRefusalLanding } from './oidc'

const config: AuthConfig = {
  enabled: true, oidcEnabled: true, devAuthEnabled: false,
  authorizationEndpoint: 'https://sso.example.com/realms/company/protocol/openid-connect/auth',
  tokenEndpoint: 'https://sso.example.com/realms/company/protocol/openid-connect/token',
  clientId: 'ptium-web',
}
const request = { state: 's', redirectUri: 'https://ptium.example.com/auth/callback', codeChallenge: 'c' }
const prompt = (url: string) => new URL(url).searchParams.get('prompt')

// prompt=none sends the browser to the provider before anyone has clicked
// anything. Whether this app does that is the administrator's setting, and
// nothing a caller passes can turn it on for them.
describe('asking the provider for the session it already holds', () => {
  it('adds prompt=none only when asked silently and the administrator allows it', () => {
    expect(prompt(authorizationUrl({ ...config, autoLogin: true }, { ...request, silent: true }))).toBe('none')
  })

  it('is a plain login when the setting is off, however it was asked', () => {
    expect(prompt(authorizationUrl({ ...config, autoLogin: false }, { ...request, silent: true }))).toBeNull()
    expect(prompt(authorizationUrl(config, { ...request, silent: true }))).toBeNull()
  })

  it('is a plain login when nobody asked for a silent one', () => {
    expect(prompt(authorizationUrl({ ...config, autoLogin: true }, request))).toBeNull()
  })

  it('carries the same PKCE request either way', () => {
    const silent = new URL(authorizationUrl({ ...config, autoLogin: true }, { ...request, silent: true })).searchParams
    const loud = new URL(authorizationUrl({ ...config, autoLogin: true }, request)).searchParams
    for (const key of ['response_type', 'client_id', 'redirect_uri', 'scope', 'state', 'code_challenge', 'code_challenge_method']) {
      expect(silent.get(key), key).toBe(loud.get(key))
    }
    expect(silent.get('code_challenge_method')).toBe('S256')
  })
})

// The provider's answer to a silent try when nobody is signed in is
// error=login_required. It is not a failure: it is "show the login screen",
// and the address it lands on carries the mark that stops the next try.
describe('what a refused silent try lands on', () => {
  const params = (query: string) => new URLSearchParams(query)

  it('lands a login_required on the marked login page, saying nothing', () => {
    for (const refusal of ['login_required', 'interaction_required', 'consent_required', 'account_selection_required']) {
      expect(silentRefusalLanding(params(`error=${refusal}`), { silent: true }), refusal).toEqual({ landing: '/login?sso=none' })
    }
  })

  it('still lands a stranger error on a marked page, but says what it was', () => {
    expect(silentRefusalLanding(params('error=invalid_client&error_description=Unknown+client'), { silent: true }))
      .toEqual({ landing: '/login?sso=error', error: 'Unknown client' })
    expect(silentRefusalLanding(params('error=server_error'), { silent: true }))
      .toEqual({ landing: '/login?sso=error', error: '로그인 제공자 오류: server_error' })
  })

  it('leaves the errors of a login somebody clicked to the caller', () => {
    expect(silentRefusalLanding(params('error=login_required'), { silent: false })).toBeNull()
    expect(silentRefusalLanding(params('error=login_required'), {})).toBeNull()
    expect(silentRefusalLanding(params('error=login_required'), null)).toBeNull()
  })

  it('is not a refusal without an error', () => {
    expect(silentRefusalLanding(params('code=abc&state=s'), { silent: true })).toBeNull()
  })
})
