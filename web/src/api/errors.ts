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
  'The presentation changed in another session':
    '다른 곳에서 이 덱이 먼저 바뀌었습니다. 새로고침한 뒤 다시 시도해 주세요.',
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
  'Built-in templates cannot be modified': '기본 제공 템플릿은 수정할 수 없습니다.',
  'Template uploads are disabled by the administrator': '관리자가 템플릿 업로드를 막아 두었습니다.',
  'The uploaded template is empty': '업로드한 템플릿이 비어 있습니다.',
  'The upload could not be read': '업로드한 파일을 읽지 못했습니다.',
  "A PowerPoint file must be supplied in the 'file' field": 'PowerPoint 파일을 file 항목으로 보내 주세요.',
  'a file field is required': '파일을 선택해 주세요.',
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
  version_conflict: '다른 곳에서 먼저 바뀌었습니다. 새로고침한 뒤 다시 시도해 주세요.',
  invalid_json: '요청 본문을 읽지 못했습니다.',
  invalid_id: '주소의 id 형식이 올바르지 않습니다.',
  invalid_upload: '업로드한 파일을 읽지 못했습니다.',
  invalid_source: '보낸 텍스트가 UTF-8이 아닙니다.',
  validation_error: '입력한 값이 올바르지 않습니다.',
  database_unavailable: '데이터베이스가 준비되지 않았습니다. 잠시 후 다시 시도해 주세요.',
  temporarily_unavailable: '서비스를 일시적으로 사용할 수 없습니다. 잠시 후 다시 시도해 주세요.',
  rate_limited: '요청이 너무 잦습니다. 잠시 후 다시 시도해 주세요.',
  presentation_has_no_slides: '슬라이드가 없습니다. 먼저 생성하거나 한 장 추가한 뒤 내보내세요.',
  presentation_has_no_printable_slides: '모든 슬라이드가 건너뛰기로 표시되어 인쇄할 장이 없습니다. 한 장 이상 발표에 넣어 주세요.',
  unsupported_export_format: '이 형식으로는 내보낼 수 없습니다.',
}

export function errorText(code: string, message: string) {
  const said = (message || '').trim()
  if (byMessage[said]) return byMessage[said]
  if (byCode[code]) return byCode[code]
  return said
}
