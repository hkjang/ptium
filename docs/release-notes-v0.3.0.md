# Ptium v0.3.0

인증 경로를 두 가지 추가했습니다. OIDC 기밀 클라이언트를 지원하고, 아이디·비밀번호로
로그인하는 초기 관리자 계정을 환경변수로 지정할 수 있습니다.

## 초기 관리자 계정 (아이디·비밀번호)

```dotenv
BOOTSTRAP_ADMIN=admin@example.com
BOOTSTRAP_ADMIN_PASSWORD=replace-with-at-least-12-characters
BOOTSTRAP_ADMIN_NAME=Ptium Administrator
```

OIDC를 붙이기 전에도 서비스를 운영할 수 있습니다.

- 비밀번호는 bcrypt(cost 12)로 저장하고 **환경변수에서만** 읽습니다. 설정 테이블에
  저장되지 않고, 로그에도 남지 않으며, 어떤 API도 반환하지 않습니다.
- 비밀번호는 계정을 만들 때 한 번만 기록됩니다. 제품 화면에서 바꾼 비밀번호는
  재시작해도 환경변수 값으로 되돌아가지 않습니다. 잊었을 때는 한 번만
  `BOOTSTRAP_ADMIN_PASSWORD_RESET=true`로 기동하면 복구됩니다.
- 없는 아이디도 실제 비교와 같은 시간을 쓰도록 미끼 해시와 대조하므로, 응답 시간으로
  계정 존재 여부를 알아낼 수 없습니다. 클라이언트에는 아이디 또는 비밀번호가 틀렸다는
  사실만 알려 줍니다.
- 실패한 시도는 클라이언트 주소별로 2초에서 5분까지 지연이 배로 늘어납니다. 제한기는
  프로세스 내에 있어 레플리카마다 독립적으로 동작합니다 — 추측 비용을 올리는 장치이며,
  각 시도를 비싸게 만드는 것은 bcrypt입니다.
- 로그인하면 `ptses_` 접두사가 붙은 무상태 세션 토큰을 발급합니다. API 키로 오인될 수
  없고, 역할은 매 요청마다 데이터베이스에서 읽으므로 관리자 권한 회수가 토큰 만료를
  기다리지 않고 즉시 반영됩니다.
- 비밀번호를 변경하면 그 이전에 발급된 세션 토큰이 모두 무효화됩니다. 다른 기기는
  로그아웃되고, 변경을 수행한 브라우저는 새 토큰을 받습니다.
- 로그인 화면에 아이디·비밀번호 폼이 추가되고, 개인화 화면에서 비밀번호를 바꿀 수
  있습니다(로컬 계정만).

## OIDC 클라이언트 시크릿

```dotenv
OIDC_CLIENT_SECRET=only-for-a-confidential-client
```

환경변수 또는 관리자 콘솔의 쓰기 전용 설정 `auth.oidc.client_secret`으로 지정합니다.

- 시크릿이 설정되면 브라우저가 아니라 Ptium이 authorization code를 교환합니다
  (`POST /api/v1/auth/token`). 시크릿이 단일 페이지 앱에 내려가지 않습니다.
- 시크릿은 요청 본문이 아니라 HTTP basic 인증으로 전달되므로 공급자 접근 로그에
  남지 않습니다. 공급자가 돌려준 리프레시 토큰은 브라우저로 전달하지 않습니다.
- 시크릿이 없으면 지금까지와 같은 공개 클라이언트(PKCE 직접 교환)로 동작합니다.
- 공개 인증 설정에 `passwordLoginEnabled`와 `tokenExchangeUrl`이 추가되어 클라이언트가
  어떤 방식을 제공해야 하는지 알 수 있습니다.

## 그 외

- 세션 수명은 `SESSION_LIFETIME`으로 조정합니다(기본 12시간).
- `users` 테이블에 `password_hash`, `password_updated_at`이 추가됩니다. 기동 시
  자동 적용되며, 신원 공급자로 만들어진 계정은 값이 비어 있습니다.
- 클라이언트 주소를 파싱할 때 포트가 없는 IPv6 주소가 잘리던 문제를 고쳤습니다.

## 오프라인 자산

`ptium-0.3.0.tar.gz`는 `ptium:0.3.0`과 `ptium-0.3.0:latest`를 담은 Docker load
가능한 gzip 스트림입니다. 데이터베이스는 포함하지 않으며 배포 환경이
`DATABASE_URL`로 제공합니다. 절차는 `docs/offline-deployment.md`를 참고하십시오.
