import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { QualityDialog } from './QualityDialog'
import type { DeckFinding, DeckScore } from '../../api/client'

const score: DeckScore = {
  total: 87,
  dimensions: [
    { key: 'readability', score: 78, counted: 2 },
    { key: 'structure', score: 96, counted: 1 },
    { key: 'visual', score: 100, counted: 0 },
    { key: 'accessibility', score: 100, counted: 0 },
    { key: 'evidence', score: 62, counted: 3 },
  ],
  slides: [{ slide: 1, score: 100 }, { slide: 2, score: 70, worst: 'overflow' }, { slide: 3, score: 88 }],
  weakest: 2,
}

const findings: DeckFinding[] = [
  { slide: 2, slot: 'body', kind: 'overflow', detail: "7 lines of text in room for 5; it must shrink to 82% of the template's size", advisory: false },
  { slide: 3, slot: '', kind: 'notes', detail: 'no speaker notes: nothing is written down to say over this slide', advisory: true },
]

const dialog = (overrides: Partial<Parameters<typeof QualityDialog>[0]> = {}) => (
  <QualityDialog
    open
    findings={findings}
    score={score}
    canSafelyFix={(finding) => finding.kind === 'notes'}
    aiFixing={null}
    sweeping={{ done: 0, total: 0 }}
    onOpenSlide={() => {}}
    onSafeFix={() => {}}
    onAIFix={() => {}}
    onFixEverything={() => {}}
    onClose={() => {}}
    {...overrides}
  />
)

describe('what the measurement found', () => {
  it('answers "is this ready" before "what should I fix"', () => {
    render(dialog())
    expect(screen.getByText('87')).toBeTruthy()
    for (const axis of ['가독성', '구성', '시각', '접근성', '근거']) {
      expect(screen.getByText(axis), axis).toBeTruthy()
    }
    // And says what the number is not, which is the whole reason it can be shown.
    expect(screen.getByText(/논지가 설득력 있는지는 재지 않습니다/)).toBeTruthy()
  })

  it('sends the reader to the slide that measured worst', () => {
    const onOpenSlide = vi.fn()
    render(dialog({ onOpenSlide }))
    fireEvent.click(screen.getByRole('button', { name: /가장 낮은 슬라이드: 2번 \(70점\)/ }))
    expect(onOpenSlide).toHaveBeenCalledWith(2)
  })

  it('writes every finding in the reader’s words', () => {
    render(dialog())
    expect(screen.getByText('5줄 자리에 7줄이 들어가 템플릿 크기의 82%로 줄여야 합니다')).toBeTruthy()
    expect(screen.getByText('발표 노트가 없습니다. 이 슬라이드에서 무엇을 말할지 적혀 있지 않습니다')).toBeTruthy()
  })

  it('offers the fix that fits: safe where it is safe, the model where it is not', () => {
    const onSafeFix = vi.fn()
    const onAIFix = vi.fn()
    render(dialog({ onSafeFix, onAIFix }))
    fireEvent.click(screen.getByRole('button', { name: /안전 수정/ }))
    expect(onSafeFix).toHaveBeenCalledWith([findings[1]])
    fireEvent.click(screen.getByRole('button', { name: '  AI로 고치기'.trim() }))
    expect(onAIFix).toHaveBeenCalledWith([findings[0]])
  })

  // Half the decks measured have no speaker notes anywhere, which arrived as
  // the same sentence once per slide and pushed everything else off the panel.
  it('says a repeated finding once, and fixes every slide it names at once', () => {
    const onSafeFix = vi.fn()
    const onOpenSlide = vi.fn()
    const missing = (slide: number): DeckFinding => ({
      slide, slot: '', kind: 'notes', advisory: true,
      detail: 'no speaker notes: nothing is written down to say over this slide',
    })
    const repeated = [findings[0], missing(3), missing(5), missing(6)]
    render(dialog({ findings: repeated, onSafeFix, onOpenSlide }))
    expect(screen.getAllByText('발표 노트가 없습니다. 이 슬라이드에서 무엇을 말할지 적혀 있지 않습니다')).toHaveLength(1)
    expect(screen.getByText('3개 슬라이드')).toBeTruthy()
    // Each slide is still one click away.
    fireEvent.click(screen.getByRole('button', { name: '5번' }))
    expect(onOpenSlide).toHaveBeenCalledWith(5)
    fireEvent.click(screen.getByRole('button', { name: /3개 한번에 수정/ }))
    expect(onSafeFix).toHaveBeenCalledWith([missing(3), missing(5), missing(6)])
  })

  it('says so plainly when nothing is wrong', () => {
    render(dialog({ findings: [], score: { ...score, total: 100, weakest: 0 } }))
    expect(screen.getByText(/모든 슬라이드가 템플릿 안에 제대로 들어갑니다/)).toBeTruthy()
    expect(screen.queryByRole('button', { name: /전부 AI로 고치기/ })).toBeNull()
  })
})
