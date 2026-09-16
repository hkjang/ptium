/**
 * Notification mail through the company SMTP relay (MAIL-STANDARD), checked
 * here the way the server checks it so a save that would be refused is said
 * before it is sent.
 */

export type MailValues = Record<string, string | number | boolean | string[]>

export const securityChoices = [
  { id: 'auto', label: '자동 — 릴레이가 STARTTLS 를 알리면 쓰고, 아니면 평문' },
  { id: 'none', label: '없음 — 평문 (사내 릴레이 25번 포트의 흔한 경우)' },
  { id: 'starttls', label: 'STARTTLS 필수' },
  { id: 'tls', label: 'TLS — 접속부터 암호화 (대개 465번 포트)' },
]

/** Says what is wrong with the section as typed, or nothing. */
export function mailProblem(values: MailValues): string {
  const host = String(values.smtp_host ?? '').trim()
  if (values.enabled === true && !host) return '메일 알림을 켜려면 SMTP 릴레이 호스트가 필요합니다.'
  if (host && (/[\s/:@]/.test(host) || host.length > 253)) return 'SMTP 호스트는 포트 없이 호스트 이름이나 주소만 적습니다.'
  const port = Number(values.smtp_port)
  if (!Number.isInteger(port) || port < 1 || port > 65535) return 'SMTP 포트는 1~65535 의 정수여야 합니다.'
  if (!securityChoices.some((choice) => choice.id === String(values.security))) return '보안 방식을 선택해 주세요.'
  const from = String(values.from_address ?? '').trim()
  if (from && (!from.includes('@') || /[\s<>,]/.test(from))) return '보내는 주소는 메일 주소 하나여야 합니다.'
  const base = String(values.base_url ?? '').trim()
  if (base && !/^https?:\/\/[^\s]+$/.test(base)) return '링크 주소는 http(s):// 로 시작하는 주소여야 합니다.'
  const seconds = Number(values.timeout_seconds)
  if (!Number.isInteger(seconds) || seconds < 1 || seconds > 120) return '제한 시간은 1~120초여야 합니다.'
  return ''
}

/** What the saved settings amount to, in one line for the top of the section. */
export function mailSummary(values: MailValues, passwordConfigured: boolean): string {
  if (values.enabled !== true) return '메일 알림이 꺼져 있습니다. 아무것도 보내지 않습니다.'
  const host = String(values.smtp_host ?? '').trim() || '(호스트 없음)'
  const port = Number(values.smtp_port) || 25
  const security = securityChoices.find((choice) => choice.id === String(values.security))?.id ?? 'auto'
  const auth = String(values.username ?? '').trim() ? `인증 ${passwordConfigured ? '있음' : '(비밀번호 없음)'}` : '인증 없음'
  return `${host}:${port} · ${security} · ${auth} 으로 보냅니다.`
}
