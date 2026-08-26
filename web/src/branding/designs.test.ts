import { describe, expect, it } from 'vitest'
import { designChoices, designFamilies, legacyThemeAliases, resolveDesignKey } from './designs'

// The library answers with its own order, and a listing can arrive in any
// order at all — the rank is what says which design comes first.
const library = [
  { id: 'four', kind: 'builtin', paletteKey: 'azure-arc', name: 'Ptium Azure Arc', designRank: 4 },
  { id: 'three', kind: 'builtin', paletteKey: 'azure-classic', name: 'Ptium Azure Classic', designRank: 3 },
  { id: 'five', kind: 'builtin', paletteKey: 'plum-rail', name: 'Ptium Plum Rail', designRank: 5 },
  { id: 'one', kind: 'builtin', paletteKey: 'slate-classic', name: 'Ptium Slate Classic', designRank: 1 },
  { id: 'two', kind: 'builtin', paletteKey: 'slate-panel', name: 'Ptium Slate Panel', designRank: 2 },
  { id: 'six', kind: 'builtin', paletteKey: 'graphite-minimal', name: 'Ptium Graphite Minimal', designRank: 6 },
  { id: 'seven', kind: 'builtin', paletteKey: 'graphite-classic', name: 'Ptium Graphite Classic', designRank: 7 },
  // An uploaded template is somebody's own file, not one of the shipped designs.
  { id: 'mine', kind: 'uploaded', paletteKey: 'slate-classic', name: '우리 회사 템플릿' },
]

describe('the designs a deployment actually ships', () => {
  it('lists each shipped design once, and no uploaded file', () => {
    const choices = designChoices(library)
    // The picker draws each design, so it needs the template the drawing is in.
    expect(choices[0].id).toBe('one')
    expect(choices.map((choice) => choice.key)).toEqual(
      ['slate-classic', 'slate-panel', 'azure-classic', 'azure-arc', 'plum-rail', 'graphite-minimal', 'graphite-classic'])
    expect(choices.some((choice) => choice.name === '우리 회사 템플릿')).toBe(false)
  })

  it('groups them by family, in the order the library lists them', () => {
    const families = designFamilies(designChoices(library))
    expect(families.map((entry) => entry.family)).toEqual(['slate', 'azure', 'plum', 'graphite'])
    expect(families[0].designs).toHaveLength(2)
  })
})

describe('what a stored theme value means', () => {
  const choices = designChoices(library)

  it('takes a design key as itself', () => {
    expect(resolveDesignKey('slate-classic', choices)).toBe('slate-classic')
    expect(resolveDesignKey(' Slate-Classic ', choices)).toBe('slate-classic')
  })

  it('reads a name an older version stored', () => {
    // The four themes this product shipped once. A screen that cannot read them
    // shows the wrong design to everybody who has been here a while.
    expect(resolveDesignKey('aurora', choices)).toBe('plum-rail')
    expect(resolveDesignKey('modern', choices)).toBe('slate-classic')
    for (const stored of Object.keys(legacyThemeAliases)) {
      expect(typeof resolveDesignKey(stored, choices), stored).toBe('string')
    }
  })

  it('reads a bare family name the way the server does', () => {
    // 'graphite' was on the screen for years and is not a design key: the server
    // answers it with that family's first design, and so does this.
    // The family's first design in the library's order, not in a listing's.
    expect(resolveDesignKey('graphite', choices)).toBe('graphite-minimal')
    expect(resolveDesignKey('azure', choices)).toBe('azure-classic')
  })

  it('falls back to the library’s first design, never to nothing', () => {
    expect(resolveDesignKey('a-design-nobody-ships', choices)).toBe('slate-classic')
    expect(resolveDesignKey('', choices)).toBe('slate-classic')
  })

  it('says the value back when the library has not loaded yet', () => {
    expect(resolveDesignKey('slate-classic', [])).toBe('slate-classic')
  })
})
