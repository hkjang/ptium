import { useEffect, useState } from 'react'
import { Check, Copy, Link2, LoaderCircle, Trash2 } from 'lucide-react'
import { Button, EmptyState, Modal } from '../../components/UI'
import type { Share } from '../../types'
import { relativeDate } from '../../utils'
import { shareLife, shareState } from './sharelife'
import { objectParticle } from '../../korean'

/**
 * Links that open this deck for someone who has no account here.
 *
 * The address is shown once, when it is made — the server keeps only a digest
 * of it — so the dialog puts the new link at the top with a copy button and
 * says as much. Everything else is housekeeping: what is open, how often it was
 * opened, and the way to close one.
 */
export function ShareDialog({
  open, deckId, onClose, load, create, revoke,
}: {
  open: boolean
  deckId: string
  onClose: () => void
  load: (id: string) => Promise<Share[]>
  create: (id: string, input: { label?: string; days?: number }) => Promise<Share>
  revoke: (id: string, shareId: string) => Promise<void>
}) {
  const [shares, setShares] = useState<Share[]>([])
  const [loading, setLoading] = useState(false)
  const [working, setWorking] = useState(false)
  const [label, setLabel] = useState('')
  const [days, setDays] = useState('0')
  const [made, setMade] = useState<Share | null>(null)
  const [copied, setCopied] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!open) { setMade(null); setCopied(false); setError(''); return }
    let active = true
    setLoading(true)
    load(deckId)
      .then((rows) => { if (active) setShares(rows) })
      .catch((problem) => { if (active) setError(problem instanceof Error ? problem.message : String(problem)) })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [open, deckId, load])

  const makeLink = async () => {
    setWorking(true); setError(''); setCopied(false)
    try {
      const share = await create(deckId, { label: label.trim(), days: Number(days) || 0 })
      setMade(share)
      setShares((current) => [share, ...current])
      setLabel('')
      if (share.url && navigator.clipboard) {
        await navigator.clipboard.writeText(share.url).then(() => setCopied(true)).catch(() => {})
      }
    } catch (problem) {
      setError(problem instanceof Error ? problem.message : String(problem))
    } finally {
      setWorking(false)
    }
  }

  const close = async (share: Share) => {
    if (!window.confirm(`"${share.label || '이름 없는 링크'}"${objectParticle(share.label || '이름 없는 링크')} 회수하면 이 주소로는 더 이상 덱이 열리지 않습니다.`)) return
    setWorking(true)
    try {
      await revoke(deckId, share.id)
      setShares((current) => current.map((row) => row.id === share.id ? { ...row, revokedAt: new Date().toISOString() } : row))
      if (made?.id === share.id) setMade(null)
    } catch (problem) {
      setError(problem instanceof Error ? problem.message : String(problem))
    } finally {
      setWorking(false)
    }
  }

  const openShares = shares.filter((share) => shareState(share) === 'open')
  return (
    <Modal open={open} onClose={onClose} title="링크로 공유"
      description="계정이 없는 사람도 이 링크로 덱을 볼 수 있습니다. 슬라이드만 보이고, 소스나 템플릿은 보이지 않습니다.">
      <div className="share-dialog">
        {error && <p className="form-error" role="alert">{error}</p>}
        <div className="share-make">
          <label className="slide-edit-field grow"><span>무엇을 위한 링크인가요</span>
            <input value={label} maxLength={120} placeholder="예: 임원 검토" onChange={(event) => setLabel(event.target.value)} />
          </label>
          <label className="slide-edit-field"><span>유효 기간</span>
            <select value={days} onChange={(event) => setDays(event.target.value)}>
              <option value="0">직접 회수할 때까지</option>
              <option value="7">7일</option>
              <option value="30">30일</option>
              <option value="90">90일</option>
            </select>
          </label>
          <Button onClick={() => void makeLink()} disabled={working}>
            {working ? <LoaderCircle className="spin" size={15} /> : <Link2 size={15} />} 링크 만들기
          </Button>
        </div>

        {made?.url && (
          <div className="share-made">
            <strong>{copied ? '복사했습니다' : '이 주소는 지금 한 번만 보입니다'}</strong>
            <code>{made.url}</code>
            <Button variant="secondary" size="small" onClick={() => {
              void navigator.clipboard?.writeText(made.url || '').then(() => setCopied(true))
            }}>{copied ? <Check size={14} /> : <Copy size={14} />} 복사</Button>
          </div>
        )}

        {loading ? <p className="inspector-help"><LoaderCircle className="spin" size={14} /> 불러오는 중…</p>
          : openShares.length === 0 && shares.length === 0
            ? <EmptyState title="아직 만든 링크가 없습니다" description="위에서 하나 만들어 보세요." />
            : <ul className="share-list">
              {shares.map((share) => (
                <li key={share.id} className={shareState(share) === 'open' ? '' : 'revoked'}>
                  <div>
                    <strong>{share.label || '이름 없는 링크'}</strong>
                    <small>
                      {shareLife(share)}
                      {' · '}{share.views}회 열림
                      {share.lastSeenAt ? ` · 마지막 ${relativeDate(share.lastSeenAt)}` : ''}
                    </small>
                  </div>
                  {shareState(share) === 'open' && (
                    <Button variant="ghost" size="small" disabled={working} onClick={() => void close(share)}>
                      <Trash2 size={14} /> 회수
                    </Button>
                  )}
                </li>
              ))}
            </ul>}
      </div>
    </Modal>
  )
}
