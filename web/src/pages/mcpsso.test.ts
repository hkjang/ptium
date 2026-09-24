import { describe, expect, it } from 'vitest'
import { mcpSsoProblem, mcpSsoSummary, resourceProblem, splitWords } from './mcpsso'

describe('MCP SSO settings are checked the way the server checks them', () => {
  it('takes the resource identifier only as https://…/mcp', () => {
    expect(resourceProblem('')).toBe('')
    expect(resourceProblem('https://slides.corp.example/mcp')).toBe('')
    expect(resourceProblem('http://localhost:8080/mcp')).toBe('')
    expect(resourceProblem('http://slides.corp.example/mcp')).toContain('HTTPS')
    expect(resourceProblem('https://slides.corp.example/')).toContain('/mcp')
    expect(resourceProblem('https://slides.corp.example/mcp?x=1')).toContain('쿼리')
  })
  it('refuses scopes a key could not hold and an empty list', () => {
    expect(mcpSsoProblem({ 'oauth.scopes': 'presentations:read templates:read' })).toBe('')
    expect(mcpSsoProblem({ 'oauth.scopes': '' })).toContain('한 개 이상')
    expect(mcpSsoProblem({ 'oauth.scopes': 'admin:settings' })).toContain('admin:settings')
    expect(mcpSsoProblem({ 'oauth.scopes': 'presentations:read', 'oauth.audience': '한글' })).toContain('ASCII')
  })
  it('reads lists without repeats and says what a saved section does', () => {
    expect(splitWords('claude-mcp, cursor claude-mcp\ncursor')).toEqual(['claude-mcp', 'cursor'])
    expect(mcpSsoSummary({ 'oauth.enabled': false })).toContain('꺼져')
    expect(mcpSsoSummary({ 'oauth.enabled': true, 'oauth.audience': 'claude-mcp', 'oauth.scopes': 'presentations:read' })).toContain('claude-mcp')
  })
})
