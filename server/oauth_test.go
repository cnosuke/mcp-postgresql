package server

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/cnosuke/mcp-postgresql/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

func testOAuthHandler() *OAuthHandler {
	cfg := &config.Config{}
	cfg.OAuth = config.OAuthConfig{
		Enabled:     true,
		Issuer:      "https://test.example.com",
		SigningKey:   "test-signing-key-that-is-at-least-32-bytes-long",
		TokenExpiry: 3600,
		Google: config.GoogleOAuthConfig{
			ClientID:     "test-google-client-id",
			ClientSecret: "test-google-secret",
		},
		Clients: []config.PreregisteredClient{
			{
				ClientID:     "test-client",
				ClientName:   "Test Client",
				RedirectURIs: []string{"https://example.com/callback", "http://localhost:3000/callback"},
			},
		},
	}
	cfg.HTTP.Endpoint = "/mcp"

	issuer := cfg.OAuth.NormalizedIssuer()
	return &OAuthHandler{
		cfg:             cfg,
		issuer:          issuer,
		resourceURL:     issuer + cfg.HTTP.Endpoint,
		store:           NewOAuthStore(),
		jwtMgr:          NewJWTManager(cfg.OAuth.SigningKey, issuer, cfg.OAuth.TokenExpiry),
		googleValidator: NewGoogleTokenValidator(),
		googleOAuth2Config: &oauth2.Config{
			ClientID:     cfg.OAuth.Google.ClientID,
			ClientSecret: cfg.OAuth.Google.ClientSecret,
			Endpoint:     google.Endpoint,
			RedirectURL:  issuer + "/callback",
		},
	}
}

func TestHandleAuthServerMetadata(t *testing.T) {
	h := testOAuthHandler()

	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil)
	rec := httptest.NewRecorder()

	h.AuthServerMetadataHandler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var meta AuthServerMetadata
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&meta))

	assert.Equal(t, "https://test.example.com", meta.Issuer)
	assert.Equal(t, "https://test.example.com/authorize", meta.AuthorizationEndpoint)
	assert.Equal(t, "https://test.example.com/token", meta.TokenEndpoint)
	assert.Equal(t, []string{"code"}, meta.ResponseTypesSupported)
	assert.Equal(t, []string{"authorization_code"}, meta.GrantTypesSupported)
	assert.Equal(t, []string{"none"}, meta.TokenEndpointAuthMethodsSupported)
	assert.Equal(t, []string{"S256"}, meta.CodeChallengeMethodsSupported)
	assert.True(t, meta.ClientIDMetadataDocumentSupported)
}

func TestHandleAuthServerMetadataOptions(t *testing.T) {
	h := testOAuthHandler()

	req := httptest.NewRequest(http.MethodOptions, "/.well-known/oauth-authorization-server", nil)
	rec := httptest.NewRecorder()

	h.AuthServerMetadataHandler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestHandleAuthServerMetadataMethodNotAllowed(t *testing.T) {
	h := testOAuthHandler()

	req := httptest.NewRequest(http.MethodPost, "/.well-known/oauth-authorization-server", nil)
	rec := httptest.NewRecorder()

	h.AuthServerMetadataHandler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestProtectedResourceMetadataHandler(t *testing.T) {
	h := testOAuthHandler()

	handler := h.ProtectedResourceMetadataHandler()
	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))

	assert.Equal(t, "https://test.example.com/mcp", body["resource"])
	assert.Equal(t, []any{"https://test.example.com"}, body["authorization_servers"])
	assert.Equal(t, []any{"header"}, body["bearer_methods_supported"])
	assert.Equal(t, "MCP PostgreSQL Server", body["resource_name"])
}

func TestHandleAuthorize_PreregisteredClient(t *testing.T) {
	h := testOAuthHandler()

	params := url.Values{
		"client_id":             {"test-client"},
		"redirect_uri":         {"https://example.com/callback"},
		"response_type":        {"code"},
		"scope":                {"mcp"},
		"state":                {"test-state"},
		"code_challenge":       {"test-challenge"},
		"code_challenge_method": {"S256"},
	}
	req := httptest.NewRequest(http.MethodGet, "/authorize?"+params.Encode(), nil)
	rec := httptest.NewRecorder()

	h.HandleAuthorize(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "text/html")
	assert.Contains(t, rec.Body.String(), "Test Client")
	assert.Contains(t, rec.Body.String(), "Authorization Request")
}

