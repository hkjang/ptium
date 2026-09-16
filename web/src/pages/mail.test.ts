import { describe, expect, it } from 'vitest'
import { mailProblem, mailSummary } from './mail'

const shipped = { enabled: false, smtp_host: '', smtp_port: 25, security: 'auto', skip_tls_verify: false, username: '', password: '', from_address: '', from_name: 'Ptium', base_url: '', timeout_seconds: 10 }

describe('mailProblem', () => {
  it('accepts the shipped defaults, which send nothing', () => {
    expect(mailProblem(shipped)).toBe('')
  })
  it('refuses switching on without a relay, as the server does', () => {
    expect(mailProblem({ ...shipped, enabled: true })).toMatch(/호스트/)
    expect(mailProblem({ ...shipped, enabled: true, smtp_host: 'relay.corp.example' })).toBe('')
  })
  it('wants a bare host, a real port, one sender address and an http link base', () => {
    expect(mailProblem({ ...shipped, smtp_host: 'relay.corp.example:25' })).toMatch(/포트 없이/)
    expect(mailProblem({ ...shipped, smtp_port: 0 })).toMatch(/포트/)
    expect(mailProblem({ ...shipped, security: 'ssl' })).toMatch(/보안/)
    expect(mailProblem({ ...shipped, from_address: 'not an address' })).toMatch(/보내는 주소/)
    expect(mailProblem({ ...shipped, base_url: 'slides.corp.example' })).toMatch(/링크 주소/)
    expect(mailProblem({ ...shipped, timeout_seconds: 600 })).toMatch(/제한 시간/)
  })
})

describe('mailSummary', () => {
  it('says off while off, and the relay and whether it authenticates while on', () => {
    expect(mailSummary(shipped, false)).toMatch(/꺼져/)
    expect(mailSummary({ ...shipped, enabled: true, smtp_host: 'relay.corp.example' }, false)).toBe('relay.corp.example:25 · auto · 인증 없음 으로 보냅니다.')
    expect(mailSummary({ ...shipped, enabled: true, smtp_host: 'relay.corp.example', username: 'ptium' }, true)).toMatch(/인증 있음/)
    expect(mailSummary({ ...shipped, enabled: true, smtp_host: 'relay.corp.example', username: 'ptium' }, false)).toMatch(/비밀번호 없음/)
  })
})
