import { useCallback, useEffect, useState } from 'react'
import { Activity, AlertTriangle, LayoutTemplate, RefreshCw, Users } from 'lucide-react'
import { api } from '../api/client'
import { AppShell } from '../components/AppShell'
import { Button, EmptyState, ErrorState, LoadingState } from '../components/UI'
import { displayError } from '../utils'

/**
 * What this deployment has been doing.
 *
 * The overview says what is true now — how many decks exist, what is queued.
 * Nothing said what a week looked like: how many decks were written, how many
 * failed, how long they took and who asked for them. On a self-hosted model
 * that time is what running the thing costs.
 */
export type UsageDay = { day: string; generated: number; failed: number; medianSeconds: number; slowestSeconds: number }
export type UsageCount = { name: string; count: number; detail?: string }
export type Usage = {
  days: UsageDay[]; owners: UsageCount[]; designs: UsageCount[]; failures: UsageCount[]
  generated: number; failed: number; timed: number
}

/** A duration said the way its size deserves. */
export function howLong(seconds: number) {
  if (!seconds) return '—'
  if (seconds < 1) return `${Math.round(seconds * 1000)}밀리초`
  if (seconds < 90) return `${seconds < 10 ? seconds.toFixed(1) : Math.round(seconds)}초`
  return `${Math.floor(seconds / 60)}분 ${Math.round(seconds % 60)}초`
}

/** How tall this day's bar is, against the busiest day on the chart. */
export function barHeight(day: UsageDay, days: UsageDay[]) {
  const busiest = Math.max(1, ...days.map((one) => one.generated))
  return Math.max(day.generated > 0 ? 4 : 0, Math.round((day.generated / busiest) * 100))
}

/** What share of the decks did not come out. */
export function failedShare(usage: Pick<Usage, 'generated' | 'failed'>) {
  if (!usage.generated) return 0
  return Math.round((usage.failed / usage.generated) * 1000) / 10
}

const spans = [7, 14, 30, 90]

export function AdminUsagePage() {
  const [usage, setUsage] = useState<Usage | null>(null)
  const [days, setDays] = useState(14)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const load = useCallback(async () => {
    setLoading(true); setError('')
    try { setUsage(await api.adminUsage(days) as Usage) }
    catch (err) { setError(displayError(err)) }
    finally { setLoading(false) }
  }, [days])
  useEffect(() => { void load() }, [load])

  return <AppShell title="사용 현황" eyebrow="WHAT THIS DEPLOYMENT HAS BEEN DOING"
    actions={<Button variant="secondary" onClick={() => void load()}><RefreshCw size={15} /> 새로고침</Button>}>
    <div className="error-toolbar">
      <div className="choice-chips">{spans.map((span) => <button key={span}
        className={days === span ? 'active' : ''} onClick={() => setDays(span)}>{span}일</button>)}</div>
    </div>
    {loading ? <LoadingState label="사용 현황을 세는 중…" />
      : error ? <ErrorState message={error} onRetry={() => void load()} />
      : !usage || usage.days.length === 0 ? <EmptyState icon={<Activity size={25} />} title="아직 셀 것이 없습니다"
          description="덱을 만들면 여기에 날짜별로 쌓입니다." />
      : <>
        <section className="error-stat-grid">
          <article><span className="metric-icon amber"><Activity size={18} /></span>
            <div><strong>{usage.generated.toLocaleString('ko-KR')}</strong><small>{days}일 동안 만든 덱</small></div></article>
          <article><span className="metric-icon red"><AlertTriangle size={18} /></span>
            <div><strong>{usage.failed.toLocaleString('ko-KR')}</strong>
              <small>실패 · 전체의 {failedShare(usage)}%</small></div></article>
          <article><span className="metric-icon mint"><Activity size={18} /></span>
            <div><strong>{howLong(Math.max(...usage.days.map((day) => day.slowestSeconds), 0))}</strong>
              <small>가장 오래 걸린 한 덱</small></div></article>
        </section>
        <section className="admin-panel usage-chart">
          <div className="section-heading"><div><h2>날짜별</h2>
            <p>막대는 만든 덱, 붉은 부분은 실패입니다. 시간을 기록한 덱은 {usage.timed.toLocaleString('ko-KR')}건입니다.</p></div></div>
          <div className="usage-bars">
            {usage.days.map((day) => <div key={day.day} className="usage-bar"
              title={`${day.day} · 생성 ${day.generated} · 실패 ${day.failed} · 중앙 ${howLong(day.medianSeconds)} · 가장 오래 ${howLong(day.slowestSeconds)}`}>
              <span style={{ height: `${barHeight(day, usage.days)}%` }}>
                {day.failed > 0 && <i style={{ height: `${Math.round((day.failed / Math.max(1, day.generated)) * 100)}%` }} />}
              </span>
              <small>{day.day.slice(5)}</small>
            </div>)}
          </div>
        </section>
        <div className="admin-overview-grid">
          <UsageList title="누가 만들었나" icon={<Users size={17} />} counts={usage.owners} />
          <UsageList title="어떤 디자인으로" icon={<LayoutTemplate size={17} />} counts={usage.designs} />
        </div>
        {usage.failures.length > 0 &&
          <UsageList title="실패한 이유" icon={<AlertTriangle size={17} />} counts={usage.failures} />}
      </>}
  </AppShell>
}

function UsageList({ title, icon, counts }: { title: string; icon: React.ReactNode; counts: UsageCount[] }) {
  const most = Math.max(1, ...counts.map((one) => one.count))
  return <section className="admin-panel">
    <div className="section-heading"><span>{icon}</span><div><h2>{title}</h2></div></div>
    {counts.length === 0 ? <p className="muted-note">없습니다.</p>
      : <ul className="usage-list">{counts.map((one) => <li key={`${one.name}-${one.detail ?? ''}`}>
        <div><strong>{one.name}</strong>{one.detail && <small>{one.detail}</small>}</div>
        <span className="usage-track"><i style={{ width: `${Math.round((one.count / most) * 100)}%` }} /></span>
        <b>{one.count.toLocaleString('ko-KR')}</b>
      </li>)}</ul>}
  </section>
}
