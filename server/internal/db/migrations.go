package db

var migrations = []string{
	`CREATE TABLE IF NOT EXISTS schema_migrations (
		version integer PRIMARY KEY,
		applied_at timestamptz NOT NULL DEFAULT now()
	)`,
	`CREATE TABLE IF NOT EXISTS users (
		id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
		subject text UNIQUE,
		email text NOT NULL DEFAULT '',
		name text NOT NULL DEFAULT '',
		roles text[] NOT NULL DEFAULT '{}',
		is_admin boolean NOT NULL DEFAULT false,
		disabled boolean NOT NULL DEFAULT false,
		last_login timestamptz,
		created_at timestamptz NOT NULL DEFAULT now(),
		updated_at timestamptz NOT NULL DEFAULT now()
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS users_email_unique ON users(lower(email)) WHERE email <> ''`,
	`CREATE TABLE IF NOT EXISTS profiles (
		user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
		display_name text NOT NULL DEFAULT '',
		company text NOT NULL DEFAULT '',
		job_title text NOT NULL DEFAULT '',
		bio text NOT NULL DEFAULT '',
		preferences jsonb NOT NULL DEFAULT '{}',
		updated_at timestamptz NOT NULL DEFAULT now()
	)`,
	`CREATE TABLE IF NOT EXISTS presentations (
		id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
		owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		title text NOT NULL,
		prompt text NOT NULL DEFAULT '',
		status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','queued','generating','completed','failed')),
		theme text NOT NULL DEFAULT 'modern',
		language text NOT NULL DEFAULT 'ko',
		audience text NOT NULL DEFAULT 'general',
		tone text NOT NULL DEFAULT 'professional',
		requested_slide_count integer NOT NULL DEFAULT 8 CHECK (requested_slide_count BETWEEN 1 AND 50),
		outline jsonb NOT NULL DEFAULT '[]',
		error_message text NOT NULL DEFAULT '',
		generation_started_at timestamptz,
		generation_ended_at timestamptz,
		created_at timestamptz NOT NULL DEFAULT now(),
		updated_at timestamptz NOT NULL DEFAULT now()
	)`,
	`CREATE INDEX IF NOT EXISTS presentations_owner_updated_idx ON presentations(owner_id, updated_at DESC)`,
	`CREATE INDEX IF NOT EXISTS presentations_queue_idx ON presentations(status, updated_at) WHERE status IN ('queued','generating')`,
	`ALTER TABLE presentations ADD COLUMN IF NOT EXISTS audience text NOT NULL DEFAULT 'general'`,
	`ALTER TABLE presentations ADD COLUMN IF NOT EXISTS tone text NOT NULL DEFAULT 'professional'`,
	`CREATE TABLE IF NOT EXISTS slides (
		id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
		presentation_id uuid NOT NULL REFERENCES presentations(id) ON DELETE CASCADE,
		position integer NOT NULL,
		title text NOT NULL DEFAULT '',
		subtitle text NOT NULL DEFAULT '',
		content jsonb NOT NULL DEFAULT '{}',
		speaker_notes text NOT NULL DEFAULT '',
		layout text NOT NULL DEFAULT 'content',
		created_at timestamptz NOT NULL DEFAULT now(),
		updated_at timestamptz NOT NULL DEFAULT now(),
		UNIQUE(presentation_id, position)
	)`,
	`CREATE TABLE IF NOT EXISTS app_settings (
		key text PRIMARY KEY,
		value jsonb NOT NULL,
		sensitive boolean NOT NULL DEFAULT false,
		description text NOT NULL DEFAULT '',
		updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
		updated_at timestamptz NOT NULL DEFAULT now()
	)`,
	`CREATE TABLE IF NOT EXISTS api_keys (
		id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
		user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		name text NOT NULL,
		key_prefix text NOT NULL UNIQUE,
		secret_hash bytea NOT NULL,
		scopes text[] NOT NULL DEFAULT '{}',
		expires_at timestamptz,
		revoked_at timestamptz,
		rotated_to_id uuid REFERENCES api_keys(id) ON DELETE SET NULL,
		grace_until timestamptz,
		last_used_at timestamptz,
		created_at timestamptz NOT NULL DEFAULT now()
	)`,
	`CREATE INDEX IF NOT EXISTS api_keys_user_idx ON api_keys(user_id, created_at DESC)`,
	`CREATE TABLE IF NOT EXISTS server_errors (
		id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
		request_id text NOT NULL DEFAULT '',
		user_id uuid REFERENCES users(id) ON DELETE SET NULL,
		kind text NOT NULL DEFAULT 'internal',
		severity text NOT NULL DEFAULT 'error',
		message text NOT NULL,
		details jsonb NOT NULL DEFAULT '{}',
		status text NOT NULL DEFAULT 'open' CHECK (status IN ('open','acknowledged','resolved','ignored')),
		notes text NOT NULL DEFAULT '',
		occurred_at timestamptz NOT NULL DEFAULT now(),
		updated_at timestamptz NOT NULL DEFAULT now(),
		resolved_at timestamptz,
		resolved_by uuid REFERENCES users(id) ON DELETE SET NULL
	)`,
	`CREATE INDEX IF NOT EXISTS server_errors_status_idx ON server_errors(status, occurred_at DESC)`,
	`ALTER TABLE server_errors ADD COLUMN IF NOT EXISTS fingerprint text NOT NULL DEFAULT ''`,
	`ALTER TABLE server_errors ADD COLUMN IF NOT EXISTS occurrence_count integer NOT NULL DEFAULT 1`,
	`ALTER TABLE server_errors ADD COLUMN IF NOT EXISTS first_occurred_at timestamptz NOT NULL DEFAULT now()`,
	`ALTER TABLE server_errors ADD COLUMN IF NOT EXISTS last_occurred_at timestamptz NOT NULL DEFAULT now()`,
	`CREATE UNIQUE INDEX IF NOT EXISTS server_errors_open_fingerprint_idx ON server_errors(fingerprint) WHERE fingerprint <> '' AND status IN ('open','acknowledged')`,
	`CREATE TABLE IF NOT EXISTS audit_logs (
		id bigserial PRIMARY KEY,
		actor_id uuid REFERENCES users(id) ON DELETE SET NULL,
		action text NOT NULL,
		target_type text NOT NULL DEFAULT '',
		target_id text NOT NULL DEFAULT '',
		metadata jsonb NOT NULL DEFAULT '{}',
		created_at timestamptz NOT NULL DEFAULT now()
	)`,
	`UPDATE app_settings SET value='"aurora"'::jsonb WHERE key='generation.default_theme' AND value='"modern"'::jsonb AND updated_by IS NULL`,
	// The shipped library moved from five themes to thirty named designs. An
	// administrator's own choice is left alone; an untouched default moves to
	// the safest design in the new library.
	`UPDATE app_settings SET value='"slate-classic"'::jsonb WHERE key='generation.default_theme'
		AND value IN ('"aurora"'::jsonb,'"modern"'::jsonb) AND updated_by IS NULL`,
	`CREATE TABLE IF NOT EXISTS templates (
		id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
		owner_id uuid REFERENCES users(id) ON DELETE CASCADE,
		name text NOT NULL,
		description text NOT NULL DEFAULT '',
		filename text NOT NULL DEFAULT '',
		kind text NOT NULL DEFAULT 'uploaded' CHECK (kind IN ('builtin','uploaded')),
		scope text NOT NULL DEFAULT 'private' CHECK (scope IN ('private','shared')),
		palette_key text NOT NULL DEFAULT '',
		size_bytes integer NOT NULL DEFAULT 0,
		checksum text NOT NULL DEFAULT '',
		manifest jsonb NOT NULL DEFAULT '{}',
		data bytea NOT NULL,
		created_at timestamptz NOT NULL DEFAULT now(),
		updated_at timestamptz NOT NULL DEFAULT now(),
		CHECK (kind = 'builtin' OR owner_id IS NOT NULL)
	)`,
	`CREATE INDEX IF NOT EXISTS templates_owner_idx ON templates(owner_id, updated_at DESC)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS templates_builtin_palette_idx ON templates(palette_key) WHERE kind='builtin'`,
	`ALTER TABLE presentations ADD COLUMN IF NOT EXISTS template_id uuid REFERENCES templates(id) ON DELETE SET NULL`,
	`CREATE INDEX IF NOT EXISTS presentations_template_idx ON presentations(template_id) WHERE template_id IS NOT NULL`,
	`ALTER TABLE slides ADD COLUMN IF NOT EXISTS layout_id text NOT NULL DEFAULT ''`,
	// Local password sign-in for the bootstrap administrator. The hash column is
	// nullable: an account provisioned by the identity provider never has one.
	// Grid components an organisation defined for itself: a RACI chart, a risk
	// matrix, a readiness checklist. The definition names colour roles rather than
	// colours, so one definition works in every template.
	`CREATE TABLE IF NOT EXISTS grids(
		id uuid PRIMARY KEY,
		owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		name text NOT NULL,
		spec jsonb NOT NULL,
		created_at timestamptz NOT NULL DEFAULT now(),
		updated_at timestamptz NOT NULL DEFAULT now())`,
	`CREATE UNIQUE INDEX IF NOT EXISTS grids_owner_name_idx ON grids(owner_id,lower(name))`,
	// Images a deck places on its slides. Kept in the database like everything
	// else, so an air-gapped deployment has no second thing to back up.
	`CREATE TABLE IF NOT EXISTS assets(
		id uuid PRIMARY KEY,
		owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		name text NOT NULL,
		content_type text NOT NULL,
		size_bytes integer NOT NULL,
		width integer NOT NULL DEFAULT 0,
		height integer NOT NULL DEFAULT 0,
		checksum text NOT NULL,
		data bytea NOT NULL,
		created_at timestamptz NOT NULL DEFAULT now())`,
	`CREATE INDEX IF NOT EXISTS assets_owner_created_idx ON assets(owner_id,created_at DESC)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS assets_owner_name_idx ON assets(owner_id,lower(name))`,
	// A deck's source is the text it was written as. Storing it makes the deck
	// editable as text and recompilable into the same slides.
	`ALTER TABLE presentations ADD COLUMN IF NOT EXISTS source text NOT NULL DEFAULT ''`,
	`ALTER TABLE users ADD COLUMN IF NOT EXISTS password_hash bytea`,
	`ALTER TABLE users ADD COLUMN IF NOT EXISTS password_updated_at timestamptz`,
	// Safe iteration: soft deletion makes an accidental delete recoverable, a
	// monotonic version prevents stale editors from overwriting newer work, and
	// compact snapshots make meaningful earlier states restorable.
	`ALTER TABLE presentations ADD COLUMN IF NOT EXISTS version bigint NOT NULL DEFAULT 1`,
	`ALTER TABLE presentations ADD COLUMN IF NOT EXISTS deleted_at timestamptz`,
	`CREATE INDEX IF NOT EXISTS presentations_owner_deleted_idx ON presentations(owner_id,deleted_at,updated_at DESC)`,
	`CREATE TABLE IF NOT EXISTS presentation_revisions(
		id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
		presentation_id uuid NOT NULL REFERENCES presentations(id) ON DELETE CASCADE,
		owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		version bigint NOT NULL,
		reason text NOT NULL DEFAULT 'edit',
		title text NOT NULL,
		slide_count integer NOT NULL DEFAULT 0,
		snapshot jsonb NOT NULL,
		created_at timestamptz NOT NULL DEFAULT now(),
		UNIQUE(presentation_id,version))`,
	`CREATE INDEX IF NOT EXISTS presentation_revisions_deck_idx ON presentation_revisions(presentation_id,created_at DESC)`,
	// An image may live on a mounted volume instead of in the row, so a
	// deployment can keep its database small and back the pictures up separately.
	// A null column means "the bytes are on the volume".
	`ALTER TABLE assets ALTER COLUMN data DROP NOT NULL`,
	// What someone marked as theirs to reach for again. One table for every kind
	// of thing a workspace collects, because "favourite" means the same thing
	// each time and a per-kind table would say it three ways.
	`CREATE TABLE IF NOT EXISTS favorites(
		owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		kind text NOT NULL,
		ref_id uuid NOT NULL,
		created_at timestamptz NOT NULL DEFAULT now(),
		PRIMARY KEY(owner_id,kind,ref_id))`,
	`CREATE INDEX IF NOT EXISTS favorites_owner_kind_idx ON favorites(owner_id,kind,created_at DESC)`,
	// Which decks place which image. Written whenever a deck's slides are saved,
	// so "used in five decks" is counted rather than remembered: an image dropped
	// from a deck stops counting the moment it is dropped, and no counter drifts.
	`CREATE TABLE IF NOT EXISTS asset_usage(
		asset_id uuid NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
		presentation_id uuid NOT NULL REFERENCES presentations(id) ON DELETE CASCADE,
		updated_at timestamptz NOT NULL DEFAULT now(),
		PRIMARY KEY(asset_id,presentation_id))`,
	`CREATE INDEX IF NOT EXISTS asset_usage_asset_idx ON asset_usage(asset_id,updated_at DESC)`,
	// Someone's own words for what an image is for: logo, 제품컷, 배경. Tags are
	// how a library of two hundred pictures stays findable.
	`ALTER TABLE assets ADD COLUMN IF NOT EXISTS tags text[] NOT NULL DEFAULT '{}'`,
	// Slides someone keeps: the company introduction, the team page, the legal
	// notice, the roadmap they redraw every quarter. Stored as deck source rather
	// than as a rendered slide, so inserting one into another deck lays it out in
	// that deck's template instead of pasting a foreign design.
	`CREATE TABLE IF NOT EXISTS snippets(
		id uuid PRIMARY KEY,
		owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		name text NOT NULL,
		source text NOT NULL,
		role text NOT NULL DEFAULT 'content',
		tags text[] NOT NULL DEFAULT '{}',
		use_count integer NOT NULL DEFAULT 0,
		last_used_at timestamptz,
		created_at timestamptz NOT NULL DEFAULT now(),
		updated_at timestamptz NOT NULL DEFAULT now())`,
	`CREATE UNIQUE INDEX IF NOT EXISTS snippets_owner_name_idx ON snippets(owner_id,lower(name))`,
	`CREATE INDEX IF NOT EXISTS snippets_owner_used_idx ON snippets(owner_id,last_used_at DESC)`,
}

