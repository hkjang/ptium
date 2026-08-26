# Ptium v1.17.6

## MCP 도구가 "생성은 기다려야 한다"고 말합니다

Claude·Cursor 같은 도구에서 MCP 로 붙는 흐름을 클라이언트처럼 처음부터 끝까지 돌려
봤습니다. 프로토콜·도구 목록·자원 목록·오류 모양은 전부 정확했고(모르는 도구는 `-32602`,
없는 덱은 도구 결과 안의 오류), 실제로 **살아 있는 모델로 5장짜리 덱까지 만들어졌습니다.**

문제는 하나였습니다. `ptium.generate_presentation` 의 설명이 이랬습니다.

> Queue generation (or regeneration) for an existing presentation.

**대기열에 넣었다는 말은 있는데, 언제 끝나는지 어떻게 아는지가 없습니다.** 에이전트는
바로 `get_presentation` 을 부르고, 슬라이드가 없는 덱을 보고, **일어나지도 않은 실패를
보고합니다.** 자체 호스팅 모델은 1~3분이 걸립니다.

도구 설명은 에이전트가 읽는 유일한 설명서이므로, 거기에 적었습니다.

- `generate_presentation`: **작업을 대기열에 넣은 시점에 반환**하며, `get_presentation`
  의 `status` 가 `completed` 또는 `failed` 가 될 때까지 확인하라고 명시
- `get_presentation`: `status` 가 무엇을 뜻하는지, **아직 진행 중이면 슬라이드가 없다**는 것
- `create_presentation`: 만들면 **슬라이드가 없는 초안**이라는 것
- 서버 `instructions`: 덱 만들기는 **두 단계이고 두 번째는 비동기**라는 것

문서(`docs/mcp.md`)에도 같은 말을 적었습니다.

## 검사

도구 정의를 직렬화해서 이 문장들이 들어 있는지 고정했습니다 — 설명을 예전 한 줄로
되돌리면 세 곳에서 실패합니다.

## 설치

```bash
gzip -dc ptium-1.17.6.tar.gz | docker load
```
