# Ptium v0.77.0

12억 원 투자를 이사회에 요청하는 덱에서 모델이 이렇게 썼습니다.

```
# 투자 제안 개요
> 결제 이중화 구축을 위한 12억 원 예산 요청
- 장애 시 자동 전환으로 가용성 99.99% 확보 목표
```

브리프는 **99.99% 를 말한 적이 없습니다.** 12억 원도, 28.5% 도, 4시간 17분도 브리프에서
왔는데 이 숫자만 모델이 만들었습니다. 그리고 이건 회의실에서 **가장 먼저 질문받는 줄** 입니다.

## 규칙이 엉뚱한 곳에 있었습니다

"숫자를 지어내지 말 것" 은 이미 있었습니다. 다만 **`Rules for components:` 아래** 있었습니다.

```
Rules for components:
- Never invent a number. Use kpi, hero, meter, share or a chart ONLY when
  the brief supplies the figures ...
```

kpi 카드와 차트만 지키면 되는 규칙이었던 셈입니다. 산문은 한 번도 적용 대상이 아니었고,
정작 근거 없는 숫자가 약속처럼 읽히는 곳은 산문입니다. 규칙을 **작성 원칙(Writing craft)**
으로 옮기고 범위를 분명히 했습니다.

```
- Never invent a figure. Every number on a slide — in a bullet, in a lead
  line, in the notes — comes from the brief or follows arithmetically from
  it. A target nobody set, a percentage nobody measured, a saving nobody
  counted: write the point without the number instead.
```

## 지시만으로는 보장이 안 됩니다 — 그래서 잽니다

지어낸 **출처** 는 지웠습니다(v0.75). 지어낸 **숫자** 는 지울 수 없습니다. 문장 안에 있어서
빼면 문장이 다른 말을 하게 됩니다. 그래서 덱은 숫자를 **그대로 두고, 무엇을 들여왔는지
말합니다.**

> 브리프에 없는 숫자가 들어 있습니다: 99.99%. 근거를 댈 수 없는 숫자는 고쳐 주세요.

- 날짜와 기간은 묻지 않습니다 — "2026년 상반기", "첫 2주" 는 주장이 아닙니다.
- 브리프의 숫자는 어떻게 적히든 같은 숫자로 봅니다 — "1,200" 과 "1200".
- 다시 쓰기(repair) 뒤에 잽니다. 고쳐 쓴 슬라이드도 숫자를 말하기 때문입니다.

## 저장된 덱 48개로 검증했습니다

브리프만으로 모델이 쓴 덱에서 걸린 것은 **모두 실제 날조** 였습니다.

| 브리프 | 걸린 숫자 |
|---|---|
| 목표 가용성 99.95%, 예산 4억 | `1,200건` · `0.2%` |
| 신규 결제 수단 도입 검토 (숫자 없음) | `15%` · `10%` · `3 개` |
| 담당 체계와 준비 상태 공유 | `60%` · `80%` · `50%` · `100%` · `20%` |

아무도 세지 않은 진척률, 아무도 잰 적 없는 실패율입니다. 자료가 첨부된 덱은 자료의 숫자를
브리프로 함께 봅니다.

규칙을 옮긴 뒤 같은 브리프를 실제 모델에 다시 태웠습니다. **지어낸 숫자 0건**, 점수 98.

## 덤으로

`pptx.Deck` 의 json 태그가 어긋나 있었습니다 — `Source` 가 `"language"` 로 나가고 `Language`
에는 태그가 없었습니다. 지금 이 구조체를 직렬화하는 곳은 없지만, 처음 직렬화하는 쪽이 밟을
함정이라 고쳤습니다.

## 설치

오프라인 설치 방법은 [docs/offline-deployment.md](offline-deployment.md) 를 보세요. 이미지에
Postgres 와 nginx 는 들어 있지 않습니다 — 배포할 때 `DATABASE_URL` 로 DSN 을 지정합니다.
