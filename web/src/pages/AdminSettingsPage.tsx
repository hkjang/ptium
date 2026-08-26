import { useEffect, useState, type ReactNode } from 'react'
import { Bot, Radio, Brush, Check, ChevronRight, CircleAlert, Eye, EyeOff, LockKeyhole, Save, ShieldCheck, Sparkles } from 'lucide-react'
import { api } from '../api/client'
import { AppShell } from '../components/AppShell'
import { Badge, Button, ErrorState, Field, Input, LoadingState, Select } from '../components/UI'
import { useToast } from '../components/Toast'
import { displayError } from '../utils'

type SectionKey = 'branding' | 'ai' | 'oidc' | 'generation' | 'security'
type SettingValue = string | number | boolean | string[]
type Values = Record<string, SettingValue>
interface SettingSection { id: SectionKey; label: string; description: string; icon: ReactNode }

const sections: SettingSection[] = [
  { id: 'branding', label: '브랜딩', description: '서비스 이름과 시각 정체성', icon: <Brush size={18} /> },
  { id: 'ai', label: 'AI 모델', description: '생성 엔진과 모델 연결', icon: <Bot size={18} /> },
  { id: 'oidc', label: 'OIDC · SSO', description: 'Keycloak과 관리자 역할', icon: <LockKeyhole size={18} /> },
  { id: 'generation', label: '생성 정책', description: '슬라이드 기본값과 제한', icon: <Sparkles size={18} /> },
  { id: 'security', label: '보안 · 키', description: 'API 키 회전과 CORS', icon: <ShieldCheck size={18} /> },
]

const defaults: Record<SectionKey, Values> = {
  branding: { product_name: 'Ptium', logo_url: '', brand_color: '#7C3AED' },
  ai: { provider: 'fallback', base_url: 'https://api.openai.com/v1', model: 'gpt-4.1-mini', api_key: '', reasoning: 'auto', max_output_tokens: 8000, timeout_seconds: 300 },
  oidc: { issuer_url: '', client_id: '', client_secret: '', admin_roles: ['ptium-admin', 'admin'] },
  generation: { default_slide_count: 10, max_slides: 50, default_theme: 'aurora', default_lang: 'ko', default_tone: 'professional', default_audience: 'general' },
  security: { api_key_grace: '24h', cors_origins: [] },
}

/** What came back, said the way an operator needs to read it. */
function providerWord(checked: Record<string, unknown> | null) {
  if (!checked) return '저장된 설정 그대로 한 번 물어봅니다. 아무것도 바뀌지 않습니다.'
  const detail = String(checked.detail || '')
  if (!checked.reachable) return `닿지 않았습니다 — ${detail || '이유가 돌아오지 않았습니다'}`
  const ms = Number(checked.milliseconds ?? 0).toLocaleString('ko-KR')
  return `${ms}ms 만에 답했습니다${detail ? ` · "${detail}"` : ''}`
}

