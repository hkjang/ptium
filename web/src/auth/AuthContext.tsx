import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { api, ApiError, session } from '../api/client'
import type { AuthConfig, User } from '../types'
import { completeOidcCallback } from './oidc'

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
    captureImplicitToken()
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
        } catch (err) {
          if (active && !(err instanceof ApiError && err.status === 401)) setError(err instanceof Error ? err.message : '사용자 정보를 불러오지 못했습니다.')
        }
      } catch (err) {
        if (!active) return
        setConfig({ enabled: true, oidcEnabled: false, devAuthEnabled: false })
        setError(err instanceof Error ? err.message : '인증 설정을 불러오지 못했습니다.')
      } finally {
        if (active) setLoading(false)
      }
    }
    void bootstrap()
    return () => { active = false }
  }, [])

  const signInDev = useCallback(async (secret: string) => {
    session.setDev(secret)
    try {
      const current = await api.me()
      setUser(current)
      setError(null)
    } catch (err) {
      session.clear()
      throw err
    }
  }, [])

  const signInPassword = useCallback(async (username: string, password: string) => {
    session.clear()
    const current = await api.passwordLogin(username, password)
    setUser(current)
    setError(null)
  }, [])

  const signOut = useCallback(async () => {
    const endSessionEndpoint = config?.endSessionEndpoint
    const clientId = config?.clientId
    session.clear()
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
