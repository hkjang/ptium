/**
 * What the visitor tracking section of the settings screen checks before it
 * saves, said the way the server would refuse it — so an administrator reads
 * the reason on this screen rather than in a 422.
 */

/** The trackers that can be chosen, in the order the screen offers them:
 * Momento first, the self-hosted collector whose data never leaves the network. */
export const trackingProviders = [
  { id: 'none', label: '사용 안 함' },
  { id: 'momento', label: 'Momento (사내 수집기)' },
  { id: 'ga4', label: 'Google Analytics 4' },
  { id: 'gtm', label: 'Google Tag Manager' },
  { id: 'matomo', label: 'Matomo' },
  { id: 'custom', label: '직접 붙여 넣기' },
] as const

/** A pasted snippet may be this many bytes, which is what the page carries. */
export const maxSnippetBytes = 8 * 1024

export function snippetBytes(snippet: string): number {
  return new TextEncoder().encode(snippet).length
}

/** The problem with what is about to be saved, or '' when there is none. */
export function trackingProblem(values: Record<string, unknown>): string {
  const provider = String(values.provider ?? 'none')
  if (!trackingProviders.some((choice) => choice.id === provider)) return '지원되는 추적 도구를 선택해 주세요.'
  const snippet = String(values.custom_snippet ?? '')
  if (snippetBytes(snippet) > maxSnippetBytes) return `붙여 넣은 스니펫은 ${maxSnippetBytes.toLocaleString('ko-KR')}바이트를 넘을 수 없습니다.`
  const hosts = splitHosts(String(values.allowed_hosts ?? ''))
  const badHost = hosts.find((host) => !isOrigin(host))
  if (badHost) return `허용 출처 "${badHost}" 는 경로 없는 http(s)://host 형식이어야 합니다.`
  const momentoURL = String(values.momento_url ?? '').trim()
  if (momentoURL && !isHTTPURL(momentoURL)) return 'Momento 주소는 올바른 HTTP(S) 주소여야 합니다.'
  const matomoURL = String(values.matomo_url ?? '').trim()
  if (matomoURL && !isHTTPURL(matomoURL)) return 'Matomo 주소는 올바른 HTTP(S) 주소여야 합니다.'
  if (values.enabled !== true) return ''
  // On, the chosen tracker has to have what it needs, or the switch stores
  // nothing on the page and the administrator goes looking elsewhere.
  if (provider === 'momento' && (!momentoURL || !String(values.momento_site_id ?? '').trim())) return 'Momento 를 켜려면 수집기 주소와 사이트 ID 가 필요합니다.'
  if ((provider === 'ga4' || provider === 'gtm') && !String(values.measurement_id ?? '').trim()) return '측정 ID(G-… 또는 GTM-…)를 입력해 주세요.'
  if (provider === 'matomo' && (!matomoURL || !String(values.matomo_site_id ?? '').trim())) return 'Matomo 를 켜려면 주소와 사이트 ID 가 필요합니다.'
  if (provider === 'custom' && !snippet.trim()) return '붙여 넣을 스니펫이 비어 있습니다.'
  return ''
}

/** The allow list, one origin per comma, space or line. */
export function splitHosts(list: string): string[] {
  return list.split(/[\s,]+/).map((host) => host.trim()).filter(Boolean)
}

function isHTTPURL(value: string): boolean {
  try {
    const parsed = new URL(value)
    return (parsed.protocol === 'https:' || parsed.protocol === 'http:') && !parsed.username && !parsed.search && !parsed.hash
  } catch { return false }
}

function isOrigin(value: string): boolean {
  if (!isHTTPURL(value)) return false
  const parsed = new URL(value)
  return parsed.pathname === '/' || parsed.pathname === ''
}

/**
 * What the policy will be told to allow for this configuration, said before
 * saving: the tracker's own origins, or nothing at all when Momento is reached
 * through this origin.
 */
export function policyOrigins(values: Record<string, unknown>): string[] {
  const provider = String(values.provider ?? 'none')
  const origins: string[] = []
  const originOf = (value: string) => {
    try { return new URL(value).origin } catch { return '' }
  }
  if (provider === 'momento' && values.momento_proxy !== true) origins.push(originOf(String(values.momento_url ?? '')))
  if (provider === 'ga4' || provider === 'gtm') origins.push('https://www.googletagmanager.com', 'https://www.google-analytics.com')
  if (provider === 'matomo') origins.push(originOf(String(values.matomo_url ?? '')))
  if (provider === 'custom') {
    for (const match of String(values.custom_snippet ?? '').matchAll(/https?:\/\/[^"'`<>\s),;\\+]+/gi)) origins.push(originOf(match[0]))
  }
  for (const host of splitHosts(String(values.allowed_hosts ?? ''))) origins.push(originOf(host))
  return [...new Set(origins.filter(Boolean))]
}
