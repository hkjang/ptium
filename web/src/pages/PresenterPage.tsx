import { useEffect, useState } from 'react'
import { LoaderCircle } from 'lucide-react'
import { api } from '../api/client'
import { PresenterScreen } from '../components/Presentation'
import type { Presentation } from '../types'
import { displayError } from '../utils'

/**
 * The presenter's window.
 *
 * It is a page rather than a panel because it belongs on the other screen: the
 * laptop in front of the speaker, while the projector shows the deck. It holds
 * no state of its own — the presenting window owns the position and this one
 * mirrors it — so the two can never disagree about which slide is up.
 */
export function PresenterPage({ id }: { id: string }) {
  const [presentation, setPresentation] = useState<Presentation | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    let active = true
    api.presentation(id)
      .then((data) => { if (active) setPresentation(data) })
      .catch((err) => { if (active) setError(displayError(err)) })
    return () => { active = false }
  }, [id])

  useEffect(() => { document.title = presentation ? `발표자 보기 · ${presentation.title}` : '발표자 보기' }, [presentation])

  if (error) return <main className="presenter-screen loading"><p>{error}</p></main>
  if (!presentation) return <main className="presenter-screen loading"><LoaderCircle className="spin" size={22} /><p>발표 자료를 불러오는 중…</p></main>
  return <PresenterScreen
    presentationId={presentation.id}
    slides={presentation.slides || []}
    version={presentation.updatedAt}
  />
}
