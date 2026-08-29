import { describe, expect, it } from 'vitest'
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join } from 'node:path'

import { errorText } from './errors'

/**
 * Every refusal the server can send has to be readable by whoever asked.
 *
 * The API answers in English — that is what an API and a log should say — and
 * the workspace writes each one again in Korean. Anything with no rule is shown
 * as the server wrote it. The measurements and the compile warnings next door
 * each had messages falling through that gap; these did not, and nothing was
 * keeping them from it.
 *
 * The exceptions are the OIDC token endpoint's own refusals. Nobody reads those
 * on a screen: they are the answer to an identity client exchanging a code, and
 * English is what that surface should speak.
 */
const SERVER = '../server/internal'

const MACHINE_ONLY = [
  'This deployment does not exchange OIDC tokens server-side',
  'Only the authorization_code grant is exchanged',
  'code, redirect_uri and code_verifier are required',
]

function goFiles(dir: string): string[] {
  return readdirSync(dir).flatMap((entry) => {
    const full = join(dir, entry)
    if (statSync(full).isDirectory()) return goFiles(full)
    return full.endsWith('.go') && !full.endsWith('_test.go') ? [full] : []
  })
}

/** The English sentence in every writeError call the server makes. */
function refusals(): string[] {
  const found = new Set<string>()
  for (const file of goFiles(SERVER)) {
    const source = readFileSync(file, 'utf8')
    for (const call of source.matchAll(/write(?:JSON)?Error\((?:[^()]|\([^()]*\))*?\)/gs)) {
      for (const piece of call[0].matchAll(/"((?:[^"\\]|\\.)*)"/g)) {
        const said = piece[1]
        if (said.length > 10 && / [a-z]/.test(said) && !/^[a-z_]+$/.test(said) && !/[가-힣]/.test(said)) {
          found.add(said)
        }
      }
    }
  }
  return [...found].filter((said) => !MACHINE_ONLY.includes(said))
}

/** The refusal as the person meets it, with the values filled in. */
const asShown = (format: string) =>
  format.replace(/%d/g, '7').replace(/%s/g, 'kpi').replace(/%q/g, '"매출"').trim()

describe('거절당했을 때도 읽는 사람의 말로', () => {
  it('서버가 보낼 수 있는 거절을 찾아낸다', () => {
    // If this ever reads nothing, the check below is passing on an empty list.
    expect(refusals().length).toBeGreaterThan(50)
  })

  it('영어 그대로 나오는 거절이 없다', () => {
    const left = refusals()
      .map(asShown)
      // The code is deliberately one no map knows. errorText falls back to the
      // code when the message has no rule, so passing a real code makes every
      // message look translated and this check says nothing at all.
      .filter((shown) => errorText('', shown) === shown)
    expect(left, left.join('\n')).toEqual([])
  })
})
