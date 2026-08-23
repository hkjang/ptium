import { LoaderCircle, WandSparkles } from 'lucide-react'
import type { DeckFinding, DeckScore } from '../../api/client'
import { Button, Modal } from '../../components/UI'
import { findingDetail, findingLabel, groupFindings, scoreDimensionLabel } from './model/findings'

/**
 * What the measurement found, and what the deck scores.
 *
 * The order is the point: the score first, because "is this ready" is the
 * question people ask before "what should I fix"; then the slide that measured
 * worst, which is where to go; then the findings themselves, each in the
 * reader's own words with the one action that fixes it.
 *
 * The note at the bottom is not decoration. A score that let itself be read as
 * a judgement of the argument would be lying.
 *
 * Findings that say the same sentence about different slides are one row. Half
 * the decks measured have no speaker notes anywhere, and listing that eight
 * times pushed everything else off the panel — including the findings that were
 * about this deck rather than about a habit. One row, every slide it applies
 * to, and one button that fixes all of them.
 */
export function QualityDialog({ open, findings, score, canSafelyFix, aiFixing, sweeping, onOpenSlide, onSafeFix, onAIFix, onFixEverything, onClose }: {
  open: boolean
  findings: DeckFinding[]
  score: DeckScore | null
  canSafelyFix: (finding: DeckFinding) => boolean
  aiFixing: number | null
  sweeping: { done: number; total: number }
  onOpenSlide: (position: number) => void
  onSafeFix: (findings: DeckFinding[]) => void
  onAIFix: (findings: DeckFinding[]) => void
  onFixEverything: () => void
  onClose: () => void
}) {
  return (
    <Modal
      open={open}
      onClose={onClose}
      title="그려진 슬라이드 측정 결과"
      description="결함은 잘못 그려진 것, 다듬을 곳은 제대로 그려졌지만 더 좋아질 수 있는 것입니다."
      footer={<>
        {findings.some((finding) => !canSafelyFix(finding)) && (
          <Button variant="secondary" disabled={sweeping.total > 0} onClick={onFixEverything}>
            <WandSparkles size={14} /> 전부 AI로 고치기
          </Button>
        )}
        <Button variant="secondary" onClick={onClose}>닫기</Button>
      </>}
    >
      {score && <div className="deck-score">
        <div className="deck-score-total"><strong>{score.total}</strong><span>측정된 품질</span></div>
        <ul className="deck-score-dimensions">{score.dimensions.map((dimension) => (
          <li key={dimension.key}>
            <span>{scoreDimensionLabel(dimension.key)}</span>
            <i><b style={{ width: `${Math.max(2, dimension.score)}%` }} /></i>
            <small>{dimension.score}</small>
          </li>
        ))}</ul>
        {score.weakest > 0 && <button type="button" className="deck-score-weakest" onClick={() => onOpenSlide(score.weakest)}>
          가장 낮은 슬라이드: {score.weakest}번 ({score.slides[score.weakest - 1]?.score ?? 0}점)
        </button>}
        <p className="deck-score-note">점수는 <b>그려진 것</b>을 잰 결과입니다. 논지가 설득력 있는지는 재지 않습니다.</p>
      </div>}
      {findings.length === 0
        ? <p className="modal-note">모든 슬라이드가 템플릿 안에 제대로 들어갑니다.</p>
        : <ul className="deck-findings">{groupFindings(findings).map((group) => {
            const first = group[0]
            const many = group.length > 1
            const busy = group.some((finding) => aiFixing === finding.slide) || sweeping.total > 0
            return (
              <li key={`${first.slide}-${first.slot}-${first.kind}-${group.length}`} className={first.advisory ? 'advisory' : 'defect'}>
                <button type="button" className="finding-target" onClick={() => onOpenSlide(first.slide)}>
                  <strong>{many ? `${group.length}개 슬라이드` : `${first.slide}번 슬라이드`}</strong>
                  <span>{findingLabel(first.kind)}</span>
                  <small>{findingDetail(first.detail)}</small>
                </button>
                {many && <span className="finding-slides">
                  {group.slice(0, 12).map((finding) => (
                    <button type="button" key={finding.slide} onClick={() => onOpenSlide(finding.slide)}>{finding.slide}번</button>
                  ))}
                  {group.length > 12 && <small>외 {group.length - 12}장</small>}
                </span>}
                {canSafelyFix(first)
                  ? <button type="button" className="finding-safe-fix" onClick={() => onSafeFix(group)}>
                      <WandSparkles size={13} /> {many ? `${group.length}개 한번에 수정` : '안전 수정'}
                    </button>
                  : <button type="button" className="finding-safe-fix ai" disabled={busy} onClick={() => onAIFix(group)}>
                      {busy ? <LoaderCircle className="spin" size={13} /> : <WandSparkles size={13} />} {many ? `${group.length}개 AI로 고치기` : 'AI로 고치기'}
                    </button>}
              </li>
            )
          })}</ul>}
    </Modal>
  )
}
