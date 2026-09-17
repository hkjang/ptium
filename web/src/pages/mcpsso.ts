/**
 * MCP over SSO (MCP-OAUTH-STANDARD): a Keycloak access token opens /mcp
 * beside the personal key. The settings are checked here the way the server
 * checks them, so a value that would be refused is said before the save.
 */

/** The scopes an SSO token may be granted: what a key may hold, minus the administrator's. */
export const grantableScopes = ['presentations:read', 'presentations:write', 'templates:read', 'templates:write', 'profile:read', 'profile:write']

/** Reads a space-, comma- or line-separated list, without repeats. */
export function splitWords(value: string): string[] {
  const seen = new Set<string>()
  const result: string[] = []
  for (const word of String(value || '').split(/[\s,;]+/)) {
    if (!word || seen.has(word)) continue
    seen.add(word)
    result.push(word)
  }
  return result
}

/** What is wrong with the resource identifier, or '' when the server would store it. */
export function resourceProblem(value: string): string {
  const trimmed = String(value || '').trim()
  if (!trimmed) return ''
  let url: URL
  try { url = new URL(trimmed) } catch { return '리소스 식별자는 https://주소/mcp 꼴의 절대 주소여야 합니다.' }
  const loopback = ['localhost', '127.0.0.1', '[::1]'].includes(url.hostname)
  if (url.protocol !== 'https:' && !(url.protocol === 'http:' && loopback)) return '리소스 식별자는 HTTPS 주소여야 합니다 (localhost 만 http 허용).'
  if (url.search || url.hash || url.username || url.password) return '리소스 식별자에는 쿼리·프래그먼트·자격 증명을 넣을 수 없습니다.'
  if (url.pathname !== '/mcp') return '리소스 식별자는 클라이언트가 접속하는 경로 /mcp 로 끝나야 합니다.'
  return ''
}

/** What is wrong with the section, or '' when the server would store it. */
export function mcpSsoProblem(values: Record<string, unknown>): string {
  const problem = resourceProblem(String(values['oauth.resource'] ?? ''))
  if (problem) return problem
  const audiences = splitWords(String(values['oauth.audience'] ?? ''))
  if (audiences.length > 50) return '허용 대상은 50개까지입니다.'
  for (const audience of audiences) {
    if (audience.length > 256 || !/^[\x21-\x7e]+$/.test(audience)) return `허용 대상 "${audience}" 은(는) 공백 없는 ASCII 256자 이하여야 합니다.`
  }
  const scopes = splitWords(String(values['oauth.scopes'] ?? ''))
  if (scopes.length === 0) return '범위를 한 개 이상 적어 주세요 (예: presentations:read templates:read).'
  for (const scope of scopes) {
    if (!grantableScopes.includes(scope)) return `"${scope}" 은(는) SSO 토큰에 줄 수 있는 범위가 아닙니다. 가능한 값: ${grantableScopes.join(', ')}`
  }
  return ''
}

/** One line saying what a saved section does. */
export function mcpSsoSummary(values: Record<string, unknown>): string {
  if (!values['oauth.enabled']) return '꺼져 있습니다. /mcp 는 개인 API 키로만 열리고, 메타데이터 주소는 404 를 답합니다.'
  const audiences = splitWords(String(values['oauth.audience'] ?? ''))
  const scopes = splitWords(String(values['oauth.scopes'] ?? ''))
  const who = audiences.length ? `허용 대상 ${audiences.join(', ')} 로 발급된 토큰` : '리소스 식별자를 aud 에 실은 토큰'
  return `받는 토큰: ${who}. 웹으로 한 번 로그인한 계정을 열고, 범위는 mcp:use 와 ${scopes.join(', ') || '(없음)'} 입니다.`
}
