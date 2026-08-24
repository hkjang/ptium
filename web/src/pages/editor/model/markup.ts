/**
 * The gestures every editor has for marking a word, over text the deck stores
 * as what it says: **굵게**, *기울임*, [보이는 말](주소).
 *
 * These are pure functions over a string and a selection so the rule can be
 * tested without a browser, and so the same three keys work wherever text is
 * typed — a region on the canvas, a text box, the deck's own source.
 */

/** The text after the gesture, and what should be selected in it. */
export type Marked = { text: string; start: number; end: number }

/**
 * Wraps the selection in a pair of marks, or takes them off again.
 *
 * Pressing the same key twice has to leave the line as it was: an editor where
 * bold cannot be undone by the key that set it makes people delete the word and
 * type it again.
 */
export function wrapSelection(value: string, start: number, end: number, mark: string, placeholder: string): Marked {
  const selected = value.slice(start, end)
  // Both marks are made of the same character, so "already marked" has to count
  // them: the word inside **굵게** is not italic, and asking for italic there
  // must add a mark rather than take the bold off.
  if (leadingMarks(selected) === mark.length && trailingMarks(selected) === mark.length &&
      selected.length > mark.length * 2) {
    const inner = selected.slice(mark.length, selected.length - mark.length)
    return { text: value.slice(0, start) + inner + value.slice(end), start, end: start + inner.length }
  }
  if (trailingMarks(value.slice(0, start)) === mark.length && leadingMarks(value.slice(end)) === mark.length) {
    const at = start - mark.length
    return {
      text: value.slice(0, at) + selected + value.slice(end + mark.length),
      start: at,
      end: at + selected.length,
    }
  }
  const inner = selected || placeholder
  return {
    text: value.slice(0, start) + mark + inner + mark + value.slice(end),
    start: start + mark.length,
    end: start + mark.length + inner.length,
  }
}

/** How many marks a stretch of text opens with, and closes with. */
function leadingMarks(text: string): number {
  let count = 0
  while (count < text.length && text[count] === '*') count++
  return count
}

function trailingMarks(text: string): number {
  let count = 0
  while (count < text.length && text[text.length - 1 - count] === '*') count++
  return count
}

/**
 * Turns the selection into a link and leaves the address selected, because
 * that is the part the person still has to type. Nothing is asked in a dialog:
 * the markup is the text, so it can be typed over.
 */
export function linkSelection(value: string, start: number, end: number): Marked {
  const label = value.slice(start, end) || '보이는 말'
  const target = 'https://'
  const written = `[${label}](${target})`
  const at = start + written.length - 1
  return { text: value.slice(0, start) + written + value.slice(end), start: at, end: at }
}

/** The gesture a key press asks for, or nothing. */
export function markupFor(event: { key: string; ctrlKey: boolean; metaKey: boolean; altKey: boolean }) {
  if (!(event.ctrlKey || event.metaKey) || event.altKey) return null
  switch (event.key.toLowerCase()) {
    case 'b': return (value: string, start: number, end: number) => wrapSelection(value, start, end, '**', '굵게')
    case 'i': return (value: string, start: number, end: number) => wrapSelection(value, start, end, '*', '기울임')
    case 'k': return linkSelection
  }
  return null
}
