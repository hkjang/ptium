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
    [/^(\w+) overlaps (\w+) by (\d+)%$/,
      (m) => `${named(m[1])}${subjectParticle(named(m[1]))} ${named(m[2])} 영역과 ${m[3]}% 겹칩니다`],
    [/^text covers (\d+)% of the layout's own (.+)$/, (m) => `글이 템플릿 자체의 ${m[2]}를 ${m[1]}% 덮습니다`],
    [/^text (\w+) on (\w+) is ([\d.]+):1, below 4\.5:1$/,
      (m) => `글자색 #${m[1]}과 배경 #${m[2]}의 대비가 ${m[3]}:1로, 기준 4.5:1에 못 미칩니다`],
    [/^(\d+) points in one region; past (\d+) an audience reads instead of listening$/,
      (m) => `한 영역에 요점이 ${m[1]}개입니다. ${m[2]}개를 넘으면 듣지 않고 읽습니다`],
    [/^the region is (\d+)% full; a slide needs room to breathe$/,
      (m) => `영역이 ${m[1]}% 찼습니다. 슬라이드에는 여백이 필요합니다`],
    [/^the same point twice: "(.+)" and "(.+)"$/, (m) => `같은 말을 두 번 합니다: "${m[1]}"와 "${m[2]}"`],
    [/^no speaker notes: .+$/, () => '발표 노트가 없습니다. 이 슬라이드에서 무엇을 말할지 적혀 있지 않습니다'],
    [/^figures with no source: .+$/, () => '숫자가 있는데 출처가 없습니다. !source 로 어디서 온 숫자인지 적어 두면 발표자 노트에 함께 나갑니다'],
    [/^the last line holds (\d+)% of a line; .+$/,
      (m) => `마지막 줄에 한 줄의 ${m[1]}%만 남았습니다. 조금 줄이거나 고쳐 쓰면 사라집니다`],
    [/^(\w+) had too little room to draw anything$/,
      (m) => `${named(m[1])}${objectParticle(named(m[1]))} 그릴 자리가 없었습니다`],
    [/^(\w+) draws "(.+)" ([\d.]+)cm taller than the room it reserved$/,
      (m) => `${named(m[1])}의 "${m[2]}"${subjectParticle(m[2])} 확보한 자리보다 ${m[3]}cm 큽니다`],
    [/^two lines of the (\w+) overlap$/, (m) => `${named(m[1])}의 두 줄이 서로 겹칩니다`],
    [/^(\w+) draws ([\d.]+)cm past the slide edge$/,
      (m) => `${named(m[1])}${subjectParticle(named(m[1]))} 슬라이드 밖으로 ${m[2]}cm 나갔습니다`],
    [/^(\w+) draws ([\d.]+)cm outside its region$/,
      (m) => `${named(m[1])}${subjectParticle(named(m[1]))} 자기 영역 밖으로 ${m[2]}cm 나갔습니다`],
  ]
  for (const [pattern, write] of rules) {
    const match = detail.match(pattern)
    if (match) return write(match)
  }
  return detail
}

/**
 * What compiling the deck source adjusted, in the reader's words.
 *
 * The compiler writes these for whoever is debugging a template — 'layout
 * "마무리" has no free body region' — and the source editor puts them in front
 * of the person editing their own deck. Same reasoning as findingDetail: the
 * author did not choose that language, and each message is written again here.
 *
 * A message with a place in it keeps the place: "line 46 (slide 8)" is how the
 * author finds what the sentence is about.
 */
