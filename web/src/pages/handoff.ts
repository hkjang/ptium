import type { HandoffTarget } from '../types'

/**
 * Passing a deck to another service, and taking a document from one, without
 * anybody downloading a file (HANDOFF-STANDARD).
 *
 * These are the parts with no screen in them: the address the receiving side
 * is opened at, what a `/handoff` address carried, and the words for each way
 * it can fail.
 */

/** The receiving service's /handoff, carrying where to fetch from and the claim. */
export function handoffReceiveUrl(target: string, source: string, claim: string): string {
  const query = new URLSearchParams({ source, claim })
  return `${target.replace(/\/+$/, '')}/handoff?${query.toString()}`
}

/** Only the destinations that take a presentation are destinations. */
export function sendableTargets(targets: HandoffTarget[]): HandoffTarget[] {
  return targets.filter((target) => Array.isArray(target.formats) && target.formats.includes('pptx') && /^https?:\/\//.test(target.origin))
}

/** The label a destination gets in the menu. */
export function targetLabel(target: HandoffTarget): string {
  const host = target.origin.replace(/^https?:\/\//, '')
  return target.name && target.name !== host ? `${target.name} (${host})` : host
}

/** What a browser brought to /handoff, or why it is not a handoff at all. */
export function parseHandoffQuery(search: string): { source: string; claim: string } | { problem: string } {
  const query = new URLSearchParams(search)
  const source = (query.get('source') || '').trim()
  const claim = (query.get('claim') || '').trim()
  if (!source || !claim) return { problem: '이 주소에는 보낸 서비스와 표가 없습니다. 보낸 쪽의 "다른 서비스로 보내기"에서 다시 열어 주세요.' }
  if (!/^https?:\/\/[^/?#\s]+\/?$/i.test(source)) return { problem: '보낸 서비스의 주소가 오리진 모양이 아닙니다.' }
  if (!/^[A-Za-z0-9_-]{16,256}$/.test(claim)) return { problem: '표가 서비스가 발급한 모양이 아닙니다.' }
  return { source, claim }
}

/**
 * Where a signed-out visitor is sent, keeping where they were. `/handoff`
 * carries a five-minute claim, and a login that forgot it would land on an
 * empty dashboard with the document still on the other side.
 */
export function loginPathKeeping(pathname: string, search: string): string {
  const here = `${pathname}${search}`
  if (pathname === '/' || pathname === '/dashboard' || pathname === '/login') return '/login'
  return `/login?return_to=${encodeURIComponent(here)}`
}
