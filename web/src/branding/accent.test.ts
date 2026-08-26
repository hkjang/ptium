import { describe, expect, it } from 'vitest'
import { accentNote, isSeededAccent, seededAccent } from './accent'

describe('the colour a screen is showing', () => {
  it('knows the seeded colour is nobody’s choice, however it is written', () => {
    for (const written of [seededAccent, '#7c3aed', ' #7C3AED ']) {
      expect(isSeededAccent(written), written).toBe(true)
    }
    expect(isSeededAccent('#0F62FE')).toBe(false)
  })

  it('takes the seeded colour from the deployment when it says one', () => {
    // A deployment can be told what it shipped with; the constant is the
    // fallback for an older server that does not say.
    expect(isSeededAccent('#123456', '#123456')).toBe(true)
    expect(isSeededAccent(seededAccent, '#123456')).toBe(false)
    expect(isSeededAccent(seededAccent, '')).toBe(true)
  })

  it('does not promise the seeded colour will be drawn', () => {
    const seeded = accentNote(seededAccent)
    expect(seeded).toContain('그대로 둡니다')
    expect(seeded).toContain('기본으로 갖고 온 색')
    // A colour somebody picked is used, and the note says where.
    const chosen = accentNote('#0F62FE')
    expect(chosen).toContain('사용됩니다')
    expect(chosen).not.toContain('기본으로 갖고 온 색')
  })
})
