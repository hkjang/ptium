import { useEffect, useMemo, useState } from 'react'
import { CaseSensitive, Replace, Search, WholeWord } from 'lucide-react'
import { Button, Input, Modal } from '../../components/UI'
import type { Slide } from '../../types'
import { findInDeck, whereLabel } from './model/search'
import { blockLabel } from './model/slides'

/**
 * Find and replace, over the whole deck.
 *
 * The list is the point: a deck of fifty slides answers "where does this word
 * appear" faster than any amount of clicking through thumbnails, and each hit
 * carries the slide number and the region it sits in — a bullet, a KPI card, a
 * table cell, the speaker notes — so the reader can tell before they jump
 * whether it is the one they meant.
 */
export function FindDialog({ open, slides, onClose, onOpenSlide, onReplace }: {
  open: boolean
  slides: Slide[]
  onClose: () => void
  onOpenSlide: (position: number) => void
  onReplace: (query: string, replacement: string, options: { matchCase: boolean; wholeWord: boolean },
              only?: { slideId?: string }) => number
}) {
  const [query, setQuery] = useState('')
  const [replacement, setReplacement] = useState('')
  const [matchCase, setMatchCase] = useState(false)
  const [wholeWord, setWholeWord] = useState(false)
  const [said, setSaid] = useState('')

  useEffect(() => { if (open) setSaid('') }, [open])

  const matches = useMemo(
    () => (open ? findInDeck(slides, query.trim(), { matchCase, wholeWord }, blockLabel) : []),
    [open, slides, query, matchCase, wholeWord])

  const bySlide = useMemo(() => {
    const grouped = new Map<number, typeof matches>()
    for (const match of matches) {
      const list = grouped.get(match.slide)
      if (list) list.push(match)
      else grouped.set(match.slide, [match])
    }
    return [...grouped.entries()]
  }, [matches])

  const replaceEverywhere = () => {
    const count = onReplace(query.trim(), replacement, { matchCase, wholeWord })
    setSaid(count > 0 ? `${count}곳을 바꿨습니다.` : '바꿀 곳이 없습니다.')
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="덱에서 찾기 · 바꾸기"
      description="제목, 본문, 컴포넌트 안의 값, 캔버스 개체, 표, 발표자 노트까지 모두 찾습니다."
      footer={<>
        <Button variant="secondary" disabled={!query.trim() || matches.length === 0} onClick={replaceEverywhere}>
          <Replace size={14} /> 전부 바꾸기{matches.length > 0 ? ` (${matches.length})` : ''}
        </Button>
        <Button variant="secondary" onClick={onClose}>닫기</Button>
      </>}
    >
      <div className="find-fields">
        <Input autoFocus value={query} onChange={(event) => setQuery(event.target.value)}
               placeholder="찾을 말" aria-label="찾을 말" />
        <Input value={replacement} onChange={(event) => setReplacement(event.target.value)}
               placeholder="바꿀 말 (비우면 지웁니다)" aria-label="바꿀 말" />
      </div>
      <div className="find-options">
        <button type="button" className={matchCase ? 'active' : ''} aria-pressed={matchCase}
                onClick={() => setMatchCase((value) => !value)} title="대소문자를 구분합니다">
          <CaseSensitive size={14} /> 대소문자
        </button>
        <button type="button" className={wholeWord ? 'active' : ''} aria-pressed={wholeWord}
                onClick={() => setWholeWord((value) => !value)} title="낱말 전체가 일치할 때만 찾습니다">
          <WholeWord size={14} /> 낱말 단위
        </button>
        <span>{query.trim() ? `${matches.length}곳` : ''}{said && ` · ${said}`}</span>
      </div>

      {query.trim() && matches.length === 0
        ? <p className="modal-note"><Search size={14} /> 이 덱에는 그 말이 없습니다.</p>
        : <ul className="find-results">{bySlide.map(([slide, hits]) => (
            <li key={slide}>
              <button type="button" className="find-slide" onClick={() => onOpenSlide(slide)}>
                <strong>{slide}번 슬라이드</strong><span>{hits.length}곳</span>
              </button>
              <ul>{hits.slice(0, 6).map((hit, index) => (
                <li key={`${hit.where}-${hit.start}-${index}`}>
                  <em>{hit.label || whereLabel(hit.where)}</em>
                  <span>
                    {hit.text.slice(Math.max(0, hit.start - 24), hit.start)}
                    <mark>{hit.text.slice(hit.start, hit.end)}</mark>
                    {hit.text.slice(hit.end, hit.end + 24)}
                  </span>
                </li>
              ))}
              {hits.length > 6 && <li><em /><span>외 {hits.length - 6}곳</span></li>}
              </ul>
              <button type="button" className="find-replace-one" disabled={!query.trim()}
                      onClick={() => {
                        const count = onReplace(query.trim(), replacement, { matchCase, wholeWord },
                                                { slideId: hits[0].slideId })
                        setSaid(`${slide}번 슬라이드에서 ${count}곳을 바꿨습니다.`)
                      }}>
                이 슬라이드만 바꾸기
              </button>
            </li>
          ))}</ul>}
    </Modal>
  )
}
