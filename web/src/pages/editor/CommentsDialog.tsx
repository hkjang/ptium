import { useEffect, useState } from 'react'
import { Check, CornerDownRight, LoaderCircle, MessageSquare, Send, Trash2, Undo2 } from 'lucide-react'
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
  open, deckId, slides, onClose, onGo, load, resolve, remove, reply,
}: {
  open: boolean
  deckId: string
  slides: Slide[]
  onClose: () => void
  onGo: (slideId: string) => void
  load: (id: string) => Promise<DeckComment[]>
  resolve: (id: string, commentId: string, resolved: boolean) => Promise<void>
  remove: (id: string, commentId: string) => Promise<void>
  reply: (id: string, parentId: string, body: string) => Promise<DeckComment>
}) {
  const [comments, setComments] = useState<DeckComment[]>([])
  const [loading, setLoading] = useState(false)
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')
  // Answering is the other half of a review. Resolving is a state; "고쳤습니다,
  // 4번은 그대로 둡니다" is a sentence, and reviews run on sentences — the
  // person holding the link reads the answer where they left the remark.
  const [answering, setAnswering] = useState('')
  const [draft, setDraft] = useState('')

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

  const send = async (parentId: string) => {
    const said = draft.trim()
    if (!said) return
    setBusy(parentId); setError('')
    try {
      const written = await reply(deckId, parentId, said)
      setComments((current) => [...current, written])
      setAnswering(''); setDraft('')
    } catch (problem) {
      setError(problem instanceof Error ? problem.message : String(problem))
    } finally {
      setBusy('')
    }
  }

  // A thread is a remark and what was said under it, oldest first.
  const roots = comments.filter((comment) => !comment.parentId)
  const repliesTo = (id: string) => comments.filter((comment) => comment.parentId === id)
  const openCount = roots.filter((comment) => !comment.resolvedAt).length
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
                {roots.map((comment) => (
                  <li key={comment.id} className={comment.resolvedAt ? 'resolved' : ''}>
                    <div className="comment-head">
                      <strong>{comment.author || '익명'}</strong>
                      <button type="button" className="comment-slide" onClick={() => { if (comment.slideId) onGo(comment.slideId) }}>
                        {slideLabel(comment.slideId)}
                      </button>
                      <small>{relativeDate(comment.createdAt)}</small>
                    </div>
                    <p>{comment.body}</p>
                    {repliesTo(comment.id).map((answer) => (
                      <div key={answer.id} className="comment-reply">
                        <CornerDownRight size={13} />
                        <div>
                          <strong>{answer.author || '익명'}</strong> <small>{relativeDate(answer.createdAt)}</small>
                          <p>{answer.body}</p>
                        </div>
                      </div>
                    ))}
                    {answering === comment.id && <div className="comment-answer">
                      <textarea autoFocus rows={2} value={draft} placeholder="무엇을 했는지 적어 두면 링크를 가진 사람이 읽습니다"
                        onChange={(event) => setDraft(event.target.value)}
                        onKeyDown={(event) => {
                          if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) { event.preventDefault(); void send(comment.id) }
                          if (event.key === 'Escape') { setAnswering(''); setDraft('') }
                        }} />
                      <Button size="small" disabled={busy === comment.id || !draft.trim()} onClick={() => void send(comment.id)}>
                        <Send size={14} /> 답글
                      </Button>
                    </div>}
                    <div className="comment-actions">
                      {answering !== comment.id && <Button variant="ghost" size="small"
                        onClick={() => { setAnswering(comment.id); setDraft('') }}>
                        <CornerDownRight size={14} /> 답글
                      </Button>}
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
