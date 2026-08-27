import { useCallback, useEffect, useState } from 'react'
import { CircleSlash, Clock, RefreshCw, RotateCw, Sparkles, User } from 'lucide-react'
import { api } from '../api/client'
import { elapsedMeans, troubled } from './queuehealth'
import { AppShell } from '../components/AppShell'
import { Badge, Button, EmptyState, ErrorState, Input } from '../components/UI'
import { useToast } from '../components/Toast'
import { displayError, relativeDate } from '../utils'

type QueuedDeck = {
  id: string; title: string; ownerEmail: string; status: string
  stage: string; errorMessage: string; waitingSeconds: number; quietSeconds?: number; updatedAt: string
}

/**
 * The queue, and the two things an operator can do about it.
 *
 * The overview learned to say that the oldest deck has been waiting twenty
 * minutes. Reading that and being able to do nothing is worse than not knowing:
 * a deck belongs to its owner, and an administrator could not see one, let
 * alone push it through.
 */
const waited = (seconds: number) => seconds >= 3600 ? `${Math.floor(seconds / 3600)}시간 ${Math.floor((seconds % 3600) / 60)}분`
  : seconds >= 60 ? `${Math.floor(seconds / 60)}분` : `${seconds}초`

export function AdminQueuePage() {
  const [queue, setQueue] = useState<QueuedDeck[]>([])
  // What the queue holds, which is not what this list carries: the list is a
  // hundred rows at most.
  const [totals, setTotals] = useState({ waiting: 0, failed: 0 })
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [working, setWorking] = useState('')
  const [reason, setReason] = useState('')
  const { showToast } = useToast()

  const load = useCallback(async () => {
    setLoading(true); setError('')
    try {
      const answer = await api.generationQueue(24)
      setQueue(answer.items)
      setTotals({ waiting: answer.waiting, failed: answer.failed })
    } catch (err) { setError(displayError(err)) } finally { setLoading(false) }
  }, [])
  useEffect(() => { void load() }, [load])
  // A queue is a moving thing; a screen of it that does not move is a screenshot.
  useEffect(() => {
    const timer = window.setInterval(() => {
      void api.generationQueue(24).then((answer) => {
        setQueue(answer.items)
        setTotals({ waiting: answer.waiting, failed: answer.failed })
      }).catch(() => {})
    }, 15000)
    return () => window.clearInterval(timer)
  }, [])

  const act = async (deck: QueuedDeck, what: 'requeue' | 'cancel') => {
    setWorking(deck.id)
    try {
      if (what === 'requeue') { await api.requeueGeneration(deck.id); showToast(`"${deck.title}"을 다시 큐에 넣었습니다.`) }
      else {
        await api.cancelGeneration(deck.id, reason.trim() || '관리자가 생성을 중단했습니다')
        showToast(`"${deck.title}" 생성을 중단했습니다. 작성자에게 이유가 보입니다.`)
      }
      await load()
    } catch (err) { showToast(displayError(err), 'error') } finally { setWorking('') }
  }

  const waiting = queue.filter((deck) => deck.status !== 'failed')
  // Not "older than fifteen minutes": a deck being written whose worker is
  // still saying it is alive is fine however long it takes.
  const stuck = waiting.filter(troubled)
  // The list is capped, so anything counted from its rows is counted from a
  // part — and the screen has to say which numbers those are.
  const truncated = totals.waiting + totals.failed > queue.length
  return <AppShell title="생성 큐" eyebrow="WHAT IS BEING WRITTEN"
    actions={<Button variant="secondary" onClick={() => void load()}><RefreshCw size={15} /> 새로고침</Button>}>
    <section className="error-stat-grid">
      <article><span className="metric-icon amber"><Sparkles size={18} /></span><div><strong>{totals.waiting.toLocaleString('ko-KR')}</strong><small>대기 · 작성 중</small></div></article>
      <article><span className="metric-icon coral"><Clock size={18} /></span><div><strong>{stuck.length}</strong><small>맡은 워커가 조용하거나 15분 넘게 대기{truncated ? ' (아래 목록 안에서)' : ''}</small></div></article>
      <article><span className="metric-icon red"><CircleSlash size={18} /></span><div><strong>{totals.failed.toLocaleString('ko-KR')}</strong><small>최근 24시간 실패</small></div></article>
    </section>
    {truncated && <p className="muted-note">
      가장 오래 기다린 {queue.length}건만 아래에 보입니다. 큐에는 모두 {(totals.waiting + totals.failed).toLocaleString('ko-KR')}건이 있습니다.
    </p>}
    <section className="admin-panel">
      <div className="error-toolbar">
        <div><span className="muted-note">중단할 때 작성자에게 보일 이유</span></div>
        <div><Input value={reason} maxLength={300} onChange={(event) => setReason(event.target.value)}
          placeholder="예: 모델 점검 중입니다. 30분 뒤 다시 시도해 주세요" /></div>
      </div>
      {loading ? <div className="table-skeleton">큐를 불러오는 중…</div>
        : error ? <ErrorState message={error} onRetry={() => void load()} />
          : queue.length === 0 ? <EmptyState icon={<Sparkles size={25} />} title="큐가 비어 있습니다"
            description="기다리는 덱도, 최근 24시간 안에 실패한 덱도 없습니다." />
            : <div className="error-list">
              <div className="error-list-head"><span>덱</span><span>작성자</span><span>상태</span><span>경과</span><span /></div>
              {queue.map((deck) => <div key={deck.id} className="error-row queue-row">
                <span className={`severity-bar severity-${deck.status === 'failed' ? 'high' : troubled(deck) ? 'medium' : 'low'}`} />
                <div className="error-summary">
                  <div><Badge tone={deck.status === 'failed' ? 'danger' : deck.status === 'generating' ? 'info' : 'warning'}>
                    {deck.status === 'failed' ? '실패' : deck.status === 'generating' ? '작성 중' : '대기'}</Badge>
                    {deck.stage && <code>{deck.stage}</code>}</div>
                  <strong>{deck.title}</strong>
                  {deck.errorMessage && <small>{deck.errorMessage}</small>}
                </div>
                <span><User size={14} /> {deck.ownerEmail || '—'}</span>
                <span>{relativeDate(deck.updatedAt)}</span>
                <strong className={troubled(deck) ? 'queue-late' : undefined}>
                  {waited(deck.waitingSeconds)}
                  {elapsedMeans(deck) === 'writing' && <small className="queue-quiet">
                    {' '}{typeof deck.quietSeconds === 'number'
                      ? troubled(deck) ? `· ${waited(deck.quietSeconds)}째 응답 없음` : '· 작성 중'
                      : '· 작성 중'}</small>}</strong>
                <span className="queue-actions">
                  <Button variant="secondary" disabled={working === deck.id} onClick={() => void act(deck, 'requeue')}>
                    <RotateCw size={14} /> 다시 큐에</Button>
                  {deck.status !== 'failed' && <Button variant="secondary" disabled={working === deck.id}
                    onClick={() => void act(deck, 'cancel')}><CircleSlash size={14} /> 중단</Button>}
                </span>
              </div>)}
            </div>}
    </section>
  </AppShell>
}
