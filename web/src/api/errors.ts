/**
 * What the server refused, in the reader's words.
 *
 * The API answers in English — "The presentation changed in another session" —
 * because that message is also read by whoever is holding a request log or
 * writing against the API. The person in the workspace did not choose that
 * language, and the message lands in front of them as a toast.
 *
 * Same treatment as the measurements and the compiler's warnings: written again
 * here, keyed on the message the server sent, with the error code as the
 * fallback for anything unlisted. A message nobody has written a rule for is
 * shown as it came, because a half-translated sentence reads worse than an
 * untranslated one.
 */

const byMessage: Record<string, string> = {
  // A shared link, read by somebody outside the workspace. This is often the
  // only part of the product they ever see.
  'This link is no longer open. Ask whoever sent it for a new one':
    '이 링크는 더 이상 열리지 않습니다. 보내 준 사람에게 새 링크를 요청해 주세요.',
  'No deck is shared at this link': '이 링크로 공유된 덱이 없습니다.',
  'This deck has no slides yet': '이 덱에는 아직 슬라이드가 없습니다.',
  'that slide is not part of this deck': '그 슬라이드는 이 덱의 것이 아닙니다.',
  'this deck has as many comments as it will hold': '이 덱에는 더 이상 의견을 남길 수 없습니다.',
  'a share needs a short label and a sensible number of days':
    '링크 이름은 짧게, 기간은 일 단위로 3650일까지 지정할 수 있습니다.',
  // Refusals the workspace can walk into and had no rule for: an upload that is
  // too large, a command sent to a deck with no slides, an export of a deck
  // whose every slide is skipped.
  'Generate or add slides before commanding the deck':
    '먼저 슬라이드를 만들거나 추가한 뒤에 덱에 지시할 수 있습니다.',
  'Generate or add slides before inspecting':
    '먼저 슬라이드를 만들거나 추가한 뒤에 점검할 수 있습니다.',
  'The deck source must be UTF-8 text within the size limit':
    '덱 소스는 UTF-8 글자여야 하고, 크기 제한을 넘지 않아야 합니다.',
  'instruction or slot exceeds its allowed length': '지시문이나 자리 이름이 너무 깁니다.',
  'Only pptx and pdf export are supported': 'pptx 와 pdf 로만 내보낼 수 있습니다.',
  'Every slide is marked skipped, so the PDF would have no pages':
    '모든 슬라이드가 건너뛰기로 표시되어 있어 PDF 에 넣을 쪽이 없습니다.',
  'That change is not in the trail': '그 변경은 기록에 없습니다.',
  'state must be open, expired or revoked': '상태는 열림 · 만료 · 회수 중 하나여야 합니다.',
  'kind must be builtin or uploaded': '종류는 내장 또는 올린 것 중 하나여야 합니다.',
  'shared must be true or false': '공유 여부는 참 또는 거짓이어야 합니다.',
  'Send the image as multipart/form-data with a file field':
    '이미지는 multipart/form-data 의 file 항목으로 보내 주세요.',
  "Send either the slide's source, or the deck and the slide to save":
    '슬라이드 소스를 보내거나, 저장할 덱과 슬라이드를 함께 보내 주세요.',
  // Messages a person meets in the workspace that had no rule written for
  // them, so they arrived in English on a screen that is Korean throughout.
  // The commonest by far is the first: on a site with no model configured,
  // every attempt to write a deck ends here.
  'This deployment has no AI provider configured':
    '이 배포에는 AI 모델이 설정되어 있지 않습니다. 관리자에게 서비스 설정을 확인해 달라고 요청하세요.',
  'The revision could not be bound to this template':
    '고쳐 쓴 내용을 이 템플릿에 맞출 수 없었습니다. 슬라이드를 직접 고치거나 다른 표현을 지시해 주세요.',
  'This file has no slides Ptium could read':
    '이 파일에서 읽을 수 있는 슬라이드를 찾지 못했습니다.',
  'The file has more slides than this deployment allows':
    '이 파일은 이 배포가 허용하는 장수보다 깁니다.',
  'The deck source is larger than this deployment accepts':
    '덱 코드가 이 배포에서 허용하는 크기를 넘습니다.',
  'Ptium reads .pptx presentations and .xlsx, .csv, .docx, .pdf and .md documents':
    'pptx 발표 파일과 xlsx · csv · docx · pdf · md 문서를 읽을 수 있습니다.',
  "This image's file is missing from the image storage volume. Upload it again.":
    '이 이미지의 원본 파일이 저장소에 없습니다. 다시 올려 주세요.',
  // Signing in.
  'Too many sign-in attempts. Try again shortly.':
    '로그인 시도가 너무 잦습니다. 잠시 후 다시 시도해 주세요.',
  'This deployment does not accept password sign-in':
    '이 배포는 비밀번호 로그인을 받지 않습니다.',
  'This account signs in through the identity provider and has no password':
    '이 계정은 외부 인증으로 로그인하므로 비밀번호가 없습니다.',
  'An API key cannot open a browser session':
    'API 키로는 브라우저 세션을 열 수 없습니다.',
  // Administration.
  'This email address is already registered to another sign-in identity. Ask an administrator to remove the old account or sign in with the original identity.':
    '이 이메일은 다른 로그인 정보에 이미 등록되어 있습니다. 관리자에게 예전 계정 삭제를 요청하거나, 원래 쓰던 로그인으로 들어와 주세요.',
  'That deck finished before it could be stopped': '그 덱은 멈추기 전에 이미 끝났습니다.',
  'That deck is not waiting or being written': '그 덱은 대기 중도 작성 중도 아닙니다.',
  // Sessions and permission.
  'Authentication is required': '로그인이 필요합니다.',
  'Administrator access is required': '관리자만 할 수 있는 작업입니다.',
  'This account has been disabled': '이 계정은 사용이 중지되었습니다. 관리자에게 문의하세요.',
  'This account is disabled': '이 계정은 사용이 중지되었습니다. 관리자에게 문의하세요.',
  'The API key does not grant this operation': 'API 키에 이 작업 권한이 없습니다.',
  'The username or password is incorrect': '아이디 또는 비밀번호가 맞지 않습니다.',
  'The current password is incorrect': '현재 비밀번호가 맞지 않습니다.',
  'Administrators cannot disable or demote their own active account':
    '관리자는 자기 계정을 정지하거나 권한을 낮출 수 없습니다.',
  'Remove the administrator role or bootstrap selector at the identity source before demoting this account':
    '이 계정은 외부 인증(OIDC)에서 관리자로 지정되어 있습니다. 그쪽에서 역할을 먼저 내려야 합니다.',
  'This deployment does not issue session cookies': '이 배포는 세션 쿠키를 발급하지 않습니다.',

  // The deck and its slides.
  // Both places that raise this are the editor's own saves, so the person
  // reading it has just typed something. Telling them to refresh without
  // saying what that costs sent them to throw their own work away: the edit is
  // still on the screen after the refusal, and gone after the reload.
  'The presentation changed in another session':
    '다른 곳에서 이 덱이 먼저 바뀌었습니다. 여기서 고친 내용은 아직 화면에 있지만 새로고침하면 사라집니다. ' +
    '필요한 부분을 복사해 두고 새로고침한 뒤 다시 넣어 주세요.',
  'The presentation does not exist': '이 프레젠테이션을 찾을 수 없습니다.',
  'The requested resource was not found': '요청한 항목을 찾을 수 없습니다.',
  'The requested layout does not exist': '요청한 레이아웃이 이 템플릿에 없습니다.',
  'Generate or add slides before exporting': '내보내려면 먼저 슬라이드를 만들어 주세요.',
  'Generate or add slides before previewing': '미리 보려면 먼저 슬라이드를 만들어 주세요.',
  'The deck source produced no slides': '이 코드에서는 슬라이드가 하나도 만들어지지 않았습니다.',
  'The deck source must be UTF-8 text': '덱 코드는 UTF-8 텍스트여야 합니다.',
  'A saved slide must be UTF-8 text': '저장한 슬라이드는 UTF-8 텍스트여야 합니다.',
  'The saved slide produced nothing': '이 저장 슬라이드에서는 아무것도 만들어지지 않았습니다.',
  'slides must contain between 1 and 50 items': '슬라이드는 1장 이상 50장 이하여야 합니다.',
  'slide IDs must be unique': '슬라이드 id가 중복되었습니다.',
  'slides can only be supplied when updating a presentation': '슬라이드는 프레젠테이션을 수정할 때만 보낼 수 있습니다.',
  'Only pptx export is currently supported': '지금은 pptx 내보내기만 지원합니다.',

  // Templates and uploads.
  'This file is not an image Ptium can place: PNG, JPEG, GIF and SVG':
    '이미지 파일이 아닙니다. PNG · JPEG · GIF · SVG만 넣을 수 있습니다 — 확장자만 바뀐 파일일 수 있습니다.',
  'Built-in templates cannot be modified': '기본 제공 템플릿은 수정할 수 없습니다.',
  'Template uploads are disabled by the administrator': '관리자가 템플릿 업로드를 막아 두었습니다.',
  'The uploaded template is empty': '업로드한 템플릿이 비어 있습니다.',
  'The upload could not be read': '업로드한 파일을 읽지 못했습니다.',
  "A PowerPoint file must be supplied in the 'file' field": 'PowerPoint 파일을 file 항목으로 보내 주세요.',
  'a file field is required': '파일을 선택해 주세요.',
  'the file is empty': '올린 파일이 비어 있습니다.',
  'an image needs a name': '이미지에 이름이 필요합니다.',
  'that name is too long for an image': '이미지 이름이 너무 깁니다.',
  'another image already has that name': '같은 이름의 이미지가 이미 있습니다.',
  'another saved slide already has that name': '같은 이름의 저장 슬라이드가 이미 있습니다.',
  'a comment needs something to say': '의견을 입력해 주세요.',
  'the selected template does not exist': '고른 템플릿이 없습니다. 목록에서 다시 골라 주세요.',
  'a slide that does not exist is not an edit': '없는 슬라이드는 고칠 수 없습니다. 화면을 새로 고쳐 주세요.',
  'The requested slide does not exist': '그 슬라이드가 없습니다. 화면을 새로 고쳐 주세요.',
  'This API key does not exist, or it has been revoked': '이 API 키가 없거나 이미 폐기되었습니다.',
  'Template name is required and must not exceed 120 characters': '템플릿 이름은 1자 이상 120자 이하여야 합니다.',
  'Template name or description is too long': '템플릿 이름이나 설명이 너무 깁니다.',
  'scope must be private or shared': '공개 범위는 private 또는 shared여야 합니다.',

  // Requests that could not be read.
  'Request body must contain one JSON value': '요청 본문은 JSON 하나여야 합니다.',
  'Request body is not valid for this operation': '이 작업에 맞지 않는 요청 본문입니다.',
  'The token request could not be read': '토큰 요청을 읽지 못했습니다.',
  'Path id must be a UUID': '주소의 id 형식이 올바르지 않습니다.',
  'revisionId must be a UUID': '버전 id 형식이 올바르지 않습니다.',
  'preferences must be valid JSON': '설정 값은 올바른 JSON이어야 합니다.',
  'setting value must be valid JSON': '설정 값은 올바른 JSON이어야 합니다.',
  'setting key must use dotted lowercase names': '설정 키는 소문자와 점으로 씁니다.',
  'gracePeriod must be a duration such as 24h': '유예 기간은 24h 처럼 적어 주세요.',
  'username and password are required': '아이디와 비밀번호를 입력해 주세요.',
  'Profile fields exceed their allowed length': '입력한 값이 너무 깁니다.',
  'invalid incident status': '올바르지 않은 오류 상태입니다.',
  'invalid incident status or notes': '올바르지 않은 오류 상태 또는 메모입니다.',

  // The service itself.
  'The server could not complete the request': '서버가 요청을 처리하지 못했습니다. 관리자에게 오류 센터를 확인해 달라고 요청하세요.',
  'Database is not ready': '데이터베이스가 준비되지 않았습니다. 잠시 후 다시 시도해 주세요.',
  Unavailable: '서비스를 일시적으로 사용할 수 없습니다. 잠시 후 다시 시도해 주세요.',
}

