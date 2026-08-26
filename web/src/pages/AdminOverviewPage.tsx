import { useEffect, useState } from 'react'
import { Activity, Bot, ArrowRight, CheckCircle2, CircleAlert, Database, FileDown, FileStack, KeyRound, Server, Settings2, Sparkles, Trash2, Users } from 'lucide-react'
import { api } from '../api/client'
import { generationTrouble, quietTooLong, waitedTooLong } from './queuehealth'
import { AppShell } from '../components/AppShell'
import { Badge, Button, ErrorState, LoadingState } from '../components/UI'
import { useToast } from '../components/Toast'
import { Link } from '../router'
import { displayError, relativeDate } from '../utils'

const number = (value: unknown) => Number(value ?? 0).toLocaleString('ko-KR')

/**
 * What the queue is doing, in the words that tell an operator whether to act.
 *
 * A queue of twelve says nothing on its own: twelve decks asked for in the last
 * minute is a busy morning and one deck waiting since three hours ago is a
 * worker that died. The age of the oldest thing still waiting is the number
 * that tells them apart, so it is what this line says.
 */
function waitingFor(data: Record<string, unknown>) {
  const queued = Number(data.queuedGenerations ?? 0)
  if (queued === 0) return '대기 없음'
  const seconds = Number(data.oldestQueuedSeconds ?? 0)
  const quiet = Number(data.quietestGenerationSeconds ?? 0)
  // Silence first: a deck nobody is writing is the thing to act on, and it can
  // sit behind a queue that is otherwise moving.
  if (quiet >= quietTooLong) return `작성 중인 덱이 ${Math.floor(quiet / 60)}분째 응답이 없습니다 — 작업자를 확인하세요`
  if (seconds >= waitedTooLong) return `가장 오래 기다린 덱 ${Math.floor(seconds / 60)}분 — 작업자를 확인하세요`
  if (seconds >= 60) return `가장 오래 기다린 덱 ${Math.floor(seconds / 60)}분`
  if (seconds > 0) return `가장 오래 기다린 덱 ${seconds}초`
  // Everything in the queue is being written, and being written is not waiting.
  return '작성 중'
}

/**
 * How much of the open-error count belongs to the build that is running.
 *
 * Eight open groups on a deployment that has been upgraded twenty times is not
 * eight things wrong with it — most of them can be faults it left behind, and
 * the count alone cannot say which. Nothing here claims a fault is fixed: it
 * says how many of the open groups this build has actually seen.
 */
export function incidentsHere(data: Record<string, unknown>) {
  const open = Number(data.openIncidents ?? 0)
  if (open === 0) return '확인 또는 해결 필요'
  const here = Number(data.openIncidentsThisBuild ?? 0)
  const elsewhere = Number(data.openIncidentsOtherBuild ?? 0)
  if (here > 0) return here === open ? '모두 현재 버전에서 발생' : `현재 버전에서 발생 ${here.toLocaleString('ko-KR')}`
  // A group recorded before the product kept the build belongs to no version at
  // all, so the count stays quiet rather than calling it an earlier one.
  if (elsewhere === open) return '모두 다른 버전에서 발생'
  if (elsewhere > 0) return `다른 버전에서 발생 ${elsewhere.toLocaleString('ko-KR')}`
  return '확인 또는 해결 필요'
}

/** What the model host said, or that this deployment does not use one. */
function providerLine(provider: Record<string, unknown> | null) {
  if (provider === null) return '제공자에 물어보는 중…'
  if (provider.provider === 'fallback') return '오프라인 작성기로 씁니다 — 모델 호스트를 쓰지 않습니다'
  if (provider.reachable) return `${String(provider.model || '')} · ${Number(provider.milliseconds ?? 0).toLocaleString('ko-KR')}ms`
  return String(provider.detail || '') || '제공자가 답하지 않습니다'
}

/** Bytes as a person reads them. */
const bytes = (value: unknown) => {
  const size = Number(value ?? 0)
  if (size >= 1024 ** 3) return `${(size / 1024 ** 3).toFixed(1)} GB`
  if (size >= 1024 ** 2) return `${(size / 1024 ** 2).toFixed(1)} MB`
  if (size >= 1024) return `${(size / 1024).toFixed(0)} KB`
  return `${size} B`
}

