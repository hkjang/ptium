# Ptium v1.27.0

앞선 세 판은 **화면이 저장된 값을 그대로 보여 주게** 했습니다. 그러면 반대쪽 질문이 남습니다 —
**저장된 값은 실제로 쓰이고 있나?**

## 저장되지만 쓰이지 않는 설정이 여덟 개 있었습니다

읽는 쪽은 이미 범위를 갖고 있습니다. 응답 제한 시간은 10~3600초 안에서만, 자동 수정은
0~10회 안에서만 적용됩니다. **그런데 API는 무엇이든 받아 저장했습니다.**

```
PUT ai.timeout_seconds = 99999        → 200 저장됨   (실제 적용: 300초)
PUT generation.repair_passes = 500    → 200 저장됨   (실제 적용: 3회)
PUT ai.reasoning = "thinking-hard"    → 200 저장됨   (실제 적용: auto)
PUT generation.outline_pass = "yes"   → 200 저장됨   (실제 적용: 사용)
PUT generation.max_template_mb = 0    → 200 저장됨   (실제 적용: 32MiB)
PUT generation.max_slides = 900       → 200 저장됨   (실제 적용: 50장)
```

그리고 **관리자 설정 화면은 저장된 값을 그대로 보여 줍니다.** 자기 배포 화면에서
"생성 후 자동 수정 500"을 읽으면서 실제로는 3회가 돌고 있었습니다.

더 나쁜 것은 **API 문서**입니다. `openapi.yaml`은 이 범위들을 이미 적어 두고 있었습니다 —
`ai.timeout_seconds: 10~3600`, `generation.repair_passes: 0~10`, `ai.reasoning: auto·off·on`.
**문서가 약속한 것을 서버가 지키지 않고 있었습니다.**

## 적용하지 않을 값은 거절합니다

```
PUT ai.timeout_seconds = 99999   → 422  ai.timeout_seconds must be a whole number between 10 and 3600
PUT ai.reasoning = "thinking-hard" → 422  ai.reasoning must be one of auto, off, on
PUT generation.outline_pass = "yes" → 422  generation.outline_pass must be true or false
```

화면에서는 우리말로 나옵니다 — **"응답 제한 시간은 10에서 3600 사이의 정수여야 합니다.
이 범위를 벗어난 값은 저장해도 적용되지 않습니다."** 조사는 낱말에 맞춰 고릅니다.

## 범위는 한 곳에만 적혀 있습니다

`internal/settings/bounds.go`가 각 설정이 **적용되는 범위**를 갖고 있고, 거절하는 쪽과
읽는 쪽이 **같은 표**를 봅니다. 둘이 갈라질 수 없습니다.

문서에 없던 세 개(`generation.max_template_mb` · `outline_pass` · `allow_user_uploads`)도
스키마에 적었고, **범위를 가진 설정이 문서에 빠지면 테스트가 실패합니다.**

## 검사

REST 훑기가 여덟 개 설정마다 "적용되지 않는 값은 거절되는지, 적용되는 값은 저장되는지"를
확인합니다. 검사를 꺼 보면 그 자리에서 잡습니다.

전체 Go 테스트 · 웹 134개 · REST 573개 · deep · edges · 화면 훑기 0 failures.

## 설치

```bash
gzip -dc ptium-1.27.0.tar.gz | docker load
```
