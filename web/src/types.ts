export type Role = 'user' | 'admin'

export interface User {
  id: string
  email: string
  name: string
  role: Role
  avatarUrl?: string
  status?: 'active' | 'invited' | 'suspended'
  createdAt?: string
  lastSeenAt?: string
  presentationsCount?: number
  /** True for an account that signs in with a password rather than through SSO. */
  hasPassword?: boolean
}

export interface AuthConfig {
  enabled: boolean
  oidcEnabled: boolean
  devAuthEnabled: boolean
  issuer?: string
  clientId?: string
  loginUrl?: string
  providerName?: string
  devAuthRequiresSecret?: boolean
  passwordLoginEnabled?: boolean
  authorizationEndpoint?: string
  tokenEndpoint?: string
  endSessionEndpoint?: string
  tokenExchangeUrl?: string
  redirectUri?: string
  scopes?: string[]
}

/**
 * Where a deck is. `queued` is waiting for a worker to pick it up and
 * `generating` is one writing it — the web used to fold the first into the
 * second, so an author whose deck was waiting in line (or whose deployment had
 * no worker running at all) was told slides were being written for them.
 */
export type PresentationStatus = 'draft' | 'queued' | 'generating' | 'ready' | 'failed'

export interface Presentation {
  id: string
  title: string
  description?: string
  prompt?: string
  status: PresentationStatus
  theme?: string
  templateId?: string
  templateName?: string
  audience?: string
  language?: string
  slideCount?: number
  /** Remarks still waiting on this deck, threads rather than messages. */
  openComments?: number
  thumbnailUrl?: string
  createdAt: string
  updatedAt: string
	version: number
	deletedAt?: string
  slides?: Slide[]
  errorMessage?: string
  /** What generation did differently from what was asked, in the deck's language. */
  generationNotes?: string[]
  /** Which pass a generation is in: planning · writing · fitting · notes · binding. */
  generationStage?: string
}

/** What happened to one slide between two versions of a deck. */
export interface SlideChange {
  kind: 'added' | 'removed' | 'changed' | 'moved'
  position: number
  from?: number
  title: string
  added?: string[]
  removed?: string[]
}

export interface PresentationRevision {
	id: string
	presentationId: string
	version: number
	reason: 'edit' | 'source' | 'generation' | 'restore' | string
	title: string
	slideCount: number
	createdAt: string
}

/** One line of text inside a template placeholder. */
export interface SlideParagraph {
  text: string
  level?: number
}

/**
 * A drawn component in a template slot — a KPI row, a timeline, a comparison
 * table. The canvas edits it in place: a generated component that can only be
 * deleted and rewritten is not an editable deck.
 */
export interface SlideBlock {
  kind: string
  heading?: string
  caption?: string
  items?: Array<Record<string, unknown>>
  rows?: string[][]
  [key: string]: unknown
}

/** Where something draws on a slide, in percentages of the slide. */
export interface SlotFrame { x: number; y: number; width: number; height: number }

/**
 * What a slide changes about one region's type. Every field is optional: an
 * author who only centres a line has not also chosen its size.
 */
export interface SlotStyle {
  /** Multiplies the template's own size. Absent means unchanged. */
  scale?: number
  color?: string
  bold?: boolean
  italic?: boolean
  align?: 'left' | 'center' | 'right' | 'justify'
}

/**
 * One region of a rendered slide, as the canvas needs it. This is what makes
 * generated content editable rather than a picture: the title the model wrote and
 * a text box the author added are the same kind of object to a click.
 */
export interface CanvasRegion {
  slot: string
  kind: 'text' | 'component' | 'picture' | 'empty'
  frame: SlotFrame
  layout: SlotFrame
  moved: boolean
  text?: string
  paragraphs?: SlideParagraph[]
  block?: SlideBlock
  image?: SlideImage
  /** Point size the region's text is set at, after any override. */
  fontSize?: number
  bold?: boolean
  italic?: boolean
  /** DrawingML alignment, set only where the slide overrides it. */
  align?: string
  /** What this slide changed about the region's type, if anything. */
  style?: SlotStyle
  color?: string
  font?: string
  name?: string
  prompt?: string
  acceptsText: boolean
  /** Set when a component placed in another slot covers this region. */
  spannedBy?: string
}

/** An image placed in a slot, by reference to the asset store. */
export interface SlideImage {
  assetId: string
  name?: string
  caption?: string
}

/** A freely positioned, editable object layered over the template. */
export interface SlideElement {
  id: string
  kind: 'text' | 'shape' | 'line' | 'image' | 'table'
  shape?: 'rect' | 'roundRect' | 'ellipse' | 'triangle' | 'diamond' | 'rightArrow' | 'star5' | 'hexagon' | 'line' | string
  /** Geometry is stored as a percentage of the slide. */
  x: number
  y: number
  width: number
  height: number
  rotation?: number
  zIndex?: number
  text?: string
  cells?: string[][]
  headerRows?: number
  headerColumns?: number
  fontFamily?: string
  fontSize?: number
  textColor?: string
  bold?: boolean
  italic?: boolean
  underline?: boolean
  align?: 'left' | 'center' | 'right' | 'justify' | string
  verticalAlign?: 'top' | 'middle' | 'bottom' | string
  fill?: string
  stroke?: string
  strokeWidth?: number
  startArrow?: 'none' | 'triangle' | 'stealth' | 'diamond' | 'oval' | string
  endArrow?: 'none' | 'triangle' | 'stealth' | 'diamond' | 'oval' | string
  dash?: 'solid' | 'dash' | 'dot' | 'dashDot' | string
  opacity?: number
  assetId?: string
  name?: string
  caption?: string
  fit?: 'cover' | 'contain' | 'fill' | string
  groupId?: string
  locked?: boolean
  hidden?: boolean
}

