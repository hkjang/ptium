import { describe, expect, it } from 'vitest'
import { maxSnippetBytes, policyOrigins, splitHosts, trackingProblem, trackingProviders } from './tracking'

describe('the tracking section refuses what the server would', () => {
  it('offers Momento first, after off', () => {
    expect(trackingProviders.map((choice) => choice.id)).toEqual(['none', 'momento', 'ga4', 'gtm', 'matomo', 'custom'])
  })

  it('lets a switched-off configuration save with anything missing', () => {
    expect(trackingProblem({ enabled: false, provider: 'momento' })).toBe('')
    expect(trackingProblem({ enabled: false, provider: 'custom', custom_snippet: '' })).toBe('')
  })

  it('wants what the chosen tracker needs once it is on', () => {
    expect(trackingProblem({ enabled: true, provider: 'momento', momento_url: 'https://m.internal' })).toContain('사이트 ID')
    expect(trackingProblem({ enabled: true, provider: 'momento', momento_url: 'https://m.internal', momento_site_id: 'ptium' })).toBe('')
    expect(trackingProblem({ enabled: true, provider: 'ga4' })).toContain('측정 ID')
    expect(trackingProblem({ enabled: true, provider: 'gtm', measurement_id: 'GTM-1' })).toBe('')
    expect(trackingProblem({ enabled: true, provider: 'matomo', matomo_url: 'https://m.example' })).toContain('사이트 ID')
    expect(trackingProblem({ enabled: true, provider: 'custom', custom_snippet: '  ' })).toContain('비어')
    expect(trackingProblem({ enabled: true, provider: 'none' })).toBe('')
    expect(trackingProblem({ enabled: true, provider: 'piwik' })).toContain('지원되는')
  })

  it('bounds the snippet in bytes, even while off', () => {
    expect(trackingProblem({ enabled: false, provider: 'custom', custom_snippet: 'x'.repeat(maxSnippetBytes) })).toBe('')
    expect(trackingProblem({ enabled: false, provider: 'custom', custom_snippet: 'x'.repeat(maxSnippetBytes + 1) })).toContain('바이트')
    // Korean is three bytes a letter: 3000 letters is over the bound.
    expect(trackingProblem({ enabled: false, provider: 'custom', custom_snippet: '가'.repeat(3000) })).toContain('바이트')
  })

  it('refuses an address that is not one, and an allowed host with a path', () => {
    expect(trackingProblem({ provider: 'momento', momento_url: 'momento.internal' })).toContain('Momento 주소')
    expect(trackingProblem({ provider: 'matomo', matomo_url: 'ftp://x' })).toContain('Matomo 주소')
    expect(trackingProblem({ provider: 'none', allowed_hosts: 'https://a.example/path' })).toContain('허용 출처')
    expect(trackingProblem({ provider: 'none', allowed_hosts: 'https://a.example, https://b.example:8443\nhttp://c.example' })).toBe('')
  })

  it('reads the allow list one origin per comma, space or line', () => {
    expect(splitHosts(' https://a.example,https://b.example\nhttps://c.example ')).toEqual(['https://a.example', 'https://b.example', 'https://c.example'])
    expect(splitHosts('')).toEqual([])
  })
})

describe('what the policy will be told to allow', () => {
  it('is nothing for Momento through this origin, and the collector when direct', () => {
    expect(policyOrigins({ provider: 'momento', momento_url: 'https://m.internal/', momento_proxy: true })).toEqual([])
    expect(policyOrigins({ provider: 'momento', momento_url: 'https://m.internal/', momento_proxy: false })).toEqual(['https://m.internal'])
  })

  it('reads the origins out of a pasted snippet and the allow list, once each', () => {
    const snippet = `<script src="https://cdn.t.example/a.js"></script><script>fetch('HTTPS://collect.t.example/v1')</script>`
    expect(policyOrigins({ provider: 'custom', custom_snippet: snippet, allowed_hosts: 'https://cdn.t.example, https://x.example' }))
      .toEqual(['https://cdn.t.example', 'https://collect.t.example', 'https://x.example'])
  })
})
