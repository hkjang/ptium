# Ptium v1.28.4

사람이 보는 화면 세 곳에서 **기다림과 작성 중**을 구분했습니다. 마지막으로 **기계가 읽는
설명**을 봤습니다.

## MCP 도구가 지킬 수 없는 시간을 약속하고 있었습니다

```
Writing a deck takes seconds to a few minutes depending on the provider.
docs/mcp.md:  A self-hosted model takes a minute or three.
```

1.28.0부터 이 제품은 **느린 생성을 그대로 둡니다.** 한 번 호출에 최대 1시간까지 허용할
수 있고, 자동 수정을 10회까지 겁니다. **30분짜리 생성은 고장이 아닙니다.**

그런데 도구 설명은 "몇 분"이라고 말합니다. 이 설명을 읽은 에이전트는 **시계를 보고
포기하고**, 잘 되고 있는 덱을 실패로 보고합니다 — 사람 화면에서 고친 것과 **같은
잘못**입니다.

## 시간 대신 상태를 보라고 말합니다

```
poll ptium.get_presentation until status is "completed" or "failed",
rather than giving up after any particular time.
The built-in generator answers in seconds; a self-hosted model with repair
passes enabled can take tens of minutes, and a deck still being written is
not a deck in trouble. While status reads "queued" no worker has picked it
up yet; "generating" is one writing it.
```

`docs/mcp.md`도 같은 내용으로 고쳤습니다.

## 검사

도구 설명에 **"a few minutes" 같은 시간 약속이 다시 들어오면 테스트가 실패합니다.**
`poll` · `queued` · `generating`을 말하는지, 그리고 폴링에 쓰는 도구가 다섯 가지 상태를
모두 이름으로 말하는지도 함께 봅니다. 예전 문장으로 되돌리면 그 자리에서 잡습니다.

전체 Go 테스트 · REST 574개 0 failures.

## 설치

```bash
gzip -dc ptium-1.28.4.tar.gz | docker load
```
