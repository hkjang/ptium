import { useEffect, useRef } from 'react'
import type { ButtonHTMLAttributes, InputHTMLAttributes, ReactNode, SelectHTMLAttributes, TextareaHTMLAttributes } from 'react'
import { AlertTriangle, Inbox, LoaderCircle, RefreshCw } from 'lucide-react'

export function Button({ variant = 'primary', size = 'medium', className = '', children, ...props }: ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: 'primary' | 'secondary' | 'ghost' | 'danger'
  size?: 'small' | 'medium' | 'large'
}) {
  return <button className={`button button-${variant} button-${size} ${className}`} {...props}>{children}</button>
}

export function Field({ label, hint, error, children, className = '' }: { label: string; hint?: string; error?: string; children: ReactNode; className?: string }) {
  return (
    <label className={`field ${className}`}>
      <span className="field-label">{label}</span>
      {children}
      {error ? <span className="field-error">{error}</span> : hint ? <span className="field-hint">{hint}</span> : null}
    </label>
  )
}

export function Input(props: InputHTMLAttributes<HTMLInputElement>) {
  return <input className={`input ${props.className || ''}`} {...props} />
}

export function Textarea(props: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return <textarea className={`textarea ${props.className || ''}`} {...props} />
}

export function Select(props: SelectHTMLAttributes<HTMLSelectElement>) {
  return <select className={`select ${props.className || ''}`} {...props} />
}

export function Badge({ children, tone = 'neutral' }: { children: ReactNode; tone?: 'neutral' | 'success' | 'warning' | 'danger' | 'info' | 'violet' }) {
  return <span className={`badge badge-${tone}`}>{children}</span>
}

export function LoadingState({ label = '불러오는 중…', compact = false }: { label?: string; compact?: boolean }) {
  return <div className={`state-panel loading-state ${compact ? 'compact' : ''}`} role="status"><LoaderCircle className="spin" size={compact ? 20 : 28} /><span>{label}</span></div>
}

export function ErrorState({ title = '문제가 발생했습니다', message, onRetry }: { title?: string; message: string; onRetry?: () => void }) {
  return (
    <div className="state-panel error-state" role="alert">
      <span className="state-icon error"><AlertTriangle size={24} /></span>
      <div><strong>{title}</strong><p>{message}</p></div>
      {onRetry && <Button variant="secondary" size="small" onClick={onRetry}><RefreshCw size={15} /> 다시 시도</Button>}
    </div>
  )
}

export function EmptyState({ icon, title, description, action }: { icon?: ReactNode; title: string; description: string; action?: ReactNode }) {
  return (
    <div className="empty-state">
      <span className="state-icon">{icon || <Inbox size={25} />}</span>
      <h3>{title}</h3><p>{description}</p>{action}
    </div>
  )
}

/**
 * The last place the keyboard stood outside any overlay.
 *
 * Watched for the whole page rather than by each overlay, because a dialog
 * rendered only while it has something to show mounts already open: it never
 * saw the click that opened it, and by the time it runs, a field of its own
 * asking for the focus has already taken it. The body is skipped — closing an
 * overlay drops the focus there on the way out, and that is nowhere to send
 * anyone back to.
 */
let cameBefore: HTMLElement | null = null
if (typeof window !== 'undefined') {
  window.addEventListener('focusin', () => {
    const on = document.activeElement
    if (on instanceof HTMLElement && on !== document.body
        && !on.closest('.modal, .error-drawer, [role="dialog"]')) cameBefore = on
  }, true)
}

/** Everything inside `root` the keyboard can stand on, in tab order. */
function focusStops(root: HTMLElement): HTMLElement[] {
  const kinds = 'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]),'
    + ' textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
  return [...root.querySelectorAll<HTMLElement>(kinds)].filter((el) => el.getClientRects().length > 0)
}

/**
 * What an overlay owes the keyboard: Escape closes it, Tab stays inside it,
 * and whatever the keyboard was standing on gets it back on the way out.
 *
 * `aria-modal` says everything behind is out of play, and saying it does not
 * make it so. Before this, opening a dialog left the keyboard on the button
 * that opened it — 95 tab presses away from the dialog's own controls, through
 * the page behind it.
 *
 * Put the returned ref on the panel and give it `tabIndex={-1}`.
 */
export function useOverlayKeys(open: boolean, onClose: () => void) {
  const panel = useRef<HTMLElement | null>(null)

  useEffect(() => {
    if (!open) return
    const held = (event: KeyboardEvent) => {
      if (event.key === 'Escape') { onClose(); return }
      if (event.key !== 'Tab' || !panel.current) return
      const stops = focusStops(panel.current)
      if (!stops.length) { event.preventDefault(); panel.current.focus(); return }
      const edge = event.shiftKey ? stops[0] : stops[stops.length - 1]
      const wrap = event.shiftKey ? stops[stops.length - 1] : stops[0]
      if (document.activeElement === edge || !panel.current.contains(document.activeElement)) {
        event.preventDefault()
        wrap.focus()
      }
    }
    window.addEventListener('keydown', held)
    return () => window.removeEventListener('keydown', held)
  }, [open, onClose])

  // The panel itself takes the focus rather than its first button, so a screen
  // reader reads the title before the controls under it — unless a field inside
  // asked for the focus first. React grants that during the commit, before this
  // runs, so taking it unconditionally threw away what the author asked for:
  // the new-key dialog opens on its name field, and every letter typed into it
  // went nowhere.
  useEffect(() => {
    if (!open) return
    const back = cameBefore
    if (!panel.current?.contains(document.activeElement)) panel.current?.focus()
    return () => { if (back && document.contains(back)) back.focus() }
  }, [open])

  return panel
}

export function Modal({ open, title, description, children, footer, onClose, wide }: {
  open: boolean; title: string; description?: string; children: ReactNode; footer?: ReactNode
  onClose: () => void
  /** A dialog that holds a gallery rather than a form needs the width. */
  wide?: boolean
}) {
  const panel = useOverlayKeys(open, onClose)
  if (!open) return null
  return (
    <div className="modal-backdrop" onMouseDown={(event) => { if (event.currentTarget === event.target) onClose() }}>
      <section ref={panel} tabIndex={-1} className={`modal ${wide ? 'modal-wide' : ''}`} role="dialog" aria-modal="true" aria-labelledby="modal-title">
        <div className="modal-header"><div><h2 id="modal-title">{title}</h2>{description && <p>{description}</p>}</div><button className="icon-button" onClick={onClose} aria-label="닫기">×</button></div>
        <div className="modal-body">{children}</div>
        {footer && <div className="modal-footer">{footer}</div>}
      </section>
    </div>
  )
}

export function Skeleton({ className = '' }: { className?: string }) {
  return <span className={`skeleton ${className}`} aria-hidden="true" />
}
