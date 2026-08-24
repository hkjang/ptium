import { BookOpen, Braces, KeyRound, LayoutTemplate, Radio } from 'lucide-react'
import { AppShell } from '../components/AppShell'

const endpoints = [
  ['GET', '/api/v1/templates', '고를 수 있는 템플릿 목록 (kind=builtin|uploaded, search=)'],
  ['GET', '/api/v1/templates/{id}', '템플릿 하나의 레이아웃과 팔레트'],
  ['GET', '/api/v1/templates/{id}/preview.svg', '템플릿 미리보기 이미지'],
  ['GET', '/api/v1/presentations', '내 프레젠테이션 목록'],
  ['POST', '/api/v1/presentations', '프레젠테이션 초안 생성'],
  ['POST', '/api/v1/presentations/generate', '템플릿을 골라 덱 생성까지 한 번에'],
  ['POST', '/api/v1/presentations/{id}/generate', '슬라이드 생성 작업 큐 등록'],
  ['PATCH', '/api/v1/presentations/{id}', '메타데이터와 슬라이드 편집'],
  ['GET', '/api/v1/presentations/{id}/export?format=pptx', 'PowerPoint 다운로드'],
  ['GET', '/api/v1/presentations/{id}/slides/{n}/regions', '한 슬라이드의 영역을 편집 가능한 개체로 조회'],
  ['POST', '/api/v1/presentations/{id}/slides/{n}/revise', '한 장만 AI로 다시 쓰기 (저장하지 않고 제안)'],
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
      <section className="admin-panel api-doc-card">
        <header><span><LayoutTemplate size={20} /></span><div><h2>템플릿을 골라 만들기</h2>
          <p>다른 서비스에서 연동할 때 쓰는 순서입니다. 목록에서 고른 템플릿 id를 그대로 생성 요청에 넣으면 그 디자인으로 나옵니다.</p></div></header>
        <pre>{`1) 고를 수 있는 것 보기\nGET /api/v1/templates?kind=builtin&search=slate\n→ data[].id · name · description · layoutCount · aspectRatio · tags · dark\n\n2) 하나를 골라 생성 요청\nPOST /api/v1/presentations/generate\n{\n  "title": "보안 교육 계획",\n  "prompt": "사내 보안 교육 계획을 팀장들에게 보고. 대상 320명, 4분기 시행.",\n  "language": "ko",\n  "slideCount": 8,\n  "templateId": "<1)에서 고른 id>"\n}\n→ 202 { "id": "...", "status": "queued", "templateId": "..." }\n\n3) 끝날 때까지 확인\nGET /api/v1/presentations/{id}   → status: completed\n\n4) 파일 받기\nGET /api/v1/presentations/{id}/export?format=pptx`}</pre>
        <p className="doc-note">API 키에는 <code>templates:read</code> 와 <code>presentations:write</code> 가 필요합니다.
          템플릿을 지정하지 않으면 워크스페이스 기본 디자인으로 만들어집니다.</p>
      </section>
    </div>
  </AppShell>
}
