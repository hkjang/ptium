import { useCallback, useEffect, useMemo, useState } from 'react'
import { ChevronRight, Download, History, RefreshCw, Search, ShieldCheck, User } from 'lucide-react'
import { api } from '../api/client'
import { AppShell } from '../components/AppShell'
import { Badge, Button, EmptyState, ErrorState, Input, Select, useOverlayKeys } from '../components/UI'
import { useToast } from '../components/Toast'
import type { AuditEntry } from '../types'
import { displayError, formatDate, relativeDate } from '../utils'

/**
 * The trail, read the way an operator arrives at it.
 *
 * Everything in this server writes an audit record and nothing read one, so
 * "who turned the provider on", "who deleted that deck" and "what did this
 * person do" were questions with a table behind them and no door. The filters
 * are those three questions; the default is what happened most recently.
 */
const PAGE = 50

// The actions a deployment writes read as machine names. What an operator is
// looking for is the sentence, so each one is said in the language of the
// screen it belongs to; anything unnamed keeps its own name rather than being
// hidden behind a guess.
const actionLabels: Record<string, string> = {
  'settings.update': '설정 변경', 'settings.update_batch': '설정 일괄 변경',
  'user.admin_update': '사용자 권한·정지 변경', 'user.login': '로그인', 'user.password_change': '비밀번호 변경',
  'presentation.create': '덱 생성', 'presentation.create_and_generate': '덱 생성 + 생성 요청',
  'presentation.update': '덱 수정', 'presentation.source_update': '덱 소스 적용',
  'presentation.trash': '덱 휴지통', 'presentation.delete': '덱 삭제', 'presentation.restore': '덱 복구',
  'presentation.duplicate': '덱 복제', 'presentation.generate': '생성 요청',
  'presentation.slide_revise': '슬라이드 다시 쓰기', 'presentation.import': '덱 가져오기',
  'presentation.share': '공유 링크 발급', 'presentation.share_revoke': '공유 링크 회수',
  'api_key.create': 'API 키 발급', 'api_key.revoke': 'API 키 회수', 'api_key.rotate': 'API 키 회전',
  'template.create': '템플릿 등록', 'template.delete': '템플릿 삭제',
  'asset.upload': '이미지 업로드', 'asset.delete': '이미지 삭제',
}

const actionTone = (action: string) =>
  action.includes('delete') || action.includes('revoke') || action.includes('trash') ? 'danger'
    : action.startsWith('settings') || action.startsWith('user.admin') ? 'warning'
      : action.includes('create') || action.includes('generate') ? 'success' : 'neutral'

