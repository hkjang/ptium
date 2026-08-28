import { useCallback, useEffect, useState } from 'react'
import { History, MoreHorizontal, Search, Shield, UserCheck, UserCog, UserMinus, Users } from 'lucide-react'
import { api } from '../api/client'
import { AppShell } from '../components/AppShell'
import { Badge, Button, EmptyState, ErrorState, Input, Modal, Select } from '../components/UI'
import { navigate } from '../router'
import { useToast } from '../components/Toast'
import type { AdminUser, Role } from '../types'
import { displayError, formatDate, relativeDate } from '../utils'

export function AdminUsersPage() {
  const [users, setUsers] = useState<AdminUser[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [query, setQuery] = useState('')
  const [roleFilter, setRoleFilter] = useState('all')
  const [statusFilter, setStatusFilter] = useState('all')
  const [selected, setSelected] = useState<AdminUser | null>(null)
  const [working, setWorking] = useState(false)
  const { showToast } = useToast()
  const [matching, setMatching] = useState(0)
  const [page, setPage] = useState(0)
  const [counts, setCounts] = useState({ total: 0, admins: 0, active: 0, suspended: 0 })
  const perPage = 50
  /**
   * A page of accounts, narrowed by the server.
   *
   * This screen used to ask for every account and sift them in the browser. At a
   * thousand accounts that was twelve requests and a thousand rows in the page
   * before anybody had typed anything, and it grew with every account a
   * deployment added. The four cards above the list count every account, which
   * is what they always meant; the table shows the page being looked at.
   */
  const load = useCallback(async () => {
    setLoading(true); setError('')
    try {
      const [found, counted] = await Promise.all([
        api.adminUserPage({
          q: query,
          role: roleFilter === 'all' ? '' : roleFilter,
          status: statusFilter === 'all' ? '' : statusFilter,
          limit: perPage,
          offset: page * perPage,
        }),
        api.adminUserCounts(),
      ])
      setUsers(found.items)
      setMatching(found.total)
      setCounts(counted)
    } catch (err) { setError(displayError(err)) } finally { setLoading(false) }
  }, [query, roleFilter, statusFilter, page])
  // Typing narrows the list; each keystroke is not a request.
  useEffect(() => {
    const timer = setTimeout(() => { void load() }, query ? 250 : 0)
    return () => clearTimeout(timer)
  }, [load, query])
  // A narrowed list starts at its own beginning rather than on page four of the
  // list somebody was looking at before.
  useEffect(() => { setPage(0) }, [query, roleFilter, statusFilter])
  const filtered = users
  const updateUser = async (user: AdminUser, changes: Record<string, unknown>) => {
    setWorking(true)
    try { const updated = await api.updateAdminUser(user.id, changes); setUsers((current) => current.map((item) => item.id === user.id ? { ...item, ...updated, presentationsCount: item.presentationsCount } : item)); setSelected((current) => current?.id === user.id ? { ...current, ...updated, presentationsCount: current.presentationsCount } : current); showToast('사용자 설정을 변경했습니다.') } catch (err) { showToast(displayError(err), 'error') } finally { setWorking(false) }
  }

  return <AppShell title="사용자 관리" eyebrow="ACCESS CONTROL">
    <section className="user-stat-row"><article><span><Users size={18} /></span><div><strong>{counts.total}</strong><small>전체 사용자</small></div></article><article><span><UserCog size={18} /></span><div><strong>{counts.admins}</strong><small>관리자</small></div></article><article><span><UserCheck size={18} /></span><div><strong>{counts.active}</strong><small>활성 사용자</small></div></article><article><span><UserMinus size={18} /></span><div><strong>{counts.suspended}</strong><small>정지됨</small></div></article></section>
    <section className="admin-panel users-panel"><div className="users-toolbar"><div className="search-box"><Search size={17} /><Input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="이름 또는 이메일 검색" /></div><div><Select value={roleFilter} onChange={(event) => setRoleFilter(event.target.value)}><option value="all">모든 역할</option><option value="admin">관리자</option><option value="user">일반 사용자</option></Select><Select value={statusFilter} onChange={(event) => setStatusFilter(event.target.value)}><option value="all">모든 상태</option><option value="active">활성</option><option value="suspended">정지됨</option></Select></div></div>
      {loading ? <div className="table-skeleton">사용자 목록을 불러오는 중…</div> : error ? <ErrorState message={error} onRetry={() => void load()} /> : filtered.length === 0 ? <EmptyState title="조건에 맞는 사용자가 없습니다" description="검색어나 필터를 변경해 보세요." /> : <div className="table-wrap"><table className="data-table users-table"><thead><tr><th>사용자</th><th>역할</th><th>상태</th><th>프레젠테이션</th><th>마지막 로그인</th><th>가입일</th><th><span className="sr-only">작업</span></th></tr></thead><tbody>{filtered.map((user) => <tr key={user.id}><td><button className="user-cell" onClick={() => setSelected(user)}><span className="avatar">{(user.name || user.email).slice(0,2).toUpperCase()}</span><div><strong>{user.name || user.email.split('@')[0]}</strong><small>{user.email}</small></div></button></td><td><Badge tone={user.role === 'admin' ? 'violet' : 'neutral'}>{user.role === 'admin' ? <><Shield size={12} /> 관리자</> : '사용자'}</Badge></td><td><Badge tone={user.status === 'active' ? 'success' : user.status === 'suspended' ? 'danger' : 'warning'}><i className="badge-dot" />{user.status === 'active' ? '활성' : user.status === 'suspended' ? '정지됨' : '초대됨'}</Badge></td><td>{user.presentationsCount || 0}</td><td>{user.lastSeenAt ? relativeDate(user.lastSeenAt) : '로그인 전'}</td><td>{formatDate(user.createdAt, { year:'numeric', month:'short', day:'numeric' })}</td><td><button className="icon-button small" onClick={() => setSelected(user)} aria-label="사용자 관리"><MoreHorizontal size={17} /></button></td></tr>)}</tbody></table></div>}
      <div className="table-footer"><span>총 {matching}명 중 {matching === 0 ? 0 : page * perPage + 1}–{Math.min((page + 1) * perPage, matching)}명 표시</span>
        {matching > perPage && <span className="table-pager">
          <Button variant="secondary" disabled={page === 0 || loading} onClick={() => setPage((at) => Math.max(0, at - 1))}>이전</Button>
          <Button variant="secondary" disabled={(page + 1) * perPage >= matching || loading} onClick={() => setPage((at) => at + 1)}>다음</Button>
        </span>}</div>
    </section>
    <Modal open={Boolean(selected)} onClose={() => setSelected(null)} title="사용자 액세스 관리" description="역할과 계정 상태 변경은 즉시 적용됩니다." footer={<><Button variant="secondary" onClick={() => selected && navigate(`/admin/audit?actor=${encodeURIComponent(selected.email)}`)}>
        <History size={15} /> 이 사용자의 기록</Button>
      <Button variant="secondary" onClick={() => setSelected(null)}>닫기</Button></>}><div className="user-detail-card"><span className="avatar xlarge">{(selected?.name || selected?.email || 'U').slice(0,2).toUpperCase()}</span><div><strong>{selected?.name}</strong><span>{selected?.email}</span><small>{formatDate(selected?.createdAt)} 등록</small></div></div><div className="user-access-fields"><label><span><b>역할</b><small>관리자는 모든 서비스 설정에 접근할 수 있습니다.</small></span><Select disabled={working} value={selected?.role || 'user'} onChange={(event) => selected && void updateUser(selected, { isAdmin: event.target.value === 'admin' })}><option value="user">일반 사용자</option><option value="admin">관리자</option></Select></label><label><span><b>계정 상태</b><small>정지된 사용자는 로그인과 API 사용이 차단됩니다.</small></span><Select disabled={working} value={selected?.status || 'active'} onChange={(event) => selected && void updateUser(selected, { disabled: event.target.value === 'suspended' })}><option value="active">활성</option><option value="suspended">정지됨</option></Select></label></div></Modal>
  </AppShell>
}
