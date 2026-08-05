import { useEffect } from 'react'
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

export function Modal({ open, title, description, children, footer, onClose }: {
  open: boolean; title: string; description?: string; children: ReactNode; footer?: ReactNode; onClose: () => void
}) {
  // Escape closes a dialog. Every other overlay in the workspace does, and one
  // that does not reads as stuck.
  useEffect(() => {
    if (!open) return
    const close = (event: KeyboardEvent) => { if (event.key === 'Escape') onClose() }
    window.addEventListener('keydown', close)
    return () => window.removeEventListener('keydown', close)
  }, [open, onClose])
  if (!open) return null
  return (
    <div className="modal-backdrop" onMouseDown={(event) => { if (event.currentTarget === event.target) onClose() }}>
      <section className="modal" role="dialog" aria-modal="true" aria-labelledby="modal-title">
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
