/**
 * The accent colour, and the one question a screen showing it has to answer:
 * will this colour actually be drawn?
 *
 * Every deployment that has never opened the branding screen carries the colour
 * this product ships with, and the personalisation screen shows it in the
 * swatch. Nobody picked it, so the drawing leaves the template's own accent
 * alone — and a screen whose hint says "used in your slides" is promising
 * something that will not happen. These say what is true instead.
 */
export const seededAccent = '#7C3AED'

/** Whether this is the colour the product seeded rather than one somebody chose. */
export function isSeededAccent(color: string, seeded: string = seededAccent) {
  return color.trim().toUpperCase() === (seeded || seededAccent).trim().toUpperCase()
}

/** What the field says under the swatch, given the colour it is showing. */
export function accentNote(color: string, seeded: string = seededAccent) {
  if (isSeededAccent(color, seeded)) {
    return '지금 값은 제품이 기본으로 갖고 온 색입니다. 이 값이면 템플릿이 가진 강조색을 그대로 둡니다 — 다른 색을 고르면 핵심 지표·단계처럼 Ptium이 직접 그리는 구성요소에 그 색을 씁니다.'
  }
  return '핵심 지표·단계처럼 Ptium이 직접 그리는 구성요소에 사용됩니다. 템플릿이 가진 강조색은 그대로 둡니다.'
}
