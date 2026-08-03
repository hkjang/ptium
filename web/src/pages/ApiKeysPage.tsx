import { useCallback, useEffect, useState } from 'react'
import { Check, Clipboard, Clock3, Code2, Eye, EyeOff, KeyRound, Plus, RefreshCw, ShieldCheck, Trash2 } from 'lucide-react'
import { api } from '../api/client'
import { AppShell } from '../components/AppShell'
import { Badge, Button, EmptyState, ErrorState, Field, Input, LoadingState, Modal, Select } from '../components/UI'
import { useToast } from '../components/Toast'
import type { ApiKey } from '../types'
import { copyText, displayError, formatDate, relativeDate } from '../utils'
import { Link } from '../router'

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
  const { showToast } = useToast()
  const load = useCallback(async () => { setLoading(true); setError(''); try { setKeys(await api.apiKeys()) } catch (err) { setError(displayError(err)) } finally { setLoading(false) } }, [])
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
  return <AppShell title="API 키" eyebrow="DEVELOPER" actions={<Button onClick={() => setCreateOpen(true)}><Plus size={16} /> 새 API 키</Button>}>
    <div className="api-intro"><span><Code2 size={22} /></span><div><h2>Ptium API와 MCP를 연결하세요</h2><p>프레젠테이션 생성과 관리를 자동화하고, AI 도구에서 MCP 서버를 사용하세요.</p><div><code>POST /api/v1/presentations</code><code>{window.location.origin}/mcp</code></div></div><Link to="/docs">API 문서 보기</Link></div>
    <section className="section-block api-keys-section"><div className="section-heading"><div><h2>발급된 키</h2><p>키는 생성 직후 한 번만 전체 값을 확인할 수 있습니다.</p></div><Badge tone="info"><ShieldCheck size={13} /> 수동 회전 · 유예 지원</Badge></div>
      {loading ? <LoadingState label="API 키를 불러오는 중…" /> : error ? <ErrorState message={error} onRetry={() => void load()} /> : keys.length === 0 ? <EmptyState icon={<KeyRound size={25} />} title="아직 발급한 API 키가 없습니다" description="자동화나 외부 도구 연결을 위해 첫 API 키를 만들어 보세요." action={<Button onClick={() => setCreateOpen(true)}><Plus size={15} /> 키 만들기</Button>} /> : <div className="table-wrap"><table className="data-table api-key-table"><thead><tr><th>이름</th><th>키</th><th>권한</th><th>마지막 사용</th><th>만료</th><th><span className="sr-only">작업</span></th></tr></thead><tbody>{keys.map((key) => <tr key={key.id}><td><div className="key-name"><span><KeyRound size={16} /></span><div><strong>{key.name}</strong><small>{formatDate(key.createdAt)} 생성 · {key.status === 'active' ? '활성' : key.status === 'rotating' ? '회전 유예 중' : key.status === 'expired' ? '만료됨' : '폐기됨'}</small></div></div></td><td><code>{key.prefix}••••••••••••</code></td><td><div className="scope-badges">{key.scopes.slice(0,2).map((scope) => <Badge key={scope}>{scope.replace('presentations:', '')}</Badge>)}{key.scopes.length > 2 && <Badge>+{key.scopes.length - 2}</Badge>}</div></td><td>{key.lastUsedAt ? relativeDate(key.lastUsedAt) : '사용 전'}</td><td>{key.expiresAt ? formatDate(key.expiresAt, { year: 'numeric', month: 'short', day: 'numeric' }) : '만료 없음'}</td><td><div className="row-actions"><button title="키 회전" onClick={() => void rotate(key)} disabled={working || key.status !== 'active'}><RefreshCw size={15} /></button><button title="키 폐기" className="danger" onClick={() => setRevokeTarget(key)} disabled={key.status === 'revoked' || key.status === 'expired'}><Trash2 size={15} /></button></div></td></tr>)}</tbody></table></div>}
    </section>
    <section className="security-note"><ShieldCheck size={20} /><div><strong>안전한 키 사용 가이드</strong><p>API 키를 소스 코드나 브라우저에 포함하지 마세요. 서버 환경 변수나 시크릿 관리 도구에 저장하고, 정기적으로 회전하는 것을 권장합니다.</p></div></section>
    <Modal open={createOpen} onClose={() => setCreateOpen(false)} title="새 API 키 만들기" description="키의 용도와 필요한 권한만 선택하세요." footer={<><Button variant="secondary" onClick={() => setCreateOpen(false)}>취소</Button><Button disabled={!name.trim() || scopes.length === 0 || working} onClick={() => void create()}><KeyRound size={15} /> {working ? '생성 중…' : '키 생성'}</Button></>}><div className="modal-form"><Field label="키 이름"><Input autoFocus maxLength={100} value={name} onChange={(event) => setName(event.target.value)} placeholder="예: 콘텐츠 자동화 서버" /></Field><Field label="만료"><Select value={expiry} onChange={(event) => setExpiry(event.target.value)}><option value="30">30일</option><option value="90">90일</option><option value="365">1년</option><option value="never">만료 없음</option></Select></Field><Field label="권한 범위"><div className="scope-options">{[{id:'presentations:read',label:'프레젠테이션 조회 · 내보내기'},{id:'presentations:write',label:'프레젠테이션 생성 · 편집'},{id:'mcp:use',label:'MCP 연결 (도구별 읽기/쓰기 권한도 필요)'},{id:'profile:read',label:'프로필 읽기'},{id:'profile:write',label:'프로필 편집'},{id:'api_keys:manage',label:'API 키 관리'}].map((item) => <label key={item.id}><input type="checkbox" checked={scopes.includes(item.id)} onChange={() => toggleScope(item.id)} /><span><i>{scopes.includes(item.id) && <Check size={12} />}</i>{item.label}</span></label>)}</div></Field></div></Modal>
    <Modal open={Boolean(secret)} onClose={() => { setSecret(''); setShowSecret(false) }} title="API 키가 준비됐습니다" description={`${secretName} 키를 지금 복사해 안전한 곳에 보관하세요.`} footer={<Button onClick={async () => { await copyText(secret); showToast('클립보드에 복사했습니다.') }}><Clipboard size={15} /> 키 복사</Button>}><div className="secret-reveal"><div><code>{showSecret ? secret : '•'.repeat(Math.min(44, Math.max(24, secret.length)))}</code><button onClick={() => setShowSecret((value) => !value)} aria-label={showSecret ? '키 숨기기' : '키 보기'}>{showSecret ? <EyeOff size={17} /> : <Eye size={17} />}</button></div><p><ShieldCheck size={14} /> 이 키는 다시 표시되지 않습니다.</p></div></Modal>
    <Modal open={Boolean(revokeTarget)} onClose={() => setRevokeTarget(null)} title="API 키를 폐기할까요?" description="이 키를 사용하는 모든 연결이 즉시 중단됩니다." footer={<><Button variant="secondary" onClick={() => setRevokeTarget(null)}>취소</Button><Button variant="danger" disabled={working} onClick={() => void revoke()}><Trash2 size={15} /> 폐기</Button></>}><div className="delete-preview"><strong>{revokeTarget?.name}</strong><code>{revokeTarget?.prefix}••••••••</code></div></Modal>
  </AppShell>
}
