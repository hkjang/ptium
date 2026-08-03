import { useEffect, useState } from 'react'
import { ImageOff, LoaderCircle } from 'lucide-react'

/**
 * Renders a server-side slide or layout preview. The SVG is fetched with the
 * session credentials and shown through an object URL, so the document stays
 * inert even though it is generated markup.
 */
export function SlidePreview({ load, alt, className, cacheKey }: {
  load: () => Promise<string>
  alt: string
  className?: string
  /** Change to force a reload, for example after an edit. */
  cacheKey?: string | number
}) {
  const [source, setSource] = useState('')
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    let active = true
    let created = ''
    setFailed(false)
    load().then((url) => {
      if (!active) { URL.revokeObjectURL(url); return }
      created = url
      setSource(url)
    }).catch(() => { if (active) setFailed(true) })
    return () => {
      active = false
      if (created) URL.revokeObjectURL(created)
    }
    // The loader is recreated on every render by design; the cache key decides
    // when a refetch is actually wanted.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [cacheKey])

  if (failed) return <div className={`slide-preview empty ${className || ''}`}><ImageOff size={18} /><span>미리보기를 불러오지 못했습니다</span></div>
  if (!source) return <div className={`slide-preview loading ${className || ''}`}><LoaderCircle className="spin" size={18} /></div>
  return <img className={`slide-preview ${className || ''}`} src={source} alt={alt} loading="lazy" />
}
