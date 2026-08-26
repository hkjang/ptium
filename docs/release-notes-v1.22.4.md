# Ptium v1.22.4

지난 판에서 관리자 개요가 **문서에 없는 항목 3개**를 답한다는 것을 찾았습니다. 그래서
**살아 있는 응답 전부를 스키마에 맞춰** 봤습니다.

## 덱을 열면 덱의 코드가 오는데, 문서에는 없었습니다

```
GET /presentations/{id}
   data.source   ← 문서에 없음
```

덱 하나를 읽으면 **그 덱의 코드(글)** 가 함께 옵니다 — 편집기가 편집하는 바로 그 텍스트입니다.
그런데 이 응답의 봉투는 닫혀 있어서(`additionalProperties: false`), 스키마대로 검증하는
클라이언트는 **덱 자신의 글 때문에 응답을 거절**합니다.

## 그리고 여덟 개의 답은 모양을 아예 말하지 않았습니다

```
GET  /admin/storage             무엇이 오는지 문서에 없음
GET  /admin/audit               〃
GET  /admin/audit/actions       〃
GET  /admin/generations         〃
GET  /admin/provider-check      〃
POST /admin/provider-check      〃
POST /admin/generations/{id}/requeue   〃
POST /admin/generations/{id}/cancel    〃
```

상태 코드와 한 줄 설명만 있고 **본문의 모양은 없었습니다.** 클라이언트를 쓰는 사람은
**요청을 보내 봐야** 무엇이 오는지 알 수 있고, 응답을 검증하는 클라이언트는 **검증할 대상이
없습니다.** 하필 이 여덟은 **운영 도구가 쓰는 것들**입니다 — 보관 용량, 감사 기록, 모델 호스트
상태, 생성 대기열과 그 위의 두 버튼.

전부 적었습니다. 살아 있는 응답으로 다시 맞춰 보니 **일곱 자리 모두 일치**합니다.

## 다시 벌어지지 않도록

**답하면서 무엇을 보내는지 말하지 않는 자리가 있으면 실패하는 검사**를 넣었습니다.
`'200': { description: … }` 처럼 한 줄로 끝나는 답도 잡습니다.

## 검사

전체 Go 테스트 · REST 526개 · edges 33 · deep 전부 0 failures.

## 설치

```bash
gzip -dc ptium-1.22.4.tar.gz | docker load
```
