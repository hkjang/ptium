# Ptium v0.1.0

Ptium의 첫 자체 호스팅 릴리스입니다. 보유한 PowerPoint 템플릿에 맞춰 AI가
프레젠테이션을 생성하고, 원본 디자인을 그대로 유지한 파일로 내보냅니다.
사용자 개인화와 별도 관리자 콘솔을 하나의 배포 단위로 제공합니다.

## 템플릿 기반 생성

- 회사 표준 `.pptx` / `.potx` 업로드 시 슬라이드 마스터, 레이아웃, 테마 색,
  글꼴, 자리 표시자 위치와 용량을 분석해 카탈로그로 관리
- 내보내기는 원본 패키지를 복제한 뒤 슬라이드 부분만 새로 작성 — 마스터,
  레이아웃, 테마, 미디어는 바이트 단위로 그대로 유지되고, 슬라이드 도형은
  레이아웃에서 위치·글꼴·색·불릿 서식을 상속
- 표지·구역·본문·2단·비교·인용·이미지·마무리 레이아웃 역할을 자동 인식하고
  각 슬라이드의 목적에 맞게 선택
- 개요 설계 → 문안 작성 2단계 AI 생성. 모델 응답의 잘못된 레이아웃 이름,
  존재하지 않는 슬롯, 마크다운 기호, 용량 초과는 자동으로 보정
- 글자 폭을 em 단위로 실측(한글·한자 1em, 라틴 약 0.5em)해 문안을 맞추고,
  넘칠 경우 `normAutofit` 축소 값을 미리 계산
- 발표자 노트는 notesSlide로 기록되며, 템플릿에 노트 마스터가 없으면 생성
- PowerPoint 없이 실제 템플릿 배치·색·글꼴로 렌더링하는 SVG 미리보기
- 기본 제공 디자인 5종(Aurora, Modern, Paper, Mint, Graphite)은 서버 기동 시
  코드에서 재생성되어 폐쇄망에서도 별도 자산 없이 동작

## 포함 기능

- Go API와 React/TypeScript 워크스페이스
- PostgreSQL DSN 단독 기동, 자동 마이그레이션 및 로컬 fallback 생성기
- OpenAI 호환 모델 설정과 쓰기 전용 암호화 시크릿
- Keycloak/표준 OIDC discovery, JWKS 검증, Authorization Code + PKCE
- 사용자 프로필/생성 기본값과 소유자 단위 덱 관리
- 관리자 설정, 사용자 상태/역할, 집계된 서버 오류 처리
- 범위/만료/마지막 사용 기록이 있는 API 키 및 유예 기간 회전
- REST API, OpenAPI 문서, 인증된 MCP Streamable HTTP 도구/리소스
  (`ptium.list_templates` 포함, `templates:read`/`templates:write` 범위 추가)
- Linux/AMD64 단일 `ptium-0.1.0:latest` 런타임 이미지(호환 태그 `ptium:0.1.0` 포함)

## 오프라인 자산

`ptium-0.1.0.tar.gz`는 Docker load 가능한 gzip 스트림이며 다음 태그를
포함합니다.

- `ptium:0.1.0`
- `ptium-0.1.0:latest`
- `postgres:16-alpine`

체크섬 파일로 무결성을 확인한 뒤 함께 제공되는 compose/env 예시를 사용하면
외부 registry 접근 없이 배포할 수 있습니다. 자세한 절차는 저장소의
`docs/offline-deployment.md`를 참고하십시오.
