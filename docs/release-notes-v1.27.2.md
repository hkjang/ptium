# Ptium v1.27.2

1.27.1이 **업그레이드할 때 적용될 수 없는 설정값을 되돌리도록** 했습니다.
이번 판은 그것이 **실제 이미지 사이에서도 그런지**를 매 릴리스마다 확인합니다.

## 실제 업그레이드로 확인했습니다

릴리스한 1.27.1 이미지를, **거절이 없던 시절(1.26.2)의 데이터베이스**에 붙여 봤습니다.

```
업그레이드 전:  ai.timeout_seconds=99999  repair_passes=500  reasoning="thinking-hard"
                ai.max_output_tokens=16000   ← 관리자가 고른 값

1.27.1 첫 기동:
  WARN a stored setting could not be honoured and was put back key=ai.reasoning …
  WARN … key=generation.repair_passes stored=500 restored=3
  WARN … key=ai.timeout_seconds stored=99999 restored=300
  INFO settings reset to a value this deployment honours count=3

업그레이드 후:  300 · 3 · "auto" · 16000  ← 고른 값은 그대로
```

## 매 릴리스마다 확인합니다

이 확인을 **업그레이드 훑기**(`scripts/e2e/upgrade.py`)에 넣었습니다. 이 훑기는
오프라인 패키지를 만들 때마다 돌아가며, **예전 이미지가 쓴 데이터베이스를 새 이미지가
여는** 진짜 업그레이드입니다. 이제 그 안에서 —

- 적용될 수 없는 값 네 개가 되돌아왔는지,
- 관리자가 고른 값 하나가 **그대로 남았는지**

를 함께 봅니다.

되돌리기가 없던 판(1.26.2)으로 업그레이드해 보면 네 개 모두 그 자리에서 잡힙니다.

```
✗ ai.timeout_seconds came through the upgrade as 99999, a value this deployment does not honour
✗ generation.repair_passes came through the upgrade as 500, …
✗ ai.reasoning came through the upgrade as "thinking-hard", …
✗ generation.outline_pass came through the upgrade as "yes", …
```

## 검사

업그레이드 훑기(1.26.2 → 1.27.2) 0 failures — 덱은 그대로 열리고 내보내지며,
1.26.2로 되돌려도 그대로입니다.

## 설치

```bash
gzip -dc ptium-1.27.2.tar.gz | docker load
```
