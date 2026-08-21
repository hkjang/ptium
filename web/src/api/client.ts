import type {
  AdminUser,
  ApiKey,
  Asset,
  AuthConfig,
  CanvasRegion,
  Incident,
  Presentation,
	PresentationRevision,
  ProfilePreferences,
  ServerError,
  Slide,
  SlideBlock,
  Snippet,
  SlideElement,
  SlideImage,
  SlideParagraph,
  SlotFrame,
  SlotStyle,
  Template,
  TemplateLayout,
  TemplatePalette,
  User,
} from '../types'

const configuredBase = (import.meta.env.VITE_API_BASE_URL as string | undefined) || '/api/v1'
export const API_BASE = configuredBase.replace(/\/$/, '')

const TOKEN_KEY = 'ptium.access_token'
const DEV_SECRET_KEY = 'ptium.dev_secret'
const DEV_MODE_KEY = 'ptium.dev_mode'

export class ApiError extends Error {
  status: number
  requestId?: string
  details?: unknown

  constructor(message: string, status: number, requestId?: string, details?: unknown) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.requestId = requestId
    this.details = details
  }
}

export const session = {
  token: () => sessionStorage.getItem(TOKEN_KEY),
  secret: () => sessionStorage.getItem(DEV_SECRET_KEY),
  devMode: () => sessionStorage.getItem(DEV_MODE_KEY) === 'true',
  set(token: string, devSecret?: string) {
    localStorage.removeItem(TOKEN_KEY)
    sessionStorage.setItem(TOKEN_KEY, token)
    sessionStorage.removeItem(DEV_SECRET_KEY)
    sessionStorage.removeItem(DEV_MODE_KEY)
    sessionStorage.removeItem('ptium.refresh_token')
    if (devSecret) sessionStorage.setItem(DEV_SECRET_KEY, devSecret)
  },
  setDev(secret: string) {
    localStorage.removeItem(TOKEN_KEY)
    sessionStorage.removeItem(TOKEN_KEY)
    sessionStorage.removeItem('ptium.refresh_token')
    sessionStorage.removeItem('ptium.oidc_transaction')
    sessionStorage.setItem(DEV_MODE_KEY, 'true')
    sessionStorage.setItem(DEV_SECRET_KEY, secret)
  },
  /** Drops only the bearer token, leaving the server's session cookie in place. */
  clearBearer() {
    localStorage.removeItem(TOKEN_KEY)
    sessionStorage.removeItem(TOKEN_KEY)
  },
  clear() {
    localStorage.removeItem(TOKEN_KEY)
    sessionStorage.removeItem(TOKEN_KEY)
    sessionStorage.removeItem(DEV_SECRET_KEY)
    sessionStorage.removeItem(DEV_MODE_KEY)
    sessionStorage.removeItem('ptium.refresh_token')
    sessionStorage.removeItem('ptium.oidc_transaction')
  },
}

function errorMessage(body: unknown, fallback: string) {
  if (!body || typeof body !== 'object') return fallback
  const value = body as Record<string, unknown>
  const nested = value.error && typeof value.error === 'object' ? value.error as Record<string, unknown> : null
  return String(value.message || nested?.message || nested?.code || (typeof value.error === 'string' ? value.error : '') || value.detail || fallback)
}

export async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const headers = new Headers(options.headers)
  // FormData sets its own content type, including the multipart boundary.
  if (!headers.has('Content-Type') && options.body && !(options.body instanceof FormData)) {
    headers.set('Content-Type', 'application/json')
  }
  headers.set('Accept', 'application/json')
  const token = session.token()
  const secret = session.secret()
  if (token && !session.devMode()) headers.set('Authorization', `Bearer ${token}`)
  if (session.devMode() && secret) headers.set('X-Ptium-Dev-Secret', secret)

  let response: Response
  try {
    response = await fetch(`${API_BASE}${path}`, { ...options, headers, credentials: 'include' })
  } catch {
    throw new ApiError('서버에 연결할 수 없습니다. 잠시 후 다시 시도해 주세요.', 0)
  }

  if (response.status === 204) return undefined as T
  const contentType = response.headers.get('content-type') || ''
  const body = contentType.includes('json') ? await response.json().catch(() => null) : await response.text().catch(() => '')
  if (!response.ok) {
    throw new ApiError(
      errorMessage(body, `요청을 처리하지 못했습니다 (${response.status})`),
      response.status,
      response.headers.get('x-request-id') || (body && typeof body === 'object' ? String((body as Record<string, unknown>).requestId || '') || undefined : undefined),
      body,
    )
  }
  return body as T
}

function unwrapList<T>(value: unknown, keys: string[]): T[] {
  if (Array.isArray(value)) return value as T[]
  if (!value || typeof value !== 'object') return []
  const record = value as Record<string, unknown>
  for (const key of keys) if (Array.isArray(record[key])) return record[key] as T[]
  return []
}

function unwrapOne<T>(value: unknown, keys: string[]): T {
  if (value && typeof value === 'object') {
    const record = value as Record<string, unknown>
    for (const key of keys) if (record[key] && typeof record[key] === 'object') return record[key] as T
  }
  return value as T
}

async function requestAllPages<T>(path: string, keys: string[]): Promise<T[]> {
  const items: T[] = []
  const limit = 100
  let offset = 0
  for (;;) {
    const separator = path.includes('?') ? '&' : '?'
    const raw = await request<unknown>(`${path}${separator}limit=${limit}&offset=${offset}`)
    const page = unwrapList<T>(raw, keys)
    items.push(...page)
    const record = raw && typeof raw === 'object' ? raw as Record<string, unknown> : {}
    const meta = record.meta && typeof record.meta === 'object' ? record.meta as Record<string, unknown> : {}
    const total = Number(meta.total)
    if (page.length === 0 || !Number.isFinite(total) || items.length >= total || page.length < limit) break
    offset += page.length
  }
  return items
}

function normalizeUser(value: User & Record<string, unknown>): User {
  const isAdmin = value.isAdmin ?? value.is_admin
  const disabled = Boolean(value.disabled)
  return {
    ...value,
    id: String(value.id),
    email: String(value.email || ''),
    name: String(value.name || value.displayName || value.display_name || String(value.email || '').split('@')[0]),
    role: isAdmin === true ? 'admin' : value.role === 'admin' ? 'admin' : 'user',
    status: disabled ? 'suspended' : value.status === 'invited' || value.status === 'suspended' ? value.status : 'active',
    avatarUrl: String(value.avatarUrl || value.avatar_url || '') || undefined,
    createdAt: String(value.createdAt || value.created_at || '') || undefined,
    lastSeenAt: String(value.lastSeenAt || value.last_seen_at || value.lastLogin || value.last_login || '') || undefined,
    presentationsCount: Number(value.presentationsCount ?? value.presentations_count ?? 0),
    hasPassword: Boolean(value.hasPassword ?? value.has_password ?? false),
  }
}

