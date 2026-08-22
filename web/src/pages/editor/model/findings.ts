/**
 * What a measurement says, in the reader's words.
 *
 * The measurement pass writes its findings in English, for whoever is
 * debugging a template: "text 1f2937 on ffffff is 3.1:1, below 4.5:1". The
 * person who asked for a deck did not choose that language and should not have
 * to read it, so every finding is written again here — and because these are
 * pure rules over strings, they are the part of the editor that can be tested
 * without a browser at all.
 */

export function scoreDimensionLabel(key: string) {
  switch (key) {
    case 'readability': return '가독성'
    case 'structure': return '구성'
    case 'visual': return '시각'
    case 'accessibility': return '접근성'
    case 'evidence': return '근거'
  }
  return key
}

export function findingLabel(kind: string) {
  switch (kind) {
    case 'overflow': return '텍스트 넘침'
    case 'outside': return '슬라이드 밖으로 나감'
    case 'collision': return '겹침'
    case 'contrast': return '대비 부족'
    case 'orphan': return '줄 끝에 한 음절만 남음'
    case 'density': return '한 장에 너무 많음'
    case 'notes': return '발표 노트 없음'
    case 'repeat': return '같은 말을 두 번 함'
    case 'source': return '숫자의 출처 없음'
  }
  return kind
}

/**
 * The measurement, in the workspace's language.
 *
 * The API states findings in English, because that is what an API and a log
 * should say. A person reading their own deck should not have to. Anything this
 * does not recognise is shown as the server wrote it, so a new measurement is
 * never swallowed.
 */
const componentNames: Record<string, string> = {
  kpi: '핵심 지표', hero: '대표 숫자', steps: '단계', timeline: '타임라인', comparison: '비교',
  columnChart: '세로 막대 차트', barChart: '가로 막대 차트', lineChart: '추이 차트', shareBar: '비중 바',
  meter: '달성률', table: '표', quote: '인용', callout: '강조', grid: '격자', bullets: '목록',
  text: '텍스트', component: '컴포넌트', picture: '이미지',
  // The regions a template names. A measurement says "body overlaps title", and
  // a sentence that translates one half and not the other reads worse than one
  // that translates neither.
  title: '제목', subtitle: '부제', body: '본문', body2: '본문 2', body3: '본문 3', body4: '본문 4',
  chart: '차트', graphic: '그래픽', header: '머리글', footer: '바닥글',
}
const named = (value: string) => componentNames[value] || value

export function findingDetail(detail: string) {
  const rules: [RegExp, (match: RegExpMatchArray) => string][] = [
    [/^(\w+) region extends ([\d.]+)cm past the slide edge$/,
      (m) => `${named(m[1])} 영역이 슬라이드 밖으로 ${m[2]}cm 나갔습니다`],
    [/^(\d+) lines of text in room for (\d+); it must shrink to (\d+)% of the template's size$/,
      (m) => `${m[2]}줄 자리에 ${m[1]}줄이 들어가 템플릿 크기의 ${m[3]}%로 줄여야 합니다`],
    [/^(\d+) lines of text in room for (\d+); it does not fit even at (\d+)%$/,
      (m) => `${m[2]}줄 자리에 ${m[1]}줄이라 ${m[3]}%로 줄여도 들어가지 않습니다`],
    [/^(\w+) overlaps (\w+) by (\d+)%$/, (m) => `${named(m[1])}이 ${named(m[2])} 영역과 ${m[3]}% 겹칩니다`],
    [/^text covers (\d+)% of the layout's own (.+)$/, (m) => `글이 템플릿 자체의 ${m[2]}를 ${m[1]}% 덮습니다`],
    [/^text (\w+) on (\w+) is ([\d.]+):1, below 4\.5:1$/,
      (m) => `글자색 #${m[1]}과 배경 #${m[2]}의 대비가 ${m[3]}:1로, 기준 4.5:1에 못 미칩니다`],
    [/^(\d+) points on one slide; past (\d+) an audience reads instead of listening$/,
      (m) => `한 장에 요점이 ${m[1]}개입니다. ${m[2]}개를 넘으면 듣지 않고 읽습니다`],
    [/^the region is (\d+)% full; a slide needs room to breathe$/,
      (m) => `영역이 ${m[1]}% 찼습니다. 슬라이드에는 여백이 필요합니다`],
    [/^the same point twice: "(.+)" and "(.+)"$/, (m) => `같은 말을 두 번 합니다: "${m[1]}"와 "${m[2]}"`],
    [/^no speaker notes: .+$/, () => '발표 노트가 없습니다. 이 슬라이드에서 무엇을 말할지 적혀 있지 않습니다'],
    [/^the last line holds (\d+)% of a line; .+$/,
      (m) => `마지막 줄에 한 줄의 ${m[1]}%만 남았습니다. 조금 줄이거나 고쳐 쓰면 사라집니다`],
    [/^(\w+) had too little room to draw anything$/, (m) => `${named(m[1])}을 그릴 자리가 없었습니다`],
    [/^(\w+) draws "(.+)" ([\d.]+)cm taller than the room it reserved$/,
      (m) => `${named(m[1])}의 "${m[2]}"가 확보한 자리보다 ${m[3]}cm 큽니다`],
    [/^two lines of the (\w+) overlap$/, (m) => `${named(m[1])}의 두 줄이 서로 겹칩니다`],
    [/^(\w+) draws ([\d.]+)cm past the slide edge$/, (m) => `${named(m[1])}이 슬라이드 밖으로 ${m[2]}cm 나갔습니다`],
    [/^(\w+) draws ([\d.]+)cm outside its region$/, (m) => `${named(m[1])}이 자기 영역 밖으로 ${m[2]}cm 나갔습니다`],
  ]
  for (const [pattern, write] of rules) {
    const match = detail.match(pattern)
    if (match) return write(match)
  }
  return detail
}

export function revisionReason(reason: string) {
  switch (reason) {
    case 'edit': return '자동 편집 체크포인트'
    case 'source': return '코드 적용 전'
    case 'generation': return '재생성 전'
    case 'restore': return '버전 복원 전'
  }
  return reason
}
