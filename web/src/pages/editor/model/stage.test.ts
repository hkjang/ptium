import { describe, expect, it } from 'vitest'
import { stageText } from './stage'

describe('what a generation is doing', () => {
  it('says each pass in the reader\'s words', () => {
    for (const stage of ['planning', 'writing', 'binding', 'fitting', 'notes']) {
      const said = stageText(stage, false)
      expect(said, stage).not.toBe('')
      // The reader's language, not the server's keys.
      expect(said, stage).not.toMatch(/[a-z]{3,}/)
    }
  })

  it('says a rewrite is a rewrite', () => {
    expect(stageText('writing', true)).toContain('다시')
    expect(stageText('writing', false)).not.toContain('다시')
  })

  // A newer server saying something new should leave the screen as it was
  // rather than printing a key at somebody.
  it('says nothing about a pass it does not know', () => {
    expect(stageText('polishing-the-brass', false)).toBe('')
    expect(stageText(undefined, false)).toBe('')
    expect(stageText('', false)).toBe('')
  })
})
