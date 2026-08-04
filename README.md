# Ptium

Ptium은 프롬프트를 구조화된 슬라이드 덱으로 바꾸는 자체 호스팅 AI
프레젠테이션 서비스입니다. 사용자 워크스페이스와 관리자 콘솔을 분리하고,
PostgreSQL 하나만을 필수 인프라로 사용합니다. Keycloak을 포함한 표준 OIDC,
REST API, MCP, API 키 회전, 서버 오류 관리가 한 서비스에 포함됩니다.

## 주요 기능

- **보유한 PowerPoint 템플릿(.pptx/.potx)에 맞춘 생성** — 업로드한 파일의 마스터,
  레이아웃, 테마 색, 글꼴, 로고를 그대로 유지하고 슬라이드만 새로 만듭니다
- 레이아웃 자동 선택 — 표지·구역·본문·2단·비교·인용·이미지·마무리 역할을 인식해
  각 슬라이드의 목적에 맞는 레이아웃에 내용을 배치
- 자리 표시자 용량 인식 — 각 슬롯이 담을 수 있는 글자 수를 측정해 문안을 맞추고,
  넘치는 경우 PowerPoint 자동 축소 값을 미리 계산
- **슬라이드 컴포넌트 14종** — KPI 타일, 히어로 수치, 프로세스 스텝, 타임라인,
  비교 카드, 막대·가로막대·선·100% 누적 차트, 미터, 표, 인용, 콜아웃을 템플릿
  테마 색과 글꼴로 그려 내보냅니다 (차트 부품 없이 순수 도형으로 생성)
- **기본 제공 디자인 30종** — 10개 팔레트 × 6개 레이아웃 계열
  (Classic, Rail, Centered, Panel, Editorial, Minimal)
- 브라우저 SVG 미리보기 — PowerPoint 없이 실제 템플릿의 배치·색·글꼴로 렌더링
- 주제·청중·톤·언어·테마·슬라이드 수를 지정하는 AI 덱 생성
- 개요 설계 → 문안 작성 2단계 생성, AI 공급자 키가 없어도 완결된 덱을 만드는
  결정적 로컬 생성기(폐쇄망 기본값)
- 슬라이드 편집, 레이아웃 전환, 발표자 노트 및 PowerPoint 내보내기
- 사용자별 회사/직무/기본 청중/톤/언어/브랜드 색상 개인화
- 브랜딩, AI, 생성 정책, OIDC, 보안 설정을 다루는 별도 관리자 콘솔
- 요청 ID, 구조화 로그, 영속화된 서버 오류 및 확인/해결 워크플로
- 범위·만료일이 있는 해시 API 키와 무중단 유예 기간 키 회전
- 같은 권한/서비스 계층을 사용하는 버전형 REST API와 MCP JSON-RPC
- PostgreSQL DSN만으로 자동 마이그레이션하며 기동하는 단일 Go 서비스
- 인터넷이 없는 Linux/AMD64 환경용 `ptium:<version>` Docker 이미지 번들

## 가장 빠른 실행

필수 값은 PostgreSQL DSN 하나입니다.

```bash
cd server
DATABASE_URL='postgres://ptium:ptium@localhost:5432/ptium?sslmode=disable' go run ./cmd/ptium
```

API는 기본적으로 `http://localhost:8080`에서 실행되며 `/healthz`와
`/readyz`를 제공합니다. 첫 실행 시 스키마와 기본 설정이 자동으로 만들어집니다.
OIDC/AI 키가 없어도 서버와 로컬 생성기는 기동합니다. 개발 중 React UI는 별도
터미널에서 실행합니다.

```bash
cd web
npm install
npm run dev
```

전체 참조 환경은 다음 한 줄로 실행할 수 있습니다.

```bash
cp .env.example .env
docker compose up --build
```

- 웹 UI: <http://localhost:8080>
- API: <http://localhost:8080/api/v1>
- MCP: <http://localhost:8080/mcp>

컨테이너 하나가 워크스페이스, REST API, MCP를 같은 포트에서 제공합니다. 리버스
프록시나 별도 웹 컨테이너는 없습니다.

`docker-compose.yml`의 개발 인증 기본값은 로컬 평가를 위한 것입니다. 외부에
노출하기 전 반드시 `.env`에서 비활성화하고 OIDC를 구성하십시오.

## 회사 템플릿 사용

