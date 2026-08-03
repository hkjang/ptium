import { BookOpen, Braces, KeyRound, Radio } from 'lucide-react'
import { AppShell } from '../components/AppShell'

const endpoints = [
  ['GET', '/api/v1/presentations', '내 프레젠테이션 목록'],
  ['POST', '/api/v1/presentations', '프레젠테이션 초안 생성'],
  ['POST', '/api/v1/presentations/{id}/generate', '슬라이드 생성 작업 큐 등록'],
  ['PATCH', '/api/v1/presentations/{id}', '메타데이터와 슬라이드 편집'],
  ['GET', '/api/v1/presentations/{id}/export?format=pptx', 'PowerPoint 다운로드'],
  ['GET', '/api/v1/profile', '개인화 프로필 조회'],
  ['GET', '/api/v1/api-keys', 'API 키 수명주기 관리'],
]

export function ApiDocsPage() {
  return <AppShell title="API · MCP 문서" eyebrow="DEVELOPER DOCS">
    <div className="api-docs-grid">
      <section className="admin-panel api-doc-card">
        <header><span><BookOpen size={20} /></span><div><h2>REST API</h2><p>성공 응답은 data, 오류 응답은 error를 담으며 JSON 응답에는 requestId가 포함됩니다.</p></div></header>
        <div className="endpoint-list">{endpoints.map(([method, path, description]) => <div key={`${method}-${path}`}><b className={`http-method method-${method.toLowerCase()}`}>{method}</b><code>{path}</code><span>{description}</span></div>)}</div>
      </section>
      <section className="admin-panel api-doc-card">
        <header><span><KeyRound size={20} /></span><div><h2>인증</h2><p>개인 API 키에는 작업에 필요한 최소 scope만 부여하세요.</p></div></header>
        <pre>{`Authorization: Bearer ptium_…\nContent-Type: application/json`}</pre>
        <p className="doc-note">브라우저 로그인은 Keycloak/OIDC Authorization Code + PKCE를 사용합니다. API 키 원문은 발급 또는 회전 직후 한 번만 표시됩니다.</p>
      </section>
      <section className="admin-panel api-doc-card">
        <header><span><Radio size={20} /></span><div><h2>MCP</h2><p>Streamable HTTP endpoint와 표준 JSON-RPC 요청을 지원합니다.</p></div></header>
        <pre>{`URL: ${window.location.origin}/mcp\nAuthorization: Bearer ptium_…\nScopes: mcp:use + presentations:read/write`}</pre>
        <div className="tool-list"><code>ptium.list_presentations</code><code>ptium.get_presentation</code><code>ptium.create_presentation</code><code>ptium.generate_presentation</code></div>
      </section>
      <section className="admin-panel api-doc-card">
        <header><span><Braces size={20} /></span><div><h2>생성 예시</h2><p>초안을 만든 후 같은 ID로 생성 작업을 등록합니다.</p></div></header>
        <pre>{`POST /api/v1/presentations\n\n{\n  "title": "2026 사업 전략",\n  "prompt": "경영진을 위한 실행 중심 전략 덱",\n  "requestedSlideCount": 10,\n  "language": "ko",\n  "tone": "professional"\n}`}</pre>
      </section>
    </div>
  </AppShell>
}
