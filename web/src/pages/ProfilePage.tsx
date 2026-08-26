import { useCallback, useEffect, useState } from 'react'
import { Check, KeyRound, Palette, Save, Sparkles, UserRound } from 'lucide-react'
import { api } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import { AppShell } from '../components/AppShell'
import { Button, ErrorState, Field, Input, LoadingState, Select, Textarea } from '../components/UI'
import { useToast } from '../components/Toast'
import { accentNote, seededAccent } from '../branding/accent'
import { designChoices, designFamilies, resolveDesignKey, type DesignChoice } from '../branding/designs'
import { SlidePreview } from '../components/SlidePreview'
import type { ProfilePreferences } from '../types'
import { displayError } from '../utils'

const defaults: ProfilePreferences = {
  name: '', jobTitle: '', company: '', bio: '', language: 'ko', defaultAudience: '경영진과 의사결정자',
  defaultTone: 'professional', defaultTheme: 'aurora', brandColor: '#8068E8',
}

export function ProfilePage() {
  const { user, refreshUser } = useAuth()
  const [profile, setProfile] = useState<ProfilePreferences>({ ...defaults, name: user?.name || '' })
  const [loading, setLoading] = useState(true)
  const [loaded, setLoaded] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  // Whether this colour is theirs. The swatch falls back to the deployment's
  // brand colour so the field is never empty, and a fallback is not a choice:
  // saving a name must not quietly write a colour nobody picked.
  const [chosenColor, setChosenColor] = useState(false)
  const [seeded, setSeeded] = useState(seededAccent)
  // The designs this deployment ships. The screen used to offer four names from
  // an older version of this product, none of which is a design key today.
  const [designs, setDesigns] = useState<DesignChoice[]>([])
  const { showToast } = useToast()
  const load = useCallback(async () => {
    setLoading(true)
    setLoaded(false)
    setError('')
    const [profileResult, settingsResult, templateResult] = await Promise.allSettled([
      api.profile(), api.publicSettings(), api.templates()])
    setDesigns(templateResult.status === 'fulfilled' ? designChoices(templateResult.value) : [])
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
      setSeeded(String(settings['branding.seeded_brand_color'] || seededAccent))
      setChosenColor(Boolean(data.brandColor))
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
  // What the stored value actually selects — a design key, a name an older
  // version wrote, or a bare family name — read the way the server reads it.
  const chosenDesign = resolveDesignKey(profile.defaultTheme, designs)
  const chosenTemplate = designs.find((design) => design.key === chosenDesign)
  const update = <K extends keyof ProfilePreferences>(key: K, value: ProfilePreferences[K]) => setProfile((current) => ({ ...current, [key]: value }))
  const save = async () => {
    if (chosenColor && !/^#[0-9a-f]{6}$/i.test(profile.brandColor)) {
      showToast('브랜드 강조 색상은 #RRGGBB 형식으로 입력해 주세요.', 'error')
      return
    }
    setSaving(true)
    try { await api.updateProfile({ ...profile, brandColor: chosenColor ? profile.brandColor : undefined }); await refreshUser(); showToast('개인화 설정을 저장했습니다.') } catch (err) { showToast(displayError(err), 'error') } finally { setSaving(false) }
  }
  return <AppShell title="개인화" eyebrow="MY PREFERENCES" actions={<Button disabled={saving || loading || !loaded} onClick={() => void save()}><Save size={16} /> {saving ? '저장 중…' : '변경사항 저장'}</Button>}>
    <div className="settings-layout"><aside className="settings-anchor-nav"><a href="#identity" className="active"><UserRound size={16} /> 기본 정보</a><a href="#defaults"><Sparkles size={16} /> 생성 기본값</a><a href="#brand"><Palette size={16} /> 브랜드 스타일</a>{user?.hasPassword && <a href="#password"><KeyRound size={16} /> 비밀번호</a>}</aside><div className="settings-content">
      {loading ? <LoadingState label="개인 설정을 불러오는 중…" /> : error ? <ErrorState message={error} onRetry={() => void load()} /> : loaded ? <>
        <section id="identity" className="settings-card"><div className="settings-card-head"><span><UserRound size={19} /></span><div><h2>기본 정보</h2><p>생성 엔진이 발표 맥락과 표현을 맞추는 데 사용하는 프로필입니다.</p></div></div><div className="settings-card-body"><div className="profile-identity"><span className="avatar large">{(profile.name || user?.email || 'P').slice(0, 2).toUpperCase()}</span><div><strong>{profile.name || '이름을 입력해 주세요'}</strong><span>{user?.email}</span></div></div><div className="form-grid two"><Field label="표시 이름"><Input maxLength={120} value={profile.name} onChange={(event) => update('name', event.target.value)} /></Field><Field label="직무"><Input maxLength={200} value={profile.jobTitle} onChange={(event) => update('jobTitle', event.target.value)} placeholder="예: 프로덕트 매니저" /></Field></div><Field label="회사 · 조직"><Input maxLength={200} value={profile.company} onChange={(event) => update('company', event.target.value)} placeholder="예: Ptium Labs" /></Field><Field label="발표 맥락" hint="슬라이드 내용과 어조를 개인화할 때 참고합니다."><Textarea value={profile.bio} onChange={(event) => update('bio', event.target.value)} maxLength={4000} placeholder="예: B2B SaaS 제품을 담당하며, 데이터에 근거한 간결한 발표를 선호합니다." /></Field></div></section>
        <section id="defaults" className="settings-card"><div className="settings-card-head"><span><Sparkles size={19} /></span><div><h2>생성 기본값</h2><p>현재 조직 기본값을 표시합니다. 저장하면 내 개인 기본값으로 고정되어 웹에서 새 프레젠테이션을 만들 때 적용됩니다.</p></div></div><div className="settings-card-body"><div className="form-grid two"><Field label="기본 청중"><Input maxLength={300} value={profile.defaultAudience} onChange={(event) => update('defaultAudience', event.target.value)} /></Field><Field label="기본 작성 언어"><Select value={profile.language} onChange={(event) => update('language', event.target.value)}><option value="ko">한국어</option><option value="en">English</option><option value="ja">日本語</option><option value="zh">中文</option></Select></Field></div><Field label="기본 발표 톤"><div className="choice-chips">{[{id:'professional',label:'전문적'},{id:'persuasive',label:'설득력 있는'},{id:'friendly',label:'친근한'},{id:'inspiring',label:'영감을 주는'}].map((item) => <button key={item.id} className={profile.defaultTone === item.id ? 'active' : ''} onClick={() => update('defaultTone', item.id)}>{profile.defaultTone === item.id && <Check size={13} />}{item.label}</button>)}</div></Field></div></section>
        <section id="brand" className="settings-card"><div className="settings-card-head"><span><Palette size={19} /></span><div><h2>브랜드 스타일</h2><p>새 프레젠테이션과 PPTX의 기본 시각 스타일입니다.</p></div></div><div className="settings-card-body"><Field label="기본 테마" hint={designs.length ? '새 프레젠테이션을 만들 때 미리 선택되는 디자인입니다. 이 배포가 갖고 있는 디자인만 나옵니다.' : '디자인 목록을 불러오지 못했습니다. 저장된 값을 그대로 둡니다.'}>{designs.length ? <div className="profile-design-picker"><Select value={chosenDesign} onChange={(event) => update('defaultTheme', event.target.value)}>{designFamilies(designs).map((group) => <optgroup key={group.family} label={group.family}>{group.designs.map((design) => <option key={design.key} value={design.key}>{design.name}</option>)}</optgroup>)}</Select>{chosenTemplate && <SlidePreview cacheKey={chosenTemplate.id} className="profile-design-preview" alt={`${chosenTemplate.name} 표지`} load={() => api.templateLayoutPreview(chosenTemplate.id, '', 520)} />}</div> : <Input value={profile.defaultTheme} readOnly />}</Field><Field label="브랜드 강조 색상" hint={accentNote(profile.brandColor, seeded)}><div className="color-input"><input type="color" value={/^#[0-9a-f]{6}$/i.test(profile.brandColor) ? profile.brandColor : '#8068E8'} onChange={(event) => { setChosenColor(true); update('brandColor', event.target.value) }} /><Input value={profile.brandColor} onChange={(event) => { setChosenColor(true); update('brandColor', event.target.value) }} /></div></Field></div></section>
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
