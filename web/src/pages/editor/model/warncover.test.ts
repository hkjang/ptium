import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'

import { warningText } from './findings'

/**
 * Every warning compiling a deck can raise has to be readable by the author.
 *
 * Same reasoning as the measurements next door, and the same failure: the
 * compiler writes in English for whoever is debugging a template, the editor
 * writes each one again in Korean, and one with no rule is shown as the
 * compiler wrote it. One was — a hero component given more than one figure —
 * so this reads the compiler's own source instead of a list kept by hand.
 *
 * The import reader writes its warnings in Korean to begin with; those are
 * skipped, because there is nothing to translate.
 */
const SOURCES = [
  '../server/internal/deck/compile.go',
  '../server/internal/deck/source.go',
  '../server/internal/deck/importdeck.go',
]

/** The complete warning at each Sprintf, including any concatenated halves. */
function warnings(source: string): string[] {
  const found: string[] = []
  const literal = /^\s*"((?:[^"\\]|\\.)*)"/
  const joins = /^\s*\+/
  for (const start of [...source.matchAll(/(?:warn|warnings|Warnings)\w*[\s\S]{0,60}?fmt\.Sprintf\(/g)]) {
    let rest = source.slice((start.index ?? 0) + start[0].length)
    let whole = ''
    for (;;) {
      const piece = rest.match(literal)
      if (!piece) break
      whole += piece[1]
      rest = rest.slice(piece[0].length)
      const plus = rest.match(joins)
      if (!plus) break
      rest = rest.slice(plus[0].length)
    }
    if (whole.length > 18 && !/[가-힣]/.test(whole)) found.push(whole)
  }
  return [...new Set(found)]
}

/** The warning as the author meets it, with a place and the values filled in. */
function asShown(format: string) {
  const placed = format.startsWith('%s: ') ? `line 12: ${format.slice(4)}` : format
  return placed.replace(/%q/g, '"제목만"').replace(/%d/g, '7').replace(/%s/g, 'lineChart')
}

describe('덱을 컴파일하며 알리는 말도 읽는 사람의 말로', () => {
  const source = SOURCES.map((path) => readFileSync(path, 'utf8')).join('\n')

  it('컴파일러가 쓸 수 있는 경고를 찾아낸다', () => {
    // If this ever reads nothing, the check below is passing on an empty list.
    expect(warnings(source).length).toBeGreaterThan(8)
  })

  it('영어 그대로 나오는 경고가 없다', () => {
    const left = warnings(source).map(asShown).filter((shown) => warningText(shown) === shown)
    expect(left, left.join('\n')).toEqual([])
  })
})
