# Ptium v0.36.0

같은 브리프를 **한국어로 한 번, 영어로 한 번** 넣고 나온 덱을 나란히 놓고 봤습니다. 한국어는
제대로 된 보고서였고, 영어는 브리프를 흉내 낸 무언가였습니다.

## 영어 제목이 브리프의 꼬리를 잘라 쓰고 있었습니다

"A plan to make the payment platform redundant across two regions for the executive team" —
여기서 나온 슬라이드 제목이 **"two regions team"**이었습니다.

이유는 단순합니다. 한국어는 **핵심 명사가 뒤에** 옵니다. 그래서 문구가 길면 앞을 버리고
꼬리를 남깁니다("결제 시스템 이중화 계획" → "이중화 계획"). 영어는 정반대입니다 — 앞이
핵심입니다. 같은 규칙을 영어에 쓰면 남는 건 꼬리, 즉 부사구의 부스러기입니다.

로마자 문구는 이제 **앞을 남기고 뒤를 버립니다.** 관사로 시작하지 않고, 잘려 나간 절을
소개하던 전치사·분사로 끝나지도 않습니다.

- `A plan to make the payment platform redundant across two regions` → **Plan to make the payment platform redundant**
- `Onboarding guide for new engineers joining the payments team` → **Onboarding guide for new engineers**

그리고 영어 제목은 **대문자로 시작합니다.** 브리프 한가운데에서 들어 올린 문구가 소문자로
시작하면 슬라이드 제목이 아니라 메모로 읽힙니다.

## 영어로 적은 숫자를 숫자로 읽지 못했습니다

`목표 가용성 99.95%, 예산 4억`은 지표 슬라이드가 됐는데, `Target availability 99.95%,
budget 400M KRW`는 그러지 못했습니다. 단위 목록이 한국어뿐이라 **"budget 400M KRW"가
슬라이드 제목이 되어 있었습니다.**

이제 통화 코드(KRW·USD·EUR·JPY·GBP), 자릿수 접미(400M·1.2bn), 기간과 수량(months·weeks·
days·people·users·cases…)을 읽습니다. 브리프가 준 숫자는 제목이 아니라 **지표 카드**로
들어갑니다.

| 영어 브리프 | 들어가는 곳 |
| --- | --- |
| Target availability 99.95% | `::kpi` — Target availability \| 99.95% |
| budget 400M KRW | `::kpi` — Budget \| 400M KRW |
| for the executive team | 표지의 "Prepared for…" (제목에서는 빠짐) |

## 같은 슬라이드가 두 번 나왔습니다

주제가 둘이고 둘 다 "계획"으로 읽히면, 두 주제가 **같은 순서로 회전**해 같은 각도에 도착했습니다.
결과는 같은 리드·같은 지표·같은 노트를 단 슬라이드 두 장이었습니다.

이제 각도는 **덱 전체에서 한 번씩** 쓰입니다. 남은 각도가 없으면 그 주제의 슬라이드를 하나 더
찍는 대신, **이 덱이 다음에 받을 질문**으로 채웁니다. 그리고 여러 장으로 나뉜 주제의 제목은
각도만 남기고 주제를 버리지 않습니다 — 그것이 서로 다른 두 주제가 똑같이 "expected outcome"
이라는 제목을 달게 된 이유였습니다.

한국어 덱도 같이 좋아졌습니다. 같은 브리프에서 `기대 효과`·`3단계 이행`이 두 번씩 나오던 것이
`리전으로 이중화하는 계획 — 기대 효과`, `3단계 이행 — 선택지 비교`로 각각 한 번씩 나옵니다.

## 아직 남은 것

일본어·중국어는 띄어쓰기가 없어 단어 단위 규칙이 그대로 통하지 않습니다. 다음 판에서 다룹니다.

## 확인

- 회귀 테스트 4건: 로마자 문구의 머리 남기기, 영어 숫자 인식, **한 덱 안에 같은 제목·같은
  리드가 두 번 나오지 않는다**(영어·한국어 모두), 로마자 제목의 첫 글자.
- 전수 점검: api 75 · package 0 실패.

## 오프라인 설치

```bash
gzip -dc ptium-0.36.0.tar.gz | docker load
sha256sum -c ptium-0.36.0.tar.gz.sha256
```