var defaultSettings = map[string]struct {
	Value       string
	Sensitive   bool
	Description string
}{
	"ai.provider":                    {`"fallback"`, false, "AI provider: fallback or openai-compatible"},
	"ai.base_url":                    {`"https://api.openai.com/v1"`, false, "OpenAI-compatible API base URL"},
	"ai.model":                       {`"gpt-4.1-mini"`, false, "Generation model"},
	"ai.api_key":                     {`""`, true, "Provider API key"},
	"ai.reasoning":                   {`"auto"`, false, "Whether to ask the provider not to think before answering: auto asks and stops if the provider rejects it, off always asks, on never asks. A reasoning model returns no answer at all while thinking is enabled"},
	"ai.max_output_tokens":           {`8000`, false, "Largest completion Ptium will ask for. A deck's source is a few thousand tokens"},
	"ai.timeout_seconds":             {`300`, false, "How long to wait for one completion. A self-hosted model answers in tens of seconds"},
	"generation.default_slide_count": {`10`, false, "Default generated slide count"},
	"generation.max_slides":          {`50`, false, "Maximum generated slides"},
	"generation.default_theme":       {`"slate-classic"`, false, "Default shipped design, or a design key from the template library"},
	"generation.default_lang":        {`"ko"`, false, "Default presentation language"},
	"branding.product_name":          {`"Ptium"`, false, "Product display name"},
	"branding.logo_url":              {`""`, false, "Public logo URL"},
	"branding.brand_color":           {`"#7C3AED"`, false, "Primary brand color"},
	"auth.oidc.issuer_url":           {`""`, false, "OIDC issuer; bootstrap environment takes precedence until restart"},
	"auth.oidc.client_id":            {`""`, false, "OIDC client identifier"},
	"auth.oidc.client_secret":        {`""`, true, "OIDC client secret; set only for a confidential client, which makes Ptium exchange authorization codes server-side"},
	"auth.oidc.admin_roles":          {`["ptium-admin","admin"]`, false, "OIDC roles mapped to Ptium administrators"},
	"security.api_key_grace":         {`"24h"`, false, "Default API-key rotation overlap"},
	"security.cors_origins":          {`[]`, false, "Additional allowed browser origins"},
	"generation.default_tone":        {`"professional"`, false, "Default writing tone"},
	"generation.default_audience":    {`"general"`, false, "Default target audience"},
	"generation.outline_pass":        {`true`, false, "Plan the deck narrative before writing slide copy"},
	"generation.repair_passes":       {`3`, false, "How many slides a generation may measure and send back to the model to be rewritten to fit. 0 turns the repair pass off"},
	"generation.max_template_mb":     {`32`, false, "Maximum uploaded template size in MiB"},
	"generation.allow_user_uploads":  {`true`, false, "Allow users to upload their own PowerPoint templates"},
}
