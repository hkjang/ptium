/**
 * What to say over a slide, when the slide cannot say it.
 *
 * The one-click note draft wrote `${title}: ${first point}` — the words already
 * on the slide, joined by a colon. A presenter reading that aloud says what the
 * room is reading, which is the thing every guide on presenting tells you not
 * to do, and a slide with only a title got its title twice.
 *
 * Speaker notes are the weakest part of this whole category of tool: reviews
 * that test them find most platforms either skip notes or produce filler, and
 * that a note worth having is a cue — what to open on, what to explain, what to
 * leave for questions, and how to hand over to the next slide.
 *
 * So the draft is a cue, chosen by what the slide actually holds. It is written
 * without a model, which is why it says what to do rather than pretending to
 * know the argument: nobody but the author knows why the number is what it is,
 * and a note that guessed would be worse than filler.
 */

import type { Slide } from '../../../types'
import { toParticle } from '../../../korean'
import { slideBodyLines } from './slides'

/** cueFor is what a presenter has to do on a slide of this shape. */
function cueFor(slide: Slide, position: number, total: number): string {
  const kinds = Object.values(slide.blocks || {}).map((block) => String(block.kind))
  const points = slideBodyLines(slide)
  const holdsAPicture = Object.keys(slide.images || {}).length > 0
  if (position === 1) {
    return '왜 지금 이 이야기를 하는지 한 문장으로 밝히고, 오늘의 결론을 먼저 말합니다.'
  }
  if (position === total && total > 1) {
    return '무엇을 결정해 달라는 것인지 한 문장으로 남기고 마칩니다.'
  }
  if (kinds.includes('steps') || kinds.includes('timeline')) {
    return '각 단계의 완료 조건을 한 문장씩 말하고, 순서를 바꿀 수 없는 이유를 덧붙입니다.'
  }
  if (kinds.includes('comparison')) {
    return '권고안을 먼저 말하고, 고르지 않은 쪽을 접은 이유를 한 가지만 덧붙입니다.'
  }
  if (kinds.includes('kpi') || kinds.includes('hero') || kinds.includes('meter')) {
    return '숫자를 읽지 말고, 그 숫자가 왜 그렇게 나왔는지부터 말합니다.'
  }
  if (kinds.some((kind) => kind.endsWith('Chart') || kind === 'shareBar')) {
    return '차트에서 눈이 가야 할 곳 한 군데를 먼저 가리키고, 나머지는 질문이 나오면 말합니다.'
  }
  if (kinds.includes('table')) {
    return '표를 다 읽지 말고, 한 줄만 짚어 그것이 무엇을 뜻하는지 말합니다.'
  }
  if (kinds.includes('quote') || kinds.includes('callout')) {
    return '소리 내어 한 번 읽고, 왜 이 말을 가져왔는지 덧붙입니다.'
  }
  if (holdsAPicture && points.length === 0) {
    return '그림이 무엇을 보여 주는지 한 문장으로 말하고 넘어갑니다.'
  }
  if (points.length >= 4) {
    return '요점을 다 읽지 말고, 가장 중요한 하나만 풀어서 말합니다.'
  }
  return '이 장에서 남길 한 가지를 정해 두고, 나머지는 질문이 나오면 말합니다.'
}

/**
 * noteDraft is the cue for one slide, with the hand-off to the next.
 *
 * The hand-off is the part a presenter cannot read off the wall: what is
 * coming is behind them. Reviews of these tools name transitions as the thing
 * generated notes miss most.
 */
export function noteDraft(slides: Slide[], index: number): string {
  const slide = slides[index]
  if (!slide) return ''
  const cue = cueFor(slide, index + 1, slides.length)
  const next = (slides[index + 1]?.title || '').trim()
  if (!next) return cue.slice(0, 4000)
  return `${cue} 그다음 ${next}${toParticle(next)} 넘어갑니다.`.slice(0, 4000)
}
