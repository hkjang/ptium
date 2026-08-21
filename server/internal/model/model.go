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
	UsageCount  int             `json:"usageCount,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

// Asset is an image a deck can place on a slide.
type Asset struct {
	ID          string    `json:"id"`
	OwnerID     string    `json:"ownerId,omitempty"`
	Name        string    `json:"name"`
	ContentType string    `json:"contentType"`
	SizeBytes   int       `json:"sizeBytes"`
	Width       int       `json:"width,omitempty"`
	Height      int       `json:"height,omitempty"`
	Checksum    string    `json:"checksum,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

type Presentation struct {
	ID                  string  `json:"id"`
	OwnerID             string  `json:"ownerId"`
	Title               string  `json:"title"`
	Prompt              string  `json:"prompt"`
	Status              string  `json:"status"`
	TemplateID          *string `json:"templateId,omitempty"`
	TemplateName        string  `json:"templateName,omitempty"`
	Theme               string  `json:"theme"`
	Language            string  `json:"language"`
	Audience            string  `json:"audience"`
	Tone                string  `json:"tone"`
	RequestedSlideCount int     `json:"requestedSlideCount"`
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