export interface Slide {
  id: string
  order: number
  layout: string
  layoutId?: string
  title: string
  subtitle?: string
  body?: string
  bullets?: string[]
  /** Template slot name to its paragraphs, as stored by the server. */
  fields?: Record<string, SlideParagraph[]>
  /** Slot name to the component drawn in it. A slot holds text or a component, never both. */
  blocks?: Record<string, SlideBlock>
  /** Slot name to the image drawn in it. */
  images?: Record<string, SlideImage>
  /** Freely positioned objects layered above the template. */
  elements?: SlideElement[]
  /** Where a template region was dragged to, per slot, in slide percentages. */
  frames?: Record<string, SlotFrame>
  /** How a template region's text is set, per slot, where it was changed. */
  styles?: Record<string, SlotStyle>
  speakerNotes?: string
  /** A slide kept in the deck and out of the talk: an appendix, a backup number. */
  skipped?: boolean
  /** A slide whose points are given to the room one at a time while presenting. */
  built?: boolean
  imageUrl?: string
  accent?: string
}

export type LayoutRole =
  | 'title' | 'section' | 'content' | 'twoContent' | 'comparison'
  | 'quote' | 'picture' | 'table' | 'chart' | 'closing' | 'blank'

/**
 * An image in someone's library.
 *
 * The last four fields are what turn a list of uploads into a library: what it
 * is for, whether they pinned it, and how much they actually use it.
 */
export interface Asset {
  id: string
  name: string
  contentType: string
  sizeBytes: number
  width: number
  height: number
  /** The bytes' hash; a picture replaced under the same name gets a new one. */
  checksum?: string
  tags: string[]
  favorite: boolean
  /** How many of this person's decks place it. */
  deckCount: number
  lastUsed?: string
  /** The upload matched an image already in the library, and this is that one. */
  reused?: boolean
  createdAt: string
}

export interface AssetTag { name: string; count: number }

/**
 * A slide someone saved to use again.
 *
 * It is kept as deck source, so inserting it into another deck lays it out in
 * that deck's template rather than pasting a foreign design.
 */
export interface Snippet {
  id: string
  name: string
  source: string
  role?: string
  tags: string[]
  favorite: boolean
  useCount: number
  lastUsed?: string
  createdAt: string
  updatedAt: string
}

/** One remark about one slide, left by someone reviewing the deck. */
export interface DeckComment {
  id: string
  presentationId: string
  slideId?: string
  author: string
  body: string
  /** The remark this one answers, when it is an answer rather than a remark. */
  parentId?: string
  resolvedAt?: string
  createdAt: string
}

/** A link that opens one deck read-only, for someone with no account here. */
export interface Share {
  id: string
  presentationId: string
  label: string
  /** Present only in the answer that made it: the token is stored as a digest. */
  url?: string
  expiresAt?: string
  revokedAt?: string
  lastSeenAt?: string
  views: number
  createdAt: string
}

export interface TemplatePlaceholder {
  slot: string
  kind: string
  region?: string
  maxChars: number
  maxLines: number
}

export interface TemplateLayout {
  id: string
  name: string
  role: LayoutRole | string
  placeholders: TemplatePlaceholder[]
}

/** What a template's own palette supports, reported by the server. */
export interface TemplatePalette {
  surface: string
  ink: string
  inkContrast: number
  dataColors: string[]
  seriesLimit: number
  rejected?: { slot: string; color: string; reason: string }[]
}

export interface Template {
  id: string
  name: string
  description?: string
  filename?: string
  kind: 'builtin' | 'uploaded'
  scope: 'private' | 'shared'
  paletteKey?: string
  sizeBytes: number
  layoutCount: number
  aspectRatio?: string
  /** What the design looks like and what it is for, for narrowing a gallery. */
  tags?: string[]
  dark?: boolean
  /** How many of this person's own decks were built on it. */
  usageCount?: number
  favorite?: boolean
  lastUsed?: string
  ownerId?: string
  layouts?: TemplateLayout[]
  palette?: TemplatePalette
  createdAt: string
  updatedAt: string
}

export interface ApiKey {
  id: string
  name: string
  prefix: string
  scopes: string[]
  status: 'active' | 'revoked' | 'rotating' | 'expired'
  createdAt: string
  lastUsedAt?: string
  expiresAt?: string
  /** When a key rotated away stops working. It has no expiry of its own. */
  graceUntil?: string
}

export interface AdminUser extends User {
  authProvider?: string
  presentationsCount?: number
}

/** One thing somebody did, as the trail recorded it. */
export interface AuditEntry {
  id: number
  action: string
  targetType: string
  targetId: string
  actorId: string
  actorEmail: string
  actorName: string
  metadata: Record<string, unknown> | null
  createdAt: string
}

export interface ServerError {
  id: string
  fingerprint?: string
  code: string
  message: string
  service?: string
  severity: 'critical' | 'high' | 'medium' | 'low'
  status: 'open' | 'investigating' | 'resolved' | 'ignored'
  occurrences: number
  firstSeenAt: string
  lastSeenAt: string
  requestId?: string
  stack?: string
  notes?: string
}

export interface Incident {
  id: string
  title: string
  status: 'investigating' | 'identified' | 'monitoring' | 'resolved'
  severity: 'critical' | 'major' | 'minor'
  message?: string
  createdAt: string
  updatedAt?: string
}

export interface ProfilePreferences {
  name: string
  jobTitle: string
  company: string
  bio: string
  language: string
  defaultAudience: string
  defaultTone: string
  defaultTheme: string
  brandColor: string
}
