import { Clock3, Copy, MoreHorizontal, Pencil, Presentation as PresentationIcon, Trash2 } from 'lucide-react'
import { useState } from 'react'
import type { Presentation } from '../types'
import { Link } from '../router'
import { Badge } from './UI'
import { relativeDate } from '../utils'

const statusMap = {
  draft: { label: '초안', tone: 'neutral' as const },
  generating: { label: '생성 중', tone: 'info' as const },
  ready: { label: '완료', tone: 'success' as const },
  failed: { label: '실패', tone: 'danger' as const },
}

export function PresentationCard({ presentation, onDelete, onDuplicate }: {
  presentation: Presentation
  onDelete?: (presentation: Presentation) => void
  onDuplicate?: (presentation: Presentation) => void
}) {
  const [menuOpen, setMenuOpen] = useState(false)
  const status = statusMap[presentation.status] || statusMap.draft
  return (
    <article className="presentation-card">
      <Link to={`/presentations/${presentation.id}/editor`} className={`deck-preview theme-${presentation.theme || 'aurora'}`} ariaLabel={`${presentation.title} 편집`}>
        {presentation.thumbnailUrl ? <img src={presentation.thumbnailUrl} alt="" /> : <div className="preview-slide">
          <span className="preview-kicker">PTIUM DECK</span><strong>{presentation.title}</strong><i /><small>{presentation.slideCount || presentation.slides?.length || 0} slides</small>
        </div>}
        {presentation.status === 'generating' && <div className="generation-overlay"><span className="loader-orbit" /><strong>생성 중</strong></div>}
      </Link>
      <div className="presentation-card-body">
        <div className="card-title-row"><div><Link to={`/presentations/${presentation.id}/editor`}>{presentation.title}</Link><span><Clock3 size={13} /> {relativeDate(presentation.updatedAt)}</span></div><div className={`card-menu-wrap ${menuOpen ? 'open' : ''}`} onMouseLeave={() => setMenuOpen(false)}><button className="icon-button small" aria-label="프레젠테이션 메뉴" aria-expanded={menuOpen} onClick={() => setMenuOpen((value) => !value)}><MoreHorizontal size={17} /></button><div className="card-menu"><Link to={`/presentations/${presentation.id}/editor`}><Pencil size={14} /> 편집</Link>{onDuplicate && <button onClick={() => { setMenuOpen(false); onDuplicate(presentation) }}><Copy size={14} /> 복제</button>}{onDelete && <button className="danger" onClick={() => { setMenuOpen(false); onDelete(presentation) }}><Trash2 size={14} /> 삭제</button>}</div></div></div>
        <div className="card-meta"><Badge tone={status.tone}>{status.label}</Badge><span><PresentationIcon size={13} /> {presentation.slideCount || presentation.slides?.length || 0}장</span></div>
      </div>
    </article>
  )
}
