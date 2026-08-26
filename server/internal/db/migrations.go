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
	// Which build was running when a fault was first and last seen. An operator
	// reading a critical incident cannot otherwise tell whether the deployment
	// they are looking at still has the bug: the record says what happened but
	// not what it happened to. Rows written before this column stay blank, and a
	// blank version is shown as unknown rather than guessed at.
	`ALTER TABLE server_errors ADD COLUMN IF NOT EXISTS first_seen_version text NOT NULL DEFAULT ''`,
	`ALTER TABLE server_errors ADD COLUMN IF NOT EXISTS last_seen_version text NOT NULL DEFAULT ''`,
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
	// What generation changed about what was asked for. It used to go to the
	// server log, where the person who asked cannot read it.
	`ALTER TABLE presentations ADD COLUMN IF NOT EXISTS generation_notes jsonb NOT NULL DEFAULT '[]'::jsonb`,
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
	// A deck is written to be shown to someone. Until now the only way to show
	// one to a person without an account was to export the file and mail it,
	// which is how a deck stops being the one in Ptium and starts being four
	// copies in four inboxes. A share is a link that opens the deck read-only:
	// the token is stored as a digest, so the row cannot hand anyone a working
	// link, and it can be revoked or left to expire.
	`CREATE TABLE IF NOT EXISTS presentation_shares(
		id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
		presentation_id uuid NOT NULL REFERENCES presentations(id) ON DELETE CASCADE,
		owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		token_digest text NOT NULL UNIQUE,
		label text NOT NULL DEFAULT '',
		expires_at timestamptz,
		revoked_at timestamptz,
		last_seen_at timestamptz,
		views integer NOT NULL DEFAULT 0,
		created_at timestamptz NOT NULL DEFAULT now())`,
	`CREATE INDEX IF NOT EXISTS presentation_shares_deck_idx ON presentation_shares(presentation_id,created_at DESC)`,
	// A link lets someone look at a deck. Looking is half of a review: the other
	// half is saying what is wrong with slide 4, and until now that came back as
	// an email the author had to hold beside the deck. A comment is attached to
	// the slide it is about — by id, so it stays on that slide when the deck is
	// reordered — and it says who left it, in the name they typed.
	`CREATE TABLE IF NOT EXISTS slide_comments(
		id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
		presentation_id uuid NOT NULL REFERENCES presentations(id) ON DELETE CASCADE,
		slide_id uuid REFERENCES slides(id) ON DELETE CASCADE,
		share_id uuid REFERENCES presentation_shares(id) ON DELETE SET NULL,
		author_name text NOT NULL DEFAULT '',
		body text NOT NULL,
		resolved_at timestamptz,
		created_at timestamptz NOT NULL DEFAULT now())`,
	`CREATE INDEX IF NOT EXISTS slide_comments_deck_idx ON slide_comments(presentation_id,created_at)`,
	// What the author asked for when they sent a deck back to be rewritten. It
	// lives on the deck because the rewrite is queued: the words have to survive
	// the wait between asking and the worker picking it up.
	`ALTER TABLE presentations ADD COLUMN IF NOT EXISTS rewrite_instruction text NOT NULL DEFAULT ''`,
	// A review is a conversation. A reviewer says the number on slide four is
	// out of date; the author says it was fixed — and until now that answer was
	// another remark in a flat list, beside the point it answered rather than
	// under it. A reply hangs off the remark it answers, one level deep: a
	// thread is a conversation, a tree is an argument.
	`ALTER TABLE slide_comments ADD COLUMN IF NOT EXISTS parent_id uuid REFERENCES slide_comments(id) ON DELETE CASCADE`,
	`CREATE INDEX IF NOT EXISTS slide_comments_parent_idx ON slide_comments(parent_id)`,
	// Where a generation has got to. A deck takes a minute or three to write on a
	// self-hosted model, and until now the screen said "생성하고 있어요" for all of
	// it — the same words at five seconds and at three minutes.
	`ALTER TABLE presentations ADD COLUMN IF NOT EXISTS generation_stage text NOT NULL DEFAULT ''`,
	// A worker writing a deck says so every half minute. What decides whether an
	// attempt was abandoned is that it stopped saying so — not how long it has
	// been going, which used to hand a slow but healthy generation to a second
	// worker while the first was still waiting on the model.
	`ALTER TABLE presentations ADD COLUMN IF NOT EXISTS generation_heartbeat_at timestamptz`,
	`ALTER TABLE presentations ADD COLUMN IF NOT EXISTS generation_lease uuid`,
}

// ShippedSetting is the value this product ships a setting with, for anything
// asking what a deployment has changed. A report listing forty values nobody
// touched hides the two somebody did.
func ShippedSetting(key string) (string, bool) {
	setting, ok := defaultSettings[key]
	return setting.Value, ok
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
	"ai.max_output_tokens":           {`8000`, false, "Largest completion Ptium will ask for, before it is stretched to the deck being written. A deck's source runs about 100 tokens a slide; a model that thinks first can spend several thousand more"},
	"ai.timeout_seconds":             {`300`, false, "How long to wait for one completion. A self-hosted 122B model answers in about 70 seconds with thinking disabled, and takes five minutes or more with it enabled"},
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
