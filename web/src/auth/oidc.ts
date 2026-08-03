import type { AuthConfig } from '../types'
import { session } from '../api/client'

const PKCE_KEY = 'ptium.oidc_transaction'

interface OidcTransaction {
  state: string
  verifier: string
  redirectUri: string
  returnTo: string
  createdAt: number
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

export async function beginOidcLogin(config: AuthConfig, returnTo = '/dashboard') {
  if (!supportsBrowserPkce(config)) throw new Error('OIDC PKCE 설정이 완전하지 않습니다.')
  const verifier = randomBase64Url(64)
  const state = randomBase64Url(32)
  const redirectUri = config.redirectUri || `${window.location.origin}/auth/callback`
  const transaction: OidcTransaction = { state, verifier, redirectUri, returnTo, createdAt: Date.now() }
  sessionStorage.setItem(PKCE_KEY, JSON.stringify(transaction))
  const url = new URL(config.authorizationEndpoint!)
  url.searchParams.set('response_type', 'code')
  url.searchParams.set('client_id', config.clientId!)
  url.searchParams.set('redirect_uri', redirectUri)
  url.searchParams.set('scope', (config.scopes?.length ? config.scopes : ['openid', 'profile', 'email']).join(' '))
  url.searchParams.set('state', state)
  url.searchParams.set('code_challenge', await sha256Base64Url(verifier))
  url.searchParams.set('code_challenge_method', 'S256')
  window.location.assign(url.toString())
}

export async function completeOidcCallback(config: AuthConfig): Promise<{ completed: boolean; returnTo?: string }> {
  if (window.location.pathname !== '/auth/callback') return { completed: false }
  const params = new URLSearchParams(window.location.search)
  const providerError = params.get('error')
  if (providerError) throw new Error(params.get('error_description') || `로그인 제공자 오류: ${providerError}`)
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
  return { completed: true, returnTo: transaction.returnTo }
}
