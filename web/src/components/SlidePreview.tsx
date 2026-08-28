import { useEffect, useRef, useState } from 'react'
import { ImageOff, LoaderCircle } from 'lucide-react'

/**
 * Renders a server-side slide or layout preview. The SVG is fetched with the
 * session credentials and shown through an object URL, so the document stays
 * inert even though it is generated markup.
 *
 * A preview is drawn by the server on request, and it is asked for only once it
 * is near enough to be looked at. Opening a deck of fifty slides drew all fifty
 * thumbnails while six of them were on the screen: the rail is seven thousand
 * pixels tall in a nine hundred pixel window, so forty-four of those drawings
 * were for nobody, and a longer deck wasted proportionally more.
 *
 * The margin is generous on purpose — scrolling should meet a picture that is
 * already there rather than a spinner that starts when it arrives.
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
  const holder = useRef<HTMLDivElement | null>(null)
  // Whether this preview is close enough to the window to be worth drawing. A
  // browser without an observer says yes at once, which is what every browser
  // used to do.
  const [near, setNear] = useState(typeof IntersectionObserver === 'undefined')

  useEffect(() => {
    if (near || typeof IntersectionObserver === 'undefined') return
    const element = holder.current
    if (!element) return
    const watcher = new IntersectionObserver((entries) => {
      if (entries.some((entry) => entry.isIntersecting)) {
        setNear(true)
        watcher.disconnect()
      }
    }, { rootMargin: '600px' })
    watcher.observe(element)
    return () => watcher.disconnect()
  }, [near])

  useEffect(() => {
    if (!near) return
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
  }, [cacheKey, near])

  if (failed) return <div className={`slide-preview empty ${className || ''}`}><ImageOff size={18} /><span>미리보기를 불러오지 못했습니다</span></div>
  if (!source) return <div ref={holder} className={`slide-preview loading ${className || ''}`}>{near ? <LoaderCircle className="spin" size={18} /> : null}</div>
  return <img className={`slide-preview ${className || ''}`} src={source} alt={alt} loading="lazy" />
}
