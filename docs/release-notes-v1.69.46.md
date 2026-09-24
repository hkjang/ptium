# Ptium 1.69.46

## MCP 클라이언트에 붙이려면 먼저 키를 발급받아 붙여넣어야 했습니다

`/mcp` 는 개인 API 키로만 열렸습니다. 사내에 Keycloak 이 있고 그것으로 웹에 로그인하는
사람도, MCP 클라이언트를 붙이려면 개발자 설정에 들어가 키를 만들고, 한 번만 보이는 문자열을
클라이언트 설정 파일에 붙여넣고, 만료 전에 손으로 갈아 끼워야 했습니다. 로그인은 이미 되어
있는데 자격증만 한 벌 더 드는 일입니다.

이제 관리자가 **MCP · SSO** 를 켜면 같은 `Authorization` 헤더로 **Keycloak 액세스 토큰**도
받습니다. 클라이언트에 주는 것은 주소 하나입니다.

```json
{ "mcpServers": { "ptium": { "type": "streamable-http",
                             "url": "https://slides.example.com/mcp" } } }
```

클라이언트는 401 을 한 번 받고, 그 401 이 가리키는 문서를 읽고, Keycloak 으로 사람을 보내
토큰을 받아 돌아옵니다. Keycloak 에 이미 로그인해 있으면 화면은 거의 지나갑니다.

## 이 서버가 하는 일은 토큰을 받는 것이지 발급하는 것이 아닙니다

MCP 인가 명세는 OAuth 2.1 이고, 그 안에서 이 서버는 **리소스 서버**입니다. `/authorize` 도
`/token` 도 동적 클라이언트 등록도 만들지 않고, 토큰을 저장하거나 세션으로 바꾸지도 않습니다.
하는 일은 셋입니다.

```
GET /.well-known/oauth-protected-resource[/mcp]   인증 없이 맨 JSON, 꺼져 있으면 404
POST /mcp  (토큰 없음)  → 401 WWW-Authenticate: Bearer realm="ptium-mcp",
                                resource_metadata="…/oauth-protected-resource/mcp"
POST /mcp  (Bearer …)   → ptium_ 로 시작하면 키, JWT 모양이면 토큰
```

토큰은 서명(제공자의 JWKS, RS/ES/PS 계열만) · `iss` · `exp` · `nbf` 를 보고, `typ` 이 `ID` 면
— 클라이언트가 액세스 토큰 자리에 ID 토큰을 보낸 것이므로 — 거절하고, `cnf` 가 있으면
(DPoP · mTLS 로 키에 묶인 토큰) 거절합니다. REST 의 401 에는 `resource_metadata` 가 붙지
않습니다. 거기서 OAuth 를 시작하라고 말한 적이 없기 때문입니다.

## 남의 앱 토큰으로 이 앱이 열리지 않게

`aud` 에 이 배포의 리소스 식별자(`https://<공개 주소>/mcp`)가 있거나, `aud` 또는 `azp` 가
관리자가 적어 둔 `mcp.oauth.audience` 에 있어야 합니다. 둘 다 아니면 거절입니다.

Keycloak 26 은 매퍼 없이는 액세스 토큰의 `aud` 에 `account` 만 싣고 클라이언트 ID 는 `azp` 에
담습니다. 그래서 `azp` 도 보고, 거절할 때는 **토큰이 실제로 무엇을 실어 왔는지와 어디에 무엇을
적으면 되는지**를 함께 돌려줍니다.

```
The token was not issued for this server (aud=[account], azp="claude-mcp"):
add "claude-mcp" to mcp.oauth.audience, or …
```

웹 로그인 클라이언트(`auth.oidc.client_id`)는 **자동으로 허용되지 않습니다.** 웹용 토큰과
MCP 용 토큰은 다른 것이고, 반대쪽도 같습니다 — 웹 로그인의 검사기가 이제 다른 클라이언트에게
발급된 토큰(`azp`)을 거절하므로, MCP 클라이언트의 토큰으로 REST API 가 열리지 않습니다.

## 토큰이 여는 것은 이미 있는 계정입니다

