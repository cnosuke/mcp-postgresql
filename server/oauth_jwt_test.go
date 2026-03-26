package server

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testSigningKey = "test-signing-key-that-is-at-least-32-bytes-long"
	testIssuer     = "https://test.example.com"
)

func TestJWTManagerRoundTrip(t *testing.T) {
	m := NewJWTManager(testSigningKey, testIssuer, 3600)

	userID := "user-123"
	email := "user@example.com"
	scope := "read write"
	audience := "https://api.example.com"
	clientID := "client-abc"

	tokenStr, err := m.IssueAccessToken(userID, email, scope, audience, clientID)
	require.NoError(t, err)
	require.NotEmpty(t, tokenStr)

	claims, err := m.VerifyAccessToken(tokenStr, audience)
	require.NoError(t, err)

	assert.Equal(t, userID, claims.Subject)
	assert.Equal(t, email, claims.Email)
	assert.Equal(t, scope, claims.Scope)
	assert.Equal(t, clientID, claims.ClientID)
	assert.Equal(t, testIssuer, claims.Issuer)
	assert.Contains(t, []string(claims.Audience), audience)
	assert.NotNil(t, claims.ExpiresAt)
	assert.NotNil(t, claims.IssuedAt)
}

func TestJWTManagerExpiredToken(t *testing.T) {
	m := NewJWTManager(testSigningKey, testIssuer, 0)

	tokenStr, err := m.IssueAccessToken("user-1", "u@example.com", "read", "aud", "cid")
	require.NoError(t, err)

	_, err = m.VerifyAccessToken(tokenStr, "aud")
	assert.Error(t, err)
}

func TestJWTManagerInvalidSignature(t *testing.T) {
	issuer := NewJWTManager(testSigningKey, testIssuer, 3600)
	verifier := NewJWTManager("different-key-that-is-also-at-least-32-bytes", testIssuer, 3600)

	tokenStr, err := issuer.IssueAccessToken("user-1", "u@example.com", "read", "aud", "cid")
	require.NoError(t, err)

	_, err = verifier.VerifyAccessToken(tokenStr, "aud")
	assert.Error(t, err)
}

func TestJWTManagerAudienceMismatch(t *testing.T) {
	m := NewJWTManager(testSigningKey, testIssuer, 3600)

	tokenStr, err := m.IssueAccessToken("user-1", "u@example.com", "read", "audience-A", "cid")
	require.NoError(t, err)

	_, err = m.VerifyAccessToken(tokenStr, "audience-B")
	assert.Error(t, err)
}

func TestJWTManagerIssuerMismatch(t *testing.T) {
	issuerA := NewJWTManager(testSigningKey, "issuer-A", 3600)
	issuerB := NewJWTManager(testSigningKey, "issuer-B", 3600)

	tokenStr, err := issuerA.IssueAccessToken("user-1", "u@example.com", "read", "aud", "cid")
	require.NoError(t, err)

	_, err = issuerB.VerifyAccessToken(tokenStr, "aud")
	assert.Error(t, err)
}

func TestMakeTokenVerifier(t *testing.T) {
	m := NewJWTManager(testSigningKey, testIssuer, 3600)

	userID := "user-456"
	email := "verifier@example.com"
	scope := "read write"
	audience := "https://api.example.com"
	clientID := "client-xyz"

	tokenStr, err := m.IssueAccessToken(userID, email, scope, audience, clientID)
	require.NoError(t, err)

	verifier := m.MakeTokenVerifier(audience)
	info, err := verifier(context.Background(), tokenStr, &http.Request{})
	require.NoError(t, err)

	assert.Equal(t, userID, info.UserID)
	assert.Equal(t, []string{"read", "write"}, info.Scopes)
	assert.False(t, info.Expiration.IsZero())
	assert.Equal(t, email, info.Extra["email"])
	assert.Equal(t, clientID, info.Extra["client_id"])
}
