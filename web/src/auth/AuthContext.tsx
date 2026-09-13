import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { api, ApiError, session } from '../api/client'
import type { AuthConfig, User } from '../types'
import { beginOidcLogin, completeOidcCallback, supportsBrowserPkce } from './oidc'
import { clearSilentSsoState, markSignedOut, returnToHere, shouldAttemptSilentSso } from './silentSso'

interface AuthState {
  user: User | null
  config: AuthConfig | null
  loading: boolean
  error: string | null
  signInDev: (secret: string) => Promise<void>
  signInPassword: (username: string, password: string) => Promise<void>
  signOut: () => Promise<void>
  refreshUser: () => Promise<void>
}

const AuthContext = createContext<AuthState | null>(null)

function captureImplicitToken() {
  const url = new URL(window.location.href)
  const hash = new URLSearchParams(url.hash.replace(/^#/, ''))
  const token = url.searchParams.get('access_token') || hash.get('access_token')
  if (!token) return
  session.set(token)
  url.searchParams.delete('access_token')
  url.searchParams.delete('token_type')
  url.hash = ''
  window.history.replaceState(null, '', `${url.pathname}${url.search}`)
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null)
  const [config, setConfig] = useState<AuthConfig | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const refreshUser = useCallback(async () => {
    try {
      const nextUser = await api.me()
      setUser(nextUser)
      setError(null)
    } catch (err) {
      setUser(null)
      if (err instanceof ApiError && err.status !== 401) setError(err.message)
    }
  }, [config?.devAuthEnabled])

  useEffect(() => {
    let active = true
    // A shared deck is opened by someone who has no account here. Asking the
    // server who they are answers 401, which is not an error worth reporting to
    // a person who came to look at a deck — and asking at all is a request they
    // did not make.
    if (window.location.pathname.startsWith('/view/')) {
      setLoading(false)
      return
    }
    captureImplicitToken()
    // Set when this page is on its way to the provider. The screen stays on
    // "preparing" rather than showing a login form that is about to be left.
    let leaving = false
    async function bootstrap() {
      setLoading(true)
      try {
        const authConfig = await api.authConfig()
        if (!active) return
        setConfig(authConfig)
        try {
          const callback = await completeOidcCallback(authConfig)
          if (callback.completed) {
            window.history.replaceState(null, '', callback.returnTo || '/dashboard')
            window.dispatchEvent(new PopStateEvent('popstate'))
          } else if (callback.landing) {
            // The provider answered a silent try with "nobody is signed in".
            // That is the login page's cue, and the marker in its address is
            // what keeps this tab from asking the same question again.
            if (active && callback.error) setError(callback.error)
            window.history.replaceState(null, '', callback.landing)
            window.dispatchEvent(new PopStateEvent('popstate'))
          }
        } catch (callbackError) {
          if (active) setError(callbackError instanceof Error ? callbackError.message : 'OIDC 로그인을 완료하지 못했습니다.')
          window.history.replaceState(null, '', '/login')
          window.dispatchEvent(new PopStateEvent('popstate'))
        }
        if (!authConfig.enabled) {
          setUser({ id: 'local', email: 'local@ptium.app', name: 'Ptium 사용자', role: 'admin', status: 'active' })
          return
        }
        try {
          const current = await api.me()
          if (active) setUser(current)
          clearSilentSsoState()
        } catch (err) {
          if (!active) return
          if (!(err instanceof ApiError && err.status === 401)) {
            setError(err instanceof Error ? err.message : '사용자 정보를 불러오지 못했습니다.')
            return
          }
          // Nobody is signed in here. Before showing a login screen, the
          // provider may be asked — once — whether it still has a session for
          // this person, and the deep link they arrived on goes along so they
          // come back to it. Every reason not to ask lives in silentSso.ts.
          if (shouldAttemptSilentSso(authConfig) && supportsBrowserPkce(authConfig)) {
            leaving = true
            try {
              await beginOidcLogin(authConfig, returnToHere(), { silent: true })
            } catch {
              leaving = false
            }
          }
        }
      } catch (err) {
        if (!active) return
        setConfig({ enabled: true, oidcEnabled: false, devAuthEnabled: false })
        setError(err instanceof Error ? err.message : '인증 설정을 불러오지 못했습니다.')
      } finally {
        if (active && !leaving) setLoading(false)
      }
    }
    void bootstrap()
    return () => { active = false }
  }, [])

  const signInDev = useCallback(async (secret: string) => {
    // Drop any session cookie first, so the developer identity is what answers.
    await api.logout()
    session.setDev(secret)
    try {
      const current = await api.me()
      setUser(current)
      setError(null)
      clearSilentSsoState()
    } catch (err) {
      session.clear()
      throw err
    }
  }, [])

  const signInPassword = useCallback(async (username: string, password: string) => {
    session.clear()
    await api.logout()
    const current = await api.passwordLogin(username, password)
    setUser(current)
    setError(null)
    clearSilentSsoState()
  }, [])

  const signOut = useCallback(async () => {
    const endSessionEndpoint = config?.endSessionEndpoint
    const clientId = config?.clientId
    // Signing out on purpose, and then being signed straight back in by the
    // provider's session, would look like sign-out does not work. Noted
    // before anything else, so a failure below cannot leave it unsaid.
    markSignedOut()
    session.clear()
    // The session cookie is HttpOnly, so only the server can clear it.
    await api.logout()
    setUser(null)
    if (endSessionEndpoint) {
      const logout = new URL(endSessionEndpoint)
      logout.searchParams.set('post_logout_redirect_uri', `${window.location.origin}/login`)
      if (clientId) logout.searchParams.set('client_id', clientId)
      window.location.assign(logout)
    }
  }, [config?.clientId, config?.endSessionEndpoint])

  const value = useMemo(() => ({ user, config, loading, error, signInDev, signInPassword, signOut, refreshUser }),
    [user, config, loading, error, signInDev, signInPassword, signOut, refreshUser])
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth() {
  const context = useContext(AuthContext)
  if (!context) throw new Error('useAuth must be used within AuthProvider')
  return context
}
