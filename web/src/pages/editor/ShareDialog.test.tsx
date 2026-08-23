import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ShareDialog } from './ShareDialog'
import type { Share } from '../../types'

const open: Share = {
  id: 's1', presentationId: 'deck', label: '임원 검토', views: 3,
  createdAt: new Date().toISOString(), lastSeenAt: new Date().toISOString(),
}

describe('links that open a deck for someone with no account', () => {
  it('shows the address once, because the server keeps only a digest of it', async () => {
    const created: Share = { ...open, id: 's2', label: '외부 감사', views: 0, url: 'https://ptium.example/view/abc123' }
    const create = vi.fn().mockResolvedValue(created)
    render(<ShareDialog open deckId="deck" onClose={() => {}} create={create}
      load={vi.fn().mockResolvedValue([])} revoke={vi.fn()} />)
    fireEvent.change(screen.getByPlaceholderText('예: 임원 검토'), { target: { value: '외부 감사' } })
    fireEvent.click(screen.getByRole('button', { name: /링크 만들기/ }))
    await waitFor(() => expect(screen.getByText('https://ptium.example/view/abc123')).toBeTruthy())
    expect(create).toHaveBeenCalledWith('deck', { label: '외부 감사', days: 0 })
    expect(screen.getByText(/한 번만 보입니다|복사했습니다/)).toBeTruthy()
  })

  it('says what each link is for and how often it was opened', async () => {
    render(<ShareDialog open deckId="deck" onClose={() => {}} create={vi.fn()}
      load={vi.fn().mockResolvedValue([open])} revoke={vi.fn()} />)
    await waitFor(() => expect(screen.getByText('임원 검토')).toBeTruthy())
    // The row says so as well as the select, so the row is what is checked.
    expect(screen.getByText(/직접 회수할 때까지 · 3회 열림/)).toBeTruthy()
  })

  it('asks before closing a link, and marks it closed without losing the row', async () => {
    const revoke = vi.fn().mockResolvedValue(undefined)
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    render(<ShareDialog open deckId="deck" onClose={() => {}} create={vi.fn()}
      load={vi.fn().mockResolvedValue([open])} revoke={revoke} />)
    await waitFor(() => expect(screen.getByText('임원 검토')).toBeTruthy())
    fireEvent.click(screen.getByRole('button', { name: /회수/ }))
    await waitFor(() => expect(revoke).toHaveBeenCalledWith('deck', 's1'))
    await waitFor(() => expect(screen.getByText(/회수됨/)).toBeTruthy())
    expect(screen.getByText('임원 검토')).toBeTruthy()
  })
})
