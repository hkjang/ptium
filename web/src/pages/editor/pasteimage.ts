/**
 * What Ctrl+V means while a deck is open for editing.
 *
 * A captured screenshot is the commonest thing anybody puts on a slide, and the
 * gesture people already know is the one Google Slides answers: capture, switch
 * to the browser, paste. The editor used to answer it only while the canvas
 * itself held focus, which is never true just after a deck is opened — so the
 * ordinary sequence pasted nothing and explained nothing. And it was wired into
 * the canvas alone, so the same keys did nothing at all in 코드 or
 * 템플릿 미리보기.
 *
 * The decision lives here, apart from the editor, because it is a rule rather
 * than a rendering: given a clipboard and where the paste landed, is this an
 * image for the current slide?
 */

/** The image files a paste is carrying, in the order the clipboard offers them. */
export function imagesOnClipboard(data: DataTransfer | null | undefined): File[] {
  if (!data) return []
  const images = Array.from(data.files || []).filter((file) => file.type.startsWith('image/'))
  if (images.length > 0) return images
  // Some browsers hand a pasted screenshot over as items rather than as files.
  // Reading both is what makes the gesture work everywhere it is tried.
  const items = Array.from(data.items || [])
  return items
    .filter((item) => item.kind === 'file' && item.type.startsWith('image/'))
    .map((item) => item.getAsFile())
    .filter((file): file is File => file !== null)
}

/** Where a paste landed, as much of it as the rule needs. */
export interface PasteLanding {
  /** The element the paste was aimed at. */
  target: Element | null
  /** True while a modal dialog is open: it owns the keyboard, so the slide does not. */
  dialogOpen: boolean
}

/** True when the paste is a text box's own business rather than the slide's. */
function typingHere(target: Element | null): boolean {
  if (!target || typeof (target as HTMLElement).closest !== 'function') return false
  return (target as HTMLElement).closest('input, textarea, [contenteditable="true"]') !== null
}

/**
 * Whether the editor should take this paste as an image for the current slide.
 *
 * The one case the slide gives way on is a paste that is also text landing in a
 * text box: somebody copying a paragraph out of a document carries a picture of
 * it too, and there the person is plainly typing. An image with no text beside
 * it means nothing to a text box — a textarea would swallow the keys and insert
 * nothing — so the slide takes it even mid-sentence in the source editor.
 */
export function pasteBelongsToSlide(data: DataTransfer | null | undefined, landing: PasteLanding): boolean {
  if (imagesOnClipboard(data).length === 0) return false
  if (landing.dialogOpen) return false
  const alsoText = Array.from(data?.types || []).includes('text/plain')
  return !(alsoText && typingHere(landing.target))
}

/** True while a modal dialog is on screen. */
export function dialogIsOpen(root: Document | { querySelector(selectors: string): Element | null }): boolean {
  return root.querySelector('[role="dialog"][aria-modal="true"]') !== null
}
