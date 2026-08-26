import { useCallback, useEffect, useState } from 'react'
import { Check, Clipboard, Clock3, Code2, Eye, EyeOff, KeyRound, Plus, RefreshCw, ShieldCheck, SlidersHorizontal, Trash2 } from 'lucide-react'
import { api } from '../api/client'
import { AppShell } from '../components/AppShell'
import { Badge, Button, EmptyState, ErrorState, Field, Input, LoadingState, Modal, Select } from '../components/UI'
import { useToast } from '../components/Toast'
import type { ApiKey } from '../types'
import { copyText, displayError, formatDate, relativeDate } from '../utils'
import { Link } from '../router'

/** One permission the server says a key may carry. */
interface ScopeChoice { id: string; admin?: boolean; grants?: string }

// What each scope means, in the words of the product rather than of the API. A
// scope with no line here still appears — named as the server names it — which
// is the point: the screen shows what the server offers, not what this file
// happens to know about.
const scopeLabels: Record<string, string> = {
  'presentations:read': '프레젠테이션 조회 · 내보내기',
  'presentations:write': '프레젠테이션 생성 · 편집',
  'templates:read': '템플릿 조회 · 미리보기',
  'templates:write': '템플릿 업로드 · 삭제',
  'profile:read': '프로필 읽기',
  'profile:write': '프로필 편집',
  'api_keys:manage': 'API 키 관리',
  'mcp:use': 'MCP 연결 (도구별 읽기/쓰기 권한도 필요)',
  'admin:settings': '관리자 · 설정 읽기/변경',
  'admin:users': '관리자 · 사용자와 역할',
  'admin:errors': '관리자 · 오류 센터',
}

/** The permission list, drawn from what the server offers. */
export function ScopeChooser({ catalogue, chosen, onToggle }: {
  catalogue: ScopeChoice[]
  chosen: string[]
  onToggle: (scope: string) => void
}) {
  if (catalogue.length === 0) return <p className="muted-note">권한 목록을 불러오지 못했습니다.</p>
  return <div className="scope-options">
    {catalogue.map((scope) => <label key={scope.id}>
      <input type="checkbox" checked={chosen.includes(scope.id)} onChange={() => onToggle(scope.id)} />
      <span>
        <i>{chosen.includes(scope.id) && <Check size={12} />}</i>
        {scopeLabels[scope.id] || scope.id}
        {scope.admin && <Badge tone="warning">관리자</Badge>}
        <small>{scope.id}</small>
      </span>
    </label>)}
  </div>
}

/**
 * A revoked key is over: it cannot be used, rotated or changed.
 *
 * A key rotated away is not revoked while its grace lasts — it still works, so
 * it still belongs on the list.
 */
export function revoked(key: Pick<ApiKey, 'status'>) { return key.status === 'revoked' }