// The tables are named for the database; the screen names them for the thing
// they hold.
const tableNames: Record<string, string> = {
  slides: '슬라이드', presentations: '프레젠테이션', presentation_revisions: '되돌리기 기록',
  assets: '이미지', templates: '템플릿', audit_logs: '감사 기록', server_errors: '오류 기록',
  snippets: '조각 슬라이드', slide_comments: '댓글',
}

/**
 * How the volume is doing, when there is one. A disk under a tenth free is the
 * thing to say out loud: uploads and generations start failing there, with
 * whatever error the layer underneath happens to raise.
 */
function diskTone(storage: Record<string, unknown>) {
  const total = Number(storage.assetDirTotalBytes ?? 0)
  const free = Number(storage.assetDirFreeBytes ?? 0)
  if (total <= 0) return 'neutral' as const
  return free / total < 0.1 ? ('warning' as const) : ('success' as const)
}

function diskWord(storage: Record<string, unknown>) {
  const total = Number(storage.assetDirTotalBytes ?? 0)
  const free = Number(storage.assetDirFreeBytes ?? 0)
  if (total <= 0) return '이미지는 데이터베이스에 있습니다'
  const share = Math.round((free / total) * 100)
  return share < 10 ? `볼륨 여유 ${share}% — 정리가 필요합니다` : `볼륨 여유 ${share}%`
}

/** Whether anything has been written lately, and whether it went wrong. */
function generationHealth(data: Record<string, unknown>) {
  const failed = Number(data.failedLastDay ?? 0)
  const last = typeof data.lastCompletedAt === 'string' ? data.lastCompletedAt : ''
  const wrote = last ? `마지막 완성 ${relativeDate(last)}` : '완성된 덱이 아직 없습니다'
  return { failed, wrote, tone: (failed > 0 ? 'warning' : 'success') as 'warning' | 'success' }
}

