import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import type { AuthConfig } from '../types'

// The login screen reads who may sign in how from the auth context. Each test
// sets that answer and looks at what the screen offers for it.
let config: AuthConfig
vi.mock('../auth/AuthContext', () => ({
  useAuth: () => ({ user: null, config, loading: false, error: null,
    signInDev: vi.fn(), signInPassword: vi.fn(), signOut: vi.fn(), refreshUser: vi.fn() }),
}))
vi.mock('../branding/BrandContext', () => ({
  useBrand: () => ({ productName: 'Ptium' }),
  BrandMark: () => null,
}))

import { LoginPage } from './LoginPage'

afterEach(() => { cleanup(); window.history.replaceState(null, '', '/') })

const oidc: AuthConfig = { enabled: true, oidcEnabled: true, devAuthEnabled: false, passwordLoginEnabled: true,
  authorizationEndpoint: 'https://sso.example.com/auth', tokenEndpoint: 'https://sso.example.com/token', clientId: 'ptium' }

// With an identity provider in front, the organisation account is the way in
// and the local account is how an administrator gets back in when it is down.
describe('조직 계정으로 로그인', () => {
  it('회사 계정 버튼을 먼저 두고, 관리자 계정은 접어 둔다', () => {
    config = oidc
    render(<LoginPage />)
    expect(screen.getByRole('button', { name: /회사 계정으로 SSO 로그인/ })).toBeTruthy()
    const toggle = screen.getByRole('button', { name: /관리자 계정으로 로그인/ })
    expect(toggle.getAttribute('aria-expanded')).toBe('false')
    expect(screen.queryByLabelText('비밀번호')).toBeNull()
    fireEvent.click(toggle)
    expect(toggle.getAttribute('aria-expanded')).toBe('true')
    expect(screen.getByText(/복구용 관리자 계정입니다/)).toBeTruthy()
  })

  it('자동 로그인이 안 된 이유를 말한다', () => {
    config = oidc
    window.history.replaceState(null, '', '/login?sso=none')
    render(<LoginPage />)
    expect(screen.getByRole('status').textContent).toContain('회사 계정 세션이 없어 자동으로 로그인하지 않았습니다')
  })

  it('제공자가 없으면 접지 않고 바로 보여 준다', () => {
    config = { ...oidc, oidcEnabled: false }
    window.history.replaceState(null, '', '/login?sso=none')
    render(<LoginPage />)
    expect(screen.queryByRole('button', { name: /회사 계정으로 SSO 로그인/ })).toBeNull()
    expect(screen.queryByRole('button', { name: /관리자 계정으로 로그인/ })).toBeNull()
    expect(screen.getByText(/최초 설치 관리자 계정입니다/)).toBeTruthy()
    // The provider's refusal is not news on a deployment that has no provider.
    expect(screen.queryByRole('status')).toBeNull()
  })

  it('개발자 로그인만 있으면 그 이름으로 접는다', () => {
    config = { ...oidc, passwordLoginEnabled: false, devAuthEnabled: true }
    render(<LoginPage />)
    expect(screen.getByRole('button', { name: /개발자 로그인/ }).getAttribute('aria-expanded')).toBe('false')
  })
})