export function AdminAuditPage() {
  const [entries, setEntries] = useState<AuditEntry[]>([])
  const [actions, setActions] = useState<{ action: string; count: number }[]>([])
  const [total, setTotal] = useState(0)
  const [offset, setOffset] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  // Opened from a user's row, the address says whose trail this is.
  const [search, setSearch] = useState(() => new URLSearchParams(window.location.search).get('actor') || '')
  const [action, setAction] = useState('')
  const [days, setDays] = useState(7)
  const [selected, setSelected] = useState<AuditEntry | null>(null)
  // The drawer is a dialog like any other: Escape closes it, Tab stays in it,
  // and the row that opened it gets the keyboard back.
  const drawer = useOverlayKeys(Boolean(selected), () => setSelected(null))
  const { showToast } = useToast()

  const load = useCallback(async (nextOffset: number) => {
    setLoading(true); setError('')
    try {
      const page = await api.auditTrail({ search, action, days, limit: PAGE, offset: nextOffset })
      setEntries(page.entries); setTotal(page.total); setOffset(nextOffset)
    } catch (err) { setError(displayError(err)) } finally { setLoading(false) }
  }, [search, action, days])

  useEffect(() => { void load(0) }, [load])
  useEffect(() => { api.auditActions(days).then(setActions).catch(() => setActions([])) }, [days])

  // The file is what is on screen, filters and all — an auditor asked for the
  // trail, not for the first fifty rows of it. The browser is handed the URL
  // rather than the bytes: the session travels with it, and a hundred thousand
  // rows do not pass through this page's memory on the way to disk.
  const exportTrail = async () => {
    try {
      const file = await api.auditTrailCsv({ search, action, days })
      const url = URL.createObjectURL(file)
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = `ptium-audit-${new Date().toISOString().slice(0, 10)}.csv`
      anchor.click()
      URL.revokeObjectURL(url)
      showToast('감사 기록을 CSV로 내려받았습니다. 화면의 조건이 그대로 적용됩니다.')
    } catch (err) { showToast(displayError(err), 'error') }
  }

  const shown = useMemo(() => `${Math.min(offset + 1, total)}–${Math.min(offset + PAGE, total)} / ${total.toLocaleString('ko-KR')}`,
    [offset, total])
  const label = (value: string) => actionLabels[value] || value
  const actor = (entry: AuditEntry) => entry.actorEmail || entry.actorName || (entry.actorId ? entry.actorId.slice(0, 8) : '시스템')

  return <AppShell title="감사 기록" eyebrow="WHO DID WHAT"
    actions={<><Button variant="secondary" onClick={() => void exportTrail()}><Download size={15} /> CSV로 내려받기</Button>
      <Button variant="secondary" onClick={() => void load(offset)}><RefreshCw size={15} /> 새로고침</Button></>}>
    <section className="admin-panel">
      <div className="error-toolbar">
        <div className="filter-tabs">
          {[1, 7, 30, 365].map((value) => <button key={value} className={days === value ? 'active' : ''}
            onClick={() => setDays(value)}>{value === 1 ? '오늘' : value === 365 ? '1년' : `${value}일`}</button>)}
        </div>
        <div>
          <div className="search-box"><Search size={16} />
            <Input value={search} onChange={(event) => setSearch(event.target.value)}
              placeholder="행위자, 대상, 기록 내용 검색" /></div>
          <Select aria-label="동작으로 거르기" value={action} onChange={(event) => setAction(event.target.value)}>
            <option value="">모든 동작</option>
            {actions.map((row) => <option key={row.action} value={row.action}>
              {label(row.action)} ({row.count.toLocaleString('ko-KR')})</option>)}
          </Select>
        </div>
      </div>
      {loading ? <div className="table-skeleton">감사 기록을 불러오는 중…</div>
        : error ? <ErrorState message={error} onRetry={() => void load(offset)} />
          : entries.length === 0 ? <EmptyState icon={<ShieldCheck size={25} />} title="이 조건에 해당하는 기록이 없습니다"
            description="기간을 넓히거나 검색어를 지워 보세요. 기록은 지워지지 않으므로, 없다면 그 일이 없었던 것입니다." />
            : <>
              <div className="error-list">
                <div className="error-list-head"><span>동작</span><span>행위자</span><span>대상</span><span>시각</span><span /></div>
                {entries.map((entry) => <button key={entry.id} className="error-row" onClick={() => setSelected(entry)}>
                  <span className={`severity-bar severity-${actionTone(entry.action) === 'danger' ? 'high' : 'low'}`} />
                  <div className="error-summary">
                    <div><Badge tone={actionTone(entry.action)}>{label(entry.action)}</Badge><code>{entry.action}</code></div>
                    <strong>{entry.targetType ? `${entry.targetType} ${entry.targetId.slice(0, 8)}` : '—'}</strong>
                  </div>
                  <span><User size={14} /> {actor(entry)}</span>
                  <span>{entry.targetType || '—'}</span>
                  <span>{relativeDate(entry.createdAt)}</span>
                  <ChevronRight size={17} />
                </button>)}
              </div>
              <div className="audit-pager">
                <span>{shown}</span>
                <Button variant="secondary" disabled={offset === 0} onClick={() => void load(Math.max(offset - PAGE, 0))}>이전</Button>
                <Button variant="secondary" disabled={offset + PAGE >= total} onClick={() => void load(offset + PAGE)}>다음</Button>
              </div>
            </>}
    </section>
    {selected && <div className="drawer-backdrop" onClick={() => setSelected(null)}>
      <aside ref={drawer as React.RefObject<HTMLElement>} tabIndex={-1} className="error-drawer" onClick={(event) => event.stopPropagation()}>
        <header><div><Badge tone={actionTone(selected.action)}>{label(selected.action)}</Badge><code>{selected.action}</code></div>
          <button className="icon-button" onClick={() => setSelected(null)} aria-label="닫기">×</button></header>
        <section className="error-drawer-title"><h2>{selected.targetType || '기록'} {selected.targetId}</h2>
          <div><span>{formatDate(selected.createdAt, { year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit' })}</span></div></section>
        <section className="error-detail-grid">
          <div><span>행위자</span><strong>{actor(selected)}</strong></div>
          <div><span>행위자 ID</span><strong>{selected.actorId || '—'}</strong></div>
          <div><span>대상 종류</span><strong>{selected.targetType || '—'}</strong></div>
          <div><span>대상 ID</span><strong>{selected.targetId || '—'}</strong></div>
        </section>
        <section className="stack-section"><div><strong><History size={16} /> 함께 기록된 내용</strong></div>
          <pre>{selected.metadata ? JSON.stringify(selected.metadata, null, 2) : '이 동작은 추가로 기록한 값이 없습니다.'}</pre></section>
        <footer><Button variant="secondary" onClick={() => { setSearch(actor(selected)); setSelected(null) }}>이 행위자의 기록 보기</Button>
          {selected.targetId && <Button variant="secondary" onClick={() => { setSearch(selected.targetId); setSelected(null) }}>이 대상의 기록 보기</Button>}</footer>
      </aside>
    </div>}
  </AppShell>
}
