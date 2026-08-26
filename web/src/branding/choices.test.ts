import { describe, expect, it } from 'vitest'
import { languageChoices, languageLabel, toneChoices, toneLabel, withStoredChoice } from './choices'

describe('a control that suggests values and stores anything', () => {
  it('offers the tones and languages the screens name', () => {
    expect(toneChoices.map((choice) => choice.id)).toContain('academic')
    expect(languageChoices.map((choice) => choice.id)).toEqual(['ko', 'en', 'ja', 'zh'])
  })

  it('keeps the list as it is when the stored value is already on it', () => {
    expect(withStoredChoice(toneChoices, 'professional')).toBe(toneChoices)
    expect(withStoredChoice(toneChoices, '')).toBe(toneChoices)
  })

  it('adds the stored value when the screen has no name for it', () => {
    // The API takes any string, an administrator's default flows into every
    // profile, and another service can store anything through the API. A tone
    // the screen cannot show is a tone the reader cannot see.
    const withOne = withStoredChoice(toneChoices, 'concise')
    expect(withOne).toHaveLength(toneChoices.length + 1)
    expect(withOne[withOne.length - 1]).toEqual({ id: 'concise', label: 'concise' })
    const withLanguage = withStoredChoice(languageChoices, 'fr')
    expect(withLanguage.map((choice) => choice.id)).toContain('fr')
  })

  it('says a stored value in the reader’s words when it has them', () => {
    expect(toneLabel('academic')).toBe('학술적인')
    expect(toneLabel('concise')).toBe('concise')
    expect(languageLabel('ja')).toBe('日本語')
    expect(languageLabel('fr')).toBe('fr')
  })
})
