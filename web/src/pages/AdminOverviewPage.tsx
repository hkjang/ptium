import { useEffect, useState } from 'react'
import { Activity, ArrowRight, CheckCircle2, CircleAlert, Database, FileStack, KeyRound, Server, Settings2, Sparkles, Users } from 'lucide-react'
import { api } from '../api/client'
import { AppShell } from '../components/AppShell'
import { Badge, ErrorState, LoadingState } from '../components/UI'
import { Link } from '../router'
import { displayError } from '../utils'

const number = (value: unknown) => Number(value ?? 0).toLocaleString('ko-KR')

export function AdminOverviewPage() {
  const [data, setData] = useState<Record<string, unknown>>({})
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  useEffect(() => { api.adminOverview().then(setData).catch((err) => setError(displayError(err))).finally(() => setLoading(false)) }, [])

  return <AppShell title="관리자 개요" eyebrow="CONTROL CENTER" actions={!loading && !error ? <div className="live-status"><i /> API · DB 연결됨</div> : undefined}>
    {loading ? <LoadingState label="서비스 현황을 불러오는 중…" /> : error ? <ErrorState message={error} /> : <>
      <section className="admin-metric-grid six-metrics">
        <article><span className="metric-icon violet"><Users size={19} /></span><div><span>전체 사용자</span><strong>{number(data.users)}</strong><small>프로비저닝된 계정</small></div></article>
        <article><span className="metric-icon coral"><FileStack size={19} /></span><div><span>프레젠테이션</span><strong>{number(data.presentations)}</strong><small>전체 저장 덱</small></div></article>
        <article><span className="metric-icon mint"><CheckCircle2 size={19} /></span><div><span>생성 완료</span><strong>{number(data.completedDecks)}</strong><small>PPTX 내보내기 가능</small></div></article>
        <article><span className="metric-icon amber"><Sparkles size={19} /></span><div><span>생성 대기</span><strong>{number(data.queuedGenerations)}</strong><small>대기 또는 처리 중</small></div></article>
        <article><span className="metric-icon coral"><Activity size={19} /></span><div><span>열린 오류</span><strong>{number(data.openIncidents)}</strong><small>확인 또는 해결 필요</small></div></article>
        <article><span className="metric-icon violet"><KeyRound size={19} /></span><div><span>키 상태 활성</span><strong>{number(data.activeApiKeys)}</strong><small>유효 기간·회전 기준, 사용자 정지 제외 전</small></div></article>
      </section>
      <div className="admin-overview-grid">
        <section className="admin-panel service-health"><div className="panel-head"><div><h2>서비스 상태</h2><p>이 화면을 제공한 실제 구성 요소 상태</p></div><Badge tone="success"><CheckCircle2 size={12} /> 응답 정상</Badge></div><div className="service-list">
          <div><span className="service-icon"><Server size={17} /></span><div><strong>API 서버</strong><small>인증된 관리자 요청 처리 중</small></div><Badge tone="success">연결됨</Badge></div>
          <div><span className="service-icon"><Database size={17} /></span><div><strong>PostgreSQL</strong><small>운영 집계 쿼리 응답 완료</small></div><Badge tone="success">연결됨</Badge></div>
          <div><span className="service-icon"><Sparkles size={17} /></span><div><strong>생성 대기열</strong><small>현재 대기 또는 처리 상태인 작업 수</small></div><Badge tone={Number(data.queuedGenerations) > 0 ? 'info' : 'neutral'}>{number(data.queuedGenerations)}개</Badge></div>
          <div><span className="service-icon"><CircleAlert size={17} /></span><div><strong>오류 센터</strong><small>{number(data.openIncidents)}개 열린 오류 그룹</small></div><Badge tone={Number(data.openIncidents) > 0 ? 'warning' : 'success'}>{Number(data.openIncidents) > 0 ? '확인 필요' : '정상'}</Badge></div>
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
