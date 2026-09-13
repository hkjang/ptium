/**
 * The administrator's list of services documents are passed to and taken
 * from (HANDOFF-STANDARD), checked here the way the server checks it, so a
 * line that would be refused is said before the save rather than after.
 */

/** The standard's format table: what each service on the network receives. */
export const receivesByService: Record<string, string[]> = {
  umm: [],
  muni: ['markdown'],
  kanpic: ['csv', 'xlsx'],
  ptium: ['markdown', 'docx', 'csv', 'xlsx', 'txt'],
  weekly: ['markdown', 'docx', 'pptx'],
}

export interface PeerLine { name: string; origin: string }

/** Reads one `name=origin` line, or says what is wrong with it. */
export function parsePeerLine(line: string): PeerLine | { problem: string } {
  const trimmed = line.trim()
  const at = trimmed.indexOf('=')
  if (at <= 0) return { problem: `"${trimmed}": 각 줄은 이름=오리진 꼴이어야 합니다 (예: weekly=https://weekly.intra).` }
  const name = trimmed.slice(0, at).trim().toLowerCase()
  const address = trimmed.slice(at + 1).trim()
  if (!name || /\s/.test(name) || name.length > 60) return { problem: `"${trimmed}": 서비스 이름은 공백 없이 60자 이하여야 합니다.` }
  let url: URL
  try { url = new URL(address) } catch { return { problem: `"${trimmed}": 주소는 경로 없는 http 또는 https 오리진이어야 합니다.` } }
  const bare = url.pathname === '/' || url.pathname === ''
  if (!/^https?:$/.test(url.protocol) || !bare || url.search || url.hash || url.username || url.password) {
    return { problem: `"${trimmed}": 주소는 경로 없는 http 또는 https 오리진이어야 합니다.` }
  }
  return { name, origin: url.origin.toLowerCase() }
}

/** What is wrong with the list, or '' when the server would store it. */
export function handoffProblem(lines: string[]): string {
  const seen = new Set<string>()
  if (lines.length > 50) return '허용 목록은 50줄까지입니다.'
  for (const line of lines) {
    if (!line.trim()) continue
    const parsed = parsePeerLine(line)
    if ('problem' in parsed) return parsed.problem
    if (seen.has(parsed.origin)) return `같은 주소가 두 번 있습니다: ${parsed.origin}`
    seen.add(parsed.origin)
  }
  return ''
}

/** What the list, as typed, would do — said under the field. */
export function handoffSummary(lines: string[]): string {
  const peers = lines.map(parsePeerLine).filter((peer): peer is PeerLine => !('problem' in peer))
  if (peers.length === 0) return '비어 있습니다 — 아무 서비스로도 보내지 않고, 아무 서비스에서도 받지 않습니다.'
  const sendTo = peers.filter((peer) => (receivesByService[peer.name] || []).includes('pptx')).map((peer) => peer.name)
  const receiveFrom = peers.map((peer) => peer.name)
  return `받는 곳: ${receiveFrom.join(', ')} · 보낼 수 있는 곳(pptx): ${sendTo.length ? sendTo.join(', ') : '없음'}`
}
