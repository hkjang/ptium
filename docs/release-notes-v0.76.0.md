# Ptium v0.76.0

지난 판에서 지어낸 출처를 걷어냈습니다. 같은 브리프를 다시 재 보니 이번에는 다른 것이
걸렸습니다 — **한 영역에 요점이 11개, 9개, 14개** 있는 슬라이드 세 장.

## 재기는 했는데, 고치지 못했습니다

로그는 정직했습니다.

```
slide 2 was left as written: body: 11 points on one slide;
  past 6 an audience reads instead of listening
1 slide(s) were measured against the template and rewritten to fit
```

수리 패스는 돌았습니다. 모델에게 다시 쓰라고 했고, 다시 쓴 것을 다시 재 봤고, **나아지지
않아서 버렸습니다**. 버린 판단은 옳았습니다. 문제는 무엇을 시켰느냐입니다.

```
Task: cut this slide to its point. Fewer words per line,
      the same number of lines or fewer.
```

*짧은 줄 11개* 는 이 지시를 완벽하게 지킵니다. 그리고 측정은 줄의 길이가 아니라 **요점의
개수** 를 셉니다. 재는 쪽이 아는 숫자를 쓰는 쪽에는 한 번도 말하지 않았던 것입니다.

## 세 곳이 같은 숫자를 말하게 했습니다

**다시 쓰기 지시** 가 숫자를 말합니다.

```
Task: this slide carries more than a room can take in. Cut it to at most
6 top-level points — fewer if it has less to say — by merging lines that
make one point together and dropping any that only restate another.
```

숫자는 `pptx.MaximumPoints` 에서 옵니다. 측정이 쓰는 바로 그 상수이고, 테스트가 둘이
갈라지는 것을 막습니다.

**작성 브리프** 도 고쳤습니다. 여태 "Three to five per slide" 라고 했는데, 측정은 슬라이드가
아니라 **영역마다** 셉니다. 2단 슬라이드는 브리프를 그대로 지키고도 지적받을 수 있었습니다.
이제 "three to five in one region, never one and never more than six" 입니다.

**지적 문구** 자체도 영역을 재고선 "한 장에" 라고 말하고 있었습니다. 한국어·영어 모두
"한 영역에" 로 바로잡았습니다.

## 결과

같은 브리프를 실제 모델(vLLM)에 두 번 다시 태웠습니다.

| | 이전 | 이번 |
|---|---|---|
| `density` 지적 | **3건** | **0건** |
| 점수 | 94 | **98** |
| 영역별 요점 수 | 11 · 9 · 14 | 5 · 1 · 5 · 2 · 5 |

## 이번 판이 말하는 것

재는 것만으로는 고쳐지지 않습니다. **재는 쪽이 아는 숫자를 쓰는 쪽이 모르면**, 수리 패스는
돌고, 모델은 답하고, 결과는 버려집니다 — 왕복 한 번을 그냥 쓴 셈입니다. 측정과 지시가 같은
상수를 가리켜야 합니다.

## 설치

오프라인 설치 방법은 [docs/offline-deployment.md](offline-deployment.md) 를 보세요. 이미지에
Postgres 와 nginx 는 들어 있지 않습니다 — 배포할 때 `DATABASE_URL` 로 DSN 을 지정합니다.