func TestHandleAuthorize_MissingCodeChallenge(t *testing.T) {
	h := testOAuthHandler()

	params := url.Values{
		"client_id":      {"test-client"},
		"redirect_uri":  {"https://example.com/callback"},
		"response_type": {"code"},
	}
	req := httptest.NewRequest(http.MethodGet, "/authorize?"+params.Encode(), nil)
	rec := httptest.NewRecorder()

	h.HandleAuthorize(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "code_challenge is required")
}

func TestHandleAuthorize_UnknownClient(t *testing.T) {
	h := testOAuthHandler()

	params := url.Values{
		"client_id":      {"unknown-client"},
		"redirect_uri":  {"https://example.com/callback"},
		"response_type": {"code"},
		"code_challenge": {"test-challenge"},
	}
	req := httptest.NewRequest(http.MethodGet, "/authorize?"+params.Encode(), nil)
	rec := httptest.NewRecorder()

	h.HandleAuthorize(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "unknown client_id")
}

func TestHandleAuthorize_InvalidRedirectURI(t *testing.T) {
	h := testOAuthHandler()

	params := url.Values{
		"client_id":      {"test-client"},
		"redirect_uri":  {"https://evil.com/callback"},
		"response_type": {"code"},
		"code_challenge": {"test-challenge"},
	}
	req := httptest.NewRequest(http.MethodGet, "/authorize?"+params.Encode(), nil)
	rec := httptest.NewRecorder()

	h.HandleAuthorize(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid redirect_uri")
}

func TestHandleToken_FullFlow(t *testing.T) {
	h := testOAuthHandler()

	codeVerifier := "test-code-verifier-that-is-long-enough-for-pkce"
	hash := sha256.Sum256([]byte(codeVerifier))
	codeChallenge := base64.RawURLEncoding.EncodeToString(hash[:])

	authCode := "test-auth-code-12345"
	h.store.StoreAuthCode(&AuthorizationCode{
		Code:                authCode,
		ClientID:            "test-client",
		RedirectURI:         "https://example.com/callback",
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: "S256",
		Resource:            "",
		UserID:              "user-123",
		Email:               "test@example.com",
		CreatedAt:           time.Now(),
	})

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {authCode},
		"redirect_uri":  {"https://example.com/callback"},
		"client_id":     {"test-client"},
		"code_verifier": {codeVerifier},
	}
	req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	h.TokenHandler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var tokenResp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&tokenResp))

	assert.NotEmpty(t, tokenResp["access_token"])
	assert.Equal(t, "Bearer", tokenResp["token_type"])
	assert.Equal(t, float64(3600), tokenResp["expires_in"])
	assert.Equal(t, "mcp", tokenResp["scope"])

	accessToken := tokenResp["access_token"].(string)
	expectedAudience := "https://test.example.com/mcp"
	claims, err := h.jwtMgr.VerifyAccessToken(accessToken, expectedAudience)
	require.NoError(t, err)
	assert.Equal(t, "user-123", claims.Subject)
	assert.Equal(t, "test@example.com", claims.Email)
	assert.Equal(t, "test-client", claims.ClientID)
	assert.Equal(t, "mcp", claims.Scope)
}

func TestHandleToken_InvalidCode(t *testing.T) {
	h := testOAuthHandler()

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {"nonexistent-code"},
		"redirect_uri":  {"https://example.com/callback"},
		"client_id":     {"test-client"},
		"code_verifier": {"some-verifier"},
	}
	req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	h.TokenHandler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid_grant")
}

