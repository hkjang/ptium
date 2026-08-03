package auth

import "errors"

var (
	// ErrNoCredentials means this authenticator did not find credentials it
	// recognizes. A composite authenticator may safely try its next mechanism.
	ErrNoCredentials = errors.New("authentication credentials are missing")
	// ErrInvalidCredentials means credentials were present but could not be
	// trusted. A composite authenticator must not fall back after this error.
	ErrInvalidCredentials = errors.New("authentication credentials are invalid")
	// ErrForbidden means an authenticated principal lacks required privileges.
	ErrForbidden = errors.New("insufficient permissions")
)
