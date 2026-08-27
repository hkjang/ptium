import { describe, expect, it } from 'vitest'
import { dialogIsOpen, imagesOnClipboard, pasteBelongsToSlide } from './pasteimage'

const png = (name = 'image.png') => new File([new Uint8Array([1, 2, 3])], name, { type: 'image/png' })

/** A clipboard, shaped the way an event hands one over. */
const clipboard = (options: { files?: File[]; items?: unknown[]; types?: string[] }) =>
  ({ files: options.files || [], items: options.items || [], types: options.types || [] }) as unknown as DataTransfer

const asItem = (file: File) => ({ kind: 'file', type: file.type, getAsFile: () => file })

const canvas = { closest: () => null } as unknown as Element
const textBox = { closest: (selector: string) => (selector.includes('textarea') ? textBox : null) } as unknown as Element

describe('what a clipboard is carrying', () => {
  it('finds a screenshot handed over as a file', () => {
    expect(imagesOnClipboard(clipboard({ files: [png()], types: ['Files'] }))).toHaveLength(1)
  })

  // The browsers that offer a pasted image only through items used to paste
  // nothing, silently.
  it('finds a screenshot handed over only as an item', () => {
    const found = imagesOnClipboard(clipboard({ items: [asItem(png())], types: ['Files'] }))
    expect(found).toHaveLength(1)
    expect(found[0].type).toBe('image/png')
  })

  it('ignores a clipboard carrying no picture', () => {
    expect(imagesOnClipboard(clipboard({ types: ['text/plain'] }))).toHaveLength(0)
    expect(imagesOnClipboard(null)).toHaveLength(0)
  })

  it('ignores a pasted file that is not a picture', () => {
    const zip = new File([new Uint8Array([1])], 'deck.zip', { type: 'application/zip' })
    expect(imagesOnClipboard(clipboard({ files: [zip], types: ['Files'] }))).toHaveLength(0)
  })
})

describe('whether a paste belongs to the slide', () => {
  // The one that was wrong: this is the ordinary sequence — capture a screen,
  // switch to the browser, press Ctrl+V without clicking anything first — and
  // the editor used to drop it because the canvas did not hold focus.
  it('takes a screenshot pasted onto the page itself', () => {
    expect(pasteBelongsToSlide(clipboard({ files: [png()], types: ['Files'] }), { target: null, dialogOpen: false })).toBe(true)
    expect(pasteBelongsToSlide(clipboard({ files: [png()], types: ['Files'] }), { target: canvas, dialogOpen: false })).toBe(true)
  })

  // Also wrong before: in 코드 the source editor has the focus, so the same
  // keys did nothing there at all.
  it('takes a screenshot even while a text box has the focus, when there is no text with it', () => {
    expect(pasteBelongsToSlide(clipboard({ files: [png()], types: ['Files'] }), { target: textBox, dialogOpen: false })).toBe(true)
  })

  it('leaves a paste that is also text to the text box it landed in', () => {
    const both = clipboard({ files: [png()], types: ['text/plain', 'Files'] })
    expect(pasteBelongsToSlide(both, { target: textBox, dialogOpen: false })).toBe(false)
    expect(pasteBelongsToSlide(both, { target: canvas, dialogOpen: false })).toBe(true)
  })

  it('leaves the paste alone while a dialog is open', () => {
    expect(pasteBelongsToSlide(clipboard({ files: [png()], types: ['Files'] }), { target: null, dialogOpen: true })).toBe(false)
  })

  it('does not answer a paste with no picture in it', () => {
    expect(pasteBelongsToSlide(clipboard({ types: ['text/plain'] }), { target: canvas, dialogOpen: false })).toBe(false)
  })
})

describe('noticing a dialog', () => {
  it('is open only for a modal one', () => {
    expect(dialogIsOpen({ querySelector: () => ({}) as Element })).toBe(true)
    expect(dialogIsOpen({ querySelector: () => null })).toBe(false)
  })
})
