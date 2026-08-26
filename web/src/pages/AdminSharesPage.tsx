import { useCallback, useEffect, useState } from 'react'
import { Link2, RefreshCw, Search, ShieldOff } from 'lucide-react'
import { api } from '../api/client'
import { AppShell } from '../components/AppShell'
import { Badge, Button, EmptyState, ErrorState, Input, LoadingState } from '../components/UI'
import { useToast } from '../components/Toast'
import { displayError, formatDate, relativeDate } from '../utils'

/**
 * What this deployment has handed out.
 *
 * A link is the one thing here that reaches somebody with no account, and only
 * the deck's owner could see their own. An operator asked "what of ours is
 * readable outside?" had no way to answer, and no way to close a link left open
 * by somebody who has since left.
 */
export type OpenShare = {
  id: string
  presentationId: string
  label?: string
  deckTitle: string
  ownerEmail?: string
  ownerName?: string
  state: 'open' | 'expired' | 'revoked'
  views: number
  expiresAt?: string
  lastSeenAt?: string
  createdAt: string
}

/** Who made this link, said with whatever the account has. */
export function whoseLink(share: Pick<OpenShare, 'ownerEmail' | 'ownerName'>) {
  return share.ownerEmail || share.ownerName || '알 수 없는 사용자'
}

/** What this link does now, and how urgently an operator should look at it. */
export function linkState(share: Pick<OpenShare, 'state' | 'expiresAt'>) {
  if (share.state === 'revoked') return { text: '회수됨', tone: 'neutral' as const }
  if (share.state === 'expired') return { text: '기한 지남', tone: 'neutral' as const }
  if (!share.expiresAt) return { text: '직접 회수할 때까지', tone: 'warning' as const }
  return { text: `${formatDate(share.expiresAt, { month: 'short', day: 'numeric' })}까지`, tone: 'info' as const }
}

const states: { id: '' | 'open' | 'expired' | 'revoked'; label: string }[] = [
  { id: 'open', label: '열려 있음' }, { id: 'expired', label: '기한 지남' },
  { id: 'revoked', label: '회수됨' }, { id: '', label: '전체' },
]

export function AdminSharesPage() {
  const [shares, setShares] = useState<OpenShare[]>([])
  // What the filter matches, which is not what fits on this page.
  const [total, setTotal] = useState(0)
  const [state, setState] = useState<'' | 'open' | 'expired' | 'revoked'>('open')
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [working, setWorking] = useState('')
  const { showToast } = useToast()

  const load = useCallback(async () => {
    setLoading(true); setError('')
    try {
      const answer = await api.adminShares({ state, search })
      setShares(answer.items as OpenShare[])
      setTotal(answer.total)
    }
    catch (err) { setError(displayError(err)) }
    finally { setLoading(false) }
  }, [state, search])
  useEffect(() => { void load() }, [load])

  const close = async (share: OpenShare) => {
    setWorking(share.id)
    try {
      const answer = await api.closeShare(share.id) as { closed?: boolean }
      showToast(answer?.closed
        ? `"${share.label || share.deckTitle}" 링크를 회수했습니다. 이 주소로는 더 이상 열리지 않습니다.`
        : '이미 회수된 링크입니다.')
      await load()
    } catch (err) { showToast(displayError(err), 'error') } finally { setWorking('') }
  }

  const open = shares.filter((share) => share.state === 'open')
  return <AppShell title="공유 링크" eyebrow="WHAT IS OPEN"
    actions={<Button variant="secondary" onClick={() => void load()}><RefreshCw size={15} /> 새로고침</Button>}>
    <section className="error-stat-grid">
      <article><span className="metric-icon amber"><Link2 size={18} /></span>
        <div><strong>{total.toLocaleString('ko-KR')}</strong>
          <small>{states.find((one) => one.id === state)?.label || '전체'} 링크</small></div></article>
      <article><span className="metric-icon coral"><ShieldOff size={18} /></span>
        <div><strong>{open.filter((share) => !share.expiresAt).length}</strong>
          <small>이 화면에서 기한 없이 열린 링크</small></div></article>
      <article><span className="metric-icon mint"><Search size={18} /></span>
        <div><strong>{shares.reduce((sum, share) => sum + (share.views || 0), 0).toLocaleString('ko-KR')}</strong>
          <small>이 화면 링크가 열린 횟수</small></div></article>
    </section>
    {total > shares.length && <p className="muted-note">
      {total.toLocaleString('ko-KR')}건 가운데 최근 {shares.length}건을 보고 있습니다. 검색으로 좁혀 주세요.</p>}
    <section className="admin-panel">
      <div className="error-toolbar">
        <div className="choice-chips">{states.map((one) => <button key={one.id || 'all'}
          className={state === one.id ? 'active' : ''} onClick={() => setState(one.id)}>{one.label}</button>)}</div>
        <div><Input value={search} placeholder="덱 제목 · 링크 이름 · 만든 사람"
          onChange={(event) => setSearch(event.target.value)} /></div>
      </div>
      {loading ? <LoadingState label="공유 링크를 불러오는 중…" />
        : error ? <ErrorState message={error} onRetry={() => void load()} />
        : shares.length === 0 ? <EmptyState icon={<Link2 size={25} />} title="이 조건에 맞는 링크가 없습니다"
            description="계정 없이 덱을 볼 수 있는 링크가 여기에 모두 나옵니다." />
        : <div className="error-list">
          <div className="error-list-head"><span>링크</span><span>만든 사람</span><span>상태</span><span>열림</span><span /></div>
          {shares.map((share) => {
            const life = linkState(share)
            return <div key={share.id} className={`error-row ${share.state === 'open' ? '' : 'revoked'}`}>
              <span className={`severity-bar severity-${share.state === 'open' && !share.expiresAt ? 'medium' : 'low'}`} />
              <div className="error-summary">
                <div><Badge tone={life.tone}>{life.text}</Badge>
                  {share.lastSeenAt && <code>마지막 {relativeDate(share.lastSeenAt)}</code>}</div>
                <strong>{share.label || '이름 없는 링크'}</strong>
                <small>{share.deckTitle}</small>
              </div>
              <span>{whoseLink(share)}</span>
              <span>{formatDate(share.createdAt, { month: 'short', day: 'numeric' })} 생성</span>
              <strong>{share.views}회</strong>
              <span className="queue-actions">
                {share.state === 'open' && <Button variant="secondary" disabled={working === share.id}
                  onClick={() => void close(share)}><ShieldOff size={14} /> 회수</Button>}
              </span>
            </div>
          })}
        </div>}
    </section>
  </AppShell>
}