function normalizeApiKey(value: ApiKey & Record<string, unknown>): ApiKey {
  const expiresAt = String(value.expiresAt || value.expires_at || '') || undefined
  const graceUntil = String(value.graceUntil || value.grace_until || '') || undefined
  const rotated = Boolean(value.rotatedToId || value.rotated_to_id)
  const expired = Boolean(expiresAt && Date.parse(expiresAt) <= Date.now())
  const graceActive = Boolean(graceUntil && Date.parse(graceUntil) > Date.now())
  return {
    ...value,
    id: String(value.id),
    name: String(value.name || 'API key'),
    prefix: String(value.prefix || value.keyPrefix || value.key_prefix || ''),
    scopes: Array.isArray(value.scopes) ? value.scopes.map(String) : [],
    status: value.revokedAt || value.revoked_at || (rotated && !graceActive) ? 'revoked' : expired ? 'expired' : rotated ? 'rotating' : 'active',
    createdAt: String(value.createdAt || value.created_at || new Date().toISOString()),
    lastUsedAt: String(value.lastUsedAt || value.last_used_at || '') || undefined,
    expiresAt,
  }
}

function normalizeServerError(value: ServerError & Record<string, unknown>): ServerError {
  const details = value.details && typeof value.details === 'object' ? value.details as Record<string, unknown> : {}
  const rawStatus = String(value.status || 'open')
  return {
    ...value,
    id: String(value.id),
    fingerprint: String(value.fingerprint || '') || undefined,
    code: String(value.code || details.code || value.kind || 'SERVER_ERROR'),
    message: String(value.message || '알 수 없는 서버 오류'),
    service: String(value.service || details.service || 'api'),
    severity: value.severity === 'critical' || value.severity === 'high' || value.severity === 'medium' || value.severity === 'low'
      ? value.severity
      : value.severity === 'error' ? 'high' : value.severity === 'warning' ? 'medium' : 'low',
    status: rawStatus === 'acknowledged' ? 'investigating' : rawStatus === 'resolved' || rawStatus === 'ignored' ? rawStatus : 'open',
    occurrences: Number(value.occurrences ?? value.occurrenceCount ?? value.occurrence_count ?? 1),
    firstSeenAt: String(value.firstSeenAt || value.firstOccurredAt || value.first_occurred_at || value.occurredAt || new Date().toISOString()),
    lastSeenAt: String(value.lastSeenAt || value.lastOccurredAt || value.last_occurred_at || value.updatedAt || new Date().toISOString()),
    requestId: String(value.requestId || value.request_id || '') || undefined,
    stack: String(value.stack || details.stack || '') || undefined,
    notes: String(value.notes || '') || undefined,
  }
}

function normalizeProfile(value: Record<string, unknown>): ProfilePreferences {
  const preferences = value.preferences && typeof value.preferences === 'object' ? value.preferences as Record<string, unknown> : {}
  return {
    name: String(value.name || value.displayName || value.display_name || ''),
    jobTitle: String(value.jobTitle || value.job_title || ''),
    company: String(value.company || ''),
    bio: String(value.bio || ''),
    language: String(preferences.language || value.language || ''),
    defaultAudience: String(preferences.defaultAudience || preferences.default_audience || value.defaultAudience || value.default_audience || ''),
    defaultTone: String(preferences.defaultTone || preferences.default_tone || value.defaultTone || value.default_tone || ''),
    defaultTheme: String(preferences.defaultTheme || preferences.default_theme || value.defaultTheme || value.default_theme || ''),
    brandColor: String(preferences.brandColor || preferences.brand_color || value.brandColor || value.brand_color || ''),
  }
}

/** One image as the library shows it: what it is, and how this person uses it. */
function normalizeAsset(value: Record<string, unknown>): Asset {
  return {
    id: String(value.id ?? ''),
    name: String(value.name ?? ''),
    contentType: String(value.contentType ?? value.content_type ?? ''),
    sizeBytes: Number(value.sizeBytes ?? value.size_bytes ?? 0),
    width: Number(value.width ?? 0),
    height: Number(value.height ?? 0),
    checksum: String(value.checksum ?? '') || undefined,
    tags: Array.isArray(value.tags) ? value.tags.map(String) : [],
    favorite: Boolean(value.favorite),
    deckCount: Number(value.deckCount ?? value.deck_count ?? 0),
    lastUsed: String(value.lastUsed ?? value.last_used ?? '') || undefined,
    reused: Boolean(value.reused),
    createdAt: String(value.createdAt ?? value.created_at ?? ''),
  }
}

/** One saved slide: its name, what it is filed under, and how much it is used. */
function normalizeSnippet(value: Record<string, unknown>): Snippet {
  return {
    id: String(value.id ?? ''),
    name: String(value.name ?? ''),
    source: String(value.source ?? ''),
    role: String(value.role ?? '') || undefined,
    tags: Array.isArray(value.tags) ? value.tags.map(String) : [],
    favorite: Boolean(value.favorite),
    useCount: Number(value.useCount ?? value.use_count ?? 0),
    lastUsed: String(value.lastUsed ?? value.last_used ?? '') || undefined,
    createdAt: String(value.createdAt ?? value.created_at ?? ''),
    updatedAt: String(value.updatedAt ?? value.updated_at ?? ''),
  }
}

function normalizeTemplate(value: Template & Record<string, unknown>): Template {
  const manifest = value.manifest && typeof value.manifest === 'object' ? value.manifest as Record<string, unknown> : {}
  const rawLayouts = Array.isArray(manifest.layouts) ? manifest.layouts as Array<Record<string, unknown>> : []
  const layouts: TemplateLayout[] = rawLayouts.map((layout) => ({
    id: String(layout.id || ''),
    name: String(layout.name || layout.id || ''),
    role: String(layout.role || 'content'),
    placeholders: (Array.isArray(layout.placeholders) ? layout.placeholders as Array<Record<string, unknown>> : []).map((placeholder) => ({
      slot: String(placeholder.slot || ''),
      kind: String(placeholder.kind || 'text'),
      region: String(placeholder.region || '') || undefined,
      maxChars: Number(placeholder.maxChars || 0),
      maxLines: Number(placeholder.maxLines || 0),
    })),
  })).filter((layout) => layout.id !== '')
  return {
    ...value,
    id: String(value.id),
    name: String(value.name || '이름 없는 템플릿'),
    description: String(value.description || '') || undefined,
    filename: String(value.filename || '') || undefined,
    kind: value.kind === 'builtin' ? 'builtin' : 'uploaded',
    scope: value.scope === 'shared' ? 'shared' : 'private',
    paletteKey: String(value.paletteKey || value.palette_key || '') || undefined,
    sizeBytes: Number(value.sizeBytes ?? value.size_bytes ?? 0),
    layoutCount: Number(value.layoutCount ?? value.layout_count ?? layouts.length),
    tags: Array.isArray(value.tags) ? value.tags.map(String) : undefined,
    dark: Boolean(value.dark),
    aspectRatio: String(value.aspectRatio || value.aspect_ratio || '') || undefined,
    usageCount: Number(value.usageCount ?? value.usage_count ?? 0),
    favorite: Boolean(value.favorite),
    lastUsed: String(value.lastUsed || value.last_used || '') || undefined,
    ownerId: String(value.ownerId || value.owner_id || '') || undefined,
    layouts: layouts.length > 0 ? layouts : undefined,
    palette: normalizeTemplatePalette(value.palette),
    createdAt: String(value.createdAt || value.created_at || new Date().toISOString()),
    updatedAt: String(value.updatedAt || value.updated_at || value.createdAt || value.created_at || new Date().toISOString()),
  }
}

/**
 * Fetches an authenticated image and returns an object URL. Rendering the
 * response through <img> rather than inlining the markup keeps server-side SVG
 * inert in the browser.
 */
