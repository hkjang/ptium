import { describe, expect, it } from 'vitest'
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join } from 'node:path'
import { endsInConsonant, objectParticle, subjectParticle, toParticle, topicParticle, withParticle } from './korean'

describe('한 낱말 뒤에 오는 조사', () => {
  it('한글은 받침으로 고른다', () => {
    expect(objectParticle('인용')).toBe('을')
    expect(objectParticle('표')).toBe('를')
    expect(subjectParticle('구축하고')).toBe('가')
    expect(subjectParticle('매출')).toBe('이')
    expect(topicParticle('보고서')).toBe('는')
    expect(topicParticle('계획안')).toBe('은')
    expect(withParticle('매출')).toBe('과')
    expect(withParticle('차트')).toBe('와')
    expect(toParticle('서울')).toBe('로')
    expect(toParticle('인용')).toBe('으로')
  })

  // A deck is called what its author called it, and that is often not Hangul.
  it('숫자와 로마자는 읽는 소리로 고른다', () => {
    expect(objectParticle('KPI')).toBe('를')          // 케이피아이
    expect(objectParticle('Excel')).toBe('을')        // 엑셀
    expect(objectParticle('2026')).toBe('을')         // 이천이십육
    expect(objectParticle('v2')).toBe('를')           // 브이투
    expect(topicParticle('Zoom')).toBe('은')          // 줌
    expect(subjectParticle('plan9')).toBe('가')       // 구
  })

  // Every one of these is written `"${name}"을`, so the quote is what the rule
  // meets first and the name is what decides.
  it('따옴표 안에 있어도 낱말을 본다', () => {
    expect(objectParticle('"매출"')).toBe('을')
    expect(objectParticle('"차트"')).toBe('를')
    expect(endsInConsonant('보고서.pptx')).toBe(false)   // 엑스
  })
})

/**
 * A particle typed straight after an interpolated name is wrong about half the
 * time, and nothing in the running product complains: "보고서.pptx을 읽고
 * 있습니다" is a whole sentence. So the source is what gets read.
 */
describe('본문에 박아 넣은 조사', () => {
  const walk = (dir: string): string[] =>
    readdirSync(dir).flatMap((entry) => {
      const full = join(dir, entry)
      if (statSync(full).isDirectory()) return walk(full)
      return /\.tsx?$/.test(full) && !/\.test\.tsx?$/.test(full) ? [full] : []
    })

  it('없다', () => {
    const guilty: string[] = []
    for (const file of walk('src')) {
      readFileSync(file, 'utf8').split('\n').forEach((line, index) => {
        // `}은 `, `}"를 `, `}'과 ` — a name closing, then a particle chosen for it.
        const caught = line.match(/\}["'”’]?(을|를|이|가|은|는|와|과|으로)[\s.,!?]/)
        if (caught) guilty.push(`${file}:${index + 1} …${caught[0].trim()}`)
      })
    }
    expect(guilty, guilty.join('\n')).toEqual([])
  })
})
