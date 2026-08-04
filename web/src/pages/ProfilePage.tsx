import { useCallback, useEffect, useState } from 'react'
import { Check, KeyRound, Palette, Save, Sparkles, UserRound } from 'lucide-react'
import { api } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import { AppShell } from '../components/AppShell'
import { Button, ErrorState, Field, Input, LoadingState, Select, Textarea } from '../components/UI'
import { useToast } from '../components/Toast'
import type { ProfilePreferences } from '../types'
import { displayError } from '../utils'

const defaults: ProfilePreferences = {
  name: '', jobTitle: '', company: '', bio: '', language: 'ko', defaultAudience: '경영진과 의사결정자',
  defaultTone: 'professional', defaultTheme: 'aurora', brandColor: '#8068E8',
}

const themes = [
  { id: 'aurora', name: 'Aurora', colors: ['#17162d', '#8d72ff'] },
  { id: 'paper', name: 'Editorial', colors: ['#eee8dc', '#cc684f'] },
  { id: 'mint', name: 'Fresh', colors: ['#dff7ec', '#42af87'] },
  { id: 'graphite', name: 'Graphite', colors: ['#23272d', '#a9b3c0'] },
]

export function ProfilePage() {
  const { user, refreshUser } = useAuth()
  const [profile, setProfile] = useState<ProfilePreferences>({ ...defaults, name: user?.name || '' })
  const [loading, setLoading] = useState(true)
  const [loaded, setLoaded] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const { showToast } = useToast()
  const load = useCallback(async () => {
    setLoading(true)
    setLoaded(false)
    setError('')
    const [profileResult, settingsResult] = await Promise.allSettled([api.profile(), api.publicSettings()])
    if (profileResult.status === 'fulfilled' && settingsResult.status === 'fulfilled') {
      const data = profileResult.value
      const settings = settingsResult.value
      setProfile({
        ...defaults,
        ...data,
        language: data.language || String(settings['generation.default_lang'] || defaults.language),
        defaultAudience: data.defaultAudience || String(settings['generation.default_audience'] || defaults.defaultAudience),
        defaultTone: data.defaultTone || String(settings['generation.default_tone'] || defaults.defaultTone),
        defaultTheme: data.defaultTheme || String(settings['generation.default_theme'] || defaults.defaultTheme),
        brandColor: data.brandColor || String(settings['branding.brand_color'] || defaults.brandColor),
      })
      setLoaded(true)
    } else {
      const failures = [
        profileResult.status === 'rejected' ? `개인 설정: ${displayError(profileResult.reason)}` : '',
        settingsResult.status === 'rejected' ? `조직 기본값: ${displayError(settingsResult.reason)}` : '',
      ].filter(Boolean)
      setError(failures.join(' · '))
    }
    setLoading(false)
  }, [])
  useEffect(() => { void load() }, [load])
  const update = <K extends keyof ProfilePreferences>(key: K, value: ProfilePreferences[K]) => setProfile((current) => ({ ...current, [key]: value }))
  const save = async () => {
    if (!/^#[0-9a-f]{6}$/i.test(profile.brandColor)) {
      showToast('브랜드 강조 색상은 #RRGGBB 형식으로 입력해 주세요.', 'error')
      return
    }
    setSaving(true)
    try { await api.updateProfile(profile); await refreshUser(); showToast('개인화 설정을 저장했습니다.') } catch (err) { showToast(displayError(err), 'error') } finally { setSaving(false) }
  }
  return <AppShell title="개인화" eyebrow="MY PREFERENCES" actions={<Button disabled={saving || loading || !loaded} onClick={() => void save()}><Save size={16} /> {saving ? '저장 중…' : '변경사항 저장'}</Button>}>
    <div className="settings-layout"><aside className="settings-anchor-nav"><a href="#identity" className="active"><UserRound size={16} /> 기본 정보</a><a href="#defaults"><Sparkles size={16} /> 생성 기본값</a><a href="#brand"><Palette size={16} /> 브랜드 스타일</a>{user?.hasPassword && <a href="#password"><KeyRound size={16} /> 비밀번호</a>}</aside><div className="settings-content">
      {loading ? <LoadingState label="개인 설정을 불러오는 중…" /> : error ? <ErrorState message={error} onRetry={() => void load()} /> : loaded ? <>
        <section id="identity" className="settings-card"><div className="settings-card-head"><span><UserRound size={19} /></span><div><h2>기본 정보</h2><p>생성 엔진이 발표 맥락과 표현을 맞추는 데 사용하는 프로필입니다.</p></div></div><div className="settings-card-body"><div className="profile-identity"><span className="avatar large">{(profile.name || user?.email || 'P').slice(0, 2).toUpperCase()}</span><div><strong>{profile.name || '이름을 입력해 주세요'}</strong><span>{user?.email}</span></div></div><div className="form-grid two"><Field label="표시 이름"><Input maxLength={120} value={profile.name} onChange={(event) => update('name', event.target.value)} /></Field><Field label="직무"><Input maxLength={200} value={profile.jobTitle} onChange={(event) => update('jobTitle', event.target.value)} placeholder="예: 프로덕트 매니저" /></Field></div><Field label="회사 · 조직"><Input maxLength={200} value={profile.company} onChange={(event) => update('company', event.target.value)} placeholder="예: Ptium Labs" /></Field><Field label="발표 맥락" hint="슬라이드 내용과 어조를 개인화할 때 참고합니다."><Textarea value={profile.bio} onChange={(event) => update('bio', event.target.value)} maxLength={4000} placeholder="예: B2B SaaS 제품을 담당하며, 데이터에 근거한 간결한 발표를 선호합니다." /></Field></div></section>
        <section id="defaults" className="settings-card"><div className="settings-card-head"><span><Sparkles size={19} /></span><div><h2>생성 기본값</h2><p>현재 조직 기본값을 표시합니다. 저장하면 내 개인 기본값으로 고정되어 웹에서 새 프레젠테이션을 만들 때 적용됩니다.</p></div></div><div className="settings-card-body"><div className="form-grid two"><Field label="기본 청중"><Input maxLength={300} value={profile.defaultAudience} onChange={(event) => update('defaultAudience', event.target.value)} /></Field><Field label="기본 작성 언어"><Select value={profile.language} onChange={(event) => update('language', event.target.value)}><option value="ko">한국어</option><option value="en">English</option><option value="ja">日本語</option><option value="zh">中文</option></Select></Field></div><Field label="기본 발표 톤"><div className="choice-chips">{[{id:'professional',label:'전문적'},{id:'persuasive',label:'설득력 있는'},{id:'friendly',label:'친근한'},{id:'inspiring',label:'영감을 주는'}].map((item) => <button key={item.id} className={profile.defaultTone === item.id ? 'active' : ''} onClick={() => update('defaultTone', item.id)}>{profile.defaultTone === item.id && <Check size={13} />}{item.label}</button>)}</div></Field></div></section>
        <section id="brand" className="settings-card"><div className="settings-card-head"><span><Palette size={19} /></span><div><h2>브랜드 스타일</h2><p>새 프레젠테이션과 PPTX의 기본 시각 스타일입니다.</p></div></div><div className="settings-card-body"><Field label="기본 테마"><div className="profile-themes">{themes.map((theme) => <button key={theme.id} className={profile.defaultTheme === theme.id ? 'active' : ''} onClick={() => update('defaultTheme', theme.id)}><span style={{ background: theme.colors[0] }}><i style={{ background: theme.colors[1] }} /></span><b>{theme.name}</b>{profile.defaultTheme === theme.id && <em><Check size={12} /></em>}</button>)}</div></Field><Field label="브랜드 강조 색상" hint="생성된 슬라이드의 강조선과 PPTX에 사용됩니다."><div className="color-input"><input type="color" value={/^#[0-9a-f]{6}$/i.test(profile.brandColor) ? profile.brandColor : '#8068E8'} onChange={(event) => update('brandColor', event.target.value)} /><Input value={profile.brandColor} onChange={(event) => update('brandColor', event.target.value)} /></div></Field></div></section>
        {user?.hasPassword && <PasswordCard />}
      </> : null}
    </div></div>
  </AppShell>
}

/**
 * Password change for a local account. Changing it retires every session token
 * issued earlier, so the API hands back a fresh one and the user stays signed in
 * here while other browsers are signed out.
 */
function PasswordCard() {
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [confirm, setConfirm] = useState('')
  const [saving, setSaving] = useState(false)
  const { showToast } = useToast()

  const tooShort = next.length > 0 && next.length < 12
  const mismatch = confirm.length > 0 && next !== confirm
  const ready = current.length > 0 && next.length >= 12 && next === confirm

  const submit = async () => {
    setSaving(true)
    try {
      await api.changePassword(current, next)
      setCurrent(''); setNext(''); setConfirm('')
      showToast('비밀번호를 변경했습니다. 다른 기기의 세션은 로그아웃됩니다.')
    } catch (err) { showToast(displayError(err), 'error') } finally { setSaving(false) }
  }

  return <section id="password" className="settings-card">
    <div className="settings-card-head"><span><KeyRound size={19} /></span><div><h2>비밀번호</h2>
      <p>이 계정은 아이디와 비밀번호로 로그인합니다. 변경하면 다른 기기의 로그인 세션이 모두 해제됩니다.</p></div></div>
    <div className="settings-card-body">
      <Field label="현재 비밀번호"><Input type="password" value={current} onChange={(event) => setCurrent(event.target.value)} autoComplete="current-password" /></Field>
      <div className="form-grid two">
        <Field label="새 비밀번호" hint="12자 이상" error={tooShort ? '12자 이상이어야 합니다.' : undefined}>
          <Input type="password" value={next} onChange={(event) => setNext(event.target.value)} autoComplete="new-password" />
        </Field>
        <Field label="새 비밀번호 확인" error={mismatch ? '두 값이 다릅니다.' : undefined}>
          <Input type="password" value={confirm} onChange={(event) => setConfirm(event.target.value)} autoComplete="new-password" />
        </Field>
      </div>
      <div><Button disabled={!ready || saving} onClick={() => void submit()}>{saving ? '변경 중…' : '비밀번호 변경'}</Button></div>
    </div>
  </section>
}
