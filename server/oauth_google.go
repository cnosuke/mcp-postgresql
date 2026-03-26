package server

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/golang-jwt/jwt/v5"
)

const (
	googleJWKSURL  = "https://www.googleapis.com/oauth2/v3/certs"
	googleIssuer1  = "accounts.google.com"
	googleIssuer2  = "https://accounts.google.com"
	jwksFallbackTTL = 1 * time.Hour
)

type GoogleTokenValidator struct {
	mu         sync.RWMutex
	jwks       map[string]*rsa.PublicKey
	jwksExpiry time.Time
	httpClient *http.Client
}

func NewGoogleTokenValidator() *GoogleTokenValidator {
	return &GoogleTokenValidator{
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type jwksResponse struct {
	Keys []jwkKey `json:"keys"`
}

type jwkKey struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func (v *GoogleTokenValidator) fetchJWKS(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, googleJWKSURL, nil)
	if err != nil {
		return errors.Wrap(err, "creating JWKS request")
	}

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "fetching JWKS")
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return errors.Newf("JWKS endpoint returned status %d", resp.StatusCode)
	}

	var jwksResp jwksResponse
	if err := json.NewDecoder(resp.Body).Decode(&jwksResp); err != nil {
		return errors.Wrap(err, "decoding JWKS response")
	}

	keys := make(map[string]*rsa.PublicKey, len(jwksResp.Keys))
	for _, k := range jwksResp.Keys {
		if k.Kty != "RSA" || k.Use != "sig" {
			continue
		}
		pub, err := parseRSAPublicKey(k.N, k.E)
		if err != nil {
			return errors.Wrapf(err, "parsing key kid=%s", k.Kid)
		}
		keys[k.Kid] = pub
	}

	expiry := time.Now().Add(jwksFallbackTTL)
	if cc := resp.Header.Get("Cache-Control"); cc != "" {
		if maxAge := parseCacheControlMaxAge(cc); maxAge > 0 {
			expiry = time.Now().Add(time.Duration(maxAge) * time.Second)
		}
	}

	v.mu.Lock()
	v.jwks = keys
	v.jwksExpiry = expiry
	v.mu.Unlock()

	return nil
}

func parseRSAPublicKey(nBase64, eBase64 string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nBase64)
	if err != nil {
		return nil, errors.Wrap(err, "decoding modulus")
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eBase64)
	if err != nil {
		return nil, errors.Wrap(err, "decoding exponent")
	}

	n := new(big.Int).SetBytes(nBytes)
	e := new(big.Int).SetBytes(eBytes)

	return &rsa.PublicKey{
		N: n,
		E: int(e.Int64()),
	}, nil
}

func parseCacheControlMaxAge(cc string) int {
	for _, directive := range strings.Split(cc, ",") {
		directive = strings.TrimSpace(directive)
		if strings.HasPrefix(directive, "max-age=") {
			val, err := strconv.Atoi(strings.TrimPrefix(directive, "max-age="))
			if err == nil {
				return val
			}
		}
	}
	return 0
}

func (v *GoogleTokenValidator) getKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.RLock()
	if v.jwks != nil && time.Now().Before(v.jwksExpiry) {
		if key, ok := v.jwks[kid]; ok {
			v.mu.RUnlock()
			return key, nil
		}
	}
	v.mu.RUnlock()

	if err := v.fetchJWKS(ctx); err != nil {
		return nil, errors.Wrap(err, "refreshing JWKS")
	}

	v.mu.RLock()
	defer v.mu.RUnlock()
	key, ok := v.jwks[kid]
	if !ok {
		return nil, errors.Newf("key kid=%s not found in JWKS", kid)
	}
	return key, nil
}

type GoogleIDTokenClaims struct {
	Issuer        string `json:"iss"`
	Subject       string `json:"sub"`
	Audience      string `json:"aud"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	HostedDomain  string `json:"hd"`
	ExpiresAt     int64  `json:"exp"`
	IssuedAt      int64  `json:"iat"`
	Name          string `json:"name"`
}

func (c *GoogleIDTokenClaims) GetExpirationTime() (*jwt.NumericDate, error) {
	if c.ExpiresAt == 0 {
		return nil, nil
	}
	return jwt.NewNumericDate(time.Unix(c.ExpiresAt, 0)), nil
}

func (c *GoogleIDTokenClaims) GetIssuedAt() (*jwt.NumericDate, error) {
	if c.IssuedAt == 0 {
		return nil, nil
	}
	return jwt.NewNumericDate(time.Unix(c.IssuedAt, 0)), nil
}

func (c *GoogleIDTokenClaims) GetNotBefore() (*jwt.NumericDate, error) {
	return nil, nil
}

func (c *GoogleIDTokenClaims) GetIssuer() (string, error) {
	return c.Issuer, nil
}

func (c *GoogleIDTokenClaims) GetSubject() (string, error) {
	return c.Subject, nil
}

func (c *GoogleIDTokenClaims) GetAudience() (jwt.ClaimStrings, error) {
	if c.Audience == "" {
		return nil, nil
	}
	return jwt.ClaimStrings{c.Audience}, nil
}

func (v *GoogleTokenValidator) ValidateGoogleIDToken(ctx context.Context, idToken, expectedAudience string) (*GoogleIDTokenClaims, error) {
	claims := &GoogleIDTokenClaims{}

	token, err := jwt.ParseWithClaims(idToken, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		kid, ok := token.Header["kid"].(string)
		if !ok {
			return nil, errors.New("missing kid in token header")
		}
		return v.getKey(ctx, kid)
	})
	if err != nil {
		return nil, errors.Wrap(err, "parsing ID token")
	}

	if !token.Valid {
		return nil, errors.New("invalid token")
	}

	if claims.Issuer != googleIssuer1 && claims.Issuer != googleIssuer2 {
		return nil, errors.Newf("invalid issuer: %s", claims.Issuer)
	}

	if claims.Audience != expectedAudience {
		return nil, errors.Newf("audience mismatch: got %s, expected %s", claims.Audience, expectedAudience)
	}

	return claims, nil
}

func CheckAccess(claims *GoogleIDTokenClaims, allowedDomains, allowedEmails []string) error {
	if len(allowedDomains) == 0 && len(allowedEmails) == 0 {
		return nil
	}

	for _, email := range allowedEmails {
		if claims.Email == email {
			return nil
		}
	}

	if len(allowedDomains) > 0 {
		if claims.HostedDomain == "" && !claims.EmailVerified {
			return errors.New("access denied: no hosted domain and email not verified")
		}
		for _, domain := range allowedDomains {
			if claims.HostedDomain == domain {
				return nil
			}
		}
	}

	return errors.New("access denied")
}