const byCode: Record<string, string> = {
  authentication_required: '로그인이 필요합니다.',
  admin_required: '관리자만 할 수 있는 작업입니다.',
  account_disabled: '이 계정은 사용이 중지되었습니다. 관리자에게 문의하세요.',
  insufficient_scope: 'API 키에 이 작업 권한이 없습니다.',
  not_found: '요청한 항목을 찾을 수 없습니다.',
  version_conflict: '다른 곳에서 먼저 바뀌었습니다. 고친 내용을 복사해 두고 새로고침한 뒤 다시 시도해 주세요.',
  invalid_json: '요청 본문을 읽지 못했습니다.',
  invalid_id: '주소의 id 형식이 올바르지 않습니다.',
  invalid_upload: '업로드한 파일을 읽지 못했습니다.',
  // The server names the limit in its message and carries the number in
  // details; a person who has just waited for a 60 MB upload deserves to be
  // told what the limit is rather than that reading stopped.
  template_too_large: '파일이 이 배포에서 허용하는 크기를 넘습니다.',
  templates_busy: '지금 다른 템플릿을 읽고 있습니다. 잠시 후 다시 시도해 주세요.',
  printing_busy: '지금 다른 문서를 그리고 있습니다. 잠시 후 다시 시도해 주세요.',
  invalid_source: '보낸 텍스트가 UTF-8이 아닙니다.',
  validation_error: '입력한 값이 올바르지 않습니다.',
  database_unavailable: '데이터베이스가 준비되지 않았습니다. 잠시 후 다시 시도해 주세요.',
  temporarily_unavailable: '서비스를 일시적으로 사용할 수 없습니다. 잠시 후 다시 시도해 주세요.',
  rate_limited: '요청이 너무 잦습니다. 잠시 후 다시 시도해 주세요.',
  presentation_has_no_slides: '슬라이드가 없습니다. 먼저 생성하거나 한 장 추가한 뒤 내보내세요.',
  presentation_has_no_printable_slides: '모든 슬라이드가 건너뛰기로 표시되어 인쇄할 장이 없습니다. 한 장 이상 발표에 넣어 주세요.',
  unsupported_export_format: '이 형식으로는 내보낼 수 없습니다.',
  unsupported_image: '이미지 파일이 아닙니다. PNG · JPEG · GIF · SVG만 넣을 수 있습니다.',
  asset_too_large: '이미지가 너무 큽니다. 더 작은 파일로 올려 주세요.',
}

