# Ptium v1.30.3

폐쇄망에 나가는 것은 **이미지 하나와 설정 파일 두 개**입니다. 이번에는 그 파일들을
봤습니다.

## 지금은 맞습니다

먼저 확인한 것들입니다 — **문제는 없었습니다.**

- `deploy/kubernetes.yaml`은 `kubectl apply --dry-run=client`을 **통과**합니다
  (ConfigMap · Deployment · Service · Ingress).
- ConfigMap과 `.env.offline.example`이 이름 붙인 설정은 **모두 서버가 실제로 읽는**
  이름입니다(`AUTH_ADMIN_ROLES`·`OIDC_ADMIN_ROLES`처럼 별칭이 함께 있는 것도 포함).

## 앞으로도 맞도록

폐쇄망 현장에서는 **이 파일들이 유일한 설명서**입니다. 서버가 어떤 이름을 더 이상 읽지
않게 되어도, 그 이름은 파일에 남아 조용히 아무 일도 하지 않습니다 — 관리자 역할을
지정했는데 아무도 관리자가 되지 않는 식으로요.

- **테스트**: 배포 파일이 이름 붙인 설정 중 서버가 읽지 않는 것이 하나라도 있으면
  실패합니다. 이름을 하나 바꿔서 확인했습니다 —
  `deploy/kubernetes.yaml ships AUTH_ADMIN_ROLES_RENAMED and the server never reads it`.
- **빌드**: 번들을 만들 때 `kubectl`이 있으면 매니페스트를 검증하고, 유효하지 않으면
  **릴리스를 멈춥니다**. (`kubectl`이 없는 호스트에서는 조용히 넘어갑니다.)

## 검사

전체 Go 테스트 · REST 586개 0 failures.

## 설치

```bash
gzip -dc ptium-1.30.3.tar.gz | docker load
```
