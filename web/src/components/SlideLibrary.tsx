import { useCallback, useEffect, useRef, useState } from 'react'
import {
  BookmarkPlus, Clock3, Layers, Loader as LoaderIcon, Pencil, Plus, Search, Star, Tag, Trash2, X,
} from 'lucide-react'
import { api } from '../api/client'
import { SlidePreview } from './SlidePreview'
import { Button, EmptyState } from './UI'
import type { Snippet, AssetTag } from '../types'
import { displayError, relativeDate } from '../utils'

/**
 * Slides someone keeps.
 *
 * Every deck has pages that are not really about this deck: the company
 * introduction, the team, how to reach us, the legal notice. Saving one keeps
 * its *text*, so dropping it into another deck lays it out in that deck's
 * template — a saved slide that carried its old design with it would be a
 * screenshot, and nobody wants a screenshot in their deck.
 */
export function SlideLibrary({ presentationId, onInsert, onSaveCurrent, notify, currentTitle }: {
  /** The deck being worked on: saved slides are previewed in its design. */
  presentationId: string
  onInsert: (snippet: Snippet) => Promise<void> | void
  /** Saves the slide the editor is on. Absent where there is nothing to save. */
  onSaveCurrent?: (name: string) => Promise<void> | void
  notify: (message: string, tone?: 'success' | 'error') => void
  currentTitle?: string
}) {
  const [snippets, setSnippets] = useState<Snippet[]>([])
  const [tags, setTags] = useState<AssetTag[]>([])
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState('')
  const [search, setSearch] = useState('')
  const [tag, setTag] = useState('')
  const [favoritesOnly, setFavoritesOnly] = useState(false)
  const [sort, setSort] = useState('lastUsed')
  const [renaming, setRenaming] = useState('')
  const [draftName, setDraftName] = useState('')
  const [tagging, setTagging] = useState('')
  const [draftTags, setDraftTags] = useState('')
  const [saving, setSaving] = useState(false)
  const [saveName, setSaveName] = useState('')
  const saveInput = useRef<HTMLInputElement>(null)

  const reload = useCallback(async () => {
    setLoading(true)
    try {
      const [items, available] = await Promise.all([
        api.snippets({ q: search, tag, favorite: favoritesOnly, sort }),
        api.snippetTags().catch(() => [] as AssetTag[]),
      ])
      setSnippets(items)
      setTags(available)
    } catch (err) { notify(displayError(err), 'error') } finally { setLoading(false) }
  }, [notify, search, tag, favoritesOnly, sort])

  useEffect(() => {
    const timer = window.setTimeout(() => { void reload() }, search ? 250 : 0)
    return () => window.clearTimeout(timer)
  }, [reload, search])

  useEffect(() => { if (saving) saveInput.current?.focus() }, [saving])

  const insert = async (snippet: Snippet) => {
    setBusy(snippet.id)
    try {
      await onInsert(snippet)
      // The count is part of what makes the shelf useful, so it is read back.
      setSnippets((current) => current.map((item) => item.id === snippet.id
        ? { ...item, useCount: item.useCount + 1, lastUsed: new Date().toISOString() } : item))
    } catch (err) { notify(displayError(err), 'error') } finally { setBusy('') }
  }

  const save = async () => {
    if (!onSaveCurrent) return
    const name = saveName.trim() || currentTitle?.trim() || ''
    setBusy('save')
    try {
      await onSaveCurrent(name)
      setSaving(false); setSaveName('')
      await reload()
    } catch (err) { notify(displayError(err), 'error') } finally { setBusy('') }
  }

  const toggleFavorite = async (snippet: Snippet) => {
    const next = !snippet.favorite
    setSnippets((current) => current.map((item) => item.id === snippet.id ? { ...item, favorite: next } : item))
    try { await api.favoriteSnippet(snippet.id, next) } catch (err) {
      setSnippets((current) => current.map((item) => item.id === snippet.id ? { ...item, favorite: !next } : item))
      notify(displayError(err), 'error')
    }
  }

  const commitRename = async (snippet: Snippet) => {
    const name = draftName.trim()
    setRenaming('')
    if (!name || name === snippet.name) return
    try {
      const updated = await api.updateSnippet(snippet.id, { name })
      setSnippets((current) => current.map((item) => item.id === snippet.id ? updated : item))
    } catch (err) { notify(displayError(err), 'error') }
  }

  const commitTags = async (snippet: Snippet) => {
    const next = draftTags.split(/[,\s]+/).map((value) => value.trim()).filter(Boolean).slice(0, 8)
    setTagging('')
    try {
      const updated = await api.updateSnippet(snippet.id, { tags: next })
      setSnippets((current) => current.map((item) => item.id === snippet.id ? updated : item))
      setTags(await api.snippetTags().catch(() => tags))
    } catch (err) { notify(displayError(err), 'error') }
  }

  const remove = async (snippet: Snippet) => {
    if (!window.confirm(`"${snippet.name}"을 라이브러리에서 지울까요? 이미 넣어 둔 슬라이드는 그대로 남습니다.`)) return
    setBusy(snippet.id)
    try {
      await api.deleteSnippet(snippet.id)
      await reload()
    } catch (err) { notify(displayError(err), 'error') } finally { setBusy('') }
  }

  const filtering = Boolean(search || tag || favoritesOnly)

  return (
    <div className="slide-library">
      {onSaveCurrent && (saving
        ? <div className="snippet-save">
            <input ref={saveInput} value={saveName} placeholder={currentTitle || '저장할 이름'}
              onChange={(event) => setSaveName(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === 'Enter') { event.preventDefault(); void save() }
                if (event.key === 'Escape') { setSaving(false); setSaveName('') }
              }} />
            <Button size="small" disabled={busy === 'save'} onClick={() => void save()}>저장</Button>
            <button type="button" className="icon-button small" onClick={() => { setSaving(false); setSaveName('') }} aria-label="취소"><X size={14} /></button>
          </div>
        : <Button variant="secondary" className="snippet-save-open" onClick={() => setSaving(true)}>
            <BookmarkPlus size={15} /> 지금 슬라이드를 라이브러리에 저장
          </Button>)}

      <div className="asset-toolbar">
        <label className="asset-search">
          <Search size={14} />
          <input value={search} onChange={(event) => setSearch(event.target.value)}
            placeholder="이름 · 내용으로 찾기" aria-label="저장한 슬라이드 검색" />
          {search && <button type="button" onClick={() => setSearch('')} aria-label="검색어 지우기"><X size={13} /></button>}
        </label>
        <div className="asset-filters">
          <button type="button" className={favoritesOnly ? 'active' : ''} onClick={() => setFavoritesOnly((value) => !value)}>
            <Star size={12} /> 즐겨찾기
          </button>
          <button type="button" className={sort === 'lastUsed' ? 'active' : ''} onClick={() => setSort('lastUsed')}>최근 사용</button>
          <button type="button" className={sort === 'used' ? 'active' : ''} onClick={() => setSort('used')}>자주 사용</button>
          <button type="button" className={sort === 'recent' ? 'active' : ''} onClick={() => setSort('recent')}>최근 저장</button>
          <button type="button" className={sort === 'name' ? 'active' : ''} onClick={() => setSort('name')}>이름순</button>
        </div>
        {tags.length > 0 && <div className="asset-tags">
          <Tag size={12} />
          <button type="button" className={tag === '' ? 'active' : ''} onClick={() => setTag('')}>전체</button>
          {tags.map((item) => (
            <button key={item.name} type="button" className={tag === item.name ? 'active' : ''}
              onClick={() => setTag(tag === item.name ? '' : item.name)}>{item.name} <i>{item.count}</i></button>
          ))}
        </div>}
      </div>

      {loading ? <div className="asset-loading"><LoaderIcon size={15} className="spin" /> 불러오는 중…</div>
        : snippets.length === 0
          ? filtering
            ? <EmptyState title="찾는 슬라이드가 없습니다" description="검색어나 태그를 지우고 다시 보세요."
                action={<Button variant="secondary" size="small" onClick={() => { setSearch(''); setTag(''); setFavoritesOnly(false) }}>필터 지우기</Button>} />
            : <EmptyState icon={<Layers size={24} />} title="저장한 슬라이드가 없습니다"
                description="회사 소개·팀·연락처처럼 매번 다시 만드는 슬라이드를 저장해 두면, 어느 덱에서든 그 덱의 디자인으로 들어갑니다." />
          : <ul className="snippet-grid">
            {snippets.map((snippet) => (
              <li key={snippet.id} className={snippet.favorite ? 'favorite' : ''}>
                <div className="snippet-thumb">
                  {/* Drawn in this deck's template: what it will look like here,
                      not what it looked like where it was saved. */}
                  <SlidePreview cacheKey={`snippet-${snippet.id}-${presentationId}-${snippet.updatedAt}`}
                    alt={`${snippet.name} 미리보기`}
                    load={() => api.snippetPreview(snippet.id, presentationId, 420)} />
                  <button type="button" className={`asset-star ${snippet.favorite ? 'on' : ''}`}
                    onClick={() => void toggleFavorite(snippet)} aria-pressed={snippet.favorite}
                    title={snippet.favorite ? '즐겨찾기 해제' : '즐겨찾기'}><Star size={13} /></button>
                </div>
                {renaming === snippet.id
                  ? <input className="asset-rename" autoFocus value={draftName} aria-label="이름"
                      onChange={(event) => setDraftName(event.target.value)}
                      onBlur={() => void commitRename(snippet)}
                      onKeyDown={(event) => {
                        if (event.key === 'Enter') { event.preventDefault(); void commitRename(snippet) }
                        if (event.key === 'Escape') setRenaming('')
                      }} />
                  : <strong title={snippet.name} onDoubleClick={() => { setRenaming(snippet.id); setDraftName(snippet.name) }}>{snippet.name}</strong>}
                <small>
                  {snippet.useCount > 0 ? <b>{snippet.useCount}번 사용</b> : '아직 쓰지 않음'}
                  {snippet.lastUsed && <> · <Clock3 size={10} /> {relativeDate(snippet.lastUsed)}</>}
                </small>
                {tagging === snippet.id
                  ? <input className="asset-rename" autoFocus value={draftTags} aria-label="태그"
                      placeholder="쉼표로 구분 (회사소개, 표준)"
                      onChange={(event) => setDraftTags(event.target.value)}
                      onBlur={() => void commitTags(snippet)}
                      onKeyDown={(event) => {
                        if (event.key === 'Enter') { event.preventDefault(); void commitTags(snippet) }
                        if (event.key === 'Escape') setTagging('')
                      }} />
                  : snippet.tags.length > 0 && <div className="asset-chiplist">
                      {snippet.tags.map((item) => <button key={item} type="button" onClick={() => setTag(item)}>{item}</button>)}
                    </div>}
                <div className="asset-actions">
                  <button type="button" className="asset-action-main" disabled={busy === snippet.id}
                    onClick={() => void insert(snippet)} title="현재 슬라이드 뒤에 넣기">
                    <Plus size={13} /> 이 덱에 넣기
                  </button>
                  <button type="button" onClick={() => { setRenaming(snippet.id); setDraftName(snippet.name) }} title="이름 바꾸기"><Pencil size={13} /></button>
                  <button type="button" onClick={() => { setTagging(snippet.id); setDraftTags(snippet.tags.join(', ')) }} title="태그"><Tag size={13} /></button>
                  <button type="button" className="danger" disabled={busy === snippet.id} onClick={() => void remove(snippet)} title="삭제"><Trash2 size={13} /></button>
                </div>
              </li>
            ))}
          </ul>}
    </div>
  )
}
