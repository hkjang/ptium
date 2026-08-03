package auth

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"
	"time"
)

type verificationKey struct {
	keyID     string
	algorithm string
	publicKey any
}

type remoteKeySet struct {
	client            *http.Client
	url               string
	ttl               time.Duration
	allowHTTP         bool
	now               func() time.Time
	mu                sync.RWMutex
	refreshMu         sync.Mutex
	keys              []verificationKey
	expiresAt         time.Time
	lastForcedRefresh time.Time
}

func newRemoteKeySet(client *http.Client, rawURL string, ttl time.Duration, allowHTTP bool, now func() time.Time) *remoteKeySet {
	return &remoteKeySet{client: client, url: rawURL, ttl: ttl, allowHTTP: allowHTTP, now: now}
}

func (keySet *remoteKeySet) verificationKeys(ctx context.Context, keyID, algorithm string) ([]verificationKey, error) {
	if err := keySet.refresh(ctx, false); err != nil {
		return nil, err
	}
	keys := keySet.matchingKeys(keyID, algorithm)
	if len(keys) > 0 {
		return keys, nil
	}
	// An unknown kid normally means the provider rotated signing keys. Refresh
	// immediately rather than waiting for the normal cache deadline.
	if err := keySet.refresh(ctx, true); err != nil {
		return nil, err
	}
	keys = keySet.matchingKeys(keyID, algorithm)
	if len(keys) == 0 {
		return nil, fmt.Errorf("%w: no matching OIDC verification key", ErrInvalidCredentials)
	}
	return keys, nil
}

func (keySet *remoteKeySet) forceVerificationKeys(ctx context.Context, keyID, algorithm string) ([]verificationKey, error) {
	if err := keySet.refresh(ctx, true); err != nil {
		return nil, err
	}
	return keySet.matchingKeys(keyID, algorithm), nil
}

func (keySet *remoteKeySet) matchingKeys(keyID, algorithm string) []verificationKey {
	keySet.mu.RLock()
	defer keySet.mu.RUnlock()
	result := make([]verificationKey, 0, len(keySet.keys))
	for _, key := range keySet.keys {
		if keyID != "" && key.keyID != keyID {
			continue
		}
		if key.algorithm != "" && key.algorithm != algorithm {
			continue
		}
		result = append(result, key)
	}
	return result
}

