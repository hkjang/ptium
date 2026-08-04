import { useCallback, useEffect, useRef, useState } from 'react'
import { ImagePlus, Loader as LoaderIcon, Trash2, Type } from 'lucide-react'
import { api } from '../api/client'
import { Button, EmptyState } from './UI'
import { displayError } from '../utils'

interface Asset {
  id: string
  name: string
  contentType: string
  sizeBytes: number
  width: number
  height: number
}

/**
 * The images a deck can place. Uploading here and writing `::image <name>` in the
 * source are two halves of one action, so the panel hands the name straight to
 * the editor rather than making anyone retype it.
 */
export function AssetLibrary({ onInsert, notify }: {
  onInsert?: (name: string) => void
  notify: (message: string, tone?: 'success' | 'error') => void
}) {
  const [assets, setAssets] = useState<Asset[]>([])
  const [previews, setPreviews] = useState<Record<string, string>>({})
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [dragging, setDragging] = useState(false)
  const input = useRef<HTMLInputElement>(null)

  const reload = useCallback(async () => {
    setLoading(true)
    try {
      setAssets(await api.assets())
    } catch (err) { notify(displayError(err), 'error') } finally { setLoading(false) }
  }, [notify])

  useEffect(() => { void reload() }, [reload])

  // Thumbnails are fetched with the session's credentials, so the blob URLs have
  // to be released when the list changes or the panel closes.
  useEffect(() => {
    let active = true
    const created: string[] = []
    void Promise.all(assets.map(async (asset) => {
      try {
        const url = await api.assetImage(asset.id)
        created.push(url)
        if (active) setPreviews((current) => ({ ...current, [asset.id]: url }))
      } catch { /* a thumbnail that will not load simply stays blank */ }
    }))
    return () => { active = false; created.forEach(URL.revokeObjectURL) }
  }, [assets])

  const upload = async (files: FileList | null) => {
    if (!files || files.length === 0) return
    setBusy(true)
    try {
      for (const file of Array.from(files)) {
        await api.uploadAsset(file)
      }
      notify(files.length === 1 ? '이미지를 올렸습니다.' : `${files.length}개를 올렸습니다.`)
      await reload()
    } catch (err) { notify(displayError(err), 'error') } finally { setBusy(false) }
  }

  const remove = async (asset: Asset) => {
    setBusy(true)
    try {
      await api.deleteAsset(asset.id)
      notify(`${asset.name}을 삭제했습니다.`)
      await reload()
    } catch (err) { notify(displayError(err), 'error') } finally { setBusy(false) }
  }

  return (
    <div className="asset-library">
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
        </div>
        <Button variant="ghost" onClick={() => input.current?.click()} disabled={busy}>파일 선택</Button>
        <input ref={input} type="file" accept="image/*" multiple hidden
          onChange={(event) => { void upload(event.target.files); event.target.value = '' }} />
      </div>

      {loading ? <div className="asset-loading"><LoaderIcon size={15} className="spin" /> 불러오는 중…</div>
        : assets.length === 0
          ? <EmptyState title="이미지가 없습니다" description="올린 이미지는 코드에서 ::image 이름 으로 불러 씁니다." />
          : <ul className="asset-grid">
            {assets.map((asset) => (
              <li key={asset.id}>
                <div className="asset-thumb">
                  {previews[asset.id]
                    ? <img src={previews[asset.id]} alt={asset.name} />
                    : <span />}
                </div>
                <strong title={asset.name}>{asset.name}</strong>
                <small>{asset.width > 0 ? `${asset.width}×${asset.height}` : asset.contentType.replace('image/', '')} · {Math.max(1, Math.round(asset.sizeBytes / 1024))}KB</small>
                <div className="asset-actions">
                  {onInsert && <button type="button" onClick={() => onInsert(asset.name)} title="코드에 ::image 넣기"><Type size={13} /> 코드에 넣기</button>}
                  <button type="button" className="danger" onClick={() => void remove(asset)} disabled={busy} title="삭제"><Trash2 size={13} /></button>
                </div>
              </li>
            ))}
          </ul>}
    </div>
  )
}