export function AdminOverviewPage() {
  const [data, setData] = useState<Record<string, unknown>>({})
  const [storage, setStorage] = useState<Record<string, unknown> | null>(null)
  const [provider, setProvider] = useState<Record<string, unknown> | null>(null)
  const [loading, setLoading] = useState(true)
  const [reporting, setReporting] = useState(false)
  const { showToast } = useToast()
  const [error, setError] = useState('')
  useEffect(() => { api.adminOverview().then(setData).catch((err) => setError(displayError(err))).finally(() => setLoading(false)) }, [])
  // Capacity is read separately: it is the slower query of the two, and a
  // deployment that cannot answer it should still show everything else.
  useEffect(() => { api.storageUsage().then(setStorage).catch(() => setStorage(null)) }, [])
  // Asked once when the screen opens. The panel used to say "연결됨" whatever
  // was true, which is worse than saying nothing: an operator reading a green
  // board while nothing can be generated is being told the wrong thing.
  useEffect(() => { api.providerStatus().then(setProvider).catch(() => setProvider({ reachable: false, detail: '' })) }, [])

  // The report is a file rather than a screen: what an operator does with it is
  // attach it to a question somebody outside the site can read.
  const downloadReport = async () => {
    setReporting(true)
    try {
      const text = await api.deploymentReport()
      const url = URL.createObjectURL(new Blob([text], { type: 'text/markdown;charset=utf-8' }))
      const link = document.createElement('a')
      link.href = url
      link.download = `ptium-점검-${new Date().toISOString().slice(0, 10)}.md`
      link.click()
      URL.revokeObjectURL(url)
      showToast('점검 리포트를 내려받았습니다. 비밀 값은 들어 있지 않습니다.')
    } catch (err) { showToast(displayError(err), 'error') } finally { setReporting(false) }
  }

  return <AppShell title="관리자 개요" eyebrow="CONTROL CENTER" actions={<>
    {/* A site with no internet cannot open a dashboard for anybody. This is the
        one file it can send: version, migrations, what was changed from what
        this product ships with, what has failed lately, what has piled up — and
        no secret in it. */}
    <Button variant="secondary" onClick={() => void downloadReport()} disabled={reporting}>
      <FileDown size={15} /> {reporting ? '만드는 중…' : '점검 리포트'}</Button>
    {!loading && !error ? <div className="live-status"><i /> API · DB 연결됨</div> : null}</>}>
    {loading ? <LoadingState label="서비스 현황을 불러오는 중…" /> : error ? <ErrorState message={error} /> : <>
      <section className="admin-metric-grid">
        <article><span className="metric-icon violet"><Users size={19} /></span><div><span>전체 사용자</span><strong>{number(data.users)}</strong><small>프로비저닝된 계정</small></div></article>
        <article><span className="metric-icon coral"><FileStack size={19} /></span><div><span>프레젠테이션</span><strong>{number(data.presentations)}</strong><small>전체 저장 덱</small></div></article>
        <article><span className="metric-icon mint"><CheckCircle2 size={19} /></span><div><span>생성 완료</span><strong>{number(data.completedDecks)}</strong><small>PPTX 내보내기 가능</small></div></article>
        <article><span className="metric-icon amber"><Sparkles size={19} /></span><div><span>생성 대기</span><strong>{number(data.queuedGenerations)}</strong><small>{waitingFor(data)}</small></div></article>
        <article><span className="metric-icon coral"><Activity size={19} /></span><div><span>열린 오류</span><strong>{number(data.openIncidents)}</strong><small>{incidentsHere(data)}</small></div></article>
        <article><span className="metric-icon violet"><KeyRound size={19} /></span><div><span>키 상태 활성</span><strong>{number(data.activeApiKeys)}</strong><small>유효 기간·회전 기준, 사용자 정지 제외 전</small></div></article>
        {/* Every other number here counts what has not been deleted. A trash
            nobody empties grows for as long as the deployment runs, and the
            operator cannot decide what to do about it without seeing it. */}
        <article><span className="metric-icon amber"><Trash2 size={19} /></span><div><span>휴지통</span><strong>{number(data.deletedDecks)}</strong><small>지웠지만 남아 있는 덱 · 자동으로 비워지지 않습니다</small></div></article>
      </section>
      {storage && <section className="admin-panel storage-panel">
        <div className="panel-head"><div><h2>보관 용량</h2><p>이 배포가 쥐고 있는 것과 남은 자리</p></div>
          <Badge tone={diskTone(storage)}>{diskWord(storage)}</Badge></div>
        <div className="storage-bars">
          {(storage.tables as { name: string; rows: number; bytes: number }[] || []).slice(0, 6).map((table) => {
            const largest = Math.max(...((storage.tables as { bytes: number }[]) || [{ bytes: 1 }]).map((row) => row.bytes), 1)
            return <div key={table.name} className="storage-row">
              <span>{tableNames[table.name] || table.name}</span>
              <i><b style={{ width: `${Math.max((table.bytes / largest) * 100, 2)}%` }} /></i>
              <strong>{bytes(table.bytes)}</strong>
              <small>{Number(table.rows).toLocaleString('ko-KR')}행</small>
            </div>
          })}
        </div>
        <div className="storage-foot">
          <span>데이터베이스 합계 <strong>{bytes(storage.databaseBytes)}</strong></span>
          {Number(storage.assetsInVolume ?? 0) > 0 && <span>볼륨의 이미지 <strong>{bytes(storage.assetsInVolume)}</strong></span>}
          {Number(storage.assetsInRows ?? 0) > 0 && <span>행 안의 이미지 <strong>{bytes(storage.assetsInRows)}</strong></span>}
          {typeof storage.assetDir === 'string' && storage.assetDir !== '' &&
            <span>{storage.assetDir} 여유 <strong>{bytes(storage.assetDirFreeBytes)}</strong> / {bytes(storage.assetDirTotalBytes)}</span>}
        </div>
      </section>}
      <section className="admin-panel generation-health">
        <div className="panel-head"><div><h2>생성 상태</h2><p>이 배포의 작성기가 실제로 돌고 있는지</p></div>
          <Badge tone={generationHealth(data).tone}>{generationHealth(data).failed > 0
            ? `24시간 내 실패 ${number(data.failedLastDay)}건` : '최근 24시간 실패 없음'}</Badge></div>
        <div className="service-list">
          <div><span className="service-icon"><Sparkles size={17} /></span>
            <div><strong>{waitingFor(data)}</strong><small>대기 {number(data.queuedGenerations)}건</small></div>
            <Badge tone={generationTrouble(Number(data.oldestQueuedSeconds ?? 0), Number(data.quietestGenerationSeconds ?? 0)) ? 'warning' : 'success'}>
              {generationTrouble(Number(data.oldestQueuedSeconds ?? 0), Number(data.quietestGenerationSeconds ?? 0)) ? '멈춤 의심' : '정상'}</Badge></div>
          <div><span className="service-icon"><CheckCircle2 size={17} /></span>
            <div><strong>{generationHealth(data).wrote}</strong><small>완성 누적 {number(data.completedDecks)}건</small></div>
            <Badge tone="success">기록됨</Badge></div>
        </div>
      </section>
      <div className="admin-overview-grid">
        <section className="admin-panel service-health"><div className="panel-head"><div><h2>서비스 상태</h2><p>이 화면을 제공한 실제 구성 요소 상태</p></div><Badge tone="success"><CheckCircle2 size={12} /> 응답 정상</Badge></div><div className="service-list">
          <div><span className="service-icon"><Server size={17} /></span><div><strong>API 서버</strong><small>인증된 관리자 요청 처리 중</small></div><Badge tone="success">연결됨</Badge></div>
          <div><span className="service-icon"><Database size={17} /></span><div><strong>PostgreSQL</strong><small>운영 집계 쿼리 응답 완료</small></div><Badge tone="success">연결됨</Badge></div>
          <div><span className="service-icon"><Bot size={17} /></span>
            <div><strong>AI 제공자</strong><small>{providerLine(provider)}</small></div>
            <Badge tone={provider === null ? 'neutral' : provider.reachable ? 'success' : provider.provider === 'fallback' ? 'neutral' : 'danger'}>
              {provider === null ? '확인 중' : provider.reachable ? '응답함' : provider.provider === 'fallback' ? '오프라인 작성기' : '응답 없음'}</Badge></div>
          <div><span className="service-icon"><Sparkles size={17} /></span><div><strong>생성 대기열</strong><small>현재 대기 또는 처리 상태인 작업 수</small></div><Badge tone={Number(data.queuedGenerations) > 0 ? 'info' : 'neutral'}>{number(data.queuedGenerations)}개</Badge></div>
          <div><span className="service-icon"><CircleAlert size={17} /></span><div><strong>오류 센터</strong><small>{number(data.openIncidents)}개 열린 오류 그룹{Number(data.openIncidents) > 0 && Number(data.openIncidentsThisBuild) === 0 && Number(data.openIncidentsOtherBuild) > 0 ? ' · 이 버전에서 기록된 것은 없음' : ''}</small></div><Badge tone={Number(data.openIncidents) > 0 ? 'warning' : 'success'}>{Number(data.openIncidents) > 0 ? '확인 필요' : '정상'}</Badge></div>
        </div></section>
        <section className="admin-panel activity-panel"><div className="panel-head"><div><h2>운영 작업</h2><p>설정과 액세스, 오류 대응으로 이동합니다.</p></div></div><div className="admin-activity-list">
          <div><span className="event-dot violet" /><div><strong>서비스 구성</strong><small>AI, OIDC, 생성 기본값, 보안 설정</small></div><Link to="/admin/settings">열기 <ArrowRight size={13} /></Link></div>
          <div><span className="event-dot mint" /><div><strong>사용자 액세스</strong><small>관리자 권한과 계정 활성 상태</small></div><Link to="/admin/users">열기 <ArrowRight size={13} /></Link></div>
          <div><span className="event-dot coral" /><div><strong>오류 수명주기</strong><small>확인, 해결, 무시와 운영 메모</small></div><Link to="/admin/errors">열기 <ArrowRight size={13} /></Link></div>
        </div></section>
      </div>
      <section className="admin-shortcuts"><Link to="/admin/settings"><span><Settings2 size={20} /></span><div><strong>서비스 설정</strong><small>AI, 인증, 보안 정책 관리</small></div><ArrowRight size={17} /></Link><Link to="/admin/users"><span><Users size={20} /></span><div><strong>사용자 관리</strong><small>역할, 상태, 액세스 제어</small></div><ArrowRight size={17} /></Link><Link to="/admin/errors"><span><CircleAlert size={20} /></span><div><strong>오류 센터</strong><small>오류 추적과 인시던트 대응</small></div><ArrowRight size={17} /></Link></section>
    </>}
  </AppShell>
}