func (keySet *remoteKeySet) refresh(ctx context.Context, force bool) error {
	keySet.mu.RLock()
	fresh := len(keySet.keys) > 0 && keySet.now().Before(keySet.expiresAt)
	keySet.mu.RUnlock()
	if fresh && !force {
		return nil
	}

	keySet.refreshMu.Lock()
	defer keySet.refreshMu.Unlock()
	keySet.mu.RLock()
	fresh = len(keySet.keys) > 0 && keySet.now().Before(keySet.expiresAt)
	hadKeys := len(keySet.keys) > 0
	lastForcedRefresh := keySet.lastForcedRefresh
	keySet.mu.RUnlock()
	if fresh && !force {
		return nil
	}
	if force && hadKeys && !lastForcedRefresh.IsZero() && keySet.now().Sub(lastForcedRefresh) < 5*time.Second {
		return nil
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, keySet.url, nil)
	if err != nil {
		return fmt.Errorf("create JWKS request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	response, err := keySet.client.Do(request)
	if err != nil {
		return fmt.Errorf("JWKS request: %w", err)
	}
	defer response.Body.Close()
	if !keySet.allowHTTP && response.Request != nil && response.Request.URL.Scheme != "https" {
		return errors.New("JWKS request redirected to a non-HTTPS endpoint")
	}
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("JWKS endpoint returned HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20+1))
	if err != nil {
		return fmt.Errorf("read JWKS: %w", err)
	}
	if len(data) > 2<<20 {
		return errors.New("JWKS response is too large")
	}
	keys, err := parseJWKS(data)
	if err != nil {
		return err
	}
	keySet.mu.Lock()
	keySet.keys = keys
	keySet.expiresAt = keySet.now().Add(keySet.ttl)
	if force && hadKeys {
		keySet.lastForcedRefresh = keySet.now()
	}
	keySet.mu.Unlock()
	return nil
}

type jwksDocument struct {
	Keys []json.RawMessage `json:"keys"`
}

type rawJWK struct {
	KeyType   string   `json:"kty"`
	KeyID     string   `json:"kid"`
	Use       string   `json:"use"`
	Algorithm string   `json:"alg"`
	KeyOps    []string `json:"key_ops"`
	N         string   `json:"n"`
	E         string   `json:"e"`
	Curve     string   `json:"crv"`
	X         string   `json:"x"`
	Y         string   `json:"y"`
}

func parseJWKS(data []byte) ([]verificationKey, error) {
	var document jwksDocument
	if err := decodeJSONObject(data, &document); err != nil {
		return nil, fmt.Errorf("decode JWKS: %w", err)
	}
	if len(document.Keys) == 0 {
		return nil, errors.New("JWKS did not contain any keys")
	}
	keys := make([]verificationKey, 0, len(document.Keys))
	for _, encoded := range document.Keys {
		var raw rawJWK
		if err := json.Unmarshal(encoded, &raw); err != nil {
			continue
		}
		if raw.Use != "" && raw.Use != "sig" {
			continue
		}
		if len(raw.KeyOps) > 0 && !contains(raw.KeyOps, "verify") {
			continue
		}
		publicKey, err := jwkPublicKey(raw)
		if err != nil {
			continue
		}
		keys = append(keys, verificationKey{keyID: raw.KeyID, algorithm: raw.Algorithm, publicKey: publicKey})
	}
	if len(keys) == 0 {
		return nil, errors.New("JWKS did not contain a usable signature key")
	}
	return keys, nil
}

func jwkPublicKey(raw rawJWK) (any, error) {
	switch raw.KeyType {
	case "RSA":
		modulus, err := decodeBase64URL(raw.N)
		if err != nil || len(modulus) == 0 {
			return nil, errors.New("invalid RSA modulus")
		}
		exponentBytes, err := decodeBase64URL(raw.E)
		if err != nil || len(exponentBytes) == 0 || len(exponentBytes) > 8 {
			return nil, errors.New("invalid RSA exponent")
		}
		exponentValue := new(big.Int).SetBytes(exponentBytes)
		if !exponentValue.IsInt64() {
			return nil, errors.New("RSA exponent is too large")
		}
		exponent := exponentValue.Int64()
		if exponent < 3 || exponent > int64(^uint(0)>>1) || exponent%2 == 0 {
			return nil, errors.New("invalid RSA exponent")
		}
		publicKey := &rsa.PublicKey{N: new(big.Int).SetBytes(modulus), E: int(exponent)}
		if publicKey.N.BitLen() < 2048 {
			return nil, errors.New("RSA modulus is smaller than 2048 bits")
		}
		return publicKey, nil
	case "EC":
		var curve elliptic.Curve
		switch raw.Curve {
		case "P-256":
			curve = elliptic.P256()
		case "P-384":
			curve = elliptic.P384()
		case "P-521":
			curve = elliptic.P521()
		default:
			return nil, errors.New("unsupported EC curve")
		}
		xBytes, errX := decodeBase64URL(raw.X)
		yBytes, errY := decodeBase64URL(raw.Y)
		if errX != nil || errY != nil {
			return nil, errors.New("invalid EC coordinates")
		}
		x, y := new(big.Int).SetBytes(xBytes), new(big.Int).SetBytes(yBytes)
		if !curve.IsOnCurve(x, y) {
			return nil, errors.New("EC point is not on the curve")
		}
		return &ecdsa.PublicKey{Curve: curve, X: x, Y: y}, nil
	case "OKP":
		if raw.Curve != "Ed25519" {
			return nil, errors.New("unsupported OKP curve")
		}
		key, err := decodeBase64URL(raw.X)
		if err != nil || len(key) != ed25519.PublicKeySize {
			return nil, errors.New("invalid Ed25519 public key")
		}
		return ed25519.PublicKey(key), nil
	default:
		return nil, errors.New("unsupported JWK key type")
	}
}

func decodeBase64URL(value string) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err == nil {
		return decoded, nil
	}
	return base64.URLEncoding.DecodeString(value)
}

func supportedAlgorithm(algorithm string) bool {
	switch algorithm {
	case "RS256", "RS384", "RS512", "PS256", "PS384", "PS512", "ES256", "ES384", "ES512", "EdDSA":
		return true
	default:
		return false
	}
}

func verifyJWTSignature(algorithm string, publicKey any, signingInput, signature []byte) error {
	if algorithm == "EdDSA" {
		key, ok := publicKey.(ed25519.PublicKey)
		if !ok || !ed25519.Verify(key, signingInput, signature) {
			return errors.New("EdDSA signature mismatch")
		}
		return nil
	}

	hash, rsaPSS, ecdsaAlgorithm := algorithmHash(algorithm)
	if hash == 0 || !hash.Available() {
		return errors.New("unsupported signature hash")
	}
	hasher := hash.New()
	_, _ = hasher.Write(signingInput)
	digest := hasher.Sum(nil)

	if ecdsaAlgorithm {
		key, ok := publicKey.(*ecdsa.PublicKey)
		if !ok {
			return errors.New("ECDSA algorithm with a non-EC key")
		}
		expectedBits := map[string]int{"ES256": 256, "ES384": 384, "ES512": 521}[algorithm]
		if key.Curve.Params().BitSize != expectedBits {
			return errors.New("ECDSA algorithm does not match the key curve")
		}
		componentBytes := (key.Curve.Params().BitSize + 7) / 8
		if len(signature) != componentBytes*2 {
			return errors.New("invalid ECDSA signature size")
		}
		r := new(big.Int).SetBytes(signature[:componentBytes])
		s := new(big.Int).SetBytes(signature[componentBytes:])
		if !ecdsa.Verify(key, digest, r, s) {
			return errors.New("ECDSA signature mismatch")
		}
		return nil
	}

	key, ok := publicKey.(*rsa.PublicKey)
	if !ok {
		return errors.New("RSA algorithm with a non-RSA key")
	}
	if rsaPSS {
		return rsa.VerifyPSS(key, hash, digest, signature, &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash, Hash: hash})
	}
	return rsa.VerifyPKCS1v15(key, hash, digest, signature)
}

func algorithmHash(algorithm string) (hash crypto.Hash, rsaPSS bool, ecdsaAlgorithm bool) {
	switch algorithm {
	case "RS256":
		return crypto.SHA256, false, false
	case "RS384":
		return crypto.SHA384, false, false
	case "RS512":
		return crypto.SHA512, false, false
	case "PS256":
		return crypto.SHA256, true, false
	case "PS384":
		return crypto.SHA384, true, false
	case "PS512":
		return crypto.SHA512, true, false
	case "ES256":
		return crypto.SHA256, false, true
	case "ES384":
		return crypto.SHA384, false, true
	case "ES512":
		return crypto.SHA512, false, true
	default:
		return 0, false, false
	}
}
