# Ptium v1.30.2

다시 쓰기를 막는 **문**이 두 개였습니다. 어제 고친 것은 그중 하나였습니다.

## 같은 사실, 다른 말

| 문 | 어제까지 | 어제 고친 뒤 |
|---|---|---|
| `POST /presentations/{id}/generate` (생성 다시 시도 · 가져온 덱 다시 생성) | 대기열에 넣고 실패 | **422** "연결된 AI 모델이 필요합니다… 덱은 그대로입니다" |
| `POST /presentations/{id}/rewrite` (**AI로 다듬기** 버튼) | **409** "관리자에게 서비스 설정의 AI 항목을 요청하세요" | 〃 |

**다듬기 버튼은 처음부터 막고 있었습니다** — 덱을 잃지도 않았습니다. 다만 **관리자를
부르라고** 했습니다. 고칠 것이 없는 배포에 대해서요.

게다가 "모델이 연결되어 있는가"를 **각자 따로 판단**하고 있었습니다(`aiProviderConfigured`와
어제 만든 `modelConnected`). 같은 질문에 대한 답이 두 벌 있으면 언젠가 갈라집니다.

## 하나로

- 판단은 **한 곳**에서 합니다.
- 두 문 모두 **`rewrite_needs_model`** 로 답하고, **덱을 만든 언어**로 같은 문장을 말합니다.
- 스키마에도 두 코드(`nothing_to_rewrite` · `rewrite_needs_model`)를 적었습니다.

```
ko: 덱을 다시 쓰려면 연결된 AI 모델이 필요합니다. … 덱은 그대로입니다.
en: Rewriting a deck needs a connected AI model. … Your deck is unchanged.
```

## 검사

- 두 문의 답이 갈라지면(관리자를 부르는 문장이 돌아오거나, 판단이 두 벌이 되거나,
  코드가 달라지면) 테스트가 실패합니다.
- REST 훑기가 다듬기 버튼도 눌러 보고, **코드와 문구**를 확인합니다.

전체 Go 테스트 · REST 586개 · deep 0 failures.

## 설치

```bash
gzip -dc ptium-1.30.2.tar.gz | docker load
```
