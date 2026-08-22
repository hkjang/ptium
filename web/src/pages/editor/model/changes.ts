import type { SlideChange } from '../../../types'

/**
 * What changed since a version, in one line.
 *
 * The list itself is exact; this is the sentence someone reads before deciding
 * whether to open it — and, more often, before deciding whether to restore.
 */
export function changeSummary(changes: SlideChange[]) {
  if (changes.length === 0) return '이 버전 이후 바뀐 것이 없습니다'
  const counts = { changed: 0, added: 0, removed: 0, moved: 0 } as Record<string, number>
  for (const change of changes) counts[change.kind] = (counts[change.kind] || 0) + 1
  const parts: string[] = []
  if (counts.changed) parts.push(`${counts.changed}장 수정`)
  if (counts.added) parts.push(`${counts.added}장 추가`)
  if (counts.removed) parts.push(`${counts.removed}장 삭제`)
  if (counts.moved) parts.push(`${counts.moved}장 이동`)
  return parts.join(' · ')
}

export function changeLabel(kind: string) {
  switch (kind) {
    case 'changed': return '수정'
    case 'added': return '추가'
    case 'removed': return '삭제'
    case 'moved': return '이동'
  }
  return kind
}
