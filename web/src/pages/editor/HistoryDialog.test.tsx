import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { HistoryDialog } from './HistoryDialog'
import type { PresentationRevision } from '../../types'

const history: PresentationRevision[] = [
  { id: 'r2', version: 4, reason: 'source', slideCount: 9, createdAt: new Date().toISOString() },
  { id: 'r1', version: 3, reason: 'edit', slideCount: 8, createdAt: new Date().toISOString() },
] as PresentationRevision[]

describe('every version this deck has been', () => {
  it('says what each checkpoint was made for, in the reader’s words', () => {
    render(<HistoryDialog open loading={false} version={5} history={history} restoring={null}
      onRestore={() => {}} onClose={() => {}} />)
    expect(screen.getByText('현재 버전 5')).toBeTruthy()
    expect(screen.getByText(/버전 4 · 코드 적용 전/)).toBeTruthy()
    expect(screen.getByText(/버전 3 · 자동 편집 체크포인트/)).toBeTruthy()
  })

  it('will not start a second restore, or close, while one is in flight', () => {
    const onClose = vi.fn()
    render(<HistoryDialog open loading={false} version={5} history={history} restoring="r1"
      onRestore={() => {}} onClose={onClose} />)
    const owned = screen.getAllByRole('button').filter((button) => /복원|닫기/.test(button.textContent || ''))
    expect(owned.length).toBeGreaterThan(1)
    for (const button of owned) {
      expect(button.hasAttribute('disabled'), button.textContent || '').toBe(true)
    }
    // The header's × is the shared modal's and stays clickable, so the dialog
    // refuses the close itself rather than trusting the affordance.
    const dismiss = screen.getAllByRole('button').find((button) => !/복원|닫기/.test(button.textContent || ''))
    if (dismiss) fireEvent.click(dismiss)
    expect(onClose).not.toHaveBeenCalled()
  })

  it('restores the checkpoint that was clicked', () => {
    const onRestore = vi.fn()
    render(<HistoryDialog open loading={false} version={5} history={history} restoring={null}
      onRestore={onRestore} onClose={() => {}} />)
    fireEvent.click(screen.getAllByRole('button', { name: /복원/ })[0])
    expect(onRestore).toHaveBeenCalledWith(history[0])
  })

  it('says a deck with no history has none, rather than showing an empty list', () => {
    render(<HistoryDialog open loading={false} version={1} history={[]} restoring={null}
      onRestore={() => {}} onClose={() => {}} />)
    expect(screen.getByText('아직 이전 버전이 없습니다')).toBeTruthy()
  })
})

describe('what changed since a version', () => {
  const checkpoint = {
    id: 'r2', presentationId: 'p1', version: 11, reason: 'source',
    title: '비용 계획', slideCount: 4, createdAt: new Date().toISOString(),
  }

  it('asks only when the reader asks, and says what it found', () => {
    const asked: string[] = []
    const { rerender } = render(<HistoryDialog
      open loading={false} version={12} history={[checkpoint]} restoring={null}
      changes={{}} openChange={null}
      onCompare={(entry) => asked.push(entry.id)}
      onRestore={() => {}} onClose={() => {}} />)
    // Nothing is fetched until it is asked for.
    expect(asked).toEqual([])
    fireEvent.click(screen.getByText('이 버전 이후 무엇이 바뀌었나'))
    expect(asked).toEqual(['r2'])

    rerender(<HistoryDialog
      open loading={false} version={12} history={[checkpoint]} restoring={null}
      changes={{ r2: [
        { kind: 'changed', position: 2, title: '비용', removed: ['- 12억 원'], added: ['- 14억 원'] },
        { kind: 'added', position: 3, title: '리스크', added: ['# 리스크'] },
      ] }}
      openChange="r2"
      onCompare={() => {}} onRestore={() => {}} onClose={() => {}} />)
    expect(screen.getByText('1장 수정 · 1장 추가')).toBeTruthy()
    expect(screen.getByText('− - 12억 원')).toBeTruthy()
    expect(screen.getByText('+ - 14억 원')).toBeTruthy()
  })

  it('says plainly when a version matches what is on screen', () => {
    render(<HistoryDialog
      open loading={false} version={12} history={[checkpoint]} restoring={null}
      changes={{ r2: [] }} openChange="r2"
      onCompare={() => {}} onRestore={() => {}} onClose={() => {}} />)
    expect(screen.getByText('이 버전 이후 바뀐 것이 없습니다')).toBeTruthy()
  })
})