async function fetchImage(path: string) {
  const headers = new Headers({ Accept: 'image/svg+xml, image/png, image/jpeg, image/gif' })
  const token = session.token(); const secret = session.secret()
  if (token && !session.devMode()) headers.set('Authorization', `Bearer ${token}`)
  if (session.devMode() && secret) headers.set('X-Ptium-Dev-Secret', secret)
  const response = await fetch(`${API_BASE}${path}`, { headers, credentials: 'include' })
  if (!response.ok) {
    const body = await response.json().catch(() => null)
    throw new ApiError(errorMessage(body, `미리보기를 불러오지 못했습니다 (${response.status})`), response.status)
  }
  return URL.createObjectURL(await response.blob())
}

export function authLoginUrl(config?: AuthConfig, returnTo = '/dashboard') {
  if (config?.loginUrl) return config.loginUrl
  return `${API_BASE}/auth/login?return_to=${encodeURIComponent(returnTo)}`
}

function normalizeStatus(status: unknown): Presentation['status'] {
  if (status === 'completed') return 'ready'
  if (status === 'queued' || status === 'processing' || status === 'running') return 'generating'
  if (status === 'draft' || status === 'generating' || status === 'ready' || status === 'failed') return status
  return 'draft'
}

/** asRecord keeps a stored object as it is, or undefined when there is none. */
function asRecord<T>(value: unknown): Record<string, T> | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined
  const entries = Object.entries(value as Record<string, unknown>).filter(([, item]) => item && typeof item === 'object')
  if (entries.length === 0) return undefined
  return Object.fromEntries(entries) as Record<string, T>
}

/** Slots that hold body copy, in the order the template exposes them. */
export function bodySlots(fields: Record<string, SlideParagraph[]> | undefined) {
  if (!fields) return []
  return Object.keys(fields).filter((slot) => slot !== 'title' && slot !== 'subtitle').sort()
}

/** The slot the editor's body textarea is bound to. */
/** A defect the server measured in a drawn slide. */
export interface DeckFinding {
  slide: number
  slot: string
  kind: 'overflow' | 'outside' | 'collision' | 'contrast' | 'orphan' | 'density' | 'notes' | string
  detail: string
  /** True for a slide that is unfinished rather than drawn wrong. */
  advisory: boolean
}

function normalizeFindings(raw: unknown): DeckFinding[] {
  if (!Array.isArray(raw)) return []
  return (raw as Record<string, unknown>[]).map((entry) => ({
    slide: Number(entry.slide ?? 0),
    slot: String(entry.slot ?? ''),
    kind: String(entry.kind ?? ''),
    detail: String(entry.detail ?? ''),
    advisory: Boolean(entry.advisory),
  }))
}

/** A grid component an organisation defined. */
export interface GridSpec {
  name: string
  title?: string
  columns?: { label?: string; weight?: number; align?: string }[]
  values?: Record<string, { label?: string; role?: string; chip?: boolean; meaning?: string }>
  order?: string[]
  zebra?: boolean
  legend?: boolean
}

function normalizeGrid(value: Record<string, unknown>): GridSpec {
  const columns = Array.isArray(value.columns)
    ? (value.columns as Record<string, unknown>[]).map((column) => ({
      label: String(column.label ?? ''),
      weight: Number(column.weight ?? 0) || undefined,
      align: String(column.align ?? '') || undefined,
    }))
    : undefined
  const rawValues = (value.values && typeof value.values === 'object' ? value.values : {}) as Record<string, Record<string, unknown>>
  const values: GridSpec['values'] = {}
  for (const [key, entry] of Object.entries(rawValues)) {
    values[key] = {
      label: String(entry.label ?? ''),
      role: String(entry.role ?? 'ink'),
      chip: Boolean(entry.chip),
      meaning: String(entry.meaning ?? ''),
    }
  }
  return {
    name: String(value.name ?? ''),
    title: String(value.title ?? '') || undefined,
    columns,
    values,
    order: Array.isArray(value.order) ? value.order.map(String) : undefined,
    zebra: Boolean(value.zebra),
    legend: Boolean(value.legend),
  }
}

function normalizeTemplatePalette(raw: unknown): TemplatePalette | undefined {
  if (!raw || typeof raw !== 'object') return undefined
  const value = raw as Record<string, unknown>
  const colors = Array.isArray(value.dataColors) ? value.dataColors.map(String) : []
  if (colors.length === 0) return undefined
  const rejected = Array.isArray(value.rejected)
    ? (value.rejected as Record<string, unknown>[]).map((entry) => ({
      slot: String(entry.slot ?? ''), color: String(entry.color ?? ''), reason: String(entry.reason ?? ''),
    }))
    : undefined
  return {
    surface: String(value.surface ?? ''),
    ink: String(value.ink ?? ''),
    inkContrast: Number(value.inkContrast ?? 0),
    dataColors: colors,
    seriesLimit: Number(value.seriesLimit ?? colors.length),
    rejected: rejected && rejected.length > 0 ? rejected : undefined,
  }
}

export function primaryBodySlot(slide: Slide, layout?: TemplateLayout) {
  const existing = bodySlots(slide.fields)
  if (existing.length > 0) return existing[0]
  const fromLayout = layout?.placeholders.find((placeholder) => placeholder.kind === 'text' && placeholder.slot !== 'title' && placeholder.slot !== 'subtitle')
  return fromLayout?.slot || 'body'
}

export function paragraphsToText(paragraphs: SlideParagraph[] | undefined) {
  return (paragraphs || []).map((paragraph) => `${'  '.repeat(Math.max(0, paragraph.level || 0))}${paragraph.text}`).join('\n')
}

export function textToParagraphs(value: string): SlideParagraph[] {
  return value.split(/\r?\n/).flatMap((line) => {
    let level = 0
    let rest = line
    while (rest.startsWith('  ') || rest.startsWith('\t')) {
      rest = rest.startsWith('\t') ? rest.slice(1) : rest.slice(2)
      level += 1
    }
    const text = rest.trim()
    return text ? [{ text, level: Math.min(level, 4) }] : []
  })
}

function normalizeFields(raw: unknown): Record<string, SlideParagraph[]> {
  if (!raw || typeof raw !== 'object') return {}
  const result: Record<string, SlideParagraph[]> = {}
  for (const [slot, value] of Object.entries(raw as Record<string, unknown>)) {
    if (!Array.isArray(value)) continue
    const paragraphs = value.map((entry) => {
      if (entry && typeof entry === 'object') {
        const record = entry as Record<string, unknown>
        return { text: String(record.text ?? ''), level: Number(record.level ?? 0) || 0 }
      }
      return { text: String(entry), level: 0 }
    }).filter((paragraph) => paragraph.text.trim().length > 0)
    if (paragraphs.length > 0) result[slot] = paragraphs
  }
  return result
}