export function AdminSettingsPage() {
  const [active, setActive] = useState<SectionKey>('branding')
  const [values, setValues] = useState(defaults)
  const [persistedValues, setPersistedValues] = useState(defaults)
  const [configuredSecrets, setConfiguredSecrets] = useState<Record<string, boolean>>({})
  const [unreadableSecrets, setUnreadableSecrets] = useState<Record<string, boolean>>({})
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const [dirty, setDirty] = useState<Set<SectionKey>>(new Set())
  const [reveal, setReveal] = useState<Set<string>>(new Set())
  const { showToast } = useToast()

  useEffect(() => {
    api.adminSettings()
      .then((data) => {
        const merged = mergeSettings(defaults, data.values)
        setValues(merged)
        setPersistedValues(structuredClone(merged))
        setConfiguredSecrets(data.configured)
        setUnreadableSecrets(data.unreadable ?? {})
      })
      .catch((err) => setError(displayError(err)))
      .finally(() => setLoading(false))
  }, [])

  const update = (section: SectionKey, key: string, value: SettingValue) => {
    setValues((current) => ({ ...current, [section]: { ...current[section], [key]: value } }))
    setDirty((current) => new Set(current).add(section))
  }
  const save = async () => {
    const section = active
    const validationError = validateSettings(section, values[section])
    if (validationError) { showToast(validationError, 'error'); return }
    setSaving(true)
    try {
      const result = await api.updateAdminSettings(section, values[section])
      setPersistedValues((current) => mergeSettings(current, result.values))
      setConfiguredSecrets((current) => ({ ...current, ...result.configured }))
      setUnreadableSecrets((current) => ({ ...current, ...result.unreadable }))
      setValues((current) => {
        const next = mergeSettings(current, result.values)
        if (section === 'ai') next.ai.api_key = ''
        if (section === 'oidc') next.oidc.client_secret = ''
        return next
      })
      if (section === 'branding') window.dispatchEvent(new Event('ptium:branding-updated'))
      setDirty((current) => { const next = new Set(current); next.delete(section); return next })
      showToast(`${sections.find((item) => item.id === section)?.label} 설정을 저장했습니다.`)
    } catch (err) {
      showToast(displayError(err), 'error')
    } finally {
      setSaving(false)
    }
  }
  const secretField = (label: string, key: string, hint?: string) => <Field label={label} hint={hint}><div className="password-input"><Input type={reveal.has(key) ? 'text' : 'password'} value={String(values[active][key] || '')} onChange={(event) => update(active, key, event.target.value)} placeholder="변경하지 않으려면 비워두세요" /><button type="button" onClick={() => setReveal((current) => { const next = new Set(current); next.has(key) ? next.delete(key) : next.add(key); return next })} aria-label="값 표시 전환">{reveal.has(key) ? <EyeOff size={16} /> : <Eye size={16} />}</button></div></Field>

  const [checked, setChecked] = useState<Record<string, unknown> | null>(null)
  const [checking, setChecking] = useState(false)
  // Asked for, never on a timer: this sends a request to somebody's model host,
  // and a screen left open should not keep knocking on it.
  const runProviderCheck = async () => {
    setChecking(true)
    try {
      const result = await api.checkProvider()
      setChecked(result)
      showToast(result.reachable
        ? `제공자가 ${Number(result.milliseconds ?? 0).toLocaleString('ko-KR')}ms 만에 답했습니다.`
        : '제공자가 답하지 않았습니다. 아래 설명을 확인하세요.', result.reachable ? undefined : 'error')
    } catch (err) { showToast(displayError(err), 'error') } finally { setChecking(false) }
  }

  const persistedProvider = String(persistedValues.ai.provider)
  const externalKeyConfigured = configuredSecrets['ai.api_key'] === true
  const oidcSecretConfigured = configuredSecrets['auth.oidc.client_secret'] === true
  const usingBuiltInGenerator = persistedProvider === 'fallback' || !externalKeyConfigured
  const engineDescription = persistedProvider === 'fallback'
    ? '외부 연결 없이 내장 초안 생성기를 사용합니다.'
    : !externalKeyConfigured
      ? '저장된 API 키가 없어 내장 초안 생성기를 사용합니다.'
      : `${persistedProvider} · ${String(persistedValues.ai.model)}`
  const engineBadge = persistedProvider === 'fallback' ? '오프라인' : externalKeyConfigured ? '키 저장됨' : '키 미설정'

  return <AppShell title="서비스 설정" eyebrow="ADMIN SETTINGS" actions={<Button disabled={saving || !dirty.has(active)} onClick={() => void save()}><Save size={16} /> {saving ? '저장 중…' : '현재 섹션 저장'}</Button>}>
    <div className="admin-settings-layout"><aside className="admin-settings-nav"><div className="admin-nav-title"><strong>설정 영역</strong><Badge tone={dirty.size ? 'warning' : 'success'}>{dirty.size ? `${dirty.size}개 미저장` : '모두 저장됨'}</Badge></div>{sections.map((section) => <button key={section.id} disabled={saving} className={active === section.id ? 'active' : ''} onClick={() => setActive(section.id)}><span>{section.icon}</span><div><strong>{section.label}</strong><small>{section.description}</small></div>{dirty.has(section.id) ? <i className="unsaved-dot" /> : <ChevronRight size={15} />}</button>)}</aside><fieldset disabled={saving} className="admin-setting-content">
      {loading ? <LoadingState label="서비스 설정을 불러오는 중…" /> : error ? <ErrorState message={error} /> : <>
        <div className="setting-section-heading"><span>{sections.find((section) => section.id === active)?.icon}</span><div><h2>{sections.find((section) => section.id === active)?.label}</h2><p>{sections.find((section) => section.id === active)?.description}</p></div></div>
        {active === 'branding' && <SettingCard title="서비스 정체성" description="로그인 후 워크스페이스와 브라우저 제목에 적용됩니다."><Field label="서비스 이름"><Input value={String(values.branding.product_name)} onChange={(event) => update('branding', 'product_name', event.target.value)} /></Field><Field label="로고 URL" hint="비워두면 Ptium 기본 마크를 사용합니다."><Input value={String(values.branding.logo_url)} onChange={(event) => update('branding', 'logo_url', event.target.value)} placeholder="https://…" /></Field><Field label="대표 색상" hint="워크스페이스 화면에 쓰입니다. 색을 직접 고르지 않은 사용자의 새 덱에도 이 색이 적용되며, 제품이 기본으로 갖고 온 색 그대로면 업로드한 템플릿의 강조색을 그대로 둡니다."><div className="color-input"><input type="color" value={String(values.branding.brand_color)} onChange={(event) => update('branding', 'brand_color', event.target.value)} /><Input value={String(values.branding.brand_color)} onChange={(event) => update('branding', 'brand_color', event.target.value)} /></div></Field></SettingCard>}
        {active === 'ai' && <><div className="configuration-status provider-check-row">
          <span><Bot size={15} /></span>
          <div><strong>제공자 응답 확인</strong>
            <p>{checking ? '물어보는 중…' : providerWord(checked)}</p></div>
          <Button variant="secondary" disabled={checking} onClick={() => void runProviderCheck()}>
            <Radio size={15} /> 지금 확인</Button>
        </div>
        <div className="configuration-status"><span><Bot size={15} /></span><div><strong>현재 적용된 생성 엔진</strong><p>{engineDescription}{dirty.has('ai') && ' 저장하지 않은 변경은 아직 적용되지 않았습니다.'}</p></div><Badge tone={usingBuiltInGenerator ? 'info' : 'success'}>{engineBadge}</Badge></div><SettingCard title="모델 연결" description="OpenAI 또는 OpenAI 호환 Chat Completions API를 연결합니다."><div className="form-grid two"><Field label="프로바이더"><Select value={String(values.ai.provider)} onChange={(event) => update('ai', 'provider', event.target.value)}><option value="fallback">내장 오프라인 생성기</option><option value="openai">OpenAI</option><option value="openai-compatible">OpenAI Compatible</option></Select></Field><Field label="모델"><Input value={String(values.ai.model)} onChange={(event) => update('ai', 'model', event.target.value)} /></Field></div><Field label="API Base URL"><Input value={String(values.ai.base_url)} onChange={(event) => update('ai', 'base_url', event.target.value)} /></Field>{secretField('API 키', 'api_key', `${externalKeyConfigured ? '저장된 키가 있습니다. ' : ''}AES-GCM으로 암호화되며 저장된 값은 다시 노출되지 않습니다.`)}{unreadableSecrets['ai.api_key'] && <div className="security-banner warning"><CircleAlert size={20} /><div><strong>저장된 API 키를 복호화할 수 없습니다</strong><p>암호화 키(KEY_ENCRYPTION_SECRET 또는 DATABASE_URL)가 바뀌었습니다. 위 입력란에 키를 다시 입력해 저장하세요.</p></div></div>}</SettingCard><SettingCard title="응답 제어" description="자체 호스팅 모델의 실제 동작에 맞춥니다. 추론(thinking) 모델은 사고 과정만 반환하고 본문을 비우는 경우가 있어 기본값은 사고를 끄도록 요청합니다."><div className="form-grid two"><Field label="추론 모드" hint="자동: 사고를 끄도록 요청하고, 서버가 거부하면 그대로 재시도합니다."><Select value={String(values.ai.reasoning)} onChange={(event) => update('ai', 'reasoning', event.target.value)}><option value="auto">자동</option><option value="off">항상 사고 끄기</option><option value="on">모델 기본값 사용</option></Select></Field><Field label="최대 출력 토큰" hint="덱 원문은 길어서 500~32000 범위를 권장합니다."><Input type="number" min="500" max="32000" value={Number(values.ai.max_output_tokens)} onChange={(event) => update('ai', 'max_output_tokens', Number(event.target.value))} /></Field></div><Field label="응답 제한 시간(초)" hint="느린 자체 호스팅 모델은 넉넉하게 두세요. 10~3600."><Input type="number" min="10" max="3600" value={Number(values.ai.timeout_seconds)} onChange={(event) => update('ai', 'timeout_seconds', Number(event.target.value))} /></Field></SettingCard></>}
        {active === 'oidc' && <><div className="security-banner"><LockKeyhole size={20} /><div><strong>표준 OIDC Discovery · PKCE</strong><p>공개 SPA 클라이언트는 Client Secret 없이 연결합니다. Keycloak에서 이 클라이언트를 <b>Confidential</b>로 두었다면 아래에 Secret을 저장하세요 — 인가 코드 교환을 브라우저 대신 서버가 수행합니다. 같은 항목의 환경변수(<code>OIDC_CLIENT_SECRET</code> 등)가 있으면 환경변수가 계속 우선하며, 그 외 저장값은 서비스 재시작 후 적용됩니다.</p></div></div><SettingCard title="Keycloak · OIDC 연결" description="Issuer URL을 비우면 OIDC가 비활성화됩니다."><Field label="Issuer URL" hint="예: https://keycloak.example.com/realms/ptium"><Input value={String(values.oidc.issuer_url)} onChange={(event) => update('oidc', 'issuer_url', event.target.value)} placeholder="https://…/realms/…" /></Field><Field label="Client ID"><Input value={String(values.oidc.client_id)} onChange={(event) => update('oidc', 'client_id', event.target.value)} placeholder="ptium-web" /></Field>{secretField('Client Secret', 'client_secret', `${oidcSecretConfigured ? '저장된 Secret이 있습니다. ' : ''}Confidential 클라이언트에만 필요합니다. AES-GCM으로 암호화되며 저장된 값은 다시 노출되지 않습니다. 지우려면 공백 한 칸을 저장하세요.`)}{unreadableSecrets['auth.oidc.client_secret'] && <div className="security-banner warning"><CircleAlert size={20} /><div><strong>저장된 Client Secret을 복호화할 수 없습니다</strong><p>암호화 키(KEY_ENCRYPTION_SECRET 또는 DATABASE_URL)가 바뀌었습니다. 위 입력란에 Secret을 다시 입력해 저장하세요.</p></div></div>}<Field label="관리자 역할" hint="쉼표로 구분합니다. realm_access.roles와 roles claim을 확인합니다."><Input value={asList(values.oidc.admin_roles).join(', ')} onChange={(event) => update('oidc', 'admin_roles', splitList(event.target.value))} /></Field></SettingCard></>}
        {active === 'generation' && <SettingCard title="생성 기본값 · 제한" description="새 프레젠테이션과 MCP 생성 요청에 즉시 적용됩니다."><div className="form-grid two"><Field label="기본 슬라이드"><Input type="number" min="1" max="50" value={Number(values.generation.default_slide_count)} onChange={(event) => update('generation', 'default_slide_count', Number(event.target.value))} /></Field><Field label="최대 슬라이드"><Input type="number" min="1" max="50" value={Number(values.generation.max_slides)} onChange={(event) => update('generation', 'max_slides', Number(event.target.value))} /></Field></div><div className="form-grid two"><Field label="기본 언어"><Select value={String(values.generation.default_lang)} onChange={(event) => update('generation', 'default_lang', event.target.value)}><option value="ko">한국어</option><option value="en">English</option><option value="ja">日本語</option><option value="zh">中文</option></Select></Field><Field label="기본 테마"><Select value={String(values.generation.default_theme)} onChange={(event) => update('generation', 'default_theme', event.target.value)}><option value="aurora">Aurora</option><option value="paper">Editorial</option><option value="mint">Fresh</option><option value="graphite">Graphite</option></Select></Field></div><div className="form-grid two"><Field label="기본 발표 톤"><Select value={String(values.generation.default_tone)} onChange={(event) => update('generation', 'default_tone', event.target.value)}><option value="professional">전문적</option><option value="persuasive">설득력 있는</option><option value="friendly">친근한</option><option value="inspiring">영감을 주는</option><option value="academic">학술적인</option></Select></Field><Field label="기본 청중"><Input value={String(values.generation.default_audience)} onChange={(event) => update('generation', 'default_audience', event.target.value)} /></Field></div><div className="form-grid two"><Field label="생성 후 자동 수정" hint="템플릿에 맞지 않는 슬라이드를 측정해 모델에게 다시 쓰게 합니다. 0이면 끕니다."><Input type="number" min="0" max="10" value={Number(values.generation.repair_passes ?? 3)} onChange={(event) => update('generation', 'repair_passes', Number(event.target.value))} /></Field><Field label="서사 계획 단계" hint="슬라이드를 쓰기 전에 덱의 흐름을 먼저 설계합니다."><Select value={String(values.generation.outline_pass ?? true)} onChange={(event) => update('generation', 'outline_pass', event.target.value === 'true')}><option value="true">사용</option><option value="false">사용 안 함</option></Select></Field></div></SettingCard>}
        {active === 'security' && <><div className="security-banner"><ShieldCheck size={20} /><div><strong>시크릿은 암호화되어 저장됩니다</strong><p>API 키 원문은 생성·회전 직후 한 번만 표시됩니다.</p></div></div><SettingCard title="API 키 회전" description="이전 키와 새 키가 함께 유효한 기본 유예 기간입니다."><Field label="회전 유예 기간" hint="Go duration 형식: 30m, 24h, 168h"><Input value={String(values.security.api_key_grace)} onChange={(event) => update('security', 'api_key_grace', event.target.value)} /></Field></SettingCard><SettingCard title="브라우저 Origin" description="동일 출처 외에 허용할 Origin입니다. 변경 후 서비스 재시작이 필요합니다."><Field label="추가 허용 Origin" hint="쉼표로 구분하며 경로 없이 https://host 형식으로 입력합니다."><Input value={asList(values.security.cors_origins).join(', ')} onChange={(event) => update('security', 'cors_origins', splitList(event.target.value))} placeholder="https://slides.example.com" /></Field></SettingCard></>}
        <div className="settings-save-bar"><span>{dirty.has(active) ? <><CircleAlert size={15} /> 저장하지 않은 변경사항이 있습니다.</> : <><Check size={15} /> 저장된 설정입니다.</>}</span><Button disabled={saving || !dirty.has(active)} onClick={() => void save()}><Save size={15} /> 저장</Button></div>
      </>}
    </fieldset></div>
  </AppShell>
}

