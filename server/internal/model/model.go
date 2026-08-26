package model

import (
	"encoding/json"
	"time"
)

type User struct {
	ID                 string    `json:"id"`
	Subject            string    `json:"subject,omitempty"`
	Email              string    `json:"email"`
	Name               string    `json:"name"`
	Roles              []string  `json:"roles"`
	IsAdmin            bool      `json:"isAdmin"`
	Disabled           bool      `json:"disabled"`
	PresentationsCount int       `json:"presentationsCount,omitempty"`
	LastLogin          time.Time `json:"lastLogin,omitempty"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
	// HasPassword marks an account that signs in with a password rather than
	// through the identity provider.
	HasPassword bool `json:"hasPassword,omitempty"`
	// PasswordUpdatedAt binds a session token to the current password. It is
	// never serialized: it is an internal revocation signal, not user data.
	PasswordUpdatedAt *time.Time `json:"-"`
}

// SessionEpoch is the value a session token records so that changing the
// password invalidates tokens issued before it.
func (u User) SessionEpoch() int64 {
	if u.PasswordUpdatedAt == nil {
		return 0
	}
	return u.PasswordUpdatedAt.Unix()
}

type Profile struct {
	UserID      string          `json:"userId"`
	DisplayName string          `json:"displayName"`
	Company     string          `json:"company"`
	JobTitle    string          `json:"jobTitle"`
	Bio         string          `json:"bio"`
	Preferences json.RawMessage `json:"preferences"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

// Template is a PowerPoint design a deck is generated into. Ptium ships a set
// of built-in templates and lets each user upload their own .pptx/.potx files.
type Template struct {
	ID          string          `json:"id"`
	OwnerID     *string         `json:"ownerId,omitempty"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Filename    string          `json:"filename,omitempty"`
	Kind        string          `json:"kind"`
	Scope       string          `json:"scope"`
	PaletteKey  string          `json:"paletteKey,omitempty"`
	SizeBytes   int             `json:"sizeBytes"`
	Checksum    string          `json:"checksum,omitempty"`
	Manifest    json.RawMessage `json:"manifest,omitempty"`
	LayoutCount int             `json:"layoutCount"`
	AspectRatio string          `json:"aspectRatio,omitempty"`
	// Tags and Dark describe a template in the terms someone choosing one thinks
	// in. They are derived when a template is read, not stored: they follow the
	// design, and the design can change with a release.
	Tags []string `json:"tags,omitempty"`
	Dark bool     `json:"dark,omitempty"`
	// UsageCount is how many of this person's own decks were built on it, and
	// Favorite is whether they pinned it. Both are personal: a library is only
	// useful if it learns what this person reaches for.
	UsageCount int        `json:"usageCount"`
	Favorite   bool       `json:"favorite"`
	LastUsed   *time.Time `json:"lastUsed,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

// Asset is an image a deck can place on a slide.
type Asset struct {
	ID          string `json:"id"`
	OwnerID     string `json:"ownerId,omitempty"`
	Name        string `json:"name"`
	ContentType string `json:"contentType"`
	SizeBytes   int    `json:"sizeBytes"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
	Checksum    string `json:"checksum,omitempty"`
	// Tags are the owner's own words for what the image is for, and Favorite is
	// the shelf they keep it on. DeckCount and LastUsed are counted from the
	// decks that place it, so "the one I always use" is a fact rather than a
	// guess.
	Tags      []string   `json:"tags"`
	Favorite  bool       `json:"favorite"`
	DeckCount int        `json:"deckCount"`
	LastUsed  *time.Time `json:"lastUsed,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	// Reused says an upload matched an image already in the library, so the
	// workspace can say so rather than showing what looks like a duplicate.
	Reused bool `json:"reused,omitempty"`
}

// Snippet is a slide someone saved to use again.
//
// It is kept as deck source, not as a drawn slide: the same saved page comes out
// in whatever template it is inserted into, which is the only way a reusable
// slide is worth having.
type Snippet struct {
	ID       string   `json:"id"`
	OwnerID  string   `json:"ownerId,omitempty"`
	Name     string   `json:"name"`
	Source   string   `json:"source"`
	Role     string   `json:"role,omitempty"`
	Tags     []string `json:"tags"`
	Favorite bool     `json:"favorite"`
	// UseCount counts insertions, which are deliberate acts. Unlike an image on a
	// slide there is nothing to count afterwards: once inserted, a snippet is an
	// ordinary slide and stops being a copy of anything.
	UseCount  int        `json:"useCount"`
	LastUsed  *time.Time `json:"lastUsed,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// AssetTag is one of the words a person files their images under, and how many
// carry it.
type AssetTag struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Comment is one remark about one slide, left by someone reviewing the deck.
//
// The author is a name they typed, not an account: the person following a
// share link has none, and asking them to make one to say "the number on slide
// 4 is out of date" is how a review does not happen.
type Comment struct {
	ID             string     `json:"id"`
	PresentationID string     `json:"presentationId"`
	SlideID        string     `json:"slideId,omitempty"`
	Author         string     `json:"author"`
	Body           string     `json:"body"`
	// ParentID is the remark this one answers. A review is a conversation, and
	// an answer beside the point it answers reads as a second point.
	ParentID   string     `json:"parentId,omitempty"`
	ResolvedAt *time.Time `json:"resolvedAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
}

// Share is a link that opens one deck read-only, for someone who has no account
// here. The token itself is not in it: it is shown once, when the link is made.
type Share struct {
	ID             string     `json:"id"`
	PresentationID string     `json:"presentationId"`
	Label          string     `json:"label,omitempty"`
	ExpiresAt      *time.Time `json:"expiresAt,omitempty"`
	RevokedAt      *time.Time `json:"revokedAt,omitempty"`
	LastSeenAt     *time.Time `json:"lastSeenAt,omitempty"`
	Views          int        `json:"views"`
	CreatedAt      time.Time  `json:"createdAt"`
	// URL is filled in when the link is made, and never afterwards.
	URL string `json:"url,omitempty"`
}

type Presentation struct {
	ID           string  `json:"id"`
	OwnerID      string  `json:"ownerId"`
	Title        string  `json:"title"`
	Prompt       string  `json:"prompt"`
	Status       string  `json:"status"`
	TemplateID   *string `json:"templateId,omitempty"`
	TemplateName string  `json:"templateName,omitempty"`
	// GenerationNotes is what generation had to change about what was asked
	// for: a deck shorter than the count requested, a layout that could not
	// hold a component, a figure with no source. The person who asked is the
	// one who needs to know, so it travels with the deck rather than to a log.
	GenerationNotes []string `json:"generationNotes,omitempty"`
	// RewriteInstruction is what the author asked for the last time they sent
	// this deck back to be rewritten, in their own words. It is read by the
	// worker, which picks the deck up minutes after the asking.
	RewriteInstruction string `json:"rewriteInstruction,omitempty"`
	// GenerationStage is the pass a generation is in right now — planning,
	// writing, fitting, notes — for the screen that waits on it.
	GenerationStage     string `json:"generationStage,omitempty"`
	Theme               string `json:"theme"`
	Language            string `json:"language"`
	Audience            string `json:"audience"`
	Tone                string `json:"tone"`
	RequestedSlideCount int    `json:"requestedSlideCount"`
	// Source is the deck written in Ptium's slide language. It is the editable
	// form of the deck: compiling it reproduces the slides.
	Source              string          `json:"source,omitempty"`
	SlideCount          int             `json:"slideCount,omitempty"`
	Outline             json.RawMessage `json:"outline,omitempty"`
	ErrorMessage        string          `json:"errorMessage,omitempty"`
	Slides              []Slide         `json:"slides,omitempty"`
	CreatedAt           time.Time       `json:"createdAt"`
	UpdatedAt           time.Time       `json:"updatedAt"`
	GenerationStartedAt *time.Time      `json:"generationStartedAt,omitempty"`
	GenerationEndedAt   *time.Time      `json:"generationEndedAt,omitempty"`
	// Version is incremented for every stored mutation. Editors send the version
	// they started from so a second tab cannot silently overwrite newer work.
	Version   int64      `json:"version"`
	DeletedAt *time.Time `json:"deletedAt,omitempty"`
}

// PresentationRevision is a restorable checkpoint. The full snapshot remains
// private to the store; list responses only need enough metadata for a person
// to choose the point they want to restore.
type PresentationRevision struct {
	ID             string    `json:"id"`
	PresentationID string    `json:"presentationId"`
	Version        int64     `json:"version"`
	Reason         string    `json:"reason"`
	Title          string    `json:"title"`
	SlideCount     int       `json:"slideCount"`
	CreatedAt      time.Time `json:"createdAt"`
}

type Slide struct {
	ID             string          `json:"id"`
	PresentationID string          `json:"presentationId"`
	Position       int             `json:"position"`
	Title          string          `json:"title"`
	Subtitle       string          `json:"subtitle,omitempty"`
	Content        json.RawMessage `json:"content"`
	SpeakerNotes   string          `json:"speakerNotes,omitempty"`
	Layout         string          `json:"layout"`
	LayoutID       string          `json:"layoutId,omitempty"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}

// AuditEntry is one thing somebody did, as the trail recorded it. The actor is
// carried as an email as well as an id: an operator reading the trail knows
// people by their address, not by a uuid.
type AuditEntry struct {
	ID         int64           `json:"id"`
	Action     string          `json:"action"`
	TargetType string          `json:"targetType,omitempty"`
	TargetID   string          `json:"targetId,omitempty"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
	CreatedAt  time.Time       `json:"createdAt"`
	ActorID    string          `json:"actorId,omitempty"`
	ActorEmail string          `json:"actorEmail,omitempty"`
	ActorName  string          `json:"actorName,omitempty"`
}

// AuditAction is one kind of entry and how many of it the trail holds.
type AuditAction struct {
	Action string `json:"action"`
	Count  int    `json:"count"`
}

type Setting struct {
	Key        string          `json:"key"`
	Value      json.RawMessage `json:"value,omitempty"`
	Sensitive  bool            `json:"sensitive"`
	Configured bool            `json:"configured"`
	// Unreadable marks a sensitive value the server can no longer decrypt,
	// which is what a rotated encryption key leaves behind. The setting has to
	// be entered again; saying so is more useful than failing the whole page.
	Unreadable  bool      `json:"unreadable,omitempty"`
	Description string    `json:"description,omitempty"`
	UpdatedBy   *string   `json:"updatedBy,omitempty"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type APIKey struct {
	ID          string     `json:"id"`
	UserID      string     `json:"userId"`
	Name        string     `json:"name"`
	Prefix      string     `json:"prefix"`
	Scopes      []string   `json:"scopes"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
	RevokedAt   *time.Time `json:"revokedAt,omitempty"`
	RotatedToID *string    `json:"rotatedToId,omitempty"`
	GraceUntil  *time.Time `json:"graceUntil,omitempty"`
	LastUsedAt  *time.Time `json:"lastUsedAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
}

type Incident struct {
	ID              string          `json:"id"`
	RequestID       string          `json:"requestId,omitempty"`
	UserID          *string         `json:"userId,omitempty"`
	Kind            string          `json:"kind"`
	Severity        string          `json:"severity"`
	Message         string          `json:"message"`
	Details         json.RawMessage `json:"details,omitempty"`
	Status          string          `json:"status"`
	Notes           string          `json:"notes,omitempty"`
	OccurredAt      time.Time       `json:"occurredAt"`
	UpdatedAt       time.Time       `json:"updatedAt"`
	ResolvedAt      *time.Time      `json:"resolvedAt,omitempty"`
	ResolvedBy      *string         `json:"resolvedBy,omitempty"`
	Fingerprint     string          `json:"fingerprint"`
	OccurrenceCount int             `json:"occurrenceCount"`
	FirstOccurredAt time.Time       `json:"firstOccurredAt"`
	LastOccurredAt  time.Time       `json:"lastOccurredAt"`
}
