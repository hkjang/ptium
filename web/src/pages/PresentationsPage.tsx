import { useCallback, useEffect, useMemo, useState } from 'react'
import { Plus, Search, Trash2 } from 'lucide-react'
import { api } from '../api/client'
import { AppShell } from '../components/AppShell'
import { PresentationCard } from '../components/PresentationCard'
import { Button, EmptyState, ErrorState, Input, Modal } from '../components/UI'
import { useToast } from '../components/Toast'
import { navigate, useLocation } from '../router'
import type { Presentation } from '../types'
import { displayError } from '../utils'

export function PresentationsPage() {
  const location = useLocation()
  const [items, setItems] = useState<Presentation[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [query, setQuery] = useState('')
  const [filter, setFilter] = useState('all')
  const [target, setTarget] = useState<Presentation | null>(null)
  const [deleting, setDeleting] = useState(false)
  const { showToast } = useToast()
  const load = useCallback(async () => { setLoading(true); setError(''); try { setItems(await api.presentations()) } catch (err) { setError(displayError(err)) } finally { setLoading(false) } }, [])
  useEffect(() => { void load() }, [load])
  useEffect(() => { setQuery(new URLSearchParams(location.search).get('q') || '') }, [location.search])
  const filtered = useMemo(() => items.filter((item) => (filter === 'all' || item.status === filter) && item.title.toLowerCase().includes(query.toLowerCase())), [items, filter, query])
  const remove = async () => {
    if (!target) return
    setDeleting(true)
    try { await api.deletePresentation(target.id); setItems((current) => current.filter((item) => item.id !== target.id)); showToast('프레젠테이션을 삭제했습니다.'); setTarget(null) } catch (err) { showToast(displayError(err), 'error') } finally { setDeleting(false) }
  }
  return (
    <AppShell title="프레젠테이션" eyebrow="MY WORKSPACE" actions={<Button onClick={() => navigate('/create')}><Plus size={16} /> 새로 만들기</Button>}>
      <div className="library-toolbar">
        <div className="search-box"><Search size={17} /><Input placeholder="프레젠테이션 검색" value={query} onChange={(event) => setQuery(event.target.value)} /></div>
        <div className="filter-tabs" role="tablist"><button className={filter === 'all' ? 'active' : ''} onClick={() => setFilter('all')}>전체 <span>{items.length}</span></button><button className={filter === 'ready' ? 'active' : ''} onClick={() => setFilter('ready')}>완료</button><button className={filter === 'draft' ? 'active' : ''} onClick={() => setFilter('draft')}>초안</button><button className={filter === 'generating' ? 'active' : ''} onClick={() => setFilter('generating')}>생성 중</button></div>
      </div>
      {loading ? <div className="presentation-grid">{[1,2,3,4,5,6].map((item) => <div key={item} className="presentation-card skeleton-card"><span /><i /><i /></div>)}</div> : error ? <ErrorState message={error} onRetry={() => void load()} /> : filtered.length === 0 ? <EmptyState title={query ? '검색 결과가 없습니다' : '아직 프레젠테이션이 없습니다'} description={query ? '다른 검색어나 필터를 사용해 보세요.' : '첫 번째 아이디어를 Ptium과 함께 완성해 보세요.'} action={!query ? <Button onClick={() => navigate('/create')}><Plus size={16} /> 새로 만들기</Button> : undefined} /> : <div className="presentation-grid library-grid">{filtered.map((item) => <PresentationCard key={item.id} presentation={item} onDelete={setTarget} />)}</div>}
      <Modal open={Boolean(target)} onClose={() => setTarget(null)} title="프레젠테이션을 삭제할까요?" description="삭제한 프레젠테이션은 복구할 수 없습니다." footer={<><Button variant="secondary" onClick={() => setTarget(null)}>취소</Button><Button variant="danger" disabled={deleting} onClick={() => void remove()}><Trash2 size={15} /> {deleting ? '삭제 중…' : '삭제'}</Button></>}><div className="delete-preview"><strong>{target?.title}</strong><span>{target?.slideCount || 0}개 슬라이드</span></div></Modal>
    </AppShell>
  )
}
