import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { RewriteDialog } from './RewriteDialog'

describe('sending the deck back with what to change', () => {
  it('passes on what the author typed', () => {
    const onRewrite = vi.fn()
    render(<RewriteDialog open busy={false} onRewrite={onRewrite} onClose={() => {}} />)
    fireEvent.change(screen.getByRole('textbox'), { target: { value: '  3장은 요점 3개로 줄여 주세요  ' } })
    fireEvent.click(screen.getByText('다시 쓰기'))
    expect(onRewrite).toHaveBeenCalledWith('3장은 요점 3개로 줄여 주세요')
  })

  // Saying nothing is still a request, and means what it always meant.
  it('asks for the whole deck when nothing is typed', () => {
    const onRewrite = vi.fn()
    render(<RewriteDialog open busy={false} onRewrite={onRewrite} onClose={() => {}} />)
    fireEvent.click(screen.getByText('다시 쓰기'))
    expect(onRewrite).toHaveBeenCalledWith('')
  })

  it('says what a rewrite keeps, before it is asked for', () => {
    render(<RewriteDialog open busy={false} onRewrite={() => {}} onClose={() => {}} />)
    expect(screen.getByText(/숫자와 사실은 그대로/)).toBeTruthy()
    expect(screen.getByText(/버전 이력/)).toBeTruthy()
  })

  it('cannot be sent twice while it is being sent', () => {
    render(<RewriteDialog open busy onRewrite={() => {}} onClose={() => {}} />)
    expect(screen.getByText(/보내는 중/).closest('button')?.hasAttribute('disabled')).toBe(true)
  })
})