function normalizeElements(raw: unknown): SlideElement[] {
  if (!Array.isArray(raw)) return []
  return raw.flatMap((entry) => {
    if (!entry || typeof entry !== 'object') return []
    const value = entry as Record<string, unknown>
    const kind = String(value.kind || '')
    if (!['text', 'shape', 'line', 'image', 'table'].includes(kind)) return []
    return [{
      id: String(value.id || `element-${crypto.randomUUID()}`),
      kind: kind as SlideElement['kind'],
      shape: String(value.shape || '') || undefined,
      x: Number(value.x) || 0,
      y: Number(value.y) || 0,
      width: Math.max(.1, Number(value.width) || 10),
      height: Math.max(.1, Number(value.height) || 10),
      rotation: Number(value.rotation) || 0,
      zIndex: Number(value.zIndex) || 0,
      text: String(value.text || '') || undefined,
      cells: Array.isArray(value.cells) ? value.cells.map((row) => Array.isArray(row) ? row.map(String) : []) : undefined,
      headerRows: Number(value.headerRows) || undefined,
      headerColumns: Number(value.headerColumns) || undefined,
      fontFamily: String(value.fontFamily || '') || undefined,
      fontSize: Number(value.fontSize) || undefined,
      textColor: String(value.textColor || '') || undefined,
      bold: Boolean(value.bold), italic: Boolean(value.italic), underline: Boolean(value.underline),
      align: String(value.align || '') || undefined,
      verticalAlign: String(value.verticalAlign || '') || undefined,
      fill: String(value.fill || '') || undefined,
      stroke: String(value.stroke || '') || undefined,
      strokeWidth: Number(value.strokeWidth) || undefined,
      startArrow: String(value.startArrow || '') || undefined,
      endArrow: String(value.endArrow || '') || undefined,
      dash: String(value.dash || '') || undefined,
      opacity: Number(value.opacity) || undefined,
      assetId: String(value.assetId || '') || undefined,
      name: String(value.name || '') || undefined,
      caption: String(value.caption || '') || undefined,
      fit: String(value.fit || '') || undefined,
      groupId: String(value.groupId || '') || undefined,
      locked: Boolean(value.locked), hidden: Boolean(value.hidden),
    }]
  })
}

/** Region overrides, kept only when they are complete and finite. */
function normalizeFrames(raw: unknown): Record<string, SlotFrame> | undefined {
  if (!raw || typeof raw !== 'object') return undefined
  const frames: Record<string, SlotFrame> = {}
  for (const [slot, value] of Object.entries(raw as Record<string, unknown>)) {
    if (!value || typeof value !== 'object') continue
    const frame = value as Record<string, unknown>
    const numbers = [frame.x, frame.y, frame.width, frame.height].map(Number)
    if (numbers.some((number) => !Number.isFinite(number))) continue
    if (numbers[2] <= 0 || numbers[3] <= 0) continue
    frames[slot] = { x: numbers[0], y: numbers[1], width: numbers[2], height: numbers[3] }
  }
  return Object.keys(frames).length > 0 ? frames : undefined
}

/** Region typography, kept only where it changes something. */
function normalizeStyles(raw: unknown): Record<string, SlotStyle> | undefined {
  if (!raw || typeof raw !== 'object') return undefined
  const styles: Record<string, SlotStyle> = {}
  for (const [slot, value] of Object.entries(raw as Record<string, unknown>)) {
    if (!value || typeof value !== 'object') continue
    const entry = value as Record<string, unknown>
    const style: SlotStyle = {}
    const scale = Number(entry.scale)
    if (Number.isFinite(scale) && scale > 0) style.scale = scale
    if (typeof entry.color === 'string' && entry.color) style.color = entry.color
    if (typeof entry.bold === 'boolean') style.bold = entry.bold
    if (typeof entry.italic === 'boolean') style.italic = entry.italic
    if (typeof entry.align === 'string' && entry.align) style.align = entry.align as SlotStyle['align']
    if (Object.keys(style).length > 0) styles[slot] = style
  }
  return Object.keys(styles).length > 0 ? styles : undefined
}

/**
 * One slide as the workspace holds it.
 *
 * The server sends a slide the same shape wherever it comes from — a stored
 * deck, a compiled source, a saved slide being inserted — so it is read the same
 * way in each case.
 */
function normalizeSlide(slide: Record<string, unknown>, index = 0): Slide {
  const content = slide.content && typeof slide.content === 'object' ? slide.content as Record<string, unknown> : {}
  const fields = normalizeFields(content.fields)
  const bodySlot = bodySlots(fields)[0]
  const bulletsFromFields = bodySlot ? fields[bodySlot] : undefined
  const bullets = bulletsFromFields
    ? bulletsFromFields.map((paragraph) => `${'  '.repeat(paragraph.level || 0)}${paragraph.text}`)
    : (Array.isArray(slide.bullets) ? slide.bullets : Array.isArray(content.bullets) ? content.bullets : []).map(String)
  return {
    id: String(slide.id || `slide-${index + 1}`),
    order: Number(slide.order ?? slide.position ?? index + 1),
    layout: String(slide.layout || content.layout || 'content'),
    layoutId: String(slide.layoutId || slide.layout_id || content.layoutId || '') || undefined,
    title: String(slide.title || content.title || (fields.title?.[0]?.text ?? '')),
    subtitle: String(slide.subtitle || content.subtitle || (fields.subtitle?.[0]?.text ?? '')) || undefined,
    body: bullets.join('\n') || String(slide.body || content.body || content.text || '') || undefined,
    bullets,
    fields,
    // Components and images travel with the slide so that editing its text
    // cannot delete the drawings the generator made.
    blocks: asRecord<SlideBlock>(content.blocks),
    images: asRecord<SlideImage>(content.images),
    elements: normalizeElements(content.elements),
    frames: normalizeFrames(content.frames),
    styles: normalizeStyles(content.styles),
    speakerNotes: String(slide.speakerNotes || slide.speaker_notes || content.speaker_notes || '') || undefined,
    imageUrl: String(slide.imageUrl || slide.image_url || content.image_url || '') || undefined,
    accent: String(slide.accent || content.accent || '') || undefined,
  }
}

function normalizePresentation(value: Presentation & Record<string, unknown>): Presentation {
  const rawSlides = Array.isArray(value.slides) ? value.slides as unknown as Array<Record<string, unknown>> : []
  const slides = rawSlides.map(normalizeSlide).sort((a, b) => a.order - b.order)
  const reportedSlideCount = Number(value.slideCount ?? value.slide_count ?? 0)
  return {
    ...value,
    id: String(value.id),
    title: String(value.title || '제목 없는 프레젠테이션'),
    templateId: String(value.templateId || value.template_id || '') || undefined,
    templateName: String(value.templateName || value.template_name || '') || undefined,
    status: normalizeStatus(value.status),
    createdAt: String(value.createdAt || value.created_at || new Date().toISOString()),
    updatedAt: String(value.updatedAt || value.updated_at || value.createdAt || value.created_at || new Date().toISOString()),
		version: Number(value.version || 1),
		deletedAt: String(value.deletedAt || value.deleted_at || '') || undefined,
    thumbnailUrl: String(value.thumbnailUrl || value.thumbnail_url || '') || undefined,
    errorMessage: String(value.errorMessage || value.error_message || '') || undefined,
    slideCount: slides.length > 0 ? slides.length : Number.isFinite(reportedSlideCount) ? reportedSlideCount : 0,
    slides,
  }
}

