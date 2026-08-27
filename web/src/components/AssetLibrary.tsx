import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  Check, Clock3, ImagePlus, Loader as LoaderIcon, Pencil, Plus, Search, Star, Tag, Trash2, Type, X,
} from 'lucide-react'
import { api } from '../api/client'
import { Button, EmptyState } from './UI'
import type { Asset, AssetTag } from '../types'
import { displayError, relativeDate } from '../utils'

export type { Asset }

/** How a library is looked through. Each of these is a different question. */
export const assetSorts = [
  { key: 'recent', label: '최근 올림' },
  { key: 'lastUsed', label: '최근 사용' },
  { key: 'used', label: '자주 사용' },
  { key: 'name', label: '이름순' },
] as const

/**
 * Someone's own picture library.
 *
 * An upload list becomes a library the moment it can answer "the one I always
 * use". Pinned images lead every ordering, the deck count is counted from the
 * decks that actually place each picture, and a tag is whatever word its owner
 * files it under — logo, 제품컷, 배경.
 *
 * Uploading here and writing `::image <name>` in the source are two halves of one
 * action, so the panel hands the name straight to the editor rather than making
 * anyone retype it.
 */
export function AssetLibrary({ onInsert, onPlace, notify, compact = true }: {
  onInsert?: (name: string) => void
  onPlace?: (asset: Asset) => void
  notify: (message: string, tone?: 'success' | 'error') => void
  /** The editor's narrow panel, rather than the full page. */
  compact?: boolean
}) {
  const [assets, setAssets] = useState<Asset[]>([])
  const [tags, setTags] = useState<AssetTag[]>([])
  const [previews, setPreviews] = useState<Record<string, string>>({})
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [dragging, setDragging] = useState(false)
  const [search, setSearch] = useState('')
  const [tag, setTag] = useState('')
  const [favoritesOnly, setFavoritesOnly] = useState(false)
  // In the editor's panel someone is reaching for a picture they have used
  // before; on the library page they are looking at what they just added.
  const [sort, setSort] = useState<string>(compact ? 'lastUsed' : 'recent')
  const [renaming, setRenaming] = useState('')
  const [draftName, setDraftName] = useState('')
  const [tagging, setTagging] = useState('')
  const [draftTags, setDraftTags] = useState('')
  const input = useRef<HTMLInputElement>(null)

  const reload = useCallback(async () => {
    setLoading(true)
    try {
      const [items, available] = await Promise.all([
        api.assets({ q: search, tag, favorite: favoritesOnly, sort }),
        api.assetTags().catch(() => [] as AssetTag[]),
      ])
      setAssets(items)
      setTags(available)
    } catch (err) { notify(displayError(err), 'error') } finally { setLoading(false) }
  }, [notify, search, tag, favoritesOnly, sort])

  // Typing in the search box should not fire a request per keystroke.
  useEffect(() => {
    const timer = window.setTimeout(() => { void reload() }, search ? 250 : 0)
    return () => window.clearTimeout(timer)
  }, [reload, search])

  // Thumbnails are fetched with the session's credentials, so each one is a blob
  // URL this component owns and must release.
  //
  // They are kept for as long as the library is open rather than rebuilt per
  // list: filtering changes the list several times a second, and a URL revoked
  // while an <img> still points at it turns every visible thumbnail into a
  // broken image — the picture is still in the library, it is just filtered out
  // of view, and it will be back. The key carries the checksum, so a picture
  // replaced under the same name is refetched rather than shown stale.
  const previewCache = useRef<Record<string, string>>({})
  useEffect(() => {
    let active = true
    void Promise.all(assets.map(async (asset) => {
      const key = `${asset.id}:${asset.checksum || ''}`
      if (previewCache.current[key]) return
      try {
        const url = await api.assetImage(asset.id)
        if (!active) { URL.revokeObjectURL(url); return }
        previewCache.current[key] = url
        setPreviews({ ...previewCache.current })
      } catch { /* a thumbnail that will not load simply stays blank */ }
    }))
    return () => { active = false }
  }, [assets])

  // Everything goes back when the library closes.
  useEffect(() => () => {
    Object.values(previewCache.current).forEach(URL.revokeObjectURL)
    previewCache.current = {}
  }, [])

  const upload = async (files: FileList | File[] | null) => {
    if (!files || files.length === 0) return
    setBusy(true)
    try {
      let reused = 0
      for (const file of Array.from(files)) {
        const asset = await api.uploadAsset(file)
        if (asset.reused) reused++
      }
      const count = Array.from(files).length
      notify(reused === count
        // The same bytes as something already there. Saying so is the difference
        // between a library that stays tidy and one that looks broken.
        ? (count === 1 ? '이미 올려 둔 이미지입니다. 그 이미지를 씁니다.' : `${count}개 모두 이미 있는 이미지입니다.`)
        : count === 1 ? '이미지를 올렸습니다.' : `${count}개를 올렸습니다.${reused > 0 ? ` (${reused}개는 이미 있던 이미지)` : ''}`)
      await reload()
    } catch (err) { notify(displayError(err), 'error') } finally { setBusy(false) }
  }

  const remove = async (asset: Asset) => {
    if (asset.deckCount > 0 && !window.confirm(`${asset.name}은 덱 ${asset.deckCount}개에서 쓰고 있습니다. 삭제하면 그 자리는 비어 보입니다. 삭제할까요?`)) return
    setBusy(true)
    try {
      await api.deleteAsset(asset.id)
      notify(`${asset.name}을 삭제했습니다.`)
      await reload()
    } catch (err) { notify(displayError(err), 'error') } finally { setBusy(false) }
  }

  // A pin answers immediately and is corrected if the server disagrees: waiting
  // for a round trip to see a star fill in feels broken.
  const toggleFavorite = async (asset: Asset) => {
    const next = !asset.favorite
    setAssets((current) => current.map((item) => item.id === asset.id ? { ...item, favorite: next } : item))
    try { await api.favoriteAsset(asset.id, next) } catch (err) {
      setAssets((current) => current.map((item) => item.id === asset.id ? { ...item, favorite: !next } : item))
      notify(displayError(err), 'error')
    }
  }

  const commitRename = async (asset: Asset) => {
    const name = draftName.trim()
    setRenaming('')
    if (!name || name === asset.name) return
    try {
      const updated = await api.updateAsset(asset.id, { name })
      setAssets((current) => current.map((item) => item.id === asset.id ? updated : item))
      notify('이름을 바꿨습니다. 코드에서는 새 이름으로 부릅니다.')
    } catch (err) { notify(displayError(err), 'error') }
  }

  const commitTags = async (asset: Asset) => {
    const next = draftTags.split(/[,\s]+/).map((value) => value.trim()).filter(Boolean).slice(0, 8)
    setTagging('')
    try {
      const updated = await api.updateAsset(asset.id, { tags: next })
      setAssets((current) => current.map((item) => item.id === asset.id ? updated : item))
      setTags(await api.assetTags().catch(() => tags))
    } catch (err) { notify(displayError(err), 'error') }
  }

  const filtering = Boolean(search || tag || favoritesOnly)
  const pinned = useMemo(() => assets.filter((asset) => asset.favorite).length, [assets])

  return (
    <div className={`asset-library ${compact ? 'compact' : 'full'}`}>
      <div
        className={`asset-dropzone ${dragging ? 'dragging' : ''}`}
        onDragOver={(event) => { event.preventDefault(); setDragging(true) }}
        onDragLeave={() => setDragging(false)}
        onDrop={(event) => { event.preventDefault(); setDragging(false); void upload(event.dataTransfer.files) }}
      >
        <ImagePlus size={20} />
        <div>
          <strong>이미지 끌어다 놓기</strong>
          <span>PNG · JPEG · GIF · SVG, 16MB까지</span>
          <span>캔버스에 바로 붙여넣거나(Ctrl+V) 끌어다 놓아도 올라갑니다</span>
        </div>
        <Button variant="ghost" onClick={() => input.current?.click()} disabled={busy}>파일 선택</Button>
        <input ref={input} type="file" accept="image/*" multiple hidden
          onChange={(event) => { void upload(event.target.files); event.target.value = '' }} />
      </div>

      <div className="asset-toolbar">
        <label className="asset-search">
          <Search size={14} />
          <input value={search} onChange={(event) => setSearch(event.target.value)}
            placeholder="이름 · 태그로 찾기" aria-label="이미지 검색" />
          {search && <button type="button" onClick={() => setSearch('')} aria-label="검색어 지우기"><X size={13} /></button>}
        </label>
        <div className="asset-filters">
          <button type="button" className={favoritesOnly ? 'active' : ''} onClick={() => setFavoritesOnly((value) => !value)}
            title="즐겨찾기만 보기"><Star size={12} /> 즐겨찾기{pinned > 0 && !favoritesOnly ? ` ${pinned}` : ''}</button>
          {assetSorts.map((option) => (
            <button key={option.key} type="button" className={sort === option.key ? 'active' : ''}
              onClick={() => setSort(option.key)}>{option.label}</button>
          ))}
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
        : assets.length === 0
          ? filtering
            ? <EmptyState title="찾는 이미지가 없습니다" description="검색어나 태그를 지우고 다시 보세요."
                action={<Button variant="secondary" size="small" onClick={() => { setSearch(''); setTag(''); setFavoritesOnly(false) }}>필터 지우기</Button>} />
            : <EmptyState title="이미지가 없습니다" description="캡처한 이미지를 Ctrl+V로 붙여넣거나 파일을 끌어다 놓으면 여기에 쌓이고, 코드에서는 ::image 이름 으로 불러 씁니다." />
          : <ul className="asset-grid">
            {assets.map((asset) => (
              <li key={asset.id} className={asset.favorite ? 'favorite' : ''}>
                <div className="asset-thumb">
                  {previews[`${asset.id}:${asset.checksum || ''}`]
                    ? <img src={previews[`${asset.id}:${asset.checksum || ''}`]} alt={asset.name} />
                    : <span />}
                  <button type="button" className={`asset-star ${asset.favorite ? 'on' : ''}`}
                    onClick={() => void toggleFavorite(asset)}
                    title={asset.favorite ? '즐겨찾기 해제' : '즐겨찾기'} aria-pressed={asset.favorite}>
                    <Star size={13} />
                  </button>
                </div>
                {renaming === asset.id
                  ? <input className="asset-rename" autoFocus value={draftName} aria-label="이미지 이름"
                      onChange={(event) => setDraftName(event.target.value)}
                      onBlur={() => void commitRename(asset)}
                      onKeyDown={(event) => {
                        if (event.key === 'Enter') { event.preventDefault(); void commitRename(asset) }
                        if (event.key === 'Escape') setRenaming('')
                      }} />
                  : <strong title={asset.name} onDoubleClick={() => { setRenaming(asset.id); setDraftName(asset.name) }}>{asset.name}</strong>}
                <small>
                  {asset.width > 0 ? `${asset.width}×${asset.height}` : asset.contentType.replace('image/', '')}
                  {' · '}{Math.max(1, Math.round(asset.sizeBytes / 1024))}KB
                  {asset.deckCount > 0 && <> · <b>덱 {asset.deckCount}개</b></>}
                </small>
                {asset.lastUsed && <small className="asset-used"><Clock3 size={10} /> {relativeDate(asset.lastUsed)} 사용</small>}
                {tagging === asset.id
                  ? <input className="asset-rename" autoFocus value={draftTags} aria-label="태그"
                      placeholder="쉼표로 구분 (로고, 제품컷)"
                      onChange={(event) => setDraftTags(event.target.value)}
                      onBlur={() => void commitTags(asset)}
                      onKeyDown={(event) => {
                        if (event.key === 'Enter') { event.preventDefault(); void commitTags(asset) }
                        if (event.key === 'Escape') setTagging('')
                      }} />
                  : asset.tags.length > 0 && <div className="asset-chiplist">
                      {asset.tags.map((item) => <button key={item} type="button" onClick={() => setTag(item)}>{item}</button>)}
                    </div>}
                <div className="asset-actions">
                  {onPlace && <button type="button" className="asset-action-main" onClick={() => onPlace(asset)} title="현재 슬라이드에 배치"><Plus size={13} /> 슬라이드에 넣기</button>}
                  {onInsert && <button type="button" className="asset-action-main" onClick={() => onInsert(asset.name)} title="코드에 ::image 넣기"><Type size={13} /> 코드에 넣기</button>}
                  <button type="button" onClick={() => { setRenaming(asset.id); setDraftName(asset.name) }} title="이름 바꾸기"><Pencil size={13} /></button>
                  <button type="button" onClick={() => { setTagging(asset.id); setDraftTags(asset.tags.join(', ')) }} title="태그"><Tag size={13} /></button>
                  <button type="button" className="danger" onClick={() => void remove(asset)} disabled={busy} title="삭제"><Trash2 size={13} /></button>
                </div>
              </li>
            ))}
          </ul>}
      {!loading && assets.length > 0 && <p className="asset-foot"><Check size={12} /> 이름을 두 번 누르면 바로 고칠 수 있습니다. 코드에서는 <code>::image 이름</code>으로 부릅니다.</p>}
    </div>
  )
}
