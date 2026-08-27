# Ptium v1.52.1

**영어 줄글 브리프의 표지도 문장 중간에서 끊깁니다.**

## 다른 언어에서도 같은 것을 쟀습니다

1.52.0은 한국어 줄글 브리프의 표지를 고쳤습니다. 같은 종류의 브리프를 영어·일본어·
중국어로도 넣어 봤습니다.

```
[en] "Onboarding for new engineers is taking far too"     ← 끊김
[en] "We are trying to set up a data governance"          ← 끊김
[ja] "来年はクラウド費用を大きく削減する必要があります"                      ← 문장 전체(길지만 끊기지 않음)
[zh] "明年我们需要大幅削减云成本, 按现在的结构会继续增长"                  ← 문장 전체
```

영어 두 개가 끊겼고 **둘 다 지적되지 않았습니다.**

## 뒤에 올 말을 꾸미다 만 낱말

끊긴 제목을 찾는 규칙에는 이미 "뒤에 올 말을 소개하는 것이 전부인 낱말" 목록이
있습니다(`of`·`for`·`the`·`and`…). **정도를 나타내는 낱말**도 같은 부류입니다 —
`far too` 로 끝나는 제목은 뒤에 올 말이 잘려 나간 것입니다.

```
전:  'Onboarding for new engineers is taking far too'  →  99점 · 지적 없음
후:  같은 제목                                        →  97점 · unfinished
```

`No regrets moves`·`Risks and mitigations`·`Q & A`·`Reducing cloud spend` 같은
멀쩡한 제목은 그대로입니다.

## 못 잡는 것과, 제가 틀렸던 것

`We are trying to set up a data governance` 는 여전히 못 잡습니다. `governance` 는
뒤에 올 명사를 꾸미다 만 낱말인데, 그것을 알아보려면 문법을 알아야 합니다.

그리고 **처음에 "제목이 낱말 중간에서 잘린다"고 봤던 것은 제 측정 스크립트가 화면에
맞추려고 44자에서 자른 것**이었습니다. 제품은 낱말 경계에서 자르고 있었습니다.
