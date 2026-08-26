import { useCallback, useEffect, useRef, useState } from 'react'
import { ChevronLeft, ChevronRight, LoaderCircle } from 'lucide-react'

/**
 * A deck someone was sent a link to.
 *
 * It carries no workspace, no session and no editing: whoever holds the link
 * sees the slides and nothing else. That is the point of the link — a deck is
 * written to be shown, and until now the only way to show one to a person
 * without an account here was to export the file and send it, after which the
 * deck in the inbox and the deck in Ptium drift apart.
 */
interface SharedPage { id: string; title: string }
interface SharedDeck { title: string; slideCount: number; slides?: SharedPage[]; titles: string[]; language?: string }
interface SharedComment { id: string; slideId?: string; parentId?: string; author: string; body: string; createdAt: string; resolvedAt?: string }

export function SharedDeckPage({ token }: { token: string }) {
  const [deck, setDeck] = useState<SharedDeck | null>(null)
  const [error, setError] = useState('')
  const [position, setPosition] = useState(1)
  // Looking is half of a review. This is the other half: whoever holds the link
  // says what is wrong with slide 4, under their own name, without an account.
  const [comments, setComments] = useState<SharedComment[]>([])
  const [author, setAuthor] = useState(() => window.localStorage.getItem('ptium.reviewer') || '')
  const [draft, setDraft] = useState('')
  // Answering happens under the remark it answers: a review is a conversation,
  // and an answer beside the point it answers reads as a second point.
  const [answering, setAnswering] = useState('')
  const [answer, setAnswer] = useState('')
  const [sending, setSending] = useState(false)
  // The drawing is drawn by this page rather than shown as a picture, because a
  // deck sent out for review is the one people read the links on: the source
  // behind a number, the document a slide refers to. Nothing inside an <img>
  // can be clicked.
  // Markup when the slide has a link on it, a picture when it does not: see
  // useSlideDrawings for why one slide's photograph is a reason not to hold
  // every slide as a string.
  const [drawn, setDrawn] = useState<Record<number, { markup?: string; url?: string }>>({})
  // What has been fetched, kept out of the effect's own dependencies: asking
  // again for a slide already drawn is the loop that fetching-on-state-change
  // walks into.
  const fetched = useRef<Set<number>>(new Set())

  useEffect(() => {
    let active = true
    void (async () => {
      // The next slide is pulled while this one is read, and nothing past the
      // end of the deck: a slide that does not exist cannot be cached as
      // fetched, so asking for it again is what a loop is made of.
      const last = deck?.slideCount ?? position
      for (const slide of [position, position + 1]) {
        if (slide < 1 || slide > last || fetched.current.has(slide)) continue
        fetched.current.add(slide)
        const response = await fetch(
          `/api/v1/shared/${encodeURIComponent(token)}/preview.svg?slide=${slide}&width=1600`)
        if (!response.ok) { fetched.current.delete(slide); return }
        const markup = await response.text()
        const drawing = markup.includes('<a href=')
          ? { markup }
          : { url: URL.createObjectURL(new Blob([markup], { type: 'image/svg+xml' })) }
        // Kept whatever happened while it was in flight. This effect runs again
        // as soon as the deck says how many slides it has, and dropping a
        // drawing that arrived during that changeover — while the slide stays
        // marked as fetched — is a slide that never appears at all.
        setDrawn((current) => ({ ...current, [slide]: drawing }))
        if (!active) return
      }
    })()
    return () => { active = false }
  }, [token, position, deck?.slideCount])

  useEffect(() => {
    let active = true
    fetch(`/api/v1/shared/${encodeURIComponent(token)}`)
      .then(async (response) => {
        const payload = await response.json().catch(() => null)
        if (!response.ok) throw new Error(payload?.error?.message || '이 링크로는 덱을 열 수 없습니다.')
        return payload.data as SharedDeck
      })
      .then((data) => { if (active) setDeck(data) })
      .catch((problem) => { if (active) setError(problem instanceof Error ? problem.message : String(problem)) })
    return () => { active = false }
  }, [token])

  const loadComments = useCallback(async () => {
    const response = await fetch(`/api/v1/shared/${encodeURIComponent(token)}/comments`)
    if (!response.ok) return
    const payload = await response.json().catch(() => null)
    setComments(Array.isArray(payload?.data) ? payload.data as SharedComment[] : [])
  }, [token])

  useEffect(() => { void loadComments() }, [loadComments])

  const slideId = deck?.slides?.[position - 1]?.id || ''
  // A thread is a remark and what was said under it. The author's answer is
  // read where the remark was left, which is the whole reason for answering.
  const onThisSlide = comments.filter((comment) => comment.slideId === slideId && !comment.parentId)
  const repliesTo = (id: string) => comments.filter((comment) => comment.parentId === id)

  const say = async (parentId = '') => {
    const said = parentId ? answer.trim() : draft.trim()
    if (!said) return
    setSending(true)
    try {
      const response = await fetch(`/api/v1/shared/${encodeURIComponent(token)}/comments`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ slideId, author: author.trim(), body: said, parentId }),
      })
      const payload = await response.json().catch(() => null)
      if (!response.ok) throw new Error(payload?.error?.message || '의견을 남기지 못했습니다.')
      window.localStorage.setItem('ptium.reviewer', author.trim())
      setDraft(''); setAnswer(''); setAnswering('')
      await loadComments()
    } catch (problem) {
      setError(problem instanceof Error ? problem.message : String(problem))
    } finally {
      setSending(false)
    }
  }

  const move = useCallback((by: number) => {
    setPosition((current) => {
      const next = current + by
      if (!deck || next < 1 || next > deck.slideCount) return current
      return next
    })
  }, [deck])

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'ArrowRight' || event.key === 'PageDown' || event.key === ' ') { event.preventDefault(); move(1) }
      if (event.key === 'ArrowLeft' || event.key === 'PageUp') { event.preventDefault(); move(-1) }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [move])

  if (error) {
    return <main className="standalone-state"><span className="eyebrow">공유 링크</span><h1>덱을 열 수 없습니다</h1><p>{error}</p></main>
  }
  if (!deck) {
    return <main className="app-bootstrap"><LoaderCircle className="spin" size={24} /><p>덱을 불러오는 중…</p></main>
  }
  return (
    <main className="shared-deck">
      <header className="shared-deck-head">
        <h1>{deck.title}</h1>
        <span>{position} / {deck.slideCount}</span>
      </header>
      <div className="shared-deck-stage">
        <button type="button" aria-label="이전 슬라이드" disabled={position <= 1} onClick={() => move(-1)}><ChevronLeft size={20} /></button>
        {drawn[position]?.markup
          ? <div
              className="shared-deck-slide"
              role="img"
              aria-label={deck.titles[position - 1] || `슬라이드 ${position}`}
              onClickCapture={(event) => {
                // A link in a shared deck goes where it points; a jump moves this
                // page to the slide it names rather than leaving the review.
                const link = (event.target as HTMLElement | null)?.closest?.('a[href]') as HTMLAnchorElement | null
                const jumped = link?.getAttribute('href')?.match(/^#slide-(\d+)$/)
                if (!jumped) return
                event.preventDefault()
                setPosition(Math.min(deck.slideCount, Math.max(1, Number(jumped[1]))))
              }}
              dangerouslySetInnerHTML={{ __html: drawn[position]!.markup! }}
            />
          : drawn[position]?.url
            ? <img src={drawn[position]!.url} alt={deck.titles[position - 1] || `슬라이드 ${position}`} />
            : <div className="shared-deck-slide loading"><LoaderCircle className="spin" size={22} /></div>}
        <button type="button" aria-label="다음 슬라이드" disabled={position >= deck.slideCount} onClick={() => move(1)}><ChevronRight size={20} /></button>
      </div>
      <section className="shared-deck-comments">
        <h2>이 슬라이드에 대한 의견 {onThisSlide.length > 0 && <span>{onThisSlide.length}</span>}</h2>
        {onThisSlide.length > 0 && (
          <ul>
            {onThisSlide.map((comment) => (
              <li key={comment.id} className={comment.resolvedAt ? 'resolved' : ''}>
                <strong>{comment.author || '익명'}</strong>
                <p>{comment.body}</p>
                {comment.resolvedAt && <small>반영됨</small>}
                {repliesTo(comment.id).map((said) => (
                  <div key={said.id} className="shared-deck-reply">
                    <strong>{said.author || '익명'}</strong>
                    <p>{said.body}</p>
                  </div>
                ))}
                {answering === comment.id
                  ? <div className="shared-deck-answer">
                      <textarea rows={2} autoFocus value={answer} placeholder="답글"
                        onChange={(event) => setAnswer(event.target.value)}
                        onKeyDown={(event) => { if (event.key === 'Escape') { setAnswering(''); setAnswer('') } }} />
                      <button type="button" disabled={sending || !answer.trim()} onClick={() => void say(comment.id)}>남기기</button>
                    </div>
                  : <button type="button" className="shared-deck-reply-open" onClick={() => { setAnswering(comment.id); setAnswer('') }}>답글</button>}
              </li>
            ))}
          </ul>
        )}
        <div className="shared-deck-say">
          <input value={author} maxLength={80} placeholder="이름" onChange={(event) => setAuthor(event.target.value)} aria-label="이름" />
          <textarea value={draft} maxLength={4000} rows={2} placeholder="무엇이 잘못됐는지, 무엇을 바꾸면 좋을지 적어 주세요."
            onChange={(event) => setDraft(event.target.value)} aria-label="의견" />
          <button type="button" disabled={sending || !draft.trim()} onClick={() => void say()}>
            {sending ? '보내는 중…' : '남기기'}
          </button>
        </div>
      </section>
      <ol className="shared-deck-rail">
        {deck.titles.map((title, index) => (
          <li key={index}>
            <button type="button" className={index + 1 === position ? 'active' : ''} onClick={() => setPosition(index + 1)}>
              <span>{index + 1}</span>{title || '제목 없음'}
              {comments.some((comment) => comment.slideId === deck.slides?.[index]?.id) && <em title="의견이 달린 슬라이드">●</em>}
            </button>
          </li>
        ))}
      </ol>
    </main>
  )
}
