# Ptium v1.27.1

어제 판(1.27.0)이 **적용되지 않는 설정값을 거절**하게 했습니다. 그 판을 올린 배포에서
바로 확인해 보니, **거절만으로는 부족했습니다.**

## 이미 저장되어 있던 값이 화면을 막습니다

1.27.0 이전에는 API가 무엇이든 받았습니다. 그래서 **업그레이드하는 배포가 적용되지 않는
값을 이미 갖고 있을 수 있습니다.** 그 상태에서 관리자가 그 영역의 다른 항목 하나를
바꾸고 저장하면 —

```
저장되어 있던 값:  generation.repair_passes = 500   ai.timeout_seconds = 99999
"생성 정책" 저장 → 422 generation.repair_passes must be a whole number between 0 and 10
"AI 모델" 저장   → 422 ai.timeout_seconds must be a whole number between 10 and 3600
```

**건드리지도 않은 항목 때문에 아무것도 저장할 수 없습니다.** 어제 고친 것이 만든
자리입니다.

## 올라올 때 되돌립니다

이제 서비스가 시작하면서 **적용될 수 없는 값을 기본값으로 되돌립니다.** 무엇이 있었는지
로그에 남깁니다.

```
WARN a stored setting could not be honoured and was put back
     key=ai.timeout_seconds stored=99999 restored=300
WARN a stored setting could not be honoured and was put back
     key=generation.repair_passes stored=500 restored=3
INFO settings reset to a value this deployment honours count=2
```

**관리자가 실제로 고른 값은 건드리지 않습니다.** 적용되는 범위 안에 있으면 그대로입니다 —
최대 출력 토큰 16000, 기본 청중 "경영진과 의사결정자"처럼요. 되돌리는 것은 **적용될 수
없는 값뿐**입니다.

이제 저장된 값과 실제로 도는 값이 **모든 배포에서 같습니다.**

## 검사

데이터베이스를 붙여 실제로 확인합니다 — 적용될 수 없는 값 네 개는 되돌아가고, 관리자가
고른 값 두 개는 그대로 남습니다. 되돌리기를 꺼 보면 네 개 모두 그 자리에서 잡힙니다.

전체 Go 테스트(DB 포함) · REST 573개 · deep · 화면 훑기 0 failures.

## 설치

```bash
gzip -dc ptium-1.27.1.tar.gz | docker load
```
