import { useEffect, useState } from 'react'
import { WandSparkles } from 'lucide-react'
import { Button, Modal } from '../../components/UI'

/**
 * Sending the whole deck back to the model, with what to change.
 *
 * The deck-wide rewrite used to be a yes/no: it improved whatever it judged
 * worth improving, which is the product deciding about somebody else's deck. A
 * person asking for a rewrite usually knows what they want — "3장은 짧게, 5장에
 * 표" — and until now had to say it slide by slide.
 *
 * Saying nothing still works, and means what it always meant.
 */
export function RewriteDialog({ open, busy, onRewrite, onClose }: {
  open: boolean
  busy: boolean
  onRewrite: (instruction: string) => void
  onClose: () => void
}) {
  const [instruction, setInstruction] = useState('')
  useEffect(() => { if (open) setInstruction('') }, [open])
  return (
    <Modal
      open={open}
      onClose={onClose}
      title="덱 전체 다시 쓰기"
      description="숫자와 사실은 그대로 두고 제목·문장·구성을 다듬습니다. 이전 덱은 버전 이력에 남습니다."
      footer={<>
        <Button variant="secondary" onClick={onClose} disabled={busy}>취소</Button>
        <Button onClick={() => onRewrite(instruction.trim())} disabled={busy}>
          <WandSparkles size={15} /> {busy ? '보내는 중…' : '다시 쓰기'}
        </Button>
      </>}
    >
      <label className="rewrite-ask">
        <span>무엇을 바꿀까요? <small>비워 두면 전체를 다듬습니다.</small></span>
        <textarea
          autoFocus
          rows={4}
          maxLength={2000}
          value={instruction}
          onChange={(event) => setInstruction(event.target.value)}
          placeholder={'예) 3장은 요점 3개로 줄이고, 5장에 비용 비교 표를 넣어 주세요.\n예) 전체를 임원 보고 톤으로, 문장은 짧게.'}
        />
      </label>
    </Modal>
  )
}
