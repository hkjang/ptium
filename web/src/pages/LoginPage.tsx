import { useEffect, useState } from 'react'
import { ArrowRight, Check, KeyRound, LockKeyhole, Presentation, ShieldCheck, Sparkles } from 'lucide-react'
import { authLoginUrl } from '../api/client'
import { beginOidcLogin, supportsBrowserPkce } from '../auth/oidc'
import { useAuth } from '../auth/AuthContext'
import { BrandMark, useBrand } from '../branding/BrandContext'
import { Button, ErrorState, Field, Input, Select } from '../components/UI'
import { navigate } from '../router'

const slides = [
  { tag: '01 · INSIGHT', title: '아이디어의 본질부터', text: '복잡한 생각을 선명한 이야기로 정리합니다.', color: 'coral' },
  { tag: '02 · STORY', title: '흐름을 설계하고', text: '청중의 집중을 놓치지 않는 서사를 만듭니다.', color: 'mint' },
  { tag: '03 · DESIGN', title: '완성도 있게 표현해요', text: '브랜드에 맞는 시각 언어를 자동으로 적용합니다.', color: 'violet' },
]

export function LoginPage() {
  const { user, config, loading, error, signInDev } = useAuth()
  const { productName } = useBrand()
  const [secret, setSecret] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [formError, setFormError] = useState('')

  const oidcLogin = async () => {
    if (!config) return
    if (!supportsBrowserPkce(config)) { window.location.assign(authLoginUrl(config)); return }
    setFormError('')
    try { await beginOidcLogin(config) } catch (err) { setFormError(err instanceof Error ? err.message : 'SSO 로그인을 시작하지 못했습니다.') }
  }

  useEffect(() => { if (!loading && user) navigate('/dashboard', true) }, [loading, user])

  const devLogin = async (event: React.FormEvent) => {
    event.preventDefault()
    setSubmitting(true); setFormError('')
    try {
      await signInDev(secret)
      navigate('/dashboard', true)
    } catch (err) {
      setFormError(err instanceof Error ? err.message : '로그인하지 못했습니다.')
    } finally { setSubmitting(false) }
  }

  return (
    <main className="login-page">
      <section className="login-showcase">
        <div className="login-brand"><BrandMark size="large" /><span>{productName}</span></div>
        <div className="showcase-copy">
          <span className="hero-pill"><Sparkles size={14} /> Presentation Studio</span>
          <h1>당신의 생각이<br /><em>발표가 되는 순간.</em></h1>
          <p>한 줄의 아이디어를 설득력 있는 스토리와 아름다운 슬라이드로 완성하세요.</p>
        </div>
        <div className="floating-deck" aria-hidden="true">
          {slides.map((slide, index) => <article key={slide.tag} className={`floating-slide floating-${slide.color}`} style={{ '--slide-index': index } as React.CSSProperties}>
            <span>{slide.tag}</span><div><strong>{slide.title}</strong><p>{slide.text}</p></div><i>{index + 1}</i>
          </article>)}
        </div>
        <div className="showcase-trust"><span><Check size={14} /> 설치형 데이터 보호</span><span><Check size={14} /> 관리자 통합 제어</span><span><Check size={14} /> 표준 API · MCP</span></div>
      </section>

      <section className="login-panel">
        <div className="mobile-login-brand"><BrandMark /><span>{productName}</span></div>
        <div className="login-card">
          <div className="login-heading"><span className="login-icon"><Presentation size={23} /></span><h2>{productName}에 오신 것을 환영해요</h2><p>프레젠테이션 워크스페이스에 로그인하세요.</p></div>
          {loading ? <div className="auth-loading"><span className="loader-orbit" /><span>로그인 방식을 확인하는 중…</span></div> : (
            <>
              {error && <ErrorState title="서버에 연결할 수 없습니다" message={error} />}
              {config?.oidcEnabled && <button type="button" className="sso-button" onClick={() => void oidcLogin()}><span className="sso-logo"><LockKeyhole size={18} /></span><span>{config.providerName || 'SSO'}로 계속하기</span><ArrowRight size={17} /></button>}
              {config?.oidcEnabled && config?.devAuthEnabled && <div className="login-divider"><span>또는 개발자 로그인</span></div>}
              {config?.devAuthEnabled && <form className="dev-login" onSubmit={devLogin}>
                <div className="dev-login-label"><KeyRound size={15} /><span>개발 환경 액세스</span></div>
                <Field label="개발 인증 시크릿" hint="서버의 DEV_AUTH_SECRET과 같은 값을 입력하세요."><Input type="password" value={secret} onChange={(event) => setSecret(event.target.value)} required autoComplete="off" /></Field>
                {formError && <p className="inline-error">{formError}</p>}
                <Button type="submit" size="large" disabled={submitting || !secret}>{submitting ? '로그인 중…' : '개발 계정으로 로그인'} <ArrowRight size={17} /></Button>
              </form>}
              {!config?.oidcEnabled && !config?.devAuthEnabled && !error && <div className="auth-unavailable"><ShieldCheck size={23} /><strong>로그인 설정이 필요합니다</strong><p>관리자에게 OIDC 또는 개발 인증 활성화를 요청하세요.</p></div>}
            </>
          )}
          <p className="login-footnote"><ShieldCheck size={14} /> 로그인 정보는 조직의 인증 정책에 따라 안전하게 처리됩니다.</p>
        </div>
        <p className="login-copyright">© 2026 {productName}. Designed for ideas worth sharing.</p>
      </section>
    </main>
  )
}