function normalizeAdminSettingsPayload(raw: unknown) {
  const entries = unwrapList<Record<string, unknown>>(raw, ['settings', 'items', 'data'])
  if (!entries.length) {
    return {
      values: unwrapOne<Record<string, Record<string, unknown>>>(raw, ['data']),
      configured: {} as Record<string, boolean>,
      unreadable: {} as Record<string, boolean>,
    }
  }
  const values: Record<string, Record<string, unknown>> = {}
  const configured: Record<string, boolean> = {}
  // A secret the server can no longer decrypt: it has to be entered again, and
  // the page has to say so rather than look like an empty optional field.
  const unreadable: Record<string, boolean> = {}
  for (const entry of entries) {
    const fullKey = String(entry.key || '')
    const separator = fullKey.indexOf('.')
    let section = separator > 0 ? fullKey.slice(0, separator) : 'operations'
    let key = separator > 0 ? fullKey.slice(separator + 1) : fullKey
    if (section === 'auth' && key.startsWith('oidc.')) {
      section = 'oidc'
      key = key.slice('oidc.'.length)
    }
    if (!values[section]) values[section] = {}
    let value = entry.value
    if (typeof value === 'string') {
      try { value = JSON.parse(value) } catch { /* keep a plain string */ }
    }
    // Sensitive values are intentionally null in admin responses. Keep their
    // configured state separately instead of treating a redacted value as empty.
    if (value !== null && value !== undefined) values[section][key] = value
    if ('configured' in entry) configured[fullKey] = Boolean(entry.configured)
    if (entry.unreadable) unreadable[fullKey] = true
  }
  return { values, configured, unreadable }
}

