import type { AuthConfig } from '../types'
import { api, session } from '../api/client'
import { markSilentSsoAttempted, safeReturnTo } from './silentSso'

const PKCE_KEY = 'ptium.oidc_transaction'

interface OidcTransaction {
  state: string
  verifier: string
  redirectUri: string
  returnTo: string
  createdAt: number
  // A silent try (prompt=none) is remembered with the transaction, because the
  // provider's answer to it is read differently: login_required is what it
  // says when nobody is signed in, and that is not an error to show.
  silent?: boolean
}

function randomBase64Url(bytes = 48) {
  const data = new Uint8Array(bytes)
  crypto.getRandomValues(data)
  let binary = ''
  data.forEach((value) => { binary += String.fromCharCode(value) })
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

async function sha256Base64Url(value: string) {
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(value))
  let binary = ''
  new Uint8Array(digest).forEach((byte) => { binary += String.fromCharCode(byte) })
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

export function supportsBrowserPkce(config: AuthConfig) {
  return Boolean(config.authorizationEndpoint && (config.tokenEndpoint || config.tokenExchangeUrl) && config.clientId)
}

/**
 * The authorization request. prompt=none goes on it only when the server
 * published autoLogin: the administrator's setting is what decides whether
 * this app sends browsers to the provider unasked, and a caller — or an
 * address somebody typed — cannot decide it instead.
 */
export function authorizationUrl(config: AuthConfig, request: { state: string; redirectUri: string; codeChallenge: string; silent?: boolean }) {
  const url = new URL(config.authorizationEndpoint!)
  url.searchParams.set('response_type', 'code')
  url.searchParams.set('client_id', config.clientId!)
  url.searchParams.set('redirect_uri', request.redirectUri)
  url.searchParams.set('scope', (config.scopes?.length ? config.scopes : ['openid', 'profile', 'email']).join(' '))
  url.searchParams.set('state', request.state)
  url.searchParams.set('code_challenge', request.codeChallenge)
  url.searchParams.set('code_challenge_method', 'S256')
  if (request.silent && config.autoLogin) url.searchParams.set('prompt', 'none')
  return url.toString()
}

export async function beginOidcLogin(config: AuthConfig, returnTo = '/dashboard', options: { silent?: boolean } = {}) {
  if (!supportsBrowserPkce(config)) throw new Error('OIDC PKCE 설정이 완전하지 않습니다.')
  const silent = Boolean(options.silent && config.autoLogin)
  const verifier = randomBase64Url(64)
  const state = randomBase64Url(32)
  const redirectUri = config.redirectUri || `${window.location.origin}/auth/callback`
  const transaction: OidcTransaction = { state, verifier, redirectUri, returnTo: safeReturnTo(returnTo), createdAt: Date.now(), silent }
  const url = authorizationUrl(config, { state, redirectUri, codeChallenge: await sha256Base64Url(verifier), silent })
  // Marked before leaving, not after coming back: a try that never comes back
  // in a readable state must still count as the one this tab gets.
  if (silent) markSilentSsoAttempted()
  sessionStorage.setItem(PKCE_KEY, JSON.stringify(transaction))
  window.location.assign(url)
}

// What the provider says to prompt=none when it holds no session. Each is the
// same ordinary answer: show the login screen.
const NO_SESSION = ['login_required', 'interaction_required', 'consent_required', 'account_selection_required']

export interface OidcCallbackResult {
  completed: boolean
  returnTo?: string
  // Where to land instead of the page that was asked for. Set when the provider
  // answered a silent try with "nobody is signed in": the login page, with the
  // marker that keeps this tab from asking again.
  landing?: string
  // A provider error worth showing, when the landing is the login page.
  error?: string
}

/**
 * The address a refused silent try lands on. A refusal of the ordinary kind
 * says nothing; any other error on a silent try is shown, but still lands on
 * the marked login page so it is not retried. A loud try's errors are left to
 * the caller, which has always shown them.
 */
export function silentRefusalLanding(params: URLSearchParams, transaction: { silent?: boolean } | null): Pick<OidcCallbackResult, 'landing' | 'error'> | null {
  const providerError = params.get('error')
  if (!providerError || !transaction?.silent) return null
  if (NO_SESSION.includes(providerError)) return { landing: '/login?sso=none' }
  return { landing: '/login?sso=error', error: params.get('error_description') || `로그인 제공자 오류: ${providerError}` }
}

function storedTransaction(): OidcTransaction | null {
  try {
    const stored = sessionStorage.getItem(PKCE_KEY)
    return stored ? JSON.parse(stored) as OidcTransaction : null
  } catch {
    return null
  }
}

export async function completeOidcCallback(config: AuthConfig): Promise<OidcCallbackResult> {
  if (window.location.pathname !== '/auth/callback') return { completed: false }
  const params = new URLSearchParams(window.location.search)
  const providerError = params.get('error')
  if (providerError) {
    const refused = silentRefusalLanding(params, storedTransaction())
    if (refused) {
      sessionStorage.removeItem(PKCE_KEY)
      return { completed: false, ...refused }
    }
    throw new Error(params.get('error_description') || `로그인 제공자 오류: ${providerError}`)
  }
  const code = params.get('code')
  if (!code) return { completed: false }
  const stored = sessionStorage.getItem(PKCE_KEY)
  if (!stored) throw new Error('로그인 요청 정보를 찾을 수 없습니다. 로그인을 다시 시작해 주세요.')
  let transaction: OidcTransaction
  try { transaction = JSON.parse(stored) as OidcTransaction } catch { throw new Error('로그인 요청 정보가 손상되었습니다.') }
  sessionStorage.removeItem(PKCE_KEY)
  if (Date.now() - transaction.createdAt > 10 * 60 * 1000) throw new Error('로그인 요청이 만료되었습니다. 다시 로그인해 주세요.')
  if (!params.get('state') || params.get('state') !== transaction.state) throw new Error('로그인 state 검증에 실패했습니다.')
  const endpoint = config.tokenExchangeUrl || config.tokenEndpoint
  if (!endpoint || !config.clientId) throw new Error('토큰 교환 설정이 없습니다.')
  const body = new URLSearchParams({
    grant_type: 'authorization_code',
    client_id: config.clientId,
    code,
    redirect_uri: transaction.redirectUri,
    code_verifier: transaction.verifier,
  })
  const response = await fetch(endpoint, {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded', Accept: 'application/json' },
    body,
    credentials: config.tokenExchangeUrl ? 'include' : 'omit',
  })
  const tokenBody = await response.json().catch(() => null) as Record<string, unknown> | null
  if (!response.ok || !tokenBody?.access_token) throw new Error(String(tokenBody?.error_description || tokenBody?.error || 'OIDC 토큰 교환에 실패했습니다.'))
  session.set(String(tokenBody.access_token))
  // The provider's access token lives minutes and cannot be refreshed here — the
  // refresh token never reaches the browser. Trade it once for a Ptium session
  // cookie, which is renewed while the person keeps working, and stop carrying the
  // provider's token: a stale one in this tab would override the cookie.
  if (await api.startSession()) session.clearBearer()
  // The tab's one silent try is not given back here: whether a session now
  // exists is known only once the server says who this is, and that is where
  // the flag is cleared. Clearing it on a code alone would let an account the
  // provider knows and this app refuses ask again on every load.
  return { completed: true, returnTo: safeReturnTo(transaction.returnTo) }
}
