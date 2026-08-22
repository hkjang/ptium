import type React from 'react'
import { Button, Input, Modal } from '../../components/UI'

/** What a command would do, before it does it. */
export interface CommandPlan {
  plan: { kind: string; reason: string }[]
  notes: string[]
  slides: number
  slidesAfter: number
}

/**
 * Telling the deck what to do, in words.
 *
 * The dialog's whole job is the pause in the middle: what was typed is read
 * into a plan, the plan is shown in the language it was typed in, and only then
 * is anything changed. A command nobody can check is a command nobody should
 * run — so the button says "무엇을 할지 보기" until there is a plan, and "적용"
 * afterwards.
 */
export function CommandDialog({ open, text, plan, busy, onText, onPlan, onRun, onClose }: {
  open: boolean
  text: string
  plan: CommandPlan | null
  busy: boolean
  onText: (value: string) => void
  onPlan: () => void
  onRun: () => void
  onClose: () => void
}) {
  return (
    <Modal
      open={open}
      onClose={onClose}
      title="덱에 명령하기"
      description="문장에서 할 일을 읽어 그대로 실행합니다. 모델을 쓰지 않으므로 폐쇄망에서도 같습니다."
      footer={<>
        {plan
          ? <Button disabled={busy} onClick={onRun}>{busy ? '적용 중…' : '적용'}</Button>
          : <Button disabled={busy || !text.trim()} onClick={onPlan}>{busy ? '읽는 중…' : '무엇을 할지 보기'}</Button>}
        <Button variant="secondary" onClick={onClose}>닫기</Button>
      </>}
    >
      <Input
        autoFocus
        value={text}
        placeholder="예: 3번과 4번 합쳐줘"
        aria-label="덱에 내릴 명령"
        onChange={(event: React.ChangeEvent<HTMLInputElement>) => onText(event.target.value)}
        onKeyDown={(event: React.KeyboardEvent<HTMLInputElement>) => {
          if (event.key !== 'Enter') return
          event.preventDefault()
          if (plan) onRun()
          else onPlan()
        }}
      />
      {plan
        ? <div className="command-plan">
            <ul>{plan.plan.map((entry, index) => <li key={index}>{entry.reason}</li>)}</ul>
            {plan.notes.map((note, index) => <small key={index}>{note}</small>)}
            <p>{plan.slides}장 → <b>{plan.slidesAfter}장</b></p>
          </div>
        : <ul className="command-examples">
            <li>3번과 4번 합쳐줘</li>
            <li>5번 삭제 · 2번과 5번 지워줘</li>
            <li>2번을 두 장으로 나눠줘</li>
            <li>6번을 2번으로 옮겨줘</li>
            <li>8장으로 줄여줘 · 10분 발표로 맞춰줘 <small>(측정 점수가 가장 낮은 장부터 빠집니다)</small></li>
          </ul>}
    </Modal>
  )
}
