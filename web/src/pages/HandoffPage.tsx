import { useEffect, useRef, useState } from 'react'
import { ArrowRightLeft } from 'lucide-react'
import { api } from '../api/client'
import { AppShell } from '../components/AppShell'
import { Button, EmptyState, ErrorState, LoadingState } from '../components/UI'
import { navigate, useLocation } from '../router'
import { displayError } from '../utils'
import { parseHandoffQuery } from './handoff'

/**
 * Where another service's "다른 서비스로 보내기" lands.
 *
 * The browser arrives with the sending service's origin and a five-minute,
 * single-use claim. The server does the fetching — only from an origin the
 * administrator listed — and reads what arrives as it would an upload; this
 * page asks once, then goes to the deck it made. When it cannot, it says why
 * in words: the claim was already used, the format cannot be read, the
 * service is not on the list.
 */
export function HandoffPage() {
  const { search } = useLocation()
  const [failure, setFailure] = useState('')
  const [title, setTitle] = useState('')
  // Asked once per address, however the effect is re-run: the claim is spent
  // by the first request, and a second would only ever be refused.
  const asked = useRef('')

  useEffect(() => {
    if (asked.current === search) return
    asked.current = search
    const parsed = parseHandoffQuery(search)
    if ('problem' in parsed) { setFailure(parsed.problem); return }
    setFailure('')
    api.receiveHandoff(parsed.source, parsed.claim)
      .then(({ presentation }) => {
        setTitle(presentation.title)
        navigate(`/presentations/${encodeURIComponent(presentation.id)}/editor`, true)
      })
      .catch((err) => setFailure(displayError(err)))
  }, [search])

  return <AppShell title="다른 서비스에서 넘겨받기" eyebrow="HANDOFF">
    {failure
      ? <ErrorState title="문서를 넘겨받지 못했습니다" message={failure} />
      : title
        ? <EmptyState icon={<ArrowRightLeft size={28} />} title={`${title} 을(를) 넘겨받았습니다`} description="편집기로 이동합니다." />
        : <LoadingState label="보낸 서비스에서 문서를 받아 슬라이드로 옮기는 중…" />}
    {failure && <p className="modal-note" style={{ marginTop: 16 }}>
      <Button variant="secondary" onClick={() => navigate('/dashboard')}>홈으로</Button>
    </p>}
  </AppShell>
}
