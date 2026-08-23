import { useEffect, useState } from 'react'
import { Check, LoaderCircle, MessageSquare, Trash2, Undo2 } from 'lucide-react'
import { Button, EmptyState, Modal } from '../../components/UI'
import type { DeckComment, Slide } from '../../types'
import { relativeDate } from '../../utils'

/**
 * What the people who were sent the link had to say.
 *
 * A remark names the slide it is about, so the list is a way into the deck
 * rather than a list of grievances: choosing one moves the editor to that
 * slide. Dealt-with remarks stay, greyed, because "we already changed that"
 * is worth reading next time the same reviewer asks.
 */
export function CommentsDialog({
  open, deckId, slides, onClose, onGo, load, resolve, remove,
}: {
  open: boolean
  deckId: string
  slides: Slide[]
  onClose: () => void
  onGo: (slideId: string) => void
  load: (id: string) => Promise<DeckComment[]>
  resolve: (id: string, commentId: string, resolved: boolean) => Promise<void>
  remove: (id: string, commentId: string) => Promise<void>
}) {
  const [comments, setComments] = useState<DeckComment[]>([])
  const [loading, setLoading] = useState(false)
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')

  useEffect(() => {
    if (!open) return
    let active = true
    setLoading(true)
    load(deckId)
      .then((rows) => { if (active) setComments(rows) })
      .catch((problem) => { if (active) setError(problem instanceof Error ? problem.message : String(problem)) })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [open, deckId, load])

  const slideLabel = (slideId?: string) => {
    if (!slideId) return '덱 전체'
    const index = slides.findIndex((slide) => slide.id === slideId)
    if (index < 0) return '삭제된 슬라이드'
    return `${index + 1}. ${slides[index].title || '제목 없음'}`
  }

  const act = async (comment: DeckComment, what: 'resolve' | 'reopen' | 'delete') => {
    setBusy(comment.id); setError('')
    try {
      if (what === 'delete') {
        if (!window.confirm('이 의견을 지웁니다. 남긴 사람에게는 알림이 가지 않습니다.')) return
        await remove(deckId, comment.id)
        setComments((current) => current.filter((row) => row.id !== comment.id))
      } else {
        const resolved = what === 'resolve'
        await resolve(deckId, comment.id, resolved)
        setComments((current) => current.map((row) => row.id === comment.id
          ? { ...row, resolvedAt: resolved ? new Date().toISOString() : undefined } : row))
      }
    } catch (problem) {
      setError(problem instanceof Error ? problem.message : String(problem))
    } finally {
      setBusy('')
    }
  }

  const openCount = comments.filter((comment) => !comment.resolvedAt).length
  return (
    <Modal open={open} onClose={onClose} title="받은 의견"
      description="공유 링크로 덱을 본 사람들이 남긴 말입니다. 어느 슬라이드에 대한 것인지 함께 옵니다.">
      <div className="comments-dialog">
        {error && <p className="form-error" role="alert">{error}</p>}
        {loading ? <p className="inspector-help"><LoaderCircle className="spin" size={14} /> 불러오는 중…</p>
          : comments.length === 0
            ? <EmptyState title="아직 받은 의견이 없습니다" description="공유 링크로 덱을 본 사람이 남기면 여기에 모입니다." />
            : <>
              <p className="inspector-help">{openCount}건이 아직 반영되지 않았습니다.</p>
              <ul className="comment-list">
                {comments.map((comment) => (
                  <li key={comment.id} className={comment.resolvedAt ? 'resolved' : ''}>
                    <div className="comment-head">
                      <strong>{comment.author || '익명'}</strong>
                      <button type="button" className="comment-slide" onClick={() => { if (comment.slideId) onGo(comment.slideId) }}>
                        {slideLabel(comment.slideId)}
                      </button>
                      <small>{relativeDate(comment.createdAt)}</small>
                    </div>
                    <p>{comment.body}</p>
                    <div className="comment-actions">
                      {comment.resolvedAt
                        ? <Button variant="ghost" size="small" disabled={busy === comment.id} onClick={() => void act(comment, 'reopen')}><Undo2 size={14} /> 다시 열기</Button>
                        : <Button variant="ghost" size="small" disabled={busy === comment.id} onClick={() => void act(comment, 'resolve')}><Check size={14} /> 반영함</Button>}
                      <Button variant="ghost" size="small" disabled={busy === comment.id} onClick={() => void act(comment, 'delete')}><Trash2 size={14} /> 삭제</Button>
                    </div>
                  </li>
                ))}
              </ul>
            </>}
      </div>
    </Modal>
  )
}

/** The icon the editor's button uses, so the import lives beside the dialog. */
export const CommentsIcon = MessageSquare