/**
 * The messages that carry a field name or a limit in them.
 *
 * These arrived as "입력한 값이 올바르지 않습니다" — the code's fallback — because
 * no exact string can match "title is required and must not exceed 200
 * characters". The server said which field and what the limit is; saying only
 * that something is wrong throws that away and leaves the person guessing which
 * box to look at.
 */
const rules: [RegExp, (match: RegExpMatchArray) => string][] = [
  // The limit is a deployment's own setting, so it arrives in the sentence.
  [/^An image must be (\d+) MiB or smaller$/, (m) => `이미지는 ${m[1]}MiB 이하여야 합니다.`],
  [/^The template must not exceed (\d+) MiB$/, (m) => `템플릿은 ${m[1]}MiB 를 넘을 수 없습니다.`],
  [/^The upload must not exceed (\d+) MiB$/, (m) => `올리는 파일은 ${m[1]}MiB 를 넘을 수 없습니다.`],
  [/^(?:slide )?(\w+) is required and must not exceed (\d+) characters$/,
    (m) => `${topic(fieldName(m[1]))} 비워 둘 수 없고 ${m[2]}자를 넘을 수 없습니다.`],
  [/^(\w+) must not exceed (\d+) characters$/,
    (m) => `${topic(fieldName(m[1]))} ${m[2]}자를 넘을 수 없습니다.`],
  [/^(\w+) must be between (\d+) and (\d+)(?: before generation)?$/,
    (m) => `${topic(fieldName(m[1]))} ${m[2]}에서 ${m[3]} 사이여야 합니다.`],
  [/^Template name is required and must not exceed (\d+) characters$/,
    (m) => `템플릿 이름은 비워 둘 수 없고 ${m[1]}자를 넘을 수 없습니다.`],
  [/^slide (\d+) does not exist$/, (m) => `${m[1]}번 슬라이드가 없습니다. 화면을 새로 고쳐 주세요.`],
  [/^invalid input: (.+)$/, (m) => byMessage[m[1]] || `입력한 값이 올바르지 않습니다: ${m[1]}`],
  [/^The deck source produced (\d+) slides; this deployment allows (\d+)$/,
    (m) => `이 코드는 ${m[1]}장을 만드는데, 이 배포는 ${m[2]}장까지만 허용합니다. ${Number(m[1]) - Number(m[2])}장을 줄이거나 덱을 나눠 주세요.`],
  [/^AI provider must be (.+)$/,
    (m) => `AI 공급자는 ${m[1].replace(/,? or /g, ' · ').replace(/, /g, ' · ')} 중 하나여야 합니다.`],
  // A setting the deployment will not honour is refused rather than stored and
  // shown back. The message names the setting and what it is honoured at.
  [/^([a-z_]+\.[a-z_]+) must be a whole number between (-?\d+) and (-?\d+)$/,
    (m) => `${topic(settingName(m[1]))} ${m[2]}에서 ${m[3]} 사이의 정수여야 합니다. 이 범위를 벗어난 값은 저장해도 적용되지 않습니다.`],
  [/^([a-z_]+\.[a-z_]+) must be true or false$/,
    (m) => `${topic(settingName(m[1]))} 사용 또는 사용 안 함만 저장할 수 있습니다.`],
  [/^([a-z_]+\.[a-z_]+) must be one of (.+)$/,
    (m) => `${topic(settingName(m[1]))} ${m[2].replace(/, /g, ' · ')} 중 하나여야 합니다.`],
]

