import { act, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useAutosave, useUnsavedWarning } from './useAutosave'

function Editor({ dirty, edits, save, onError }: {
  dirty: boolean; edits: number; save: () => Promise<boolean>; onError: (error: unknown) => void
}) {
  useAutosave({ dirty, edits, save, onError })
  return null
}

describe('when unsaved work gets written down', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  it('waits for a pause in the typing rather than posting every word', () => {
    const save = vi.fn().mockResolvedValue(true)
    const { rerender } = render(<Editor dirty edits={1} save={save} onError={() => {}} />)
    act(() => { vi.advanceTimersByTime(900) })
    rerender(<Editor dirty edits={2} save={save} onError={() => {}} />)
    act(() => { vi.advanceTimersByTime(900) })
    // Two edits 900ms apart: still nothing saved, because the wait restarted.
    expect(save).not.toHaveBeenCalled()
    act(() => { vi.advanceTimersByTime(200) })
    expect(save).toHaveBeenCalledOnce()
  })

  it('saves nothing while there is nothing to save', () => {
    const save = vi.fn().mockResolvedValue(true)
    render(<Editor dirty={false} edits={0} save={save} onError={() => {}} />)
    act(() => { vi.advanceTimersByTime(5000) })
    expect(save).not.toHaveBeenCalled()
  })

  it('says so when a save fails instead of losing it quietly', async () => {
    const failure = new Error('네트워크')
    const save = vi.fn().mockRejectedValue(failure)
    const onError = vi.fn()
    render(<Editor dirty edits={1} save={save} onError={onError} />)
    await act(async () => { vi.advanceTimersByTime(1000) })
    expect(onError).toHaveBeenCalledWith(failure)
  })

  it('writes down the last edit when the editor goes away', () => {
    const save = vi.fn().mockResolvedValue(true)
    const { unmount } = render(<Editor dirty edits={1} save={save} onError={() => {}} />)
    unmount()
    expect(save).toHaveBeenCalledOnce()
  })

  it('leaves a clean editor alone when it goes away', () => {
    const save = vi.fn().mockResolvedValue(true)
    const { unmount } = render(<Editor dirty={false} edits={0} save={save} onError={() => {}} />)
    unmount()
    expect(save).not.toHaveBeenCalled()
  })
})

function Guard({ hasWork }: { hasWork: () => boolean }) {
  useUnsavedWarning(hasWork)
  return null
}

describe('asking the browser to warn before a tab closes', () => {
  it('asks only when there is work in hand', () => {
    let work = false
    const { unmount } = render(<Guard hasWork={() => work} />)

    const quiet = new Event('beforeunload', { cancelable: true }) as BeforeUnloadEvent
    window.dispatchEvent(quiet)
    expect(quiet.defaultPrevented).toBe(false)

    work = true
    const busy = new Event('beforeunload', { cancelable: true }) as BeforeUnloadEvent
    window.dispatchEvent(busy)
    expect(busy.defaultPrevented).toBe(true)

    // And stops asking once the editor is gone.
    unmount()
    const after = new Event('beforeunload', { cancelable: true }) as BeforeUnloadEvent
    window.dispatchEvent(after)
    expect(after.defaultPrevented).toBe(false)
  })
})
