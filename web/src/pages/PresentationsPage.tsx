import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { ArchiveRestore, FileUp, Plus, Search, Trash2 } from 'lucide-react'
import { api, beingWritten } from '../api/client'
import { AppShell } from '../components/AppShell'
import { PresentationCard } from '../components/PresentationCard'
import { Button, EmptyState, ErrorState, Input, Modal } from '../components/UI'
import { useToast } from '../components/Toast'
import { navigate, useLocation } from '../router'
import type { Presentation } from '../types'
import { displayError } from '../utils'

type LibraryFilter = 'all' | 'ready' | 'draft' | 'generating' | 'trash'

export function PresentationsPage() {
  const location = useLocation()
  const [items, setItems] = useState<Presentation[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [query, setQuery] = useState('')
  const [filter, setFilter] = useState<LibraryFilter>('all')
  const [target, setTarget] = useState<Presentation | null>(null)
  const [deleteForever, setDeleteForever] = useState(false)
  const [emptying, setEmptying] = useState(false)
  const [working, setWorking] = useState(false)
  const [importing, setImporting] = useState(false)
  const importInput = useRef<HTMLInputElement>(null)
  const { showToast } = useToast()
  const trash = filter === 'trash'

  // What the account holds, and what is on screen. The server counts and
  // searches; the page asks for one screenful at a time and for more when the
  // reader asks for more.
  const [total, setTotal] = useState(0)
  const [more, setMore] = useState(false)
  const load = useCallback(async (search: string) => {
    setLoading(true); setError('')
    try {
      const page = await api.presentationPage({ deleted: trash, q: search })
      setItems(page.items); setTotal(page.total)
    } catch (err) { setError(displayError(err)) } finally { setLoading(false) }
  }, [trash])
  // A search runs when the typing stops, not on every keystroke.
  useEffect(() => {
    const timer = window.setTimeout(() => { void load(query) }, query ? 250 : 0)
    return () => window.clearTimeout(timer)
  }, [load, query])
  useEffect(() => { setQuery(new URLSearchParams(location.search).get('q') || '') }, [location.search])

  const loadMore = useCallback(async () => {
    setMore(true)
    try {
      const page = await api.presentationPage({ deleted: trash, q: query, offset: items.length })
      setItems((current) => {
        const seen = new Set(current.map((item) => item.id))
        return [...current, ...page.items.filter((item) => !seen.has(item.id))]
      })
      setTotal(page.total)
    } catch (err) { setError(displayError(err)) } finally { setMore(false) }
  }, [trash, query, items.length])

  const filtered = useMemo(() => items.filter((item) =>
    // "생성 중" means on its way: waiting for a worker counts, and used to be
    // the same status as being written.
    trash || filter === 'all' || item.status === filter ||
    (filter === 'generating' && beingWritten(item.status))), [items, filter, trash])

  const duplicate = async (presentation: Presentation) => {
    setWorking(true)
    try {
      const copied = await api.duplicatePresentation(presentation.id)
      setItems((current) => [copied, ...current])
      showToast('프레젠테이션을 복제했습니다.')
    } catch (err) { showToast(displayError(err), 'error') } finally { setWorking(false) }
  }

  const restore = async (presentation: Presentation) => {
    setWorking(true)
    try {
      await api.restoreDeletedPresentation(presentation.id)
      setItems((current) => current.filter((item) => item.id !== presentation.id))
      showToast('프레젠테이션을 복원했습니다.')
    } catch (err) { showToast(displayError(err), 'error') } finally { setWorking(false) }
  }

  // The recycle bin could only be cleared a deck at a time, which is no way to
  // clear a thousand of them — so it filled up and stayed full. Nothing goes
  // without being asked for: the count is on the button and again here.
  const emptyTrash = async () => {
    setWorking(true)
    try {
      const said = await api.emptyTrash()
      setItems([]); setTotal(0); setEmptying(false)
      showToast(`휴지통에서 ${said.deleted}개를 영구 삭제했습니다.`)
      await load(query)
    } catch (err) { showToast(displayError(err), 'error') } finally { setWorking(false) }
  }

  const remove = async () => {
    if (!target) return
    setWorking(true)
    try {
      if (deleteForever) {
        await api.permanentlyDeletePresentation(target.id)
        showToast('프레젠테이션을 영구 삭제했습니다.')
      } else {
        await api.deletePresentation(target.id)
        showToast('프레젠테이션을 휴지통으로 이동했습니다.')
      }
      setItems((current) => current.filter((item) => item.id !== target.id))
      setTarget(null)
    } catch (err) { showToast(displayError(err), 'error') } finally { setWorking(false) }
  }

  /**
   * A deck someone already has, read in as text and redrawn in a Ptium design.
   * The file is theirs; what Ptium keeps is the argument.
   */
  const importDeck = async (file?: File) => {
    if (!file) return
    setImporting(true)
    showToast(`${file.name}을 읽고 있습니다…`)
    try {
      const result = await api.importPresentation(file)
      // Only what the import did with their file. What the compiler adjusted is
      // in the response for anyone debugging a template, and is not the thing to
      // greet someone with.
      showToast([`${result.slides}장을 가져왔습니다.`, ...result.warnings].join(' '))
      navigate(`/presentations/${result.presentation.id}/editor`)
    } catch (err) { showToast(displayError(err), 'error') } finally { setImporting(false) }
  }

  const askDelete = (presentation: Presentation, permanent = false) => {
    setDeleteForever(permanent)
    setTarget(presentation)
  }

  return (
    <AppShell title="프레젠테이션" eyebrow="MY WORKSPACE" actions={<>
      <Button variant="secondary" disabled={importing} onClick={() => importInput.current?.click()}
        title="발표자료(.pptx)뿐 아니라 엑셀·워드·PDF·CSV·마크다운도 슬라이드가 됩니다. 각 장에 출처가 붙습니다.">
        <FileUp size={16} /> {importing ? '가져오는 중…' : '기존 자료 가져오기'}
      </Button>
      <input ref={importInput} type="file" accept=".pptx,.potx,.xlsx,.csv,.tsv,.docx,.pdf,.md,.markdown,.txt" hidden
        onChange={(event) => { void importDeck(event.target.files?.[0]); event.target.value = '' }} />
      <Button onClick={() => navigate('/create')}><Plus size={16} /> 새로 만들기</Button>
    </>}>
      <div className="library-toolbar">
        <div className="search-box"><Search size={17} /><Input placeholder="제목과 프롬프트 검색" value={query} onChange={(event) => setQuery(event.target.value)} /></div>
        <div className="filter-tabs" role="tablist">
          <button className={filter === 'all' ? 'active' : ''} onClick={() => setFilter('all')}>전체 {!trash && <span>{total || items.length}</span>}</button>
          <button className={filter === 'ready' ? 'active' : ''} onClick={() => setFilter('ready')}>완료</button>
          <button className={filter === 'draft' ? 'active' : ''} onClick={() => setFilter('draft')}>초안</button>
          <button className={filter === 'generating' ? 'active' : ''} onClick={() => setFilter('generating')}>생성 중</button>
          <button className={filter === 'trash' ? 'active' : ''} onClick={() => setFilter('trash')}><Trash2 size={13} /> 휴지통</button>
        </div>
        {trash && items.length > 0 && <Button variant="secondary" disabled={working} onClick={() => setEmptying(true)}>
          <Trash2 size={15} /> 휴지통 비우기 ({total || items.length})
        </Button>}
      </div>
      {loading ? <div className="presentation-grid">{[1,2,3,4,5,6].map((item) => <div key={item} className="presentation-card skeleton-card"><span /><i /><i /></div>)}</div> : error ? <ErrorState message={error} onRetry={() => void load(query)} /> : filtered.length === 0 ? <EmptyState icon={trash ? <ArchiveRestore size={25} /> : undefined} title={trash ? '휴지통이 비어 있습니다' : query ? '검색 결과가 없습니다' : '아직 프레젠테이션이 없습니다'} description={trash ? '삭제한 프레젠테이션을 이곳에서 복원할 수 있습니다.' : query ? '다른 검색어나 필터를 사용해 보세요.' : '첫 번째 아이디어를 Ptium과 함께 완성해 보세요.'} action={!trash && !query ? <Button onClick={() => navigate('/create')}><Plus size={16} /> 새로 만들기</Button> : undefined} /> : <div className="presentation-grid library-grid">{filtered.map((item) => <PresentationCard key={item.id} presentation={item} onDuplicate={trash || working ? undefined : duplicate} onDelete={trash || working ? undefined : (entry) => askDelete(entry)} onRestore={!trash || working ? undefined : restore} onDeleteForever={!trash || working ? undefined : (entry) => askDelete(entry, true)} />)}</div>}
      {!loading && !error && items.length < total && <div className="load-more">
        <Button variant="secondary" disabled={more} onClick={() => void loadMore()}>
          {more ? '불러오는 중…' : `더 보기 (${items.length} / ${total})`}
        </Button>
      </div>}
      <Modal open={emptying} onClose={() => { if (!working) setEmptying(false) }} title="휴지통을 비울까요?"
        description="휴지통에 있는 프레젠테이션이 버전 이력까지 모두 삭제되며 복구할 수 없습니다. 휴지통 밖의 프레젠테이션은 그대로 있습니다."
        footer={<><Button variant="secondary" disabled={working} onClick={() => setEmptying(false)}>취소</Button>
          <Button variant="danger" disabled={working} onClick={() => void emptyTrash()}><Trash2 size={15} /> {working ? '삭제 중…' : `${total || items.length}개 영구 삭제`}</Button></>}>
        <div className="delete-preview"><strong>휴지통 {total || items.length}개</strong><span>이 계정이 지운 프레젠테이션</span></div>
      </Modal>
      <Modal open={Boolean(target)} onClose={() => { if (!working) setTarget(null) }} title={deleteForever ? '영구 삭제할까요?' : '휴지통으로 이동할까요?'} description={deleteForever ? '프레젠테이션과 모든 버전 이력이 삭제되며 복구할 수 없습니다.' : '휴지통에서 언제든 다시 복원할 수 있습니다.'} footer={<><Button variant="secondary" disabled={working} onClick={() => setTarget(null)}>취소</Button><Button variant="danger" disabled={working} onClick={() => void remove()}><Trash2 size={15} /> {working ? '처리 중…' : deleteForever ? '영구 삭제' : '휴지통으로 이동'}</Button></>}><div className="delete-preview"><strong>{target?.title}</strong><span>{target?.slideCount || 0}개 슬라이드</span></div></Modal>
    </AppShell>
  )
}
