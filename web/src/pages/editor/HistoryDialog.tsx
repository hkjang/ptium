import { Check, History, LoaderCircle, RotateCcw } from 'lucide-react'
import { Button, EmptyState, LoadingState, Modal } from '../../components/UI'
import type { PresentationRevision } from '../../types'
import { relativeDate } from '../../utils'
import { revisionReason } from './model/findings'

/**
 * Every version this deck has been, and the way back to one.
 *
 * The description is doing work: people expect a checkpoint per keystroke and
 * find a handful instead, so the dialog says what is kept and what is grouped.
 * Restoring is itself checkpointed, which is why restoring can be undone — and
 * why the buttons lock while one is in flight rather than letting a second
 * restore race the first.
 */
export function HistoryDialog({ open, loading, version, history, restoring, onRestore, onClose }: {
  open: boolean
  loading: boolean
  version: number
  history: PresentationRevision[]
  restoring: string | null
  onRestore: (checkpoint: PresentationRevision) => void
  onClose: () => void
}) {
  const busy = Boolean(restoring)
  return (
    <Modal
      open={open}
      onClose={() => { if (!busy) onClose() }}
      title="버전 이력"
      description="자동 편집은 5분 단위로 묶고, 코드 적용·재생성·복원 전에는 별도 체크포인트를 남깁니다. 복원 직전 상태도 다시 기록됩니다."
      footer={<Button variant="secondary" disabled={busy} onClick={onClose}>닫기</Button>}
    >
      {loading
        ? <LoadingState compact label="버전 이력을 불러오는 중…" />
        : history.length === 0
          ? <EmptyState icon={<History size={24} />} title="아직 이전 버전이 없습니다"
              description="첫 변경을 저장하면 복원 가능한 체크포인트가 만들어집니다." />
          : <ol className="revision-list">
              <li className="revision-current">
                <span><Check size={14} /></span>
                <div><strong>현재 버전 {version}</strong><small>지금 편집 중인 내용</small></div>
              </li>
              {history.map((checkpoint) => (
                <li key={checkpoint.id}>
                  <span><History size={14} /></span>
                  <div>
                    <strong>버전 {checkpoint.version} · {revisionReason(checkpoint.reason)}</strong>
                    <small>{checkpoint.slideCount}장 · {relativeDate(checkpoint.createdAt)}</small>
                  </div>
                  <Button variant="secondary" size="small" disabled={busy} onClick={() => onRestore(checkpoint)}>
                    {restoring === checkpoint.id ? <LoaderCircle className="spin" size={13} /> : <RotateCcw size={13} />} 복원
                  </Button>
                </li>
              ))}
            </ol>}
    </Modal>
  )
}
