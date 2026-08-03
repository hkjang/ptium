package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// APIKeyVerifier is the storage hook for hashed API keys, expiry, revocation,
// rotation, scopes, and audit metadata. Implementations should return
// ErrInvalidCredentials for an unknown or revoked key and another error for a
// temporary backend failure.
type APIKeyVerifier interface {
	VerifyAPIKey(context.Context, string) (*Principal, error)
}

// APIKeyVerifierFunc adapts a verification function.
type APIKeyVerifierFunc func(context.Context, string) (*Principal, error)

func (fn APIKeyVerifierFunc) VerifyAPIKey(ctx context.Context, key string) (*Principal, error) {
	return fn(ctx, key)
}

// APIKeyAuthenticator accepts X-API-Key and, when AllowBearer is set, opaque
// bearer tokens. Put an OIDCAuthenticator before it in a
// CompositeAuthenticator so signed JWTs are never downgraded to API-key
// verification.
type APIKeyAuthenticator struct {
	Verifier    APIKeyVerifier
	Header      string
	AllowBearer bool
	MaxKeyBytes int
}

// NewAPIKeyAuthenticator returns the conventional Ptium configuration, which
// accepts both X-API-Key and bearer API keys.
func NewAPIKeyAuthenticator(verifier APIKeyVerifier) *APIKeyAuthenticator {
	return &APIKeyAuthenticator{
		Verifier:    verifier,
		AllowBearer: true,
		MaxKeyBytes: 4096,
	}
}

func (authenticator APIKeyAuthenticator) Authenticate(ctx context.Context, request *http.Request) (*Principal, error) {
	if authenticator.Verifier == nil {
		return nil, errors.New("API key verifier is not configured")
	}
	header := strings.TrimSpace(authenticator.Header)
	if header == "" {
		header = "X-API-Key"
	}
	maxBytes := authenticator.MaxKeyBytes
	if maxBytes == 0 {
		maxBytes = 4096
	}
	if maxBytes < 32 {
		return nil, errors.New("API key max size is too small")
	}

	values := request.Header.Values(header)
	var key string
	if len(values) > 1 {
		return nil, fmt.Errorf("%w: multiple API key headers", ErrInvalidCredentials)
	}
	if len(values) == 1 {
		key = strings.TrimSpace(values[0])
		if key == "" {
			return nil, fmt.Errorf("%w: empty API key", ErrInvalidCredentials)
		}
		if _, _, err := authorizationToken(request); !errors.Is(err, ErrNoCredentials) {
			return nil, fmt.Errorf("%w: multiple authentication credentials", ErrInvalidCredentials)
		}
	} else {
		scheme, token, err := authorizationToken(request)
		if err != nil {
			return nil, err
		}
		if strings.EqualFold(scheme, "ApiKey") || (authenticator.AllowBearer && strings.EqualFold(scheme, "Bearer")) {
			key = token
		} else {
			return nil, ErrNoCredentials
		}
	}

	if len(key) > maxBytes {
		return nil, fmt.Errorf("%w: API key is too large", ErrInvalidCredentials)
	}
	principal, err := authenticator.Verifier.VerifyAPIKey(ctx, key)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("verify API key: %w", err)
	}
	if principal == nil || strings.TrimSpace(principal.Subject) == "" {
		return nil, errors.New("API key verifier returned an empty principal")
	}
	principal = principal.Clone()
	if principal.AuthMethod == "" {
		principal.AuthMethod = "api_key"
	}
	principal.Roles = normalizedRoles(principal.Roles)
	principal.Scopes = normalizedRoles(principal.Scopes)
	return principal, nil
}
