import { useEffect, useRef } from 'react'

/**
 * When unsaved work gets written down.
 *
 * Three moments, and they are easy to get wrong separately: a pause in typing,
 * leaving the page, and closing the tab. The first two are this hook; the third
 * is useUnsavedWarning below, because the browser only lets us ask.
 *
 * The timer restarts on every edit rather than firing on a schedule — someone
 * typing a sentence should not have each word posted — and it is cleared when
 * the editor goes away, after one last save, so an edit made a second before
 * navigating is not the edit that gets lost.
 */
export function useAutosave({ dirty, edits, save, onError, delay = 1000 }: {
  /** Whether there is work not yet written down. */
  dirty: boolean
  /** A counter that changes on every edit, so the wait restarts. */
  edits: number
  save: () => Promise<boolean>
  onError: (error: unknown) => void
  delay?: number
}) {
  const timer = useRef<number | null>(null)
  // The latest save and error handler, so the unmount effect never calls last
  // render's closure.
  const latest = useRef({ save, onError, dirty })
  latest.current = { save, onError, dirty }

  useEffect(() => {
    if (!dirty) return
    if (timer.current) window.clearTimeout(timer.current)
    timer.current = window.setTimeout(() => {
      void latest.current.save().catch((error) => latest.current.onError(error))
    }, delay)
    return () => { if (timer.current) window.clearTimeout(timer.current) }
  }, [dirty, edits, delay])

  useEffect(() => () => {
    if (timer.current) window.clearTimeout(timer.current)
    // Leaving the editor with work in hand: save it. A full-page exit is guarded
    // separately, because there the browser decides how long we get.
    if (latest.current.dirty) {
      void latest.current.save().catch(() => { /* the unload warning has this case */ })
    }
  }, [])
}

/**
 * Asking the browser to warn before a tab with unsaved work closes.
 *
 * The browser decides whether to show it, and says nothing about our wording.
 * All we control is whether we ask — so we ask exactly when there is work in
 * hand or a save still in flight.
 */
export function useUnsavedWarning(hasWork: () => boolean) {
  const latest = useRef(hasWork)
  latest.current = hasWork

  useEffect(() => {
    const warn = (event: BeforeUnloadEvent) => {
      if (!latest.current()) return
      event.preventDefault()
      event.returnValue = ''
    }
    window.addEventListener('beforeunload', warn)
    return () => window.removeEventListener('beforeunload', warn)
  }, [])
}
