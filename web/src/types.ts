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

export type PresentationStatus = 'draft' | 'generating' | 'ready' | 'failed'

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
  thumbnailUrl?: string
  createdAt: string
  updatedAt: string
	version: number
	deletedAt?: string
  slides?: Slide[]
  errorMessage?: string
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
 * table. The editor does not edit its insides; it carries it so that editing the
 * text around it cannot destroy it.
 */
export interface SlideBlock {
  kind: string
  heading?: string
  caption?: string
  items?: Array<Record<string, unknown>>
  rows?: string[][]
  [key: string]: unknown
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
  speakerNotes?: string
  imageUrl?: string
  accent?: string
}

export type LayoutRole =
  | 'title' | 'section' | 'content' | 'twoContent' | 'comparison'
  | 'quote' | 'picture' | 'table' | 'chart' | 'closing' | 'blank'

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
  usageCount?: number
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
}

export interface AdminUser extends User {
  authProvider?: string
  presentationsCount?: number
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