func TestHandleToken_PKCEFailure(t *testing.T) {
	h := testOAuthHandler()

	correctVerifier := "correct-verifier-long-enough-for-testing"
	hash := sha256.Sum256([]byte(correctVerifier))
	codeChallenge := base64.RawURLEncoding.EncodeToString(hash[:])

	authCode := "pkce-test-code"
	h.store.StoreAuthCode(&AuthorizationCode{
		Code:                authCode,
		ClientID:            "test-client",
		RedirectURI:         "https://example.com/callback",
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: "S256",
		UserID:              "user-123",
		Email:               "test@example.com",
		CreatedAt:           time.Now(),
	})

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {authCode},
		"redirect_uri":  {"https://example.com/callback"},
		"client_id":     {"test-client"},
		"code_verifier": {"wrong-verifier"},
	}
	req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	h.TokenHandler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "PKCE verification failed")
}

func TestHandleToken_ClientIDMismatch(t *testing.T) {
	h := testOAuthHandler()

	codeVerifier := "verifier-for-client-mismatch-test"
	hash := sha256.Sum256([]byte(codeVerifier))
	codeChallenge := base64.RawURLEncoding.EncodeToString(hash[:])

	authCode := "client-mismatch-code"
	h.store.StoreAuthCode(&AuthorizationCode{
		Code:                authCode,
		ClientID:            "test-client",
		RedirectURI:         "https://example.com/callback",
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: "S256",
		UserID:              "user-123",
		Email:               "test@example.com",
		CreatedAt:           time.Now(),
	})

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {authCode},
		"redirect_uri":  {"https://example.com/callback"},
		"client_id":     {"wrong-client"},
		"code_verifier": {codeVerifier},
	}
	req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	h.TokenHandler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "client_id mismatch")
}

func TestIsLoopbackRedirectURI(t *testing.T) {
	tests := []struct {
		name string
		uri  string
		want bool
	}{
		{"localhost http", "http://localhost:3000/callback", true},
		{"127.0.0.1 http", "http://127.0.0.1:8080/callback", true},
		{"localhost no port", "http://localhost/callback", true},
		{"127.0.0.1 no port", "http://127.0.0.1/callback", true},
		{"https localhost", "https://localhost:3000/callback", false},
		{"https 127.0.0.1", "https://127.0.0.1:8080/callback", false},
		{"external host", "http://example.com/callback", false},
		{"https external", "https://example.com/callback", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isLoopbackRedirectURI(tt.uri))
		})
	}
}

func TestValidateRedirectURI(t *testing.T) {
	allowed := []string{"https://example.com/callback", "http://localhost:3000/callback"}

	tests := []struct {
		name    string
		uri     string
		allowed []string
		want    bool
	}{
		{"exact match https", "https://example.com/callback", allowed, true},
		{"exact match localhost", "http://localhost:3000/callback", allowed, true},
		{"loopback different port", "http://localhost:9999/callback", allowed, true},
		{"loopback 127.0.0.1 no match", "http://127.0.0.1:5000/callback", allowed, false},
		{"loopback 127.0.0.1 with allowed", "http://127.0.0.1:5000/callback", append(allowed, "http://127.0.0.1:3000/callback"), true},
		{"no match", "https://evil.com/callback", allowed, false},
		{"no match different path", "https://example.com/other", allowed, false},
		{"empty allowed list", "https://example.com/callback", []string{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, validateRedirectURI(tt.uri, tt.allowed))
		})
	}
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{"10.0.0.1", "10.0.0.1", true},
		{"10.255.255.255", "10.255.255.255", true},
		{"172.16.0.1", "172.16.0.1", true},
		{"172.31.255.255", "172.31.255.255", true},
		{"192.168.0.1", "192.168.0.1", true},
		{"192.168.255.255", "192.168.255.255", true},
		{"127.0.0.1", "127.0.0.1", true},
		{"127.0.0.2", "127.0.0.2", true},
		{"::1", "::1", true},
		{"8.8.8.8", "8.8.8.8", false},
		{"1.1.1.1", "1.1.1.1", false},
		{"203.0.113.1", "203.0.113.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			require.NotNil(t, ip, "failed to parse IP: %s", tt.ip)
			assert.Equal(t, tt.want, isPrivateIP(ip))
		})
	}
}