/** What a settings key is called on the screen that sets it. */
export function settingName(key: string) {
  const named: Record<string, string> = {
    'ai.timeout_seconds': '응답 제한 시간',
    'ai.max_output_tokens': '최대 출력 토큰',
    'ai.reasoning': '추론 모드',
    'ai.provider': 'AI 공급자',
    'generation.repair_passes': '생성 후 자동 수정',
    'generation.outline_pass': '서사 계획 단계',
    'generation.default_slide_count': '기본 슬라이드',
    'generation.max_slides': '최대 슬라이드',
    'generation.max_template_mb': '템플릿 최대 크기',
    'generation.allow_user_uploads': '사용자 템플릿 업로드',
  }
  return named[key] || key
}

/**
 * topic writes the 은/는 a Korean sentence marks its subject with.
 *
 * "제목은(는)" is what a message looks like when nobody chose, and Korean
 * chooses by whether the last syllable ends in a consonant — which is
 * arithmetic on the character, not a lookup.
 */
function topic(word: string) {
  const last = word.trim().slice(-1)
  const code = last.charCodeAt(0)
  if (code < 0xac00 || code > 0xd7a3) return `${word}은(는)`
  return `${word}${(code - 0xac00) % 28 === 0 ? '는' : '은'}`
}

/** fieldName names a field the way the screen labels it. */
function fieldName(field: string) {
  const named: Record<string, string> = {
    title: '제목', name: '이름', description: '설명', prompt: '브리프', body: '내용',
    audience: '청중', language: '언어', theme: '테마', tone: '어조', company: '회사',
    requestedSlideCount: '슬라이드 수', slideCount: '슬라이드 수', email: '이메일',
    username: '아이디', password: '비밀번호', displayName: '이름', jobTitle: '직함',
    label: '이름', instruction: '요청', source: '덱 코드', scope: '공개 범위',
  }
  return named[field] || field
}

export function errorText(code: string, message: string) {
  const said = (message || '').trim()
  if (byMessage[said]) return byMessage[said]
  for (const [pattern, write] of rules) {
    const match = said.match(pattern)
    if (match) return write(match)
  }
  if (byCode[code]) return byCode[code]
  return said
}
