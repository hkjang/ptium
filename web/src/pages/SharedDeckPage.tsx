import { useCallback, useEffect, useState } from 'react'
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
interface SharedDeck { title: string; slideCount: number; titles: string[]; language?: string }

export function SharedDeckPage({ token }: { token: string }) {
  const [deck, setDeck] = useState<SharedDeck | null>(null)
  const [error, setError] = useState('')
  const [position, setPosition] = useState(1)

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
        <img
          src={`/api/v1/shared/${encodeURIComponent(token)}/preview.svg?slide=${position}&width=1600`}
          alt={deck.titles[position - 1] || `슬라이드 ${position}`}
        />
        <button type="button" aria-label="다음 슬라이드" disabled={position >= deck.slideCount} onClick={() => move(1)}><ChevronRight size={20} /></button>
      </div>
      <ol className="shared-deck-rail">
        {deck.titles.map((title, index) => (
          <li key={index}>
            <button type="button" className={index + 1 === position ? 'active' : ''} onClick={() => setPosition(index + 1)}>
              <span>{index + 1}</span>{title || '제목 없음'}
            </button>
          </li>
        ))}
      </ol>
    </main>
  )
}
