/**
 * What a generation is doing right now, in the reader's words.
 *
 * A deck takes a minute or three on a self-hosted model, and the screen used to
 * say the same sentence for all of it — at five seconds and at three minutes,
 * which is indistinguishable from a screen that has stopped. The server reports
 * the pass it is in; these are those passes, said plainly.
 *
 * A stage this does not know is not shown: a newer server saying something new
 * should leave the screen as it was rather than printing a key at somebody.
 */
export function stageText(stage: string | undefined, rewriting: boolean): string {
  switch (stage) {
    case 'planning': return '무엇을 어떤 순서로 말할지 정하고 있어요'
    case 'writing': return rewriting ? '슬라이드를 다시 쓰고 있어요' : '슬라이드를 쓰고 있어요'
    case 'binding': return '템플릿에 맞춰 슬라이드를 앉히고 있어요'
    case 'fitting': return '자리에 맞는지 재고, 넘치는 장을 고치고 있어요'
    case 'notes': return '무엇을 말할지 발표 노트를 채우고 있어요'
  }
  return ''
}