export const api = {
  async readiness() {
    const root = API_BASE.endsWith('/api/v1') ? API_BASE.slice(0, -'/api/v1'.length) : ''
    try {
      const response = await fetch(`${root}/readyz`, { headers: { Accept: 'application/json' }, credentials: 'include' })
      if (!response.ok) return false
      const payload = await response.json() as { data?: { status?: string } }
      return payload.data?.status === 'ready'
    } catch {
      return false
    }
  },
  async authConfig(): Promise<AuthConfig> {
    const response = await request<Record<string, unknown>>('/auth/config')
    const raw = unwrapOne<Record<string, unknown>>(response, ['data'])
    const oidc = (raw.oidc && typeof raw.oidc === 'object' ? raw.oidc : {}) as Record<string, unknown>
    const dev = (raw.dev_auth && typeof raw.dev_auth === 'object' ? raw.dev_auth : {}) as Record<string, unknown>
    const rawScopes = raw.scopes ?? oidc.scopes
    return {
      enabled: Boolean(raw.enabled ?? raw.auth_enabled ?? true),
      oidcEnabled: Boolean(raw.oidcEnabled ?? raw.oidc_enabled ?? oidc.enabled ?? false),
      devAuthEnabled: Boolean(raw.devAuthEnabled ?? raw.dev_auth_enabled ?? dev.enabled ?? false),
      issuer: String(raw.issuer ?? oidc.issuer ?? '') || undefined,
      clientId: String(raw.clientId ?? raw.client_id ?? oidc.client_id ?? '') || undefined,
      loginUrl: String(raw.login_url ?? raw.authorization_url ?? oidc.login_url ?? '') || undefined,
      providerName: String(raw.provider_name ?? oidc.provider_name ?? 'SSO'),
      devAuthRequiresSecret: Boolean(raw.dev_auth_requires_secret ?? dev.requires_secret ?? false),
      passwordLoginEnabled: Boolean(raw.passwordLoginEnabled ?? raw.password_login_enabled ?? false),
      authorizationEndpoint: String(raw.authorizationEndpoint ?? raw.authorization_endpoint ?? oidc.authorization_endpoint ?? '') || undefined,
      tokenEndpoint: String(raw.tokenEndpoint ?? raw.token_endpoint ?? oidc.token_endpoint ?? '') || undefined,
      endSessionEndpoint: String(raw.endSessionEndpoint ?? raw.end_session_endpoint ?? oidc.end_session_endpoint ?? '') || undefined,
      tokenExchangeUrl: String(raw.tokenExchangeUrl ?? raw.token_exchange_url ?? oidc.token_exchange_url ?? '') || undefined,
      redirectUri: String(raw.redirectUri ?? raw.redirect_uri ?? oidc.redirect_uri ?? '') || undefined,
      scopes: Array.isArray(rawScopes)
        ? rawScopes.map(String)
        : typeof rawScopes === 'string'
          ? rawScopes.split(/\s+/).filter(Boolean)
          : ['openid', 'profile', 'email'],
    }
  },
  /**
   * Signs in a local account. The session lives in the HttpOnly cookie the server
   * sets, not in this tab's storage: a token kept here would be gone the moment
   * the tab closed, and would go stale as soon as the server renewed the session.
   */
  async passwordLogin(username: string, password: string) {
    const raw = await request<unknown>('/auth/login', {
      method: 'POST', body: JSON.stringify({ username, password }),
    })
    const payload = unwrapOne<Record<string, unknown>>(raw, ['data'])
    session.clear()
    return normalizeUser(unwrapOne<User & Record<string, unknown>>(payload, ['user']))
  },
  /**
   * Changes the signed-in account's password. Every earlier session is retired,
   * including this browser's, which the server replaces via the cookie.
   */
  async changePassword(currentPassword: string, newPassword: string) {
    await request<unknown>('/auth/password', {
      method: 'POST', body: JSON.stringify({ currentPassword, newPassword }),
    })
  },
  /**
   * Trades the current identity for a renewable Ptium session cookie. Returns
   * whether one was issued; the caller keeps its bearer token if not.
   */
  async startSession() {
    try {
      await request<unknown>('/auth/session', { method: 'POST' })
      return true
    } catch {
      return false
    }
  },
  /** Grid definitions: the caller's own, plus the shipped ones they have not replaced. */
  async grids() {
    const raw = await request<unknown>('/grids')
    return unwrapList<Record<string, unknown>>(raw, ['grids', 'items', 'data']).map(normalizeGrid)
  },
  async saveGrid(spec: GridSpec) {
    const raw = await request<unknown>(`/grids/${encodeURIComponent(spec.name)}`, {
      method: 'PUT', body: JSON.stringify(spec),
    })
    return normalizeGrid(unwrapOne<Record<string, unknown>>(raw, ['data']))
  },
  async deleteGrid(name: string) {
    await request<void>(`/grids/${encodeURIComponent(name)}`, { method: 'DELETE' })
  },
  /** Images a deck can place on its slides, in the order asked for. */
  async assets(query: { q?: string; tag?: string; favorite?: boolean; sort?: string; limit?: number } = {}) {
    const search = new URLSearchParams({ limit: String(query.limit || 100) })
    if (query.q) search.set('q', query.q)
    if (query.tag) search.set('tag', query.tag)
    if (query.favorite) search.set('favorite', 'true')
    if (query.sort) search.set('sort', query.sort)
    const raw = await request<unknown>(`/assets?${search}`)
    return unwrapList<Record<string, unknown>>(raw, ['assets', 'items', 'data']).map(normalizeAsset)
  },
  /** The words this person files images under, most used first. */
  async assetTags() {
    const raw = await request<unknown>('/assets/tags')
    return unwrapList<Record<string, unknown>>(raw, ['tags', 'items', 'data'])
      .map((value) => ({ name: String(value.name ?? ''), count: Number(value.count ?? 0) }))
      .filter((tag) => tag.name !== '')
  },
  /** Renames or retags an image. */
  async updateAsset(id: string, patch: { name?: string; tags?: string[] }) {
    const raw = await request<unknown>(`/assets/${encodeURIComponent(id)}`, {
      method: 'PATCH', body: JSON.stringify(patch),
    })
    return normalizeAsset(unwrapOne<Record<string, unknown>>(raw, ['data']))
  },
  /** Pins an image to the top of the library. */
  async favoriteAsset(id: string, favorite: boolean) {
    const raw = await request<unknown>(`/assets/${encodeURIComponent(id)}/favorite`, {
      method: 'PUT', body: JSON.stringify({ favorite }),
    })
    return normalizeAsset(unwrapOne<Record<string, unknown>>(raw, ['data']))
  },
  /** Uploads an image. A second upload under the same name replaces it. */
  async uploadAsset(file: File, name?: string) {
    const form = new FormData()
    form.append('file', file)
    if (name && name.trim()) form.append('name', name.trim())
    const raw = await request<unknown>('/assets', { method: 'POST', body: form })
    // The pixel size comes back with the upload, and placing an image without it
    // would guess the aspect ratio of something already measured. `reused` says
    // the same bytes were already in the library and this is that image.
    return normalizeAsset(unwrapOne<Record<string, unknown>>(raw, ['data']))
  },
  async deleteAsset(id: string) {
    await request<void>(`/assets/${encodeURIComponent(id)}`, { method: 'DELETE' })
  },
  /** The image's bytes as a blob URL the caller must revoke. */
  assetImage(id: string) {
    return fetchImage(`/assets/${encodeURIComponent(id)}`)
  },
  /** Measures a stored deck as it will be drawn. */
  async inspectPresentation(id: string) {
    const raw = await request<unknown>(`/presentations/${encodeURIComponent(id)}/inspect`)
    const data = unwrapOne<Record<string, unknown>>(raw, ['data'])
    return { clean: Boolean(data.clean), findings: normalizeFindings(data.findings) }
  },
  /** Clears the session cookie. Safe to call when already signed out. */
  async logout() {
    try { await request<void>('/auth/logout', { method: 'POST' }) } catch { /* signing out must always succeed locally */ }
  },
  async me() {
    try {
      const raw = await request<unknown>('/me')
      const data = unwrapOne<Record<string, unknown>>(raw, ['data'])
      return normalizeUser(unwrapOne<User & Record<string, unknown>>(data, ['user']))
    } catch (error) {
      if (error instanceof ApiError && error.status === 404) {
        const raw = await request<unknown>('/auth/me')
        const data = unwrapOne<Record<string, unknown>>(raw, ['data'])
        return normalizeUser(unwrapOne<User & Record<string, unknown>>(data, ['user']))
      }
      throw error
    }
  },
  async presentations(deleted = false) {
		const path = deleted ? '/presentations?deleted=true' : '/presentations'
    return (await requestAllPages<Presentation & Record<string, unknown>>(path, ['presentations', 'items', 'data'])).map(normalizePresentation)
  },
  async presentation(id: string) {
    return normalizePresentation(unwrapOne<Presentation & Record<string, unknown>>(await request<unknown>(`/presentations/${encodeURIComponent(id)}`), ['presentation', 'data']))
  },
  async generatePresentation(input: Record<string, unknown>) {
    const payload = {
      title: input.title,
      prompt: input.prompt,
      requestedSlideCount: input.requestedSlideCount ?? input.slide_count,
      theme: input.theme,
      templateId: input.templateId,
      language: input.language,
      audience: input.audience,
      tone: input.tone,
    }
    const generated = unwrapOne<Presentation & Record<string, unknown>>(await request<unknown>('/presentations/generate', {
      method: 'POST', body: JSON.stringify(payload),
    }), ['presentation', 'data'])
    return normalizePresentation(generated)
  },
  async updatePresentation(id: string, input: Record<string, unknown>) {
    return normalizePresentation(unwrapOne<Presentation & Record<string, unknown>>(await request<unknown>(`/presentations/${encodeURIComponent(id)}`, {
      method: 'PATCH', body: JSON.stringify(input),
    }), ['presentation', 'data']))
  },
  /** Reads the deck as source: the text form that compiles to these slides. */
  async presentationSource(id: string) {
    const raw = await request<unknown>(`/presentations/${encodeURIComponent(id)}/source`)
    const data = unwrapOne<Record<string, unknown>>(raw, ['data'])
    return {
      source: String(data.source ?? ''),
      slideCount: Number(data.slideCount ?? 0),
      blockKinds: Array.isArray(data.blockKinds) ? data.blockKinds.map(String) : [],
      layouts: Array.isArray(data.layouts)
        ? (data.layouts as Record<string, string>[]).map((layout) => ({
          id: String(layout.id ?? ''), name: String(layout.name ?? ''), role: String(layout.role ?? ''),
        }))
        : [],
    }
  },
  /**
   * Compiles deck source. With dryRun the deck is left alone and only the
   * result is reported, which is how the editor checks before applying.
   */
  async applyPresentationSource(id: string, source: string, dryRun = false, version?: number) {
    const raw = await request<unknown>(`/presentations/${encodeURIComponent(id)}/source`, {
      method: 'PUT', body: JSON.stringify({ source, dryRun, version }),
    })
    const data = unwrapOne<Record<string, unknown>>(raw, ['data'])
    return {
      applied: Boolean(data.applied),
      warnings: Array.isArray(data.warnings) ? data.warnings.map(String) : [],
      findings: normalizeFindings(data.findings),
      presentation: data.presentation
        ? normalizePresentation(data.presentation as Presentation & Record<string, unknown>)
        : null,
      slideCount: Array.isArray(data.slides) ? data.slides.length : undefined,
    }
  },
  /**
   * Renders one slide of source that has not been applied, so the editor can show
   * a slide as it is typed. Returns a blob URL the caller must revoke.
   */
  async sourcePreview(id: string, source: string, slide: number, width = 1000) {
    const headers = new Headers({ 'Content-Type': 'application/json', Accept: 'image/svg+xml' })
    const token = session.token()
    if (token && !session.devMode()) headers.set('Authorization', `Bearer ${token}`)
    if (session.devMode() && session.secret()) headers.set('X-Ptium-Dev-Secret', session.secret() as string)
    const response = await fetch(
      `${API_BASE}/presentations/${encodeURIComponent(id)}/source/preview.svg?slide=${slide}&width=${width}`,
      { method: 'POST', headers, body: JSON.stringify({ source }), credentials: 'include' },
    )
    if (!response.ok) {
      const body = await response.json().catch(() => null)
      throw new ApiError(errorMessage(body, `미리보기를 만들지 못했습니다 (${response.status})`), response.status)
    }
    return {
      url: URL.createObjectURL(await response.blob()),
      slideCount: Number(response.headers.get('x-ptium-slide-count') || 0),
    }
  },
  async retryPresentation(id: string) {
    return normalizePresentation(unwrapOne<Presentation & Record<string, unknown>>(await request<unknown>(`/presentations/${encodeURIComponent(id)}/generate`, {
      method: 'POST', body: JSON.stringify({}),
    }), ['presentation', 'data']))
  },
  deletePresentation: (id: string) => request<void>(`/presentations/${encodeURIComponent(id)}`, { method: 'DELETE' }),
	async duplicatePresentation(id: string) {
		return normalizePresentation(unwrapOne<Presentation & Record<string, unknown>>(await request<unknown>(`/presentations/${encodeURIComponent(id)}/duplicate`, {
			method: 'POST',
		}), ['presentation', 'data']))
	},
	async restoreDeletedPresentation(id: string) {
		return normalizePresentation(unwrapOne<Presentation & Record<string, unknown>>(await request<unknown>(`/presentations/${encodeURIComponent(id)}/restore`, {
			method: 'POST',
		}), ['presentation', 'data']))
	},
	permanentlyDeletePresentation: (id: string) => request<void>(`/presentations/${encodeURIComponent(id)}/permanent`, { method: 'DELETE' }),
	async presentationRevisions(id: string) {
		const items = await requestAllPages<PresentationRevision & Record<string, unknown>>(
			`/presentations/${encodeURIComponent(id)}/revisions`, ['revisions', 'items', 'data'],
		)
		return items.map((value) => ({
			...value,
			id: String(value.id),
			presentationId: String(value.presentationId || value.presentation_id || id),
			version: Number(value.version || 0),
			reason: String(value.reason || 'edit'),
			title: String(value.title || ''),
			slideCount: Number(value.slideCount ?? value.slide_count ?? 0),
			createdAt: String(value.createdAt || value.created_at || new Date().toISOString()),
		})) as PresentationRevision[]
	},
	async restorePresentationRevision(id: string, revisionId: string) {
		return normalizePresentation(unwrapOne<Presentation & Record<string, unknown>>(await request<unknown>(
			`/presentations/${encodeURIComponent(id)}/revisions/${encodeURIComponent(revisionId)}/restore`, { method: 'POST' },
		), ['presentation', 'data']))
	},
  async exportPresentation(id: string, format: 'pptx' | 'pdf' = 'pptx') {
    const headers = new Headers({ Accept: format === 'pptx' ? 'application/vnd.openxmlformats-officedocument.presentationml.presentation' : 'application/pdf' })
    const token = session.token(); const secret = session.secret()
    if (token && !session.devMode()) headers.set('Authorization', `Bearer ${token}`)
    if (session.devMode() && secret) headers.set('X-Ptium-Dev-Secret', secret)
    const response = await fetch(`${API_BASE}/presentations/${encodeURIComponent(id)}/export?format=${format}`, { headers, credentials: 'include' })
    if (!response.ok) {
      const body = await response.json().catch(() => null)
      throw new ApiError(errorMessage(body, `내보내기에 실패했습니다 (${response.status})`), response.status, response.headers.get('x-request-id') || undefined, body)
    }
    return response.blob()
  },

  /** Slides someone saved to use again. */
  async snippets(query: { q?: string; tag?: string; favorite?: boolean; sort?: string; limit?: number } = {}) {
    const search = new URLSearchParams({ limit: String(query.limit || 100) })
    if (query.q) search.set('q', query.q)
    if (query.tag) search.set('tag', query.tag)
    if (query.favorite) search.set('favorite', 'true')
    if (query.sort) search.set('sort', query.sort)
    const raw = await request<unknown>(`/snippets?${search}`)
    return unwrapList<Record<string, unknown>>(raw, ['snippets', 'items', 'data']).map(normalizeSnippet)
  },
  async snippetTags() {
    const raw = await request<unknown>('/snippets/tags')
    return unwrapList<Record<string, unknown>>(raw, ['tags', 'items', 'data'])
      .map((value) => ({ name: String(value.name ?? ''), count: Number(value.count ?? 0) }))
      .filter((tag) => tag.name !== '')
  },
  /** Saves one slide of a deck. The server writes it down as deck source. */
  async saveSnippet(input: { name?: string; tags?: string[]; presentationId?: string; slide?: number; source?: string }) {
    const raw = await request<unknown>('/snippets', { method: 'POST', body: JSON.stringify(input) })
    return normalizeSnippet(unwrapOne<Record<string, unknown>>(raw, ['data']))
  },
  async updateSnippet(id: string, patch: { name?: string; tags?: string[]; source?: string }) {
    const raw = await request<unknown>(`/snippets/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(patch) })
    return normalizeSnippet(unwrapOne<Record<string, unknown>>(raw, ['data']))
  },
  async deleteSnippet(id: string) {
    await request<void>(`/snippets/${encodeURIComponent(id)}`, { method: 'DELETE' })
  },
  async favoriteSnippet(id: string, favorite: boolean) {
    const raw = await request<unknown>(`/snippets/${encodeURIComponent(id)}/favorite`, {
      method: 'PUT', body: JSON.stringify({ favorite }),
    })
    return normalizeSnippet(unwrapOne<Record<string, unknown>>(raw, ['data']))
  },
  /** Lays a saved slide out in a deck's template, ready to insert. */
  async renderSnippet(id: string, presentationId: string) {
    const raw = await request<unknown>(`/snippets/${encodeURIComponent(id)}/render`, {
      method: 'POST', body: JSON.stringify({ presentationId }),
    })
    const data = unwrapOne<Record<string, unknown>>(raw, ['data'])
    return {
      slide: normalizeSlide(data.slide as Record<string, unknown>),
      warnings: Array.isArray(data.warnings) ? data.warnings.map(String) : [],
      name: String(data.name ?? ''),
    }
  },
  /** The saved slide drawn in a deck's template. */
  snippetPreview(id: string, presentationId: string, width = 480) {
    const query = new URLSearchParams({ width: String(width) })
    if (presentationId) query.set('presentationId', presentationId)
    return fetchImage(`/snippets/${encodeURIComponent(id)}/preview.svg?${query}`)
  },

  async templates() {
    return (await requestAllPages<Template & Record<string, unknown>>('/templates', ['templates', 'items', 'data'])).map(normalizeTemplate)
  },
  async template(id: string) {
    return normalizeTemplate(unwrapOne<Template & Record<string, unknown>>(await request<unknown>(`/templates/${encodeURIComponent(id)}`), ['template', 'data']))
  },
  async uploadTemplate(file: File, meta: { name?: string; description?: string; scope?: 'private' | 'shared' } = {}) {
    const form = new FormData()
    form.append('file', file, file.name)
    if (meta.name) form.append('name', meta.name)
    if (meta.description) form.append('description', meta.description)
    form.append('scope', meta.scope || 'private')
    // The browser must set the multipart boundary, so no Content-Type here.
    const raw = await request<unknown>('/templates', { method: 'POST', body: form })
    return normalizeTemplate(unwrapOne<Template & Record<string, unknown>>(raw, ['template', 'data']))
  },
  async updateTemplate(id: string, input: { name?: string; description?: string; scope?: 'private' | 'shared' }) {
    const raw = await request<unknown>(`/templates/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(input) })
    return normalizeTemplate(unwrapOne<Template & Record<string, unknown>>(raw, ['template', 'data']))
  },
  deleteTemplate: (id: string) => request<void>(`/templates/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  /** Pins a design for this person. It changes nobody else's copy. */
  async favoriteTemplate(id: string, favorite: boolean) {
    await request<unknown>(`/templates/${encodeURIComponent(id)}/favorite`, {
      method: 'PUT', body: JSON.stringify({ favorite }),
    })
  },
  templateLayoutPreview(id: string, layoutId: string, width = 640) {
    const path = layoutId
      ? `/templates/${encodeURIComponent(id)}/layouts/${encodeURIComponent(layoutId)}/preview.svg`
      : `/templates/${encodeURIComponent(id)}/preview.svg`
    return fetchImage(`${path}?width=${width}`)
  },
  slidePreview(presentationId: string, slide: number, width = 960, includeFreeform = true,
    options: { exclude?: string[]; only?: string } = {}) {
    const query = new URLSearchParams({ slide: String(slide), width: String(width), freeform: String(includeFreeform) })
    // The canvas asks for one region on its own to drag it, and for the page
    // without the regions it is moving, so the drawing itself moves rather than an
    // outline over a stale copy of it.
    if (options.only) query.set('only', options.only)
    if (options.exclude?.length) query.set('exclude', options.exclude.join(','))
    return fetchImage(`/presentations/${encodeURIComponent(presentationId)}/preview.svg?${query.toString()}`)
  },

  /** The slide's template regions, as objects the canvas can select and edit. */
  async slideRegions(presentationId: string, slide: number) {
    const raw = await request<unknown>(`/presentations/${encodeURIComponent(presentationId)}/slides/${slide}/regions`)
    const data = unwrapOne<Record<string, unknown>>(raw, ['data'])
    const regions = Array.isArray(data.regions) ? data.regions as CanvasRegion[] : []
    return {
      slide: Number(data.slide) || slide,
      layoutId: String(data.layoutId || ''),
      layoutName: String(data.layoutName || ''),
      aspectRatio: Number(data.aspectRatio) || 16 / 9,
      slideHeightPoints: Number(data.slideHeightPoints) || 540,
      regions: regions.filter((region) => region && typeof region.slot === 'string'),
    }
  },

  /** Asks the model for another draft of one slide. Nothing is saved. */
  async reviseSlide(presentationId: string, slide: number, input: { action: string; instruction?: string; slot?: string }) {
    const raw = await request<unknown>(`/presentations/${encodeURIComponent(presentationId)}/slides/${slide}/revise`, {
      method: 'POST',
      body: JSON.stringify({ action: input.action, instruction: input.instruction || '', slot: input.slot || '' }),
    })
    const data = unwrapOne<Record<string, unknown>>(raw, ['data'])
    const proposal = data.proposal && typeof data.proposal === 'object' ? data.proposal as Record<string, unknown> : {}
    const content = proposal.content && typeof proposal.content === 'object' ? proposal.content as Record<string, unknown> : {}
    return {
      source: String(data.source || ''),
      warnings: Array.isArray(data.warnings) ? data.warnings.map(String) : [],
      findings: normalizeFindings(data.findings),
      slide: {
        title: String(proposal.title || ''),
        subtitle: String(proposal.subtitle || '') || undefined,
        speakerNotes: String(proposal.speakerNotes || proposal.speaker_notes || '') || undefined,
        layoutId: String(proposal.layoutId || proposal.layout_id || content.layoutId || '') || undefined,
        fields: normalizeFields(content.fields),
        blocks: asRecord<SlideBlock>(content.blocks),
        images: asRecord<SlideImage>(content.images),
        accent: String(content.accent || '') || undefined,
      },
    }
  },

  async profile() {
    return normalizeProfile(unwrapOne<Record<string, unknown>>(await request<unknown>('/profile'), ['profile', 'data']))
  },
  async updateProfile(input: Partial<ProfilePreferences>) {
    const raw = await request<unknown>('/profile', {
      method: 'PUT', body: JSON.stringify({
        displayName: input.name,
        jobTitle: input.jobTitle,
        company: input.company,
        bio: input.bio,
        preferences: {
          language: input.language, defaultAudience: input.defaultAudience, defaultTone: input.defaultTone,
          defaultTheme: input.defaultTheme, brandColor: input.brandColor,
        },
      }),
    })
    return normalizeProfile(unwrapOne<Record<string, unknown>>(raw, ['profile', 'data']))
  },

  async apiKeys() {
    return unwrapList<ApiKey & Record<string, unknown>>(await request<unknown>('/api-keys'), ['api_keys', 'keys', 'items', 'data']).map(normalizeApiKey)
  },
  async createApiKey(input: { name: string; scopes: string[]; expiresInDays?: number }) {
    const expiresAt = input.expiresInDays ? new Date(Date.now() + input.expiresInDays * 86400000).toISOString() : undefined
    const raw = await request<Record<string, unknown>>('/api-keys', { method: 'POST', body: JSON.stringify({ name: input.name, scopes: input.scopes, expiresAt }) })
    const payload = unwrapOne<Record<string, unknown>>(raw, ['data'])
    const keyValue = unwrapOne<ApiKey & Record<string, unknown>>(payload, ['apiKey', 'api_key', 'key'])
    return { ...normalizeApiKey(keyValue), key: typeof payload.key === 'string' ? payload.key : undefined, secret: typeof payload.secret === 'string' ? payload.secret : undefined }
  },
  async rotateApiKey(id: string) {
    const raw = await request<Record<string, unknown>>(`/api-keys/${encodeURIComponent(id)}/rotate`, { method: 'POST' })
    const payload = unwrapOne<Record<string, unknown>>(raw, ['data'])
    const keyValue = unwrapOne<ApiKey & Record<string, unknown>>(payload, ['apiKey', 'api_key'])
    return { ...normalizeApiKey(keyValue), key: typeof payload.key === 'string' ? payload.key : undefined, secret: typeof payload.secret === 'string' ? payload.secret : undefined }
  },
  async publicSettings() {
    const raw = await request<unknown>('/settings')
    return unwrapOne<Record<string, unknown>>(raw, ['data'])
  },
  revokeApiKey: (id: string) => request<void>(`/api-keys/${encodeURIComponent(id)}/revoke`, { method: 'POST' }),

  async adminOverview() {
    const raw = await request<Record<string, unknown>>('/admin/overview')
    return unwrapOne<Record<string, unknown>>(raw, ['data'])
  },
  async adminSettings() {
    return normalizeAdminSettingsPayload(await request<unknown>('/admin/settings'))
  },
  async updateAdminSettings(section: string, values: Record<string, unknown>) {
    const raw = await request<unknown>('/admin/settings', {
      method: 'PUT', body: JSON.stringify({ settings: Object.entries(values)
        .filter(([key, value]) => value !== undefined && value !== null && !(value === '' && (key.includes('api_key') || key.includes('client_secret') || key.endsWith('secret'))))
        .map(([key, value]) => ({ key: `${section === 'oidc' ? 'auth.oidc' : section}.${key}`, value })) }),
    })
    return normalizeAdminSettingsPayload(raw)
  },
  async adminUsers() {
    return (await requestAllPages<AdminUser & Record<string, unknown>>('/admin/users', ['users', 'items', 'data'])).map((user) => normalizeUser(user) as AdminUser)
  },
  async updateAdminUser(id: string, input: Record<string, unknown>) {
    const raw = await request<unknown>(`/admin/users/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(input) })
    return normalizeUser(unwrapOne<AdminUser & Record<string, unknown>>(raw, ['data', 'user'])) as AdminUser
  },
  async serverErrors() {
    return (await requestAllPages<ServerError & Record<string, unknown>>('/admin/errors', ['errors', 'items', 'data'])).map(normalizeServerError)
  },
  async updateServerError(id: string, input: Record<string, unknown>) {
    const status = input.status === 'investigating' ? 'acknowledged' : input.status
    const raw = await request<unknown>(`/admin/errors/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify({ status, notes: input.notes }) })
    return normalizeServerError(unwrapOne<ServerError & Record<string, unknown>>(raw, ['data', 'error']))
  },
  async incidents() {
    return unwrapList<Incident>(await request<unknown>('/admin/incidents'), ['incidents', 'items', 'data'])
  },
  createIncident: (input: Record<string, unknown>) => request<Incident>('/admin/incidents', {
    method: 'POST', body: JSON.stringify(input),
  }),
}
