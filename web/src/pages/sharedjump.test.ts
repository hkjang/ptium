import { describe, expect, it } from 'vitest'
import { sharedJumpTarget } from './SharedDeckPage'

// A link written [3장](#3) names the deck's third slide. A shared link leaves
// out the slides the author is skipping, so its third place is a different
// slide: sent the number alone, a reviewer clicking it landed a slide further
// on for every skipped slide before it.
describe('a jump inside a shared link', () => {
  const deck = {
    slideCount: 4,
    slides: [
      { id: 'a', title: '첫 장', position: 1 },
      { id: 'c', title: '셋째 장', position: 3 },
      { id: 'd', title: '넷째 장', position: 4 },
      { id: 'e', title: '다섯째 장', position: 5 },
    ],
  }

  it('lands on the slide the deck numbers, not the place it sits in', () => {
    expect(sharedJumpTarget(deck, 3)).toBe(2)
    expect(sharedJumpTarget(deck, 4)).toBe(3)
    expect(sharedJumpTarget(deck, 5)).toBe(4)
    expect(sharedJumpTarget(deck, 1)).toBe(1)
  })

  it('goes to the next slide it is showing when the jump names a skipped one', () => {
    expect(sharedJumpTarget(deck, 2)).toBe(2)
  })

  it('stays inside the deck when the jump points past the end', () => {
    expect(sharedJumpTarget(deck, 99)).toBe(4)
  })

  it('falls back to the number when a cached page has no positions', () => {
    expect(sharedJumpTarget({ slideCount: 4, slides: [] }, 3)).toBe(3)
    expect(sharedJumpTarget({ slideCount: 4 }, 9)).toBe(4)
  })
})
