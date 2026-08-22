import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { CommandDialog } from './CommandDialog'

const plan = {
  plan: [{ kind: 'merge', reason: '2번과 3번을 한 장으로 합칩니다' }],
  notes: ['3번의 요점과 노트, 출처를 2번으로 옮겼습니다'],
  slides: 5,
  slidesAfter: 4,
}

describe('telling the deck what to do', () => {
  it('reads before it runs: no plan, no apply', () => {
    const onPlan = vi.fn()
    const onRun = vi.fn()
    render(<CommandDialog open text="2번과 3번 합쳐줘" plan={null} busy={false}
      onText={() => {}} onPlan={onPlan} onRun={onRun} onClose={() => {}} />)
    expect(screen.queryByRole('button', { name: '적용' })).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: '무엇을 할지 보기' }))
    expect(onPlan).toHaveBeenCalledOnce()
    expect(onRun).not.toHaveBeenCalled()
  })

  it('shows what it understood, and how many slides that leaves', () => {
    render(<CommandDialog open text="2번과 3번 합쳐줘" plan={plan} busy={false}
      onText={() => {}} onPlan={() => {}} onRun={() => {}} onClose={() => {}} />)
    expect(screen.getByText('2번과 3번을 한 장으로 합칩니다')).toBeTruthy()
    expect(screen.getByText('3번의 요점과 노트, 출처를 2번으로 옮겼습니다')).toBeTruthy()
    expect(screen.getByText(/5장 →/)).toBeTruthy()
    expect(screen.getByText('4장')).toBeTruthy()
  })

  it('will not offer to read an empty sentence', () => {
    render(<CommandDialog open text="   " plan={null} busy={false}
      onText={() => {}} onPlan={() => {}} onRun={() => {}} onClose={() => {}} />)
    expect(screen.getByRole('button', { name: '무엇을 할지 보기' }).hasAttribute('disabled')).toBe(true)
  })

  it('runs on Enter once there is a plan, and reads on Enter before that', () => {
    const onPlan = vi.fn()
    const onRun = vi.fn()
    const { rerender } = render(<CommandDialog open text="5번 삭제" plan={null} busy={false}
      onText={() => {}} onPlan={onPlan} onRun={onRun} onClose={() => {}} />)
    fireEvent.keyDown(screen.getByLabelText('덱에 내릴 명령'), { key: 'Enter' })
    expect(onPlan).toHaveBeenCalledOnce()
    rerender(<CommandDialog open text="5번 삭제" plan={plan} busy={false}
      onText={() => {}} onPlan={onPlan} onRun={onRun} onClose={() => {}} />)
    fireEvent.keyDown(screen.getByLabelText('덱에 내릴 명령'), { key: 'Enter' })
    expect(onRun).toHaveBeenCalledOnce()
  })
})
