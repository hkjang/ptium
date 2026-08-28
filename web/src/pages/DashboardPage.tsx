import { useCallback, useEffect, useState } from 'react'
import { ArrowRight, BookOpen, CircleUserRound, Clock3, LifeBuoy, Plus, Sparkles, WandSparkles } from 'lucide-react'
import { api } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import { useBrand } from '../branding/BrandContext'
import { AppShell } from '../components/AppShell'
import { PresentationCard } from '../components/PresentationCard'
import { Button, EmptyState, ErrorState, LoadingState } from '../components/UI'
import { Link, navigate } from '../router'
import type { Presentation } from '../types'
import { displayError } from '../utils'

export function DashboardPage() {
  const { user } = useAuth()
  const { productName } = useBrand()
  const [items, setItems] = useState<Presentation[]>([])
  const [counted, setCounted] = useState({ total: 0, slides: 0, ready: 0 })
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  /**
   * Three cards and three numbers, in two requests.
   *
   * This page used to fetch every deck the account has, a hundred at a time, and
   * work the numbers out in the browser: 2,656 decks meant twenty-seven requests
   * and about four seconds before the front page settled, and it grew with every
   * deck anybody made. The server counts them in one query and the cards need
   * three decks.
   */
  const load = useCallback(async () => {
    setLoading(true); setError('')
    try {
      const [page, summary] = await Promise.all([
        api.presentationPage({ limit: 3 }),
        api.workspaceSummary(),
      ])
      setItems(page.items)
      setCounted(summary)
    } catch (err) { setError(displayError(err)) } finally { setLoading(false) }
  }, [])
  useEffect(() => { void load() }, [load])

  const firstName = (user?.name || user?.email?.split('@')[0] || '사용자').split(' ')[0]
  const { ready, slides } = counted

  return (
    <AppShell>
      <section className="dashboard-hero">
        <div className="hero-copy"><span className="eyebrow">MY WORKSPACE</span><h1>{firstName}님, 어떤 이야기를<br />만들어 볼까요?</h1><p>아이디어를 입력하면 {productName}이 구조와 콘텐츠를 슬라이드로 완성합니다.</p></div>
        <div className="hero-orb" aria-hidden="true"><i /><i /><i /><span><Sparkles size={23} /></span></div>
        <button className="prompt-launcher" onClick={() => navigate('/create')}><WandSparkles size={20} /><span>만들고 싶은 프레젠테이션을 설명해 주세요</span><kbd>시작하기 <ArrowRight size={14} /></kbd></button>
      </section>

      <section className="quick-start-grid" aria-label="빠른 시작">
        <button onClick={() => navigate('/create')} className="quick-action primary"><span><Sparkles size={21} /></span><div><strong>새 초안 만들기</strong><p>주제와 옵션으로 편집 가능한 덱을 구성해요</p></div><ArrowRight size={18} /></button>
        <button onClick={() => navigate('/presentations')} className="quick-action"><span><BookOpen size={21} /></span><div><strong>내 작업 이어가기</strong><p>저장된 덱을 열어 편집하고 내보내요</p></div><ArrowRight size={18} /></button>
        <button onClick={() => navigate('/profile')} className="quick-action"><span><CircleUserRound size={21} /></span><div><strong>개인화 설정</strong><p>직무와 청중 맥락을 생성에 반영해요</p></div><ArrowRight size={18} /></button>
        <button onClick={() => navigate('/guide')} className="quick-action"><span><LifeBuoy size={21} /></span><div><strong>사용 가이드</strong><p>브리프 쓰는 법부터 발표·단축키까지</p></div><ArrowRight size={18} /></button>
      </section>

      <section className="section-block">
        <div className="section-heading"><div><h2>최근 프레젠테이션</h2><p>마지막으로 작업한 콘텐츠를 이어서 완성하세요.</p></div><Link to="/presentations" className="text-link">전체 보기 <ArrowRight size={15} /></Link></div>
        {loading ? <div className="card-loading-grid">{[1, 2, 3].map((item) => <div key={item} className="presentation-card skeleton-card"><span /><i /><i /></div>)}</div> : error ? <ErrorState message={error} onRetry={() => void load()} /> : items.length === 0 ? <EmptyState icon={<BookOpen size={25} />} title="첫 프레젠테이션을 만들어 보세요" description={`한 문장의 아이디어로 시작할 수 있어요. 구성은 ${productName}이 도와드립니다.`} action={<Button onClick={() => navigate('/create')}><Plus size={16} /> 새로 만들기</Button>} /> : <div className="presentation-grid">{items.slice(0, 3).map((item) => <PresentationCard key={item.id} presentation={item} />)}</div>}
      </section>

      <section className="dashboard-lower-grid">
        <article className="usage-card"><div className="card-eyebrow"><Clock3 size={15} /> 워크스페이스 현황</div><div className="usage-content"><div><strong>{counted.total}</strong><span>전체 프레젠테이션</span></div><div><strong>{slides}</strong><span>저장된 슬라이드</span></div><div><strong>{ready}</strong><span>내보내기 준비 완료</span></div></div></article>
        <article className="tip-card"><span className="tip-icon"><Sparkles size={18} /></span><div><span className="eyebrow">PTIUM TIP</span><h3>더 좋은 결과를 위한 한 문장</h3><p>청중, 발표 목적, 원하는 분위기를 함께 적으면 훨씬 정확한 자료가 만들어져요.</p><Link to="/create">프롬프트 작성해 보기 <ArrowRight size={14} /></Link><Link to="/guide">사용 가이드 읽기 <ArrowRight size={14} /></Link></div></article>
      </section>
    </AppShell>
  )
}
