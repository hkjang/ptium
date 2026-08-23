import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { CommentsDialog } from './CommentsDialog'
import type { DeckComment, Slide } from '../../types'

const slides = [
  { id: 's1', order: 1, layout: 'cover', title: '표지' },
  { id: 's2', order: 2, layout: 'content', title: '현황 요약' },
] as Slide[]

const comments: DeckComment[] = [
  { id: 'c1', presentationId: 'deck', slideId: 's2', author: '박검토', body: '이 숫자는 지난 분기 기준입니다.', createdAt: new Date().toISOString() },
  { id: 'c2', presentationId: 'deck', slideId: 'gone', author: '이감사', body: '삭제된 장에 달린 의견', createdAt: new Date().toISOString(), resolvedAt: new Date().toISOString() },
]

describe('what the people who were sent the link had to say', () => {
  it('names the slide each remark is about, and says which are still waiting', async () => {
    render(<CommentsDialog open deckId="deck" slides={slides} onClose={() => {}} onGo={() => {}}
      load={vi.fn().mockResolvedValue(comments)} resolve={vi.fn()} remove={vi.fn()} />)
    await waitFor(() => expect(screen.getByText('박검토')).toBeTruthy())
    expect(screen.getByText('2. 현황 요약')).toBeTruthy()
    expect(screen.getByText(/1건이 아직 반영되지 않았습니다/)).toBeTruthy()
    // A remark about a slide that no longer exists still reads as a remark.
    expect(screen.getByText('삭제된 슬라이드')).toBeTruthy()
  })

  it('takes the author to the slide a remark is about', async () => {
    const onGo = vi.fn()
    render(<CommentsDialog open deckId="deck" slides={slides} onClose={() => {}} onGo={onGo}
      load={vi.fn().mockResolvedValue(comments)} resolve={vi.fn()} remove={vi.fn()} />)
    await waitFor(() => expect(screen.getByText('2. 현황 요약')).toBeTruthy())
    fireEvent.click(screen.getByText('2. 현황 요약'))
    expect(onGo).toHaveBeenCalledWith('s2')
  })

  it('keeps a remark after it is dealt with, greyed rather than gone', async () => {
    const resolve = vi.fn().mockResolvedValue(undefined)
    render(<CommentsDialog open deckId="deck" slides={slides} onClose={() => {}} onGo={() => {}}
      load={vi.fn().mockResolvedValue(comments)} resolve={resolve} remove={vi.fn()} />)
    await waitFor(() => expect(screen.getByText('박검토')).toBeTruthy())
    fireEvent.click(screen.getByRole('button', { name: /반영함/ }))
    await waitFor(() => expect(resolve).toHaveBeenCalledWith('deck', 'c1', true))
    await waitFor(() => expect(screen.getAllByRole('button', { name: /다시 열기/ }).length).toBe(2))
    expect(screen.getByText('박검토')).toBeTruthy()
  })
})
