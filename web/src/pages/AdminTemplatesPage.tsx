import { useCallback, useEffect, useState } from 'react'
import { Check, LayoutTemplate, Lock, RefreshCw, Star, Users } from 'lucide-react'
import { api } from '../api/client'
import { AppShell } from '../components/AppShell'
import { Badge, Button, EmptyState, ErrorState, Input, LoadingState } from '../components/UI'
import { useToast } from '../components/Toast'
import { displayError } from '../utils'

/**
 * The designs this deployment writes decks in.
 *
 * A person's own screens show the designs they may use. An operator asked which
 * designs their organisation actually writes in — or asked to make one team's
 * upload the standard for everybody — could see none of it: an upload is
 * private to whoever uploaded it, and the standard was a text field whose value
 * meant nothing to anyone reading it.
 */
export type DeploymentTemplate = {
  id: string
  name: string
  kind: 'builtin' | 'uploaded'
  scope: 'private' | 'shared'
  paletteKey?: string
  layoutCount?: number
  decks: number
  recent: number
  standard: boolean
  ownerEmail?: string
  ownerName?: string
}

/** Who may write a deck in this design. */
export function whoMayUse(design: Pick<DeploymentTemplate, 'kind' | 'scope' | 'ownerEmail' | 'ownerName'>) {
  if (design.kind === 'builtin') return '모두 (내장)'
  if (design.scope === 'shared') return '모두 (공개된 업로드)'
  return `${design.ownerEmail || design.ownerName || '올린 사람'}만`
}

/** What a design's use says about it. */
export function useWord(design: Pick<DeploymentTemplate, 'decks' | 'recent'>) {
  if (design.decks === 0) return '아직 쓰이지 않았습니다'
  if (design.recent === 0) return `${design.decks.toLocaleString('ko-KR')}개 · 최근 30일은 없음`
  return `${design.decks.toLocaleString('ko-KR')}개 · 최근 30일 ${design.recent.toLocaleString('ko-KR')}개`
}

const kinds: { id: '' | 'uploaded' | 'builtin'; label: string }[] = [
  { id: '', label: '전체' }, { id: 'uploaded', label: '올린 템플릿' }, { id: 'builtin', label: '내장 디자인' },
]

export function AdminTemplatesPage() {
  const [designs, setDesigns] = useState<DeploymentTemplate[]>([])
  const [total, setTotal] = useState(0)
  const [kind, setKind] = useState<'' | 'uploaded' | 'builtin'>('')
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [working, setWorking] = useState('')
  const { showToast } = useToast()

  const load = useCallback(async () => {
    setLoading(true); setError('')
    try {
      const answer = await api.adminTemplates({ kind, search })
      setDesigns(answer.items as DeploymentTemplate[]); setTotal(answer.total)
    } catch (err) { setError(displayError(err)) } finally { setLoading(false) }
  }, [kind, search])
  useEffect(() => { void load() }, [load])

  const makeStandard = async (design: DeploymentTemplate) => {
    setWorking(design.id)
    try {
      await api.setStandardTemplate(design.id)
      showToast(`새 덱은 이제 "${design.name}"으로 만들어집니다.`)
      await load()
    } catch (err) { showToast(displayError(err), 'error') } finally { setWorking('') }
  }
  const setShared = async (design: DeploymentTemplate, shared: boolean) => {
    setWorking(design.id)
    try {
      await api.shareTemplate(design.id, shared)
      showToast(shared ? `"${design.name}"을 모두가 쓸 수 있게 했습니다.` : `"${design.name}"을 올린 사람만 쓰도록 되돌렸습니다.`)
      await load()
    } catch (err) { showToast(displayError(err), 'error') } finally { setWorking('') }
  }

  const uploaded = designs.filter((design) => design.kind === 'uploaded')
  return <AppShell title="디자인" eyebrow="WHAT DECKS ARE WRITTEN IN"
    actions={<Button variant="secondary" onClick={() => void load()}><RefreshCw size={15} /> 새로고침</Button>}>
    <section className="error-stat-grid">
      <article><span className="metric-icon amber"><LayoutTemplate size={18} /></span>
        <div><strong>{total.toLocaleString('ko-KR')}</strong><small>이 배포의 디자인</small></div></article>
      <article><span className="metric-icon mint"><Users size={18} /></span>
        <div><strong>{uploaded.filter((design) => design.scope === 'shared').length}</strong>
          <small>모두에게 공개된 업로드</small></div></article>
      <article><span className="metric-icon coral"><Lock size={18} /></span>
        <div><strong>{uploaded.filter((design) => design.scope === 'private').length}</strong>
          <small>올린 사람만 쓰는 업로드</small></div></article>
    </section>
    <section className="admin-panel">
      <div className="error-toolbar">
        <div className="choice-chips">{kinds.map((one) => <button key={one.id || 'all'}
          className={kind === one.id ? 'active' : ''} onClick={() => setKind(one.id)}>{one.label}</button>)}</div>
        <div><Input value={search} placeholder="이름으로 찾기"
          onChange={(event) => setSearch(event.target.value)} /></div>
      </div>
      {loading ? <LoadingState label="디자인을 불러오는 중…" />
        : error ? <ErrorState message={error} onRetry={() => void load()} />
        : designs.length === 0 ? <EmptyState icon={<LayoutTemplate size={25} />} title="해당하는 디자인이 없습니다"
            description="내장 디자인과 사용자가 올린 템플릿이 모두 여기에 나옵니다." />
        : <div className="error-list">
          <div className="error-list-head"><span>디자인</span><span>누가 쓸 수 있나</span><span>쓰인 덱</span><span /></div>
          {designs.map((design) => <div key={design.id} className="error-row">
            <span className={`severity-bar severity-${design.standard ? 'medium' : 'low'}`} />
            <div className="error-summary">
              <div>{design.standard && <Badge tone="warning"><Star size={12} /> 표준</Badge>}
                <Badge>{design.kind === 'builtin' ? '내장' : '업로드'}</Badge>
                {design.layoutCount ? <code>레이아웃 {design.layoutCount}</code> : null}</div>
              <strong>{design.name}</strong>
              <small>{useWord(design)}</small>
            </div>
            <span>{whoMayUse(design)}</span>
            <strong>{design.decks.toLocaleString('ko-KR')}</strong>
            <span className="queue-actions">
              {!design.standard && <Button variant="secondary" disabled={working === design.id}
                onClick={() => void makeStandard(design)}><Star size={14} /> 표준으로</Button>}
              {design.kind === 'uploaded' && (design.scope === 'shared'
                ? <Button variant="ghost" disabled={working === design.id}
                    onClick={() => void setShared(design, false)}><Lock size={14} /> 비공개로</Button>
                : <Button variant="ghost" disabled={working === design.id}
                    onClick={() => void setShared(design, true)}><Check size={14} /> 모두에게</Button>)}
            </span>
          </div>)}
        </div>}
    </section>
  </AppShell>
}
