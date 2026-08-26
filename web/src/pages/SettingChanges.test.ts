import { describe, expect, it } from 'vitest'
import { revertable, said } from './SettingChanges'

describe('a settings change, read back', () => {
  it('says a stored value the way the screen sets it', () => {
    expect(said(300)).toBe('300')
    expect(said(true)).toBe('사용')
    expect(said(false)).toBe('사용 안 함')
    expect(said(['ptium-admin', 'admin'])).toBe('ptium-admin · admin')
    expect(said([])).toBe('없음')
    expect(said('')).toBe('비어 있음')
    expect(said(undefined)).toBe('없음')
  })

  it('can put back a change that recorded what it replaced', () => {
    expect(revertable({ id: 1, createdAt: '', metadata: { key: 'ai.timeout_seconds', from: 300, to: 600 } })).toBe(true)
    // false is a value like any other: a flag turned off is still revertable.
    expect(revertable({ id: 2, createdAt: '', metadata: { key: 'generation.outline_pass', from: false, to: true } })).toBe(true)
  })

  it('cannot put back a secret, because the value was never written down', () => {
    expect(revertable({ id: 3, createdAt: '', metadata: { key: 'ai.api_key', sensitive: true } })).toBe(false)
  })

  it('cannot put back a setting that had no value before', () => {
    expect(revertable({ id: 4, createdAt: '', metadata: { key: 'branding.logo_url', to: 'https://x' } })).toBe(false)
  })
})
