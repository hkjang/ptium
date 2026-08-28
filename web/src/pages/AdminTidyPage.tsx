import { useCallback, useEffect, useState } from 'react'
import { Archive, FileWarning, History, ImageOff, Link2Off, PencilLine, RefreshCw, ScrollText, Trash2 } from 'lucide-react'
import { api } from '../api/client'
import { AppShell } from '../components/AppShell'
import { Button, ErrorState, LoadingState } from '../components/UI'
import { displayError, formatDate } from '../utils'

/**
 * What has accumulated in this deployment and is going nowhere.
 *
 * This screen deletes nothing and proposes no rule. What to keep and for how
 * long is a decision somebody has to make, and it cannot be made without
 * knowing what is there: a deployment running a year holds decks somebody
 * binned in March and images no deck has ever drawn, and neither shows anywhere.
 */
export type TidyItem = { kind: string; count: number; bytes?: number; oldest?: string }

const said: Record<string, { title: string; note: string; icon: React.ReactNode }> = {
  trashed: { title: '휴지통에 있는 덱', icon: <Trash2 size={17} />,
    note: '작성자가 지운 덱입니다. 아무것도 자동으로 지워지지 않으므로 그대로 남아 있습니다.' },
  failedOldDecks: { title: '30일 넘게 실패로 남은 덱', icon: <FileWarning size={17} />,
    note: '생성에 실패한 뒤 아무도 다시 시도하지 않은 덱입니다.' },
  untouchedDrafts: { title: '90일 넘게 손대지 않은 초안', icon: <PencilLine size={17} />,
    note: '슬라이드가 만들어지지 않은 채 남은 초안입니다.' },
  expiredLinks: { title: '기한이 지난 공유 링크', icon: <Link2Off size={17} />,
    note: '이미 열리지 않는 링크입니다. 목록에는 남아 있습니다.' },
  unusedImages: { title: '어느 덱도 쓰지 않는 이미지', icon: <ImageOff size={17} />,
    note: '오늘 올려 아직 넣지 않은 이미지도 여기 포함됩니다. 지울 대상으로 보기 전에 아래 항목을 보세요.' },
  unusedImagesOverAMonth: { title: '한 달 넘게 쓰이지 않은 이미지', icon: <Archive size={17} />,
    note: '올린 지 30일이 지나도록 어느 덱에도 들어가지 않은 이미지입니다.' },
  // These two grow with every day the deployment is used rather than with
  // anything anybody forgot to tidy, and on a deployment a week old each was
  // larger than everything above them.
  deckRevisions: { title: '덱을 고칠 때마다 쌓인 판본', icon: <History size={17} />,
    note: '되돌리기가 딛고 있는 기록입니다. 덱을 고칠 때마다 한 판본씩 늘어납니다. 지우면 그만큼 되돌릴 수 없습니다.' },
  auditHistory: { title: '감사 기록', icon: <ScrollText size={17} />,
    note: '누가 무엇을 했는지 남긴 기록입니다. 쓸수록 늘어납니다. 얼마나 오래 보관할지는 이 배포가 정할 일입니다.' },
}

/** A size said the way an operator reads it. */
export function howBig(bytes?: number) {
  if (!bytes) return ''
  if (bytes < 1024 * 1024) return `${Math.max(1, Math.round(bytes / 1024))}KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)}MB`
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)}GB`
}

/** What this row says when there is nothing of its kind. */
export function nothingWord(item: TidyItem) {
  return item.count === 0 ? '없습니다' : ''
}

export function AdminTidyPage() {
  const [items, setItems] = useState<TidyItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const load = useCallback(async () => {
    setLoading(true); setError('')
    try { setItems((await api.adminTidy()).items as TidyItem[]) }
    catch (err) { setError(displayError(err)) } finally { setLoading(false) }
  }, [])
  useEffect(() => { void load() }, [load])

  return <AppShell title="정리할 것" eyebrow="WHAT HAS PILED UP"
    actions={<Button variant="secondary" onClick={() => void load()}><RefreshCw size={15} /> 새로고침</Button>}>
    <div className="security-banner">
      <Archive size={20} />
      <div><strong>이 화면은 아무것도 지우지 않습니다</strong>
        <p>무엇을 얼마나 오래 두느냐는 정해야 하는 일이고, 무엇이 쌓였는지 모르면 정할 수 없습니다.
          아래 숫자는 그 판단을 위한 것입니다. 자동 삭제는 없습니다.</p></div>
    </div>
    {loading ? <LoadingState label="쌓인 것을 세는 중…" />
      : error ? <ErrorState message={error} onRetry={() => void load()} />
      : <section className="admin-panel"><ul className="tidy-list">
        {items.map((item) => {
          const words = said[item.kind] || { title: item.kind, note: '', icon: <Archive size={17} /> }
          return <li key={item.kind} className={item.count === 0 ? 'quiet' : ''}>
            <span className="tidy-icon">{words.icon}</span>
            <div>
              <strong>{words.title}</strong>
              <small>{words.note}</small>
              {item.oldest && <small className="muted-note">가장 오래된 것 {formatDate(item.oldest, { year: 'numeric', month: 'short', day: 'numeric' })}</small>}
            </div>
            <b>{item.count === 0 ? nothingWord(item) : `${item.count.toLocaleString('ko-KR')}개`}</b>
            <span className="tidy-size">{howBig(item.bytes)}</span>
          </li>
        })}
      </ul></section>}
  </AppShell>
}
