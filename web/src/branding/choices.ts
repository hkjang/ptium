/**
 * A control that suggests a few values and stores whatever is there.
 *
 * Tone, language and audience are free text: the API takes any string up to
 * eighty characters, an administrator's default flows into every profile, and
 * another service can store anything at all through the API. The screens
 * offered a fixed handful — four tones, four languages — and a value outside
 * that handful vanished from the screen: the chips showed nothing selected, and
 * a `<select>` holding a value none of its options carry displays its first
 * option instead, so the screen named a tone the deployment was not using.
 *
 * Nothing here narrows what can be stored. It only makes sure the value that is
 * stored is one of the choices on the screen.
 */
export type Choice = { id: string; label: string }

const toneLabels: Record<string, string> = {
  professional: '전문적',
  persuasive: '설득력 있는',
  friendly: '친근한',
  inspiring: '영감을 주는',
  academic: '학술적인',
}

const languageLabels: Record<string, string> = {
  ko: '한국어',
  en: 'English',
  ja: '日本語',
  zh: '中文',
}

export const toneChoices: Choice[] = Object.entries(toneLabels).map(([id, label]) => ({ id, label }))
export const languageChoices: Choice[] = Object.entries(languageLabels).map(([id, label]) => ({ id, label }))

/**
 * The choices, with the stored value among them. A value the screen has no name
 * for is shown as it is written rather than left off: the reader is entitled to
 * see what their deck will actually be written with.
 */
export function withStoredChoice(choices: Choice[], stored: string): Choice[] {
  const value = String(stored || '').trim()
  if (!value || choices.some((choice) => choice.id === value)) return choices
  return [...choices, { id: value, label: value }]
}

/** What to call a stored value in a sentence. */
export function toneLabel(stored: string) {
  const value = String(stored || '').trim()
  return toneLabels[value] || value
}

export function languageLabel(stored: string) {
  const value = String(stored || '').trim()
  return languageLabels[value] || value
}
