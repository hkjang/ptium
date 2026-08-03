package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Authenticator authenticates one HTTP request. Implementations must return
// ErrNoCredentials only when it is safe for a CompositeAuthenticator to try a
// different mechanism.
type Authenticator interface {
	Authenticate(context.Context, *http.Request) (*Principal, error)
}

// AuthenticatorFunc adapts a function into an Authenticator.
type AuthenticatorFunc func(context.Context, *http.Request) (*Principal, error)

func (fn AuthenticatorFunc) Authenticate(ctx context.Context, request *http.Request) (*Principal, error) {
	return fn(ctx, request)
}

// CompositeAuthenticator tries mechanisms in order. Invalid credentials stop
// the chain; only ErrNoCredentials permits fallback.
type CompositeAuthenticator struct {
	Authenticators []Authenticator
}

func (composite CompositeAuthenticator) Authenticate(ctx context.Context, request *http.Request) (*Principal, error) {
	for _, authenticator := range composite.Authenticators {
		if authenticator == nil {
			continue
		}
		principal, err := authenticator.Authenticate(ctx, request)
		if err == nil {
			if principal == nil || strings.TrimSpace(principal.Subject) == "" {
				return nil, fmt.Errorf("authenticator returned an empty principal")
			}
			return principal, nil
		}
		if errors.Is(err, ErrNoCredentials) {
			continue
		}
		return nil, err
	}
	return nil, ErrNoCredentials
}

func authorizationToken(request *http.Request) (scheme, token string, err error) {
	values := request.Header.Values("Authorization")
	if len(values) == 0 {
		return "", "", ErrNoCredentials
	}
	if len(values) != 1 {
		return "", "", fmt.Errorf("%w: multiple authorization headers", ErrInvalidCredentials)
	}
	parts := strings.Fields(values[0])
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("%w: malformed authorization header", ErrInvalidCredentials)
	}
	return parts[0], parts[1], nil
}

func bearerToken(request *http.Request) (string, error) {
	scheme, token, err := authorizationToken(request)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(scheme, "Bearer") {
		return "", ErrNoCredentials
	}
	return token, nil
}