**템플릿** 화면에서 회사 표준 `.pptx` 또는 서식 파일 `.potx`를 업로드하면 Ptium이
그 파일의 슬라이드 마스터와 레이아웃을 분석해 목록으로 보여 줍니다. 이후 생성되는
덱은 원본 패키지를 그대로 복제한 뒤 슬라이드 부분만 새로 써 넣기 때문에, 결과
파일은 해당 템플릿으로 직접 만든 자료와 구분되지 않습니다.

```bash
curl -H 'Authorization: Bearer <key>' \
     -F 'file=@company-template.pptx' -F 'name=사내 표준 제안서' \
     http://localhost:8080/api/v1/templates
```

- 슬라이드가 들어 있는 일반 발표 파일도 템플릿으로 쓸 수 있습니다. 원본 슬라이드는
  생성 시 사용되지 않고 마스터·레이아웃·테마만 재사용됩니다.
- 업로드한 템플릿은 기본적으로 개인용이며, 조직 전체에 공유할 수 있습니다.
- 템플릿을 선택하지 않으면 요청한 디자인 키에 해당하는 기본 제공 템플릿이
  사용됩니다. 기본 디자인 30종은 서버 기동 시 코드에서 다시 생성되므로
  폐쇄망에서도 항상 사용할 수 있습니다.

### 업로드한 템플릿의 디자인을 그대로 사용합니다

템플릿의 정체성은 색 조합이 아니라 배경 사진, 브랜드 바, 로고, 그라디언트 패널에
있습니다. Ptium은 마스터와 레이아웃이 그리는 요소를 그리는 순서대로 읽어 미리보기에
그대로 표시하고(사진은 미리보기 크기에 맞춰 다시 인코딩해 SVG에 삽입), 텍스트가 들어갈
자리는 그 그림이 비워 둔 영역에서 찾습니다.

- 플레이스홀더가 없는 레이아웃도 사용합니다. 구글 슬라이드에서 내보낸 파일이나 디자이너가
  만든 파일은 본문 상자 없이 그림만으로 구성된 경우가 많습니다. 이런 레이아웃은 그림이
  차지하지 않은 가장 큰 사각형을 찾아 제목·리드·본문 영역을 만들고, 글자 크기는 슬라이드
  기준으로, 글꼴은 테마에서, 글자 색은 뒤에 놓인 그림의 평균 색과의 대비로 결정합니다.
- 이렇게 만든 영역은 내보낼 때 일반 텍스트 상자로 나가므로 레이아웃의 그림은 그대로
  남고, PowerPoint에서 계속 편집할 수 있습니다.

## 코드로 쓰고 바로 PPT로

덱은 텍스트로 작성됩니다. 편집기의 **코드** 탭에서 덱 전체를 텍스트로 열어 고치고
적용하면, 템플릿에 맞춰 다시 그립니다. 반대로 캔버스에서 고친 내용도 텍스트로 다시
읽힙니다.

```
# 전환 대상과 우선순위
> 42개 시스템을 세 묶음으로 나눴습니다.
::kpi 규모
- 전환 대상 | 42개
- 예상 절감 | 18%
::
!notes 1차 범위만 승인받으면 나머지는 실적으로 설득합니다.
```

`#` 제목, `@` 슬라이드 종류, `>` 리드, `-` 항목, `::종류 … ::` 컴포넌트,
`!notes` 발표 노트. 이것이 전부입니다. 전체 문법과 컴포넌트 목록은
[docs/deck-source.md](docs/deck-source.md)에 있습니다.

입력을 멈추면 오른쪽에 해당 슬라이드가 실제 템플릿으로 그려집니다. 저장하지 않은
텍스트를 그대로 컴파일해 그리는 것이므로, 적용하기 전까지 덱은 바뀌지 않습니다.

프롬프트도 이 텍스트를 통해 반영됩니다. AI 공급자가 없는 폐쇄망에서는 프롬프트에서
주제·기간·숫자를 읽어 각 주제를 어떤 방식으로 논증할지(순서, 타당성, 비교, 리스크,
기대효과) 결정한 뒤 그 구조를 텍스트로 씁니다. "3장으로 정리해줘"처럼 분량을 말하면
슬라이드 수 설정보다 프롬프트를 우선합니다.

## 디자인 체계

생성된 슬라이드는 템플릿에서 파생된 하나의 디자인 시스템을 따릅니다.

