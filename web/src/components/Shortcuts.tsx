import { useCallback, useEffect, useMemo, useState } from 'react'
import { Modal } from './UI'

/**
 * The keys the workspace answers to.
 *
 * Every one of these was already implemented and known only to whoever read the
 * source. Written down once, they can be shown in the editor, over a talk, and
 * in the guide — three places that must never disagree, which they will if each
 * keeps its own list.
 */
export interface Shortcut { keys: string[]; label: string }
export interface ShortcutGroup { title: string; items: Shortcut[] }

const modifier = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform) ? '⌘' : 'Ctrl'

export const editorShortcuts: ShortcutGroup[] = [
  {
    title: '덱',
    items: [
      { keys: [modifier, 'S'], label: '지금 저장 (평소에는 자동 저장)' },
      { keys: ['F5'], label: '발표 시작' },
      { keys: ['?'], label: '이 단축키 목록' },
      { keys: ['Esc'], label: '선택 해제 · 편집 종료' },
    ],
  },
  {
    title: '슬라이드',
    items: [
      { keys: [modifier, 'Enter'], label: '슬라이드 추가' },
      { keys: ['Alt', '↑'], label: '슬라이드를 앞으로' },
      { keys: ['Alt', '↓'], label: '슬라이드를 뒤로' },
      { keys: ['Alt', 'PageUp'], label: '이전 슬라이드 선택' },
      { keys: ['Alt', 'PageDown'], label: '다음 슬라이드 선택' },
    ],
  },
  {
    title: '캔버스 · 개체',
    items: [
      { keys: ['더블클릭'], label: '글상자 · 영역 텍스트 편집' },
      { keys: ['끌기'], label: '빈 곳에서 끌면 여러 개 선택' },
      { keys: ['Shift', '끌기'], label: '축 고정 이동 · 비율 유지 · 15° 회전' },
      { keys: ['Alt', '끌기'], label: '복제하며 끌기' },
      { keys: [modifier, 'Z'], label: '실행 취소 (Shift로 다시 실행)' },
      { keys: [modifier, 'C'], label: '복사' },
      { keys: [modifier, 'V'], label: '붙여넣기' },
      { keys: [modifier, 'D'], label: '복제' },
      { keys: [modifier, 'A'], label: '모두 선택' },
      { keys: [modifier, 'G'], label: '그룹 · Shift로 해제' },
      { keys: [modifier, ']'], label: '맨 앞으로' },
      { keys: [modifier, '['], label: '맨 뒤로' },
      { keys: ['←', '→', '↑', '↓'], label: '조금씩 이동 (Shift로 크게)' },
      { keys: ['Delete'], label: '삭제' },
    ],
  },
  {
    title: '이미지',
    items: [
      { keys: [modifier, 'V'], label: '복사한 이미지를 캔버스에 붙여넣기' },
      { keys: ['끌어다 놓기'], label: '이미지 파일을 캔버스에 놓아 올리기' },
    ],
  },
]

export const presentationShortcuts: ShortcutGroup[] = [
  {
    title: '넘기기',
    items: [
      { keys: ['→', 'Space', 'PageDown'], label: '다음' },
      { keys: ['←', 'PageUp'], label: '이전' },
      { keys: ['숫자', 'Enter'], label: '그 번호의 슬라이드로' },
      { keys: ['Home', 'End'], label: '처음 · 마지막' },
    ],
  },
  {
    title: '화면',
    items: [
      { keys: ['G'], label: '전체 슬라이드 목록' },
      { keys: ['B'], label: '검은 화면' },
      { keys: ['W'], label: '흰 화면' },
      { keys: ['L'], label: '레이저 포인터' },
      { keys: ['F'], label: '전체 화면' },
      { keys: ['P'], label: '발표자 보기 창 열기' },
      { keys: ['?'], label: '이 단축키 목록' },
      { keys: ['Esc'], label: '발표 종료' },
    ],
  },
]

export function ShortcutTable({ groups }: { groups: ShortcutGroup[] }) {
  return (
    <div className="shortcut-groups">
      {groups.map((group) => (
        <section key={group.title} className="shortcut-group">
          <h4>{group.title}</h4>
          <dl>
            {group.items.map((item) => (
              <div key={item.label + item.keys.join()}>
                <dt>{item.keys.map((key) => <kbd key={key}>{key}</kbd>)}</dt>
                <dd>{item.label}</dd>
              </div>
            ))}
          </dl>
        </section>
      ))}
    </div>
  )
}

export function ShortcutSheet({ open, onClose, groups, title = '단축키' }: {
  open: boolean; onClose: () => void; groups: ShortcutGroup[]; title?: string
}) {
  return (
    <Modal open={open} onClose={onClose} title={title} description="? 를 누르면 언제든 다시 열립니다." wide>
      <ShortcutTable groups={groups} />
    </Modal>
  )
}

/**
 * useShortcutSheet answers "?" the way every tool with shortcuts does.
 *
 * Typing into a field must never open it: a question mark belongs in the
 * sentence someone is writing.
 */
export function useShortcutSheet() {
  const [open, setOpen] = useState(false)
  const close = useCallback(() => setOpen(false), [])
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== '?' || event.ctrlKey || event.metaKey || event.altKey) return
      const target = event.target as HTMLElement | null
      if (target?.matches?.('input, textarea, select, [contenteditable="true"]')) return
      event.preventDefault()
      setOpen((value) => !value)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])
  // A stable object: callers put it in effect dependencies.
  return useMemo(() => ({ open, setOpen, close }), [open, close])
}
