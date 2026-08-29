/**
 * Korean particles, chosen by the word in front of them.
 *
 * 을/를, 이/가, 은/는, 와/과 are picked by whether the last syllable of the
 * preceding word ends in a consonant. The word is usually a deck title, a file
 * name or a template somebody named, so it cannot be decided when the sentence
 * is written — and a message that guesses is wrong about half the time.
 *
 * "제목을(를)" is what it looks like when nobody chose. Writing one form and
 * hoping is quieter but no better: "보고서.pptx을 읽고 있습니다" is the same
 * mistake with the seam hidden.
 *
 * A name does not have to be Hangul. Latin letters and digits are read aloud in
 * Korean, and their names decide the particle the same way: KPI ends in 아이 and
 * takes 를, Excel ends in 엘 and takes 을, 2026 ends in 육 and takes 을.
 */

/** The names of the digits, for a word that ends in one. */
const digitEndsInConsonant: Record<string, boolean> = {
  // 이 사 오 구 end open; 영 일 삼 육 칠 팔 end on a consonant — 영 ends on ㅇ,
  // and a number written with a trailing zero is read 십 · 백 · 천 · 만 · 억,
  // every one of which closes on a consonant too.
  '0': true, '1': true, '2': false, '3': true, '4': false,
  '5': false, '6': true, '7': true, '8': true, '9': false,
}

/** The Latin letters read with a closing consonant: 엘, 엠, 엔. */
const letterEndsInConsonant = new Set(['l', 'm', 'n'])

/**
 * Whether the word ends on a consonant — 받침 — once read aloud.
 *
 * Returns null for a word that ends in something with no Korean reading at all
 * (a bracket, a full stop), where there is nothing to decide from.
 */
export function endsInConsonant(word: string): boolean | null {
  const last = word.trim().replace(/["'”’」』\]\)]+$/, '').trim().slice(-1)
  if (!last) return null
  const code = last.charCodeAt(0)
  if (code >= 0xac00 && code <= 0xd7a3) return (code - 0xac00) % 28 !== 0
  if (last >= '0' && last <= '9') return digitEndsInConsonant[last]
  const letter = last.toLowerCase()
  if (letter >= 'a' && letter <= 'z') return letterEndsInConsonant.has(letter)
  return null
}

const pick = (word: string, closed: string, open: string) =>
  endsInConsonant(word) === true ? closed : open

/** 을 / 를 */
export const objectParticle = (word: string) => pick(word, '을', '를')

/** 이 / 가 */
export const subjectParticle = (word: string) => pick(word, '이', '가')

/** 은 / 는 */
export const topicParticle = (word: string) => pick(word, '은', '는')

/** 와 / 과 */
export const withParticle = (word: string) => pick(word, '과', '와')

/** 으로 / 로 — ㄹ takes 로, like 서울로. */
export function toParticle(word: string) {
  const last = word.trim().replace(/["'”’」』\]\)]+$/, '').trim().slice(-1)
  const code = last.charCodeAt(0)
  if (code >= 0xac00 && code <= 0xd7a3 && (code - 0xac00) % 28 === 8) return '로'
  return endsInConsonant(word) === true ? '으로' : '로'
}
