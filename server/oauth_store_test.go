package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStorePendingAuthorization(t *testing.T) {
	store := NewOAuthStore()

	pa := &PendingAuthorization{
		ClientID:            "client-1",
		ClientName:          "Test Client",
		RedirectURI:         "https://example.com/callback",
		State:               "original-state",
		CodeChallenge:       "challenge-abc",
		CodeChallengeMethod: "S256",
		Scope:               "openid email",
		Resource:            "https://resource.example.com",
		GoogleState:         "google-state-123",
		CSRFToken:           "csrf-token-456",
		CreatedAt:           time.Now(),
	}

	store.StorePendingAuthorization(pa)

	got := store.ConsumePendingByCSRF("csrf-token-456")
	require.NotNil(t, got)
	assert.Equal(t, "client-1", got.ClientID)
	assert.Equal(t, "Test Client", got.ClientName)
	assert.Equal(t, "https://example.com/callback", got.RedirectURI)
	assert.Equal(t, "original-state", got.State)
	assert.Equal(t, "challenge-abc", got.CodeChallenge)
	assert.Equal(t, "S256", got.CodeChallengeMethod)
	assert.Equal(t, "openid email", got.Scope)
	assert.Equal(t, "https://resource.example.com", got.Resource)
	assert.Equal(t, "google-state-123", got.GoogleState)
	assert.Equal(t, "csrf-token-456", got.CSRFToken)
}

func TestConsumePendingByGoogleState(t *testing.T) {
	store := NewOAuthStore()

	pa := &PendingAuthorization{
		ClientID:    "client-1",
		GoogleState: "google-state-789",
		CSRFToken:   "csrf-token-abc",
		CreatedAt:   time.Now(),
	}

	store.StorePendingAuthorization(pa)

	got := store.ConsumePendingByGoogleState("google-state-789")
	require.NotNil(t, got)
	assert.Equal(t, "client-1", got.ClientID)

	got2 := store.ConsumePendingByGoogleState("google-state-789")
	assert.Nil(t, got2)
}

func TestGetPendingByCSRFNotFound(t *testing.T) {
	store := NewOAuthStore()

	got := store.ConsumePendingByCSRF("unknown-csrf-token")
	assert.Nil(t, got)
}

func TestStoreAndConsumeAuthCode(t *testing.T) {
	store := NewOAuthStore()

	ac := &AuthorizationCode{
		Code:                "auth-code-123",
		ClientID:            "client-1",
		RedirectURI:         "https://example.com/callback",
		CodeChallenge:       "challenge-xyz",
		CodeChallengeMethod: "S256",
		Resource:            "https://resource.example.com",
		UserID:              "user-42",
		Email:               "user@example.com",
		CreatedAt:           time.Now(),
	}

	store.StoreAuthCode(ac)

	got := store.ConsumeAuthCode("auth-code-123")
	require.NotNil(t, got)
	assert.Equal(t, "client-1", got.ClientID)
	assert.Equal(t, "user-42", got.UserID)
	assert.Equal(t, "user@example.com", got.Email)

	store.ConsumeAuthCode("auth-code-123")
	got2 := store.ConsumeAuthCode("auth-code-123")
	assert.Nil(t, got2)
}

func TestConsumeAuthCodeNotFound(t *testing.T) {
	store := NewOAuthStore()

	got := store.ConsumeAuthCode("unknown-code")
	assert.Nil(t, got)
}

func TestCleanupExpiredPending(t *testing.T) {
	store := NewOAuthStore()

	pa := &PendingAuthorization{
		ClientID:    "client-1",
		GoogleState: "expired-state",
		CSRFToken:   "expired-csrf",
		CreatedAt:   time.Now().Add(-11 * time.Minute),
	}

	store.StorePendingAuthorization(pa)

	store.Cleanup()

	got := store.ConsumePendingByCSRF("expired-csrf")
	assert.Nil(t, got)

	got2 := store.ConsumePendingByGoogleState("expired-state")
	assert.Nil(t, got2)
}

func TestCleanupExpiredAuthCode(t *testing.T) {
	store := NewOAuthStore()

	ac := &AuthorizationCode{
		Code:      "expired-code",
		ClientID:  "client-1",
		CreatedAt: time.Now().Add(-6 * time.Minute),
	}

	store.StoreAuthCode(ac)

	store.Cleanup()

	got := store.ConsumeAuthCode("expired-code")
	assert.Nil(t, got)
}

func TestCleanupKeepsFresh(t *testing.T) {
	store := NewOAuthStore()

	pa := &PendingAuthorization{
		ClientID:    "client-fresh",
		GoogleState: "fresh-state",
		CSRFToken:   "fresh-csrf",
		CreatedAt:   time.Now(),
	}
	store.StorePendingAuthorization(pa)

	ac := &AuthorizationCode{
		Code:      "fresh-code",
		ClientID:  "client-fresh",
		CreatedAt: time.Now(),
	}
	store.StoreAuthCode(ac)

	store.Cleanup()

	gotPA := store.ConsumePendingByCSRF("fresh-csrf")
	require.NotNil(t, gotPA)
	assert.Equal(t, "client-fresh", gotPA.ClientID)

	gotAC := store.ConsumeAuthCode("fresh-code")
	require.NotNil(t, gotAC)
	assert.Equal(t, "client-fresh", gotAC.ClientID)
}