export function warningText(warning: string) {
  const [place, rest] = splitPlace(warning)
  const rules: [RegExp, (match: RegExpMatchArray) => string][] = [
    [/^the template exposes no usable layout$/, () => '이 템플릿에는 쓸 수 있는 레이아웃이 없습니다'],
    [/^layout "(.+)" cannot hold this slide; used "(.+)" instead$/,
      (m) => `"${m[1]}" 레이아웃에는 이 슬라이드가 들어가지 않아 "${m[2]}"를 썼습니다`],
    [/^layout "(.+)" does not exist in this template; used "(.+)" instead$/,
      (m) => `이 템플릿에 "${m[1]}" 레이아웃이 없어 "${m[2]}"를 썼습니다`],
    [/^this template has no (\w+) layout; used "(.+)", which has room for the points$/,
      (m) => `이 템플릿에는 ${roleName(m[1])} 레이아웃이 없어, 요점이 들어갈 자리가 있는 "${m[2]}"를 썼습니다`],
    [/^the "(.+)" layout has no room for this slide's points; used "(.+)" instead$/,
      (m) => `"${m[1]}" 레이아웃에는 요점이 들어갈 자리가 없어 "${m[2]}"를 썼습니다`],
    [/^layout "(.+)" has no free body region, so its points were kept as plain text$/,
      (m) => `"${m[1]}" 레이아웃에는 본문 영역이 없어 요점을 제목 아래 줄로 적었습니다`],
    [/^(\w+) has no free region in layout "(.+)" and was written as text$/,
      (m) => `"${m[2]}" 레이아웃에 ${named(m[1])}${objectParticle(named(m[1]))} 그릴 자리가 없어 글로 적었습니다`],
    [/^(\w+) did not have enough room and was written as text$/,
      (m) => `${named(m[1])}${objectParticle(named(m[1]))} 그릴 자리가 모자라 글로 적었습니다`],
    [/^(\w+) had no numeric values and was drawn as (\w+)$/,
      (m) => `${named(m[1])}에 숫자가 없어 ${named(m[2])}${toParticle(named(m[2]))} 그렸습니다`],
    [/^layout "(.+)" has no region for an image$/,
      (m) => `"${m[1]}" 레이아웃에는 이미지가 들어갈 자리가 없습니다`],
    [/^no uploaded image is named "(.+)"$/, (m) => `"${m[1]}"이라는 이름의 이미지가 없습니다`],
    [/^images cannot be resolved here, so "(.+)" was skipped$/,
      (m) => `여기서는 이미지를 찾을 수 없어 "${m[1]}"을 건너뛰었습니다`],
    [/^no grid is defined as "(.+)"$/, (m) => `"${m[1]}"이라는 격자 정의가 없습니다`],
    [/^a slide may cite at most (\d+) sources; the rest were dropped$/,
      (m) => `한 슬라이드에는 출처를 ${m[1]}개까지 달 수 있어 나머지는 빠졌습니다`],
    [/^::image needs the name or id of an uploaded image$/, () => '::image 에는 올려 둔 이미지의 이름이나 id가 필요합니다'],
    [/^unknown component "(.+)"$/, (m) => `"${m[1]}"은 없는 컴포넌트입니다`],
    [/^unknown slide kind "(.+)"$/, (m) => `"${m[1]}"은 없는 슬라이드 종류입니다`],
    [/^unknown directive "(.+)"$/, (m) => `"${m[1]}"은 없는 지시어입니다`],
    [/^@layout needs a layout id$/, () => '@layout 에는 레이아웃 id가 필요합니다'],
    [/^!source needs a title: (.+)$/, (m) => `!source 에는 출처 이름이 필요합니다: ${m[1]}`],
  ]
  for (const [pattern, write] of rules) {
    const match = rest.match(pattern)
    if (match) return place + write(match)
  }
  return warning
}

/** "line 46 (slide 8): …" — the place stays as written; only the sentence changes. */
function splitPlace(warning: string): [string, string] {
  const match = warning.match(/^((?:line \d+|slide \d+)[^:]*): (.+)$/)
  if (!match) return ['', warning]
  return [`${match[1]}: `, match[2]]
}

/**
 * Korean particles agree with the sound before them: 표를 but 차트를, 표로 but
 * 차트으로 — get it wrong and the sentence reads as machine output, which is
 * exactly what these sentences are trying not to be.
 *
 * The rule is the final consonant of the last syllable. Anything that is not a
 * Hangul syllable — a number, a Latin word — is treated as ending open, which
 * is what Korean does when reading them aloud in these positions.
 */
function batchim(word: string): number | null {
  const last = word.trim().slice(-1)
  if (!last) return null
  const code = last.charCodeAt(0)
  if (code < 0xac00 || code > 0xd7a3) return null
  return (code - 0xac00) % 28
}

/** 이 / 가 */
export function subjectParticle(word: string) {
  const final = batchim(word)
  return final === null || final === 0 ? '가' : '이'
}

/** 을 / 를 */
export function objectParticle(word: string) {
  const final = batchim(word)
  return final === null || final === 0 ? '를' : '을'
}

/** 으로 / 로 — ㄹ takes 로, like 서울로. */
export function toParticle(word: string) {
  const final = batchim(word)
  return final === null || final === 0 || final === 8 ? '로' : '으로'
}

const roleNames: Record<string, string> = {
  title: '표지', section: '구역', closing: '마무리', content: '본문',
  twoContent: '2단', comparison: '비교', quote: '인용', picture: '이미지', blank: '빈',
}
const roleName = (value: string) => roleNames[value] || value

export function revisionReason(reason: string) {
  switch (reason) {
    case 'edit': return '자동 편집 체크포인트'
    case 'source': return '코드 적용 전'
    case 'generation': return '재생성 전'
    case 'restore': return '버전 복원 전'
  }
  return reason
}