- **색 역할** — 배경은 슬라이드가 실제로 칠하는 색에서 읽고, 본문·보조·흐린
  텍스트는 대비 4.5:1 / 3:1을 만족하는 지점까지만 배경 쪽으로 흐려집니다.
  텍스트는 절대 데이터 색을 입지 않습니다.
- **데이터 색 순서** — 테마의 액센트 6개에서 카테고리 순서를 만들고, 회색에
  가까운 색과 앞 색과 구분되지 않는 색은 제외합니다. 구분은 OKLab 거리로
  계산하며 눈대중하지 않습니다. 한도를 넘는 계열은 새 색을 만들지 않고 회색으로
  물러납니다.
- **폼 선택** — 숫자 몇 개는 차트가 아니라 KPI 타일, 한 개는 히어로 수치,
  크기 비교는 단일 색조에 강조 하나, 부분-전체는 100% 누적 막대입니다.
  이중 축과 원형 차트는 쓰지 않습니다.
- **마크 규격** — 막대는 밴드를 채우지 않고 값 끝만 둥글며 기준선은 각지게,
  선은 2px에 끝점만 직접 라벨, 격자선 대신 직접 라벨, 누적 구간은 테두리가
  아니라 배경색 간격으로 분리합니다.
- **글자 폭 실측** — 한글·한자는 1em, 라틴은 약 0.5em으로 계산해 문안을 맞추고,
  넘칠 때만 PowerPoint 자동 축소 값을 미리 넣습니다.

30종 팔레트는 명도 대역, 채도 하한, 인접 색 구분(정상 시각과 적·녹색약 모두),
배경 대비를 모두 통과한 값입니다.

## 첫 로그인: 로컬 관리자

OIDC를 붙이기 전에도 서비스를 운영할 수 있도록, 아이디와 비밀번호로 로그인하는
관리자 계정을 환경변수로 지정합니다.

```dotenv
BOOTSTRAP_ADMIN=admin@example.com
BOOTSTRAP_ADMIN_PASSWORD=replace-with-at-least-12-characters
BOOTSTRAP_ADMIN_NAME=Ptium Administrator
```

- 비밀번호는 **처음 기동할 때 한 번만** 기록됩니다. 이후 제품 화면(개인화 →
  비밀번호)에서 바꾼 비밀번호는 재시작해도 환경변수 값으로 되돌아가지 않습니다.
- 비밀번호를 잊었다면 한 번만 `BOOTSTRAP_ADMIN_PASSWORD_RESET=true`로 기동하면
  환경변수 값으로 다시 설정됩니다.
- 비밀번호는 bcrypt로 저장되며 환경변수에서만 읽습니다. 설정 테이블에 저장되지
  않고 API로도 반환되지 않습니다.
- 로그인 시도는 클라이언트 주소 기준으로 지연이 늘어나며, 비밀번호를 변경하면
  이전에 발급된 세션 토큰이 모두 무효화됩니다.

### 세션 유지

로그인 세션은 `ptium_session` HttpOnly 쿠키에 담깁니다. 탭을 닫거나 새 탭을 열어도
로그인이 유지되고, 스크립트가 값을 읽을 수 없습니다. 수명은
`SESSION_LIFETIME`(기본 12시간)이며, 절반을 지난 세션은 다음 요청에서 자동으로
갱신되므로 작업 중에 갑자기 로그아웃되지 않습니다. 아무 요청이 없는 세션은 예정대로
만료됩니다.

OIDC로 로그인한 경우에도 같은 쿠키를 사용합니다. 공급자의 액세스 토큰은 보통 몇 분
안에 만료되고 리프레시 토큰은 브라우저로 내려가지 않으므로, 로그인 직후 한 번
`POST /api/v1/auth/session`으로 Ptium 세션으로 교환합니다.

## Keycloak 연결

Keycloak에서 공개 클라이언트를 하나 만들고 Standard Flow와 PKCE(S256)를
활성화합니다. 웹 출처와 redirect URI를 Ptium 주소로 제한한 다음 두 값만
부트스트랩하면 discovery, authorization endpoint, token endpoint와 JWKS를
자동으로 찾습니다.

```dotenv
OIDC_ISSUER_URL=https://sso.example.com/realms/company
OIDC_CLIENT_ID=ptium-web
BOOTSTRAP_ADMIN_EMAILS=admin@example.com
```

