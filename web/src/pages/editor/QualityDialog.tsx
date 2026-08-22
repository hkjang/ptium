import { LoaderCircle, WandSparkles } from 'lucide-react'
import type { DeckFinding, DeckScore } from '../../api/client'
import { Button, Modal } from '../../components/UI'
import { findingDetail, findingLabel, scoreDimensionLabel } from './model/findings'

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
 */
export function QualityDialog({ open, findings, score, canSafelyFix, aiFixing, sweeping, onOpenSlide, onSafeFix, onAIFix, onFixEverything, onClose }: {
  open: boolean
  findings: DeckFinding[]
  score: DeckScore | null
  canSafelyFix: (finding: DeckFinding) => boolean
  aiFixing: number | null
  sweeping: { done: number; total: number }
  onOpenSlide: (position: number) => void
  onSafeFix: (finding: DeckFinding) => void
  onAIFix: (finding: DeckFinding) => void
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
        : <ul className="deck-findings">{findings.map((finding) => (
            <li key={`${finding.slide}-${finding.slot}-${finding.kind}`} className={finding.advisory ? 'advisory' : 'defect'}>
              <button type="button" className="finding-target" onClick={() => onOpenSlide(finding.slide)}>
                <strong>{finding.slide}번 슬라이드</strong>
                <span>{findingLabel(finding.kind)}</span>
                <small>{findingDetail(finding.detail)}</small>
              </button>
              {canSafelyFix(finding)
                ? <button type="button" className="finding-safe-fix" onClick={() => onSafeFix(finding)}><WandSparkles size={13} /> 안전 수정</button>
                : <button type="button" className="finding-safe-fix ai" disabled={aiFixing === finding.slide} onClick={() => onAIFix(finding)}>
                    {aiFixing === finding.slide ? <LoaderCircle className="spin" size={13} /> : <WandSparkles size={13} />} AI로 고치기
                  </button>}
            </li>
          ))}</ul>}
    </Modal>
  )
}