export function ApiKeysPage() {
  const [keys, setKeys] = useState<ApiKey[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [createOpen, setCreateOpen] = useState(false)
  const [name, setName] = useState('')
  const [expiry, setExpiry] = useState('90')
  const [scopes, setScopes] = useState<string[]>(['presentations:read', 'presentations:write', 'mcp:use'])
  const [working, setWorking] = useState(false)
  const [secret, setSecret] = useState('')
  const [secretName, setSecretName] = useState('')
  const [showSecret, setShowSecret] = useState(false)
  const [revokeTarget, setRevokeTarget] = useState<ApiKey | null>(null)
  // What this deployment may put on a key, asked of the server rather than
  // listed here: a scope the server adds has to appear on the screen that
  // grants it, and the list this page kept had drifted from it.
  const [catalogue, setCatalogue] = useState<ScopeChoice[]>([])
  const [scopeTarget, setScopeTarget] = useState<ApiKey | null>(null)
  const [editScopes, setEditScopes] = useState<string[]>([])
  const [showRevoked, setShowRevoked] = useState(false)
  const { showToast } = useToast()
  const load = useCallback(async () => {
    setLoading(true); setError('')
    try {
      const [issued, offered] = await Promise.all([api.apiKeys(), api.apiKeyScopes().catch(() => [])])
      setKeys(issued)
      if (offered.length) setCatalogue(offered)
    } catch (err) { setError(displayError(err)) } finally { setLoading(false) }
  }, [])
  useEffect(() => { void load() }, [load])
  const create = async () => {
    setWorking(true)
    try { const result = await api.createApiKey({ name, scopes, expiresInDays: expiry === 'never' ? undefined : Number(expiry) }); setSecret(result.key || result.secret || ''); setSecretName(name); setCreateOpen(false); setName(''); await load() } catch (err) { showToast(displayError(err), 'error') } finally { setWorking(false) }
  }
  const rotate = async (key: ApiKey) => {
    setWorking(true)
    try { const result = await api.rotateApiKey(key.id); setSecret(result.key || result.secret || ''); setSecretName(key.name); await load(); showToast('키를 회전했습니다. 이전 키는 유예 기간 후 만료됩니다.') } catch (err) { showToast(displayError(err), 'error') } finally { setWorking(false) }
  }
  const revoke = async () => {
    if (!revokeTarget) return
    setWorking(true)
    try { await api.revokeApiKey(revokeTarget.id); setRevokeTarget(null); await load(); showToast('API 키를 폐기했습니다.') } catch (err) { showToast(displayError(err), 'error') } finally { setWorking(false) }
  }
  const toggleScope = (scope: string) => setScopes((current) => current.includes(scope) ? current.filter((item) => item !== scope) : [...current, scope])
  const saveScopes = async () => {
    if (!scopeTarget) return
    setWorking(true)
    try {
      await api.updateApiKeyScopes(scopeTarget.id, editScopes)
      setScopeTarget(null); await load(); showToast('키의 권한을 바꿨습니다. 키 값은 그대로입니다.')
    } catch (err) { showToast(displayError(err), 'error') } finally { setWorking(false) }
  }
  // A revoked key is over: it cannot be used, rotated or changed, and the list
  // is about the keys somebody is running. A key rotated away is not revoked
  // while its grace lasts — it still works, so it still shows.
  const retired = keys.filter(revoked)
  const running = showRevoked ? keys : keys.filter((key) => !revoked(key))
  return <AppShell title="API 키" eyebrow="DEVELOPER" actions={<Button onClick={() => setCreateOpen(true)}><Plus size={16} /> 새 API 키</Button>}>
    <div className="api-intro"><span><Code2 size={22} /></span><div><h2>Ptium API와 MCP를 연결하세요</h2><p>프레젠테이션 생성과 관리를 자동화하고, AI 도구에서 MCP 서버를 사용하세요.</p><div><code>POST /api/v1/presentations</code><code>{window.location.origin}/mcp</code></div></div><Link to="/docs">API 문서 보기</Link></div>
    <section className="section-block api-keys-section"><div className="section-heading"><div><h2>발급된 키</h2><p>키는 생성 직후 한 번만 전체 값을 확인할 수 있습니다.</p></div><Badge tone="info"><ShieldCheck size={13} /> 수동 회전 · 유예 지원</Badge></div>
      {loading ? <LoadingState label="API 키를 불러오는 중…" /> : error ? <ErrorState message={error} onRetry={() => void load()} /> : running.length === 0 ? <EmptyState icon={<KeyRound size={25} />} title="아직 발급한 API 키가 없습니다" description="자동화나 외부 도구 연결을 위해 첫 API 키를 만들어 보세요." action={<Button onClick={() => setCreateOpen(true)}><Plus size={15} /> 키 만들기</Button>} /> : <div className="table-wrap"><table className="data-table api-key-table"><thead><tr><th>이름</th><th>키</th><th>권한</th><th>마지막 사용</th><th>만료</th><th><span className="sr-only">작업</span></th></tr></thead><tbody>{running.map((key) => <tr key={key.id}><td><div className="key-name"><span><KeyRound size={16} /></span><div><strong>{key.name}</strong><small>{formatDate(key.createdAt)} 생성 · {key.status === 'active' ? '활성' : key.status === 'rotating' ? '회전 유예 중' : key.status === 'expired' ? '만료됨' : '폐기됨'}</small></div></div></td><td><code>{key.prefix}••••••••••••</code></td><td><div className="scope-badges">{key.scopes.slice(0,2).map((scope) => <Badge key={scope}>{scope.replace('presentations:', '')}</Badge>)}{key.scopes.length > 2 && <Badge>+{key.scopes.length - 2}</Badge>}</div></td><td>{key.lastUsedAt ? relativeDate(key.lastUsedAt) : '사용 전'}</td><td>{key.expiresAt ? formatDate(key.expiresAt, { year: 'numeric', month: 'short', day: 'numeric' }) : '만료 없음'}</td><td><div className="row-actions"><button title="권한 수정" onClick={() => { setScopeTarget(key); setEditScopes(key.scopes) }} disabled={working || key.status === 'revoked' || key.status === 'expired'}><SlidersHorizontal size={15} /></button><button title="키 회전" onClick={() => void rotate(key)} disabled={working || key.status !== 'active'}><RefreshCw size={15} /></button><button title="키 폐기" className="danger" onClick={() => setRevokeTarget(key)} disabled={key.status === 'revoked' || key.status === 'expired'}><Trash2 size={15} /></button></div></td></tr>)}</tbody></table></div>}
      {retired.length > 0 && <p className="muted-note retired-keys">
        폐기된 키 {retired.length}개는 목록에서 감춰져 있습니다.{' '}
        <button type="button" className="link-button" onClick={() => setShowRevoked((value) => !value)}>
          {showRevoked ? '감추기' : '보기'}
        </button>
      </p>}
    </section>
    <section className="security-note"><ShieldCheck size={20} /><div><strong>안전한 키 사용 가이드</strong><p>API 키를 소스 코드나 브라우저에 포함하지 마세요. 서버 환경 변수나 시크릿 관리 도구에 저장하고, 정기적으로 회전하는 것을 권장합니다.</p></div></section>
    <Modal open={createOpen} onClose={() => setCreateOpen(false)} title="새 API 키 만들기" description="키의 용도와 필요한 권한만 선택하세요." footer={<><Button variant="secondary" onClick={() => setCreateOpen(false)}>취소</Button><Button disabled={!name.trim() || scopes.length === 0 || working} onClick={() => void create()}><KeyRound size={15} /> {working ? '생성 중…' : '키 생성'}</Button></>}><div className="modal-form"><Field label="키 이름"><Input autoFocus maxLength={100} value={name} onChange={(event) => setName(event.target.value)} placeholder="예: 콘텐츠 자동화 서버" /></Field><Field label="만료"><Select value={expiry} onChange={(event) => setExpiry(event.target.value)}><option value="30">30일</option><option value="90">90일</option><option value="365">1년</option><option value="never">만료 없음</option></Select></Field><Field label="권한 범위"><ScopeChooser catalogue={catalogue} chosen={scopes} onToggle={toggleScope} /></Field></div></Modal>
    <Modal open={Boolean(secret)} onClose={() => { setSecret(''); setShowSecret(false) }} title="API 키가 준비됐습니다" description={`${secretName} 키를 지금 복사해 안전한 곳에 보관하세요.`} footer={<Button onClick={async () => { await copyText(secret); showToast('클립보드에 복사했습니다.') }}><Clipboard size={15} /> 키 복사</Button>}><div className="secret-reveal"><div><code>{showSecret ? secret : '•'.repeat(Math.min(44, Math.max(24, secret.length)))}</code><button onClick={() => setShowSecret((value) => !value)} aria-label={showSecret ? '키 숨기기' : '키 보기'}>{showSecret ? <EyeOff size={17} /> : <Eye size={17} />}</button></div><p><ShieldCheck size={14} /> 이 키는 다시 표시되지 않습니다.</p></div></Modal>
    <Modal open={Boolean(scopeTarget)} onClose={() => setScopeTarget(null)} title="이 키의 권한" description={`${scopeTarget?.name || ''} 키가 무엇을 할 수 있는지 바꿉니다. 키 값은 그대로이므로 쓰는 쪽을 고칠 필요가 없습니다.`} footer={<><Button variant="secondary" onClick={() => setScopeTarget(null)}>취소</Button><Button disabled={working || editScopes.length === 0} onClick={() => void saveScopes()}>{working ? '저장 중…' : '권한 저장'}</Button></>}>
      <div className="modal-form">
        <ScopeChooser catalogue={catalogue} chosen={editScopes} onToggle={(scope) => setEditScopes((current) => current.includes(scope) ? current.filter((item) => item !== scope) : [...current, scope])} />
      </div>
    </Modal>
    <Modal open={Boolean(revokeTarget)} onClose={() => setRevokeTarget(null)} title="API 키를 폐기할까요?" description="이 키를 사용하는 모든 연결이 즉시 중단됩니다." footer={<><Button variant="secondary" onClick={() => setRevokeTarget(null)}>취소</Button><Button variant="danger" disabled={working} onClick={() => void revoke()}><Trash2 size={15} /> 폐기</Button></>}><div className="delete-preview"><strong>{revokeTarget?.name}</strong><code>{revokeTarget?.prefix}••••••••</code></div></Modal>
  </AppShell>
}