클라이언트에 시크릿이 필요한 **기밀 클라이언트**라면 `OIDC_CLIENT_SECRET`을
설정하십시오. 시크릿은 브라우저로 내려가지 않고, Ptium이 authorization code
교환을 서버에서 대신 수행합니다(`POST /api/v1/auth/token`). 시크릿을 비워 두면
브라우저가 PKCE로 직접 교환하는 공개 클라이언트로 동작합니다.

```dotenv
OIDC_CLIENT_SECRET=only-for-a-confidential-client
```

Keycloak 역할 `ptium-admin` 또는 `admin`은 기본적으로 Ptium 관리자에 매핑됩니다.
첫 관리자가 로그인한 뒤에는 관리자 콘솔에서 OIDC 및 역할 정책을 관리하고
부트스트랩 선택자는 제거하는 것을 권장합니다. HTTP 기반 로컬 Keycloak은
개발 환경에서만 `OIDC_ALLOW_HTTP=true`로 허용합니다.

## 인증 개발 모드

개발 인증은 기본적으로 꺼져 있습니다. 사용할 때는 32자 이상의 별도 비밀값과
고정 개발 주체를 지정합니다. 브라우저가 임의로 관리자 역할을 선택할 수 없으며,
원격 접속은 명시적으로 허용하지 않는 한 거부됩니다.

```dotenv
DEV_AUTH_ENABLED=true
DEV_AUTH_SECRET=replace-with-at-least-32-random-characters
DEV_AUTH_EMAIL=developer@example.com
DEV_AUTH_ROLES=ptium-admin,user
```

## API와 MCP

REST 경로는 `/api/v1` 아래에 버전이 지정됩니다. 브라우저 OIDC 토큰 또는
`Authorization: Bearer ptium_...` API 키를 사용할 수 있습니다. 키는 UI의
**개발자 설정**에서 만들며 원문은 한 번만 표시됩니다. MCP도 같은 bearer 키와
범위 검사를 사용합니다.

```bash
curl -H 'Authorization: Bearer <key>' http://localhost:8080/api/v1/presentations
```

MCP 클라이언트 설정과 도구 목록은 [MCP 문서](docs/mcp.md), REST 스키마는
[`api/openapi.yaml`](api/openapi.yaml)을 참고하십시오.

## 관리자 운영

`/admin`은 서버에서도 관리자 역할을 다시 검사합니다. 콘솔에서 다음을 관리할
수 있습니다.

- 제품명/로고/브랜드 색상
- AI 공급자 URL, 모델, 쓰기 전용 API 키
- 기본 언어/테마/슬라이드 제한과 키 회전 유예 기간
- OIDC issuer/client/역할 정책
- 사용자 활성화 및 관리자 역할
- 서버 오류의 심각도, 상태, 메모와 해결 이력

응답의 `X-Request-ID`는 구조화 로그와 오류 레코드를 연결합니다. 민감 헤더와
설정값은 관리자 응답과 오류 컨텍스트에서 제거됩니다.

## 검증

```bash
cd server && go test ./...
cd ../web && npm run typecheck && npm run build
docker compose config
docker build -t ptium:dev .
```

요구사항별 검증 기준은 [objective traceability](docs/requirements.md), 구조와
보안 결정은 [architecture](docs/architecture.md)와 [security](docs/security.md)에
정리되어 있습니다.

## 오프라인 릴리스

릴리스 자산 `ptium-<version>.tar.gz`에는 배포용 literal 이미지명
`ptium-<version>:latest`와 호환 태그 `ptium:<version>`만 들어 있습니다.
데이터베이스는 번들에 포함하지 않고, 배포 시 `DATABASE_URL`로 이미 운영 중인
PostgreSQL을 지정합니다. 인터넷 연결이 있는 빌드 호스트에서 다음 명령으로
동일한 번들을 재현할 수 있습니다.

```bash
./scripts/build-offline.sh
```

```powershell
.\scripts\build-offline.ps1
```

쿠버네티스 배포 예시는 [`deploy/kubernetes.yaml`](deploy/kubernetes.yaml)에
있습니다. 시크릿에 `DATABASE_URL`과 `KEY_ENCRYPTION_SECRET`만 넣으면 됩니다.

체크섬 확인, 이미지 로드, 폐쇄망 compose 실행 및 업그레이드는
[offline deployment runbook](docs/offline-deployment.md)을 참고하십시오.