function mergeSettings(current: typeof defaults, incoming: Record<string, unknown>) {
  const next = structuredClone(current)
  for (const section of Object.keys(current) as SectionKey[]) {
    if (incoming[section] && typeof incoming[section] === 'object') Object.assign(next[section], incoming[section])
  }
  return next
}

function asList(value: SettingValue): string[] {
  if (Array.isArray(value)) return value.map(String)
  return splitList(String(value || ''))
}

function splitList(value: string): string[] {
  return value.split(',').map((item) => item.trim()).filter(Boolean)
}

function validateSettings(section: SectionKey, values: Values): string {
  if (section === 'branding') {
    if (!String(values.product_name || '').trim()) return '서비스 이름을 입력해 주세요.'
    if (!/^#[0-9a-f]{6}$/i.test(String(values.brand_color))) return '대표 색상은 #RRGGBB 형식이어야 합니다.'
    if (values.logo_url && !validHTTPURL(String(values.logo_url))) return '로고 URL은 올바른 HTTP(S) 주소여야 합니다.'
  }
  if (section === 'ai') {
    if (!['fallback', 'openai', 'openai-compatible'].includes(String(values.provider))) return '지원되는 AI 프로바이더를 선택해 주세요.'
    if (!validHTTPURL(String(values.base_url))) return 'API Base URL은 올바른 HTTP(S) 주소여야 합니다.'
    if (!String(values.model || '').trim()) return '모델 이름을 입력해 주세요.'
    const tokens = Number(values.max_output_tokens)
    if (!Number.isInteger(tokens) || tokens < 500 || tokens > 32000) return '최대 출력 토큰은 500~32000의 정수여야 합니다.'
    const seconds = Number(values.timeout_seconds)
    if (!Number.isInteger(seconds) || seconds < 10 || seconds > 3600) return '응답 제한 시간은 10~3600초여야 합니다.'
  }
  if (section === 'oidc' && String(values.client_secret || '').trim() && !String(values.client_id || '').trim()) {
    return 'Client Secret을 저장하려면 Client ID가 필요합니다.'
  }
  if (section === 'oidc' && values.issuer_url) {
    if (!validHTTPURL(String(values.issuer_url), true)) return 'OIDC Issuer는 HTTPS 주소여야 합니다.'
    if (!String(values.client_id || '').trim()) return 'OIDC Client ID를 입력해 주세요.'
    if (asList(values.admin_roles).length === 0) return '관리자 역할을 한 개 이상 입력해 주세요.'
  }
  if (section === 'generation') {
    const defaults = Number(values.default_slide_count)
    const maximum = Number(values.max_slides)
    if (!Number.isInteger(defaults) || !Number.isInteger(maximum) || defaults < 1 || maximum > 50 || defaults > maximum) return '슬라이드 수는 1~50의 정수이며 기본값이 최대값보다 클 수 없습니다.'
  }
  if (section === 'security') {
    const duration = String(values.api_key_grace || '')
    const hours = goDurationHours(duration)
    if (hours === null || hours < 0 || hours > 720) return 'API 키 유예 기간은 0~720시간의 Go duration 형식이어야 합니다.'
    if (asList(values.cors_origins).some((origin) => !validHTTPURL(origin))) return '허용 Origin은 쉼표로 구분한 HTTP(S) 주소여야 합니다.'
  }
  return ''
}

function validHTTPURL(value: string, httpsOnly = false) {
  try {
    const parsed = new URL(value)
    return parsed.protocol === 'https:' || (!httpsOnly && parsed.protocol === 'http:')
  } catch { return false }
}

function goDurationHours(value: string): number | null {
  if (value === '0') return 0
  if (!/^(?:\d+(?:\.\d+)?(?:ms|s|m|h))+$/.test(value)) return null
  let milliseconds = 0
  for (const match of value.matchAll(/(\d+(?:\.\d+)?)(ms|s|m|h)/g)) {
    const factor = match[2] === 'h' ? 3_600_000 : match[2] === 'm' ? 60_000 : match[2] === 's' ? 1_000 : 1
    milliseconds += Number(match[1]) * factor
  }
  return milliseconds / 3_600_000
}

function SettingCard({ title, description, children }: { title: string; description?: string; children: ReactNode }) {
  return <section className="admin-setting-card"><div className="admin-setting-card-head"><h3>{title}</h3>{description && <p>{description}</p>}</div><div className="admin-setting-card-body">{children}</div></section>
}
