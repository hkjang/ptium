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
  slides?: Slide[]
  errorMessage?: string
}

/** One line of text inside a template placeholder. */
export interface SlideParagraph {
  text: string
  level?: number
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
