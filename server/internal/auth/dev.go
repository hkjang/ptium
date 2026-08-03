package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/mail"
	"strings"
)

// DevAuthenticator authenticates development requests with a secret in a
// dedicated header. A bearer token may select a development identity using
// dev:<email>:<comma-separated-roles>; requested roles must be allowed by the
// configured development principal.
type DevAuthenticator struct {
	config DevConfig
}

func NewDevAuthenticator(config DevConfig) (*DevAuthenticator, error) {
	if strings.TrimSpace(config.Header) == "" {
		config.Header = defaultDevHeader
	}
	if strings.TrimSpace(config.Principal.Subject) == "" {
		config.Principal.Subject = "dev:local"
	}
	if strings.TrimSpace(config.Principal.Name) == "" {
		config.Principal.Name = "Ptium Developer"
	}
	if len(config.Principal.Roles) == 0 {
		config.Principal.Roles = []string{"ptium-admin", "user"}
	}
	if len(config.Principal.Scopes) == 0 {
		config.Principal.Scopes = []string{"presentations:read", "presentations:write", "mcp:use"}
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if principal := config.Principal.Clone(); principal != nil {
		config.Principal = *principal
		config.Principal.Roles = normalizedRoles(config.Principal.Roles)
		config.Principal.Scopes = normalizedRoles(config.Principal.Scopes)
		if config.Principal.AuthMethod == "" {
			config.Principal.AuthMethod = "dev"
		}
	}
	return &DevAuthenticator{config: config}, nil
}

func (authenticator *DevAuthenticator) Authenticate(_ context.Context, request *http.Request) (*Principal, error) {
	if authenticator == nil || !authenticator.config.Enabled {
		return nil, ErrNoCredentials
	}
	if !authenticator.config.AllowRemote && !isLoopbackRemote(request.RemoteAddr) {
		return nil, fmt.Errorf("%w: development auth is restricted to loopback clients", ErrInvalidCredentials)
	}
	header := authenticator.config.Header
	if header == "" {
		header = defaultDevHeader
	}
	values := request.Header.Values(header)
	if len(values) == 0 {
		return nil, ErrNoCredentials
	}
	if len(values) != 1 || values[0] == "" {
		return nil, fmt.Errorf("%w: malformed development secret", ErrInvalidCredentials)
	}
	providedDigest := sha256.Sum256([]byte(values[0]))
	secret := authenticator.config.Secret
	if secret == "" {
		secret = authenticator.config.Token
	}
	expectedDigest := sha256.Sum256([]byte(secret))
	if subtle.ConstantTimeCompare(providedDigest[:], expectedDigest[:]) != 1 {
		return nil, ErrInvalidCredentials
	}

	principal := authenticator.config.Principal.Clone()
	token, err := bearerToken(request)
	if errors.Is(err, ErrNoCredentials) {
		return principal, nil
	}
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(token, "dev:") {
		return nil, fmt.Errorf("%w: development bearer token has the wrong prefix", ErrInvalidCredentials)
	}
	dynamic, err := authenticator.dynamicPrincipal(token)
	if err != nil {
		return nil, err
	}
	return dynamic, nil
}

func (authenticator *DevAuthenticator) dynamicPrincipal(token string) (*Principal, error) {
	parts := strings.SplitN(strings.TrimPrefix(token, "dev:"), ":", 2)
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return nil, fmt.Errorf("%w: development identity email is required", ErrInvalidCredentials)
	}
	email := strings.TrimSpace(parts[0])
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email {
		return nil, fmt.Errorf("%w: development identity email is invalid", ErrInvalidCredentials)
	}
	roles := authenticator.config.Principal.Roles
	if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
		roles = splitList(parts[1])
		for _, role := range roles {
			if !authenticator.config.Principal.HasRole(role) {
				return nil, fmt.Errorf("%w: development role is not allowed", ErrInvalidCredentials)
			}
		}
	}
	return &Principal{
		Subject:    "dev:" + email,
		Email:      email,
		Name:       email,
		Roles:      normalizedRoles(roles),
		Scopes:     append([]string(nil), authenticator.config.Principal.Scopes...),
		AuthMethod: "dev",
	}, nil
}

func isLoopbackRemote(remoteAddress string) bool {
	host := strings.TrimSpace(remoteAddress)
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	host = strings.Trim(host, "[]")
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