토큰의 `sub` 로 **이미 등록된 활성 계정**만 찾습니다.

- 계정을 만들지 않습니다. 웹으로 한 번 로그인하는 것이 등록이고, 그 전에는
  `This SSO account is not registered here. Sign in to the web once first` 입니다.
  `/mcp` 는 웹 로그인의 계정 생성 검사기가 없는 자기 체인을 씁니다.
- 정지된 계정을 되살리지 않고, 토큰에 실린 role 로 권한이 올라가지 않습니다. 권한은 계정의
  것입니다.
- 범위는 토큰의 `scope` 가 아니라 관리자가 정한 `mcp.oauth.scopes` 입니다(기본
  `presentations:read templates:read` — 쓰기는 한 번 고르는 일입니다). 토큰이 이 앱의 범위
  어휘를 실어 오면 교집합을 취하고, 키가 지나는 것과 **같은 문**을 지납니다. 범위 밖 도구는
  키일 때와 똑같이 403 입니다.

꺼진 것이 기본이고, 켜지 않은 배포에서는 아무것도 달라지지 않습니다. **키는 전과 완전히
같습니다** — 폐쇄망 스크립트는 계속 키를 쓰고, 사람은 SSO 를 씁니다. 스위치는 켜져 있는데
Issuer 나 리소스 식별자가 없으면 토큰을 받지 않고, 로그에
`MCP SSO is switched on but cannot take tokens` 와 이유를 남깁니다. 저장은 즉시 적용됩니다.

거절된 토큰은 클라이언트에게 무엇을 고치라는 문장을 주고, **실제로 실패한 검사**(서명 · 발급자 ·
만료 중 어느 것인지)는 그 요청의 request id 와 함께 서버 로그의
`authentication failed … path=/mcp` 줄에 남습니다. 401 본문이 어느 검사에서 걸렸는지까지
알려 주면 토큰을 찍어 보는 쪽에 답을 주는 셈이므로, 둘을 나눴습니다.

관리자 콘솔 **MCP · SSO** 영역, `ADMIN_GUIDE` 3.6 절(Keycloak 클라이언트 설정과 거부
메시지별 조치 표), `docs/mcp.md` 의 "Connecting with SSO, without a key" 에 적었습니다.
이 서버는 introspection 을 하지 않으므로, Keycloak 에서 로그아웃해도 이미 발급된 토큰은
만료(몇 분)까지 삽니다.

## 지키는 장치

`internal/mcpoauth/authenticator_test.go` 는 토큰 하나가 등록된 계정을 여는 것 · 대상 규칙이
리소스 식별자와 관리자 목록 둘뿐인 것 · 표준 이름들(`iss` · `exp` · `nbf` · `typ=ID` · `cnf` ·
서명)을 보는 것 · **토큰이 아무도 등록하지 않는 것** · 스위치가 꺼져 있으면 토큰이 나쁜 키와
똑같이 거절되고 키는 그대로인 것 · 토큰이 어휘를 실어 올 때 범위가 좁아지는 것 · MCP
클라이언트의 토큰이 REST API 를 열지 못하는 것을 봅니다.

`internal/httpapi/mcpoauth_test.go` 는 문 앞에서 봅니다 — 메타데이터가 인증 없이 읽히는 맨
JSON 이고 꺼져 있으면 없는 것 · `/mcp` 의 401 만 메타데이터를 가리키고 REST 의 401 은 아닌 것 ·
거절된 토큰의 원인이 클라이언트가 받은 request id 아래 로그에 남는 것 · SSO 주체가 키처럼
범위에 갇히는 것 · 아무 일도 하지 못할 스위치는 저장이 거부되는 것.

`web/src/pages/mcpsso.test.ts` 는 화면이 서버와 같은 방식으로 검사하는지를 봅니다 — 리소스
식별자는 `https://…/mcp` 만, 키가 가질 수 없는 범위와 빈 목록은 거부.

`go test -race ./...` 과 `go vet ./...`, 웹 타입 검사 · 빌드 · 266 검사 모두 통과합니다.
