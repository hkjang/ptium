import { useCallback, useEffect, useState } from 'react'
import { History, Undo2 } from 'lucide-react'
import { api } from '../api/client'
import { settingName } from '../api/errors'
import { Badge, Button } from '../components/UI'
import { useToast } from '../components/Toast'
import { displayError, relativeDate } from '../utils'

/**
 * What was changed in the settings, and putting one back.
 *
 * The trail recorded a settings change as "settings.update_batch" with a count:
 * which setting, what it had been and what it became were nowhere. An operator
 * asking who turned the repair pass off last month, and what it was before, had
 * a number. These are the settings that decide how every deck in the deployment
 * is written, so the answer belongs on the screen that changes them.
 */
export type SettingChange = {
  id: number
  targetId?: string
  createdAt: string
  actorName?: string
  actorEmail?: string
  metadata?: { key?: string; from?: unknown; to?: unknown; sensitive?: boolean }
}

/** What a stored value reads as in a sentence. */
export function said(value: unknown) {
  if (value === undefined || value === null) return '없음'
  if (typeof value === 'boolean') return value ? '사용' : '사용 안 함'
  if (Array.isArray(value)) return value.length ? value.join(' · ') : '없음'
  if (typeof value === 'object') return JSON.stringify(value)
  const text = String(value)
  return text.trim() === '' ? '비어 있음' : text
}

/** Whether this change can be undone from the trail alone. */
export function revertable(change: SettingChange) {
  const detail = change.metadata || {}
  return !detail.sensitive && detail.from !== undefined && detail.from !== null
}

export function SettingChanges({ onReverted }: { onReverted?: () => void }) {
  const [changes, setChanges] = useState<SettingChange[]>([])
  const [working, setWorking] = useState(0)
  const { showToast } = useToast()
  const load = useCallback(async () => {
    try { setChanges(await api.settingChanges(8) as SettingChange[]) } catch { setChanges([]) }
  }, [])
  useEffect(() => { void load() }, [load])
  if (changes.length === 0) return null

  const revert = async (change: SettingChange) => {
    setWorking(change.id)
    try {
      await api.revertSettingChange(change.id)
      showToast(`${settingName(change.metadata?.key || change.targetId || '')}을(를) 이전 값으로 되돌렸습니다.`)
      await load()
      onReverted?.()
    } catch (err) { showToast(displayError(err), 'error') } finally { setWorking(0) }
  }

  return <section className="setting-changes">
    <div className="setting-changes-head"><History size={15} /> <strong>최근 설정 변경</strong>
      <Badge tone="info">{changes.length}건</Badge></div>
    <ul>
      {changes.map((change) => {
        const detail = change.metadata || {}
        const name = settingName(detail.key || change.targetId || '')
        return <li key={change.id}>
          <div>
            <strong>{name}</strong>
            <small>{detail.sensitive
              ? '값이 바뀌었습니다 — 비밀 값은 기록하지 않습니다'
              : `${said(detail.from)} → ${said(detail.to)}`}</small>
            <small className="muted-note">{change.actorName || change.actorEmail || '알 수 없는 사용자'} · {relativeDate(change.createdAt)}</small>
          </div>
          {revertable(change)
            ? <Button variant="secondary" size="small" disabled={working === change.id}
                onClick={() => void revert(change)}><Undo2 size={14} /> 되돌리기</Button>
            : <span className="muted-note">되돌릴 수 없음</span>}
        </li>
      })}
    </ul>
  </section>
}
