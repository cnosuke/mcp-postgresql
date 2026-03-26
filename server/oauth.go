package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cnosuke/mcp-postgresql/config"
	"github.com/cockroachdb/errors"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type OAuthHandler struct {
	cfg                *config.Config
	issuer             string
	resourceURL        string
	store              *OAuthStore
	jwtMgr             *JWTManager
	googleValidator    *GoogleTokenValidator
	googleOAuth2Config *oauth2.Config
	cimdClient         *http.Client
}

func NewOAuthHandler(ctx context.Context, cfg *config.Config) *OAuthHandler {
	store := NewOAuthStore()
	store.StartCleanupLoop(ctx, 1*time.Minute)

	issuer := cfg.OAuth.NormalizedIssuer()

	return &OAuthHandler{
		cfg:             cfg,
		issuer:          issuer,
		resourceURL:     issuer + cfg.HTTP.Endpoint,
		store:           store,
		jwtMgr:          NewJWTManager(cfg.OAuth.SigningKey, issuer, cfg.OAuth.TokenExpiry),
		googleValidator: NewGoogleTokenValidator(),
		googleOAuth2Config: &oauth2.Config{
			ClientID:     cfg.OAuth.Google.ClientID,
			ClientSecret: cfg.OAuth.Google.ClientSecret,
			Scopes:       []string{"openid", "email", "profile"},
			Endpoint:     google.Endpoint,
			RedirectURL:  issuer + "/callback",
		},
		cimdClient: &http.Client{
			Timeout: 5 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return errors.New("redirects are not allowed")
			},
			Transport: &http.Transport{
				DialContext: ssrfSafeDialContext,
			},
		},
	}
}

func (h *OAuthHandler) ProtectedResourceMetadataHandler() http.Handler {
	metadata := &oauthex.ProtectedResourceMetadata{
		Resource:               h.resourceURL,
		AuthorizationServers:   []string{h.issuer},
		BearerMethodsSupported: []string{"header"},
		ResourceName:           "MCP PostgreSQL Server",
	}
	return auth.ProtectedResourceMetadataHandler(metadata)
}

func (h *OAuthHandler) ResourceMetadataURL() string {
	return h.issuer + "/.well-known/oauth-protected-resource"
}

func (h *OAuthHandler) MakeOAuthMiddleware(next http.Handler) http.Handler {
	verifier := h.jwtMgr.MakeTokenVerifier(h.resourceURL)
	middleware := auth.RequireBearerToken(verifier, &auth.RequireBearerTokenOptions{
		ResourceMetadataURL: h.ResourceMetadataURL(),
	})
	return middleware(next)
}

func (h *OAuthHandler) AuthServerMetadataHandler() http.Handler {
	metadata := AuthServerMetadata{
		Issuer:                            h.issuer,
		AuthorizationEndpoint:             h.issuer + "/authorize",
		TokenEndpoint:                     h.issuer + "/token",
		ResponseTypesSupported:            []string{"code"},
		GrantTypesSupported:               []string{"authorization_code"},
		TokenEndpointAuthMethodsSupported: []string{"none"},
		CodeChallengeMethodsSupported:     []string{"S256"},
		ClientIDMetadataDocumentSupported: true,
	}
	body, err := json.Marshal(metadata)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal auth server metadata: %v", err))
	}

	return withCORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}), "GET")
}

func (h *OAuthHandler) HandleAuthorize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()
	clientID := q.Get("client_id")
	redirectURI := q.Get("redirect_uri")
	responseType := q.Get("response_type")
	scope := q.Get("scope")
	state := q.Get("state")
	codeChallenge := q.Get("code_challenge")
	codeChallengeMethod := q.Get("code_challenge_method")
	resource := q.Get("resource")

	if responseType != "code" {
		http.Error(w, "unsupported response_type", http.StatusBadRequest)
		return
	}
	if codeChallenge == "" {
		http.Error(w, "code_challenge is required", http.StatusBadRequest)
		return
	}
	if codeChallengeMethod == "" {
		codeChallengeMethod = "S256"
	}
	if codeChallengeMethod != "S256" {
		http.Error(w, "unsupported code_challenge_method: only S256 is supported", http.StatusBadRequest)
		return
	}

	var clientName string
	if strings.HasPrefix(clientID, "https://") {
		meta, err := h.fetchCIMDMetadata(r.Context(), clientID)
		if err != nil {
			zap.S().Warnw("failed to fetch CIMD metadata", "client_id", clientID, "error", err)
			http.Error(w, "failed to fetch client metadata", http.StatusBadRequest)
			return
		}
		if !validateRedirectURI(redirectURI, meta.RedirectURIs) {
			http.Error(w, "invalid redirect_uri", http.StatusBadRequest)
			return
		}
		clientName = meta.ClientName
		if clientName == "" {
			clientName = clientID
		}
	} else {
		var found *config.PreregisteredClient
		for i := range h.cfg.OAuth.Clients {
			if h.cfg.OAuth.Clients[i].ClientID == clientID {
				found = &h.cfg.OAuth.Clients[i]
				break
			}
		}
		if found == nil {
			http.Error(w, "unknown client_id", http.StatusBadRequest)
			return
		}
		if !validateRedirectURI(redirectURI, found.RedirectURIs) {
			http.Error(w, "invalid redirect_uri", http.StatusBadRequest)
			return
		}
		clientName = found.ClientName
	}

	googleState := randomHex(32)
	csrfToken := randomHex(32)

	h.store.StorePendingAuthorization(&PendingAuthorization{
		ClientID:            clientID,
		ClientName:          clientName,
		RedirectURI:         redirectURI,
		State:               state,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
		Scope:               scope,
		Resource:            resource,
		GoogleState:         googleState,
		CSRFToken:           csrfToken,
		CreatedAt:           time.Now(),
	})

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := consentPageTemplate.Execute(w, map[string]string{
		"ClientName":  clientName,
		"RedirectURI": redirectURI,
		"Scope":       scope,
		"GoogleState": googleState,
		"CSRFToken":   csrfToken,
	}); err != nil {
		zap.S().Errorw("failed to render consent page", "error", err)
	}
}

func (h *OAuthHandler) HandleConsent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form data", http.StatusBadRequest)
		return
	}

	googleState := r.FormValue("google_state")
	csrfToken := r.FormValue("csrf_token")

	pa := h.store.GetPendingByCSRF(csrfToken)
	if pa == nil {
		http.Error(w, "invalid or expired csrf token", http.StatusBadRequest)
		return
	}
	if pa.GoogleState != googleState {
		http.Error(w, "state mismatch", http.StatusBadRequest)
		return
	}

	authURL := h.googleOAuth2Config.AuthCodeURL(googleState, oauth2.AccessTypeOnline)
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (h *OAuthHandler) HandleCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	code := r.URL.Query().Get("code")
	googleState := r.URL.Query().Get("state")

	if code == "" || googleState == "" {
		http.Error(w, "missing code or state", http.StatusBadRequest)
		return
	}

	pa := h.store.ConsumePendingByGoogleState(googleState)
	if pa == nil {
		http.Error(w, "invalid or expired state", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	token, err := h.googleOAuth2Config.Exchange(ctx, code)
	if err != nil {
		zap.S().Errorw("failed to exchange code", "error", err)
		http.Error(w, "token exchange failed", http.StatusInternalServerError)
		return
	}

	idTokenStr, ok := token.Extra("id_token").(string)
	if !ok {
		http.Error(w, "missing id_token in response", http.StatusInternalServerError)
		return
	}

	claims, err := h.googleValidator.ValidateGoogleIDToken(ctx, idTokenStr, h.cfg.OAuth.Google.ClientID)
	if err != nil {
		zap.S().Errorw("failed to validate ID token", "error", err)
		http.Error(w, "ID token validation failed", http.StatusForbidden)
		return
	}

	if err := CheckAccess(claims, h.cfg.OAuth.Google.AllowedDomains, h.cfg.OAuth.Google.AllowedEmails); err != nil {
		zap.S().Warnw("access denied", "email", claims.Email, "error", err)
		http.Error(w, "access denied", http.StatusForbidden)
		return
	}

	authCode := randomHex(32)
	h.store.StoreAuthCode(&AuthorizationCode{
		Code:                authCode,
		ClientID:            pa.ClientID,
		RedirectURI:         pa.RedirectURI,
		CodeChallenge:       pa.CodeChallenge,
		CodeChallengeMethod: pa.CodeChallengeMethod,
		Resource:            pa.Resource,
		UserID:              claims.Subject,
		Email:               claims.Email,
		CreatedAt:           time.Now(),
	})

	redirectURL, err := url.Parse(pa.RedirectURI)
	if err != nil {
		http.Error(w, "invalid redirect_uri", http.StatusInternalServerError)
		return
	}
	q := redirectURL.Query()
	q.Set("code", authCode)
	q.Set("state", pa.State)
	redirectURL.RawQuery = q.Encode()

	http.Redirect(w, r, redirectURL.String(), http.StatusFound)
}

func (h *OAuthHandler) TokenHandler() http.Handler {
	return withCORS(http.HandlerFunc(h.handleToken), "POST")
}

func (h *OAuthHandler) handleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form data", http.StatusBadRequest)
		return
	}

	grantType := r.FormValue("grant_type")
	code := r.FormValue("code")
	redirectURI := r.FormValue("redirect_uri")
	clientID := r.FormValue("client_id")
	codeVerifier := r.FormValue("code_verifier")

	if grantType != "authorization_code" {
		writeTokenError(w, "unsupported_grant_type", "only authorization_code is supported")
		return
	}

	ac := h.store.ConsumeAuthCode(code)
	if ac == nil {
		writeTokenError(w, "invalid_grant", "invalid or expired authorization code")
		return
	}

	if ac.ClientID != clientID {
		writeTokenError(w, "invalid_grant", "client_id mismatch")
		return
	}
	if ac.RedirectURI != redirectURI {
		writeTokenError(w, "invalid_grant", "redirect_uri mismatch")
		return
	}

	hash := sha256.Sum256([]byte(codeVerifier))
	computed := base64.RawURLEncoding.EncodeToString(hash[:])
	if subtle.ConstantTimeCompare([]byte(computed), []byte(ac.CodeChallenge)) != 1 {
		writeTokenError(w, "invalid_grant", "PKCE verification failed")
		return
	}

	audience := ac.Resource
	if audience == "" {
		audience = h.resourceURL
	}

	accessToken, err := h.jwtMgr.IssueAccessToken(ac.UserID, ac.Email, "mcp", audience, ac.ClientID)
	if err != nil {
		zap.S().Errorw("failed to issue access token", "error", err)
		http.Error(w, "failed to issue token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token": accessToken,
		"token_type":   "Bearer",
		"expires_in":   h.cfg.OAuth.TokenExpiry,
		"scope":        "mcp",
	})
}

func withCORS(next http.Handler, methods string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", methods+", OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeTokenError(w http.ResponseWriter, errCode, description string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":             errCode,
		"error_description": description,
	})
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	return hex.EncodeToString(b)
}

func isLoopbackRedirectURI(uri string) bool {
	u, err := url.Parse(uri)
	if err != nil {
		return false
	}
	if u.Scheme != "http" {
		return false
	}
	host := u.Hostname()
	return host == "localhost" || host == "127.0.0.1"
}

func validateRedirectURI(requestURI string, allowedURIs []string) bool {
	if isLoopbackRedirectURI(requestURI) {
		reqURL, err := url.Parse(requestURI)
		if err != nil {
			return false
		}
		for _, allowed := range allowedURIs {
			if isLoopbackRedirectURI(allowed) {
				allowedURL, err := url.Parse(allowed)
				if err != nil {
					continue
				}
				// RFC 8252: loopback URIs match on scheme+host+path, port is ignored
				if reqURL.Scheme == allowedURL.Scheme &&
					reqURL.Hostname() == allowedURL.Hostname() &&
					reqURL.Path == allowedURL.Path {
					return true
				}
			}
		}
		return false
	}

	for _, allowed := range allowedURIs {
		if requestURI == allowed {
			return true
		}
	}
	return false
}

type cimdMetadata struct {
	ClientName   string   `json:"client_name"`
	RedirectURIs []string `json:"redirect_uris"`
	ClientURI    string   `json:"client_uri"`
}

var privateNetworks = []net.IPNet{
	{IP: net.IP{10, 0, 0, 0}, Mask: net.CIDRMask(8, 32)},
	{IP: net.IP{172, 16, 0, 0}, Mask: net.CIDRMask(12, 32)},
	{IP: net.IP{192, 168, 0, 0}, Mask: net.CIDRMask(16, 32)},
	{IP: net.IP{127, 0, 0, 0}, Mask: net.CIDRMask(8, 32)},
	{IP: net.IP{169, 254, 0, 0}, Mask: net.CIDRMask(16, 32)},
}

var privateNetworks6 = []net.IPNet{
	{IP: net.ParseIP("::1"), Mask: net.CIDRMask(128, 128)},
	{IP: net.ParseIP("fe80::"), Mask: net.CIDRMask(10, 128)},
}

func isPrivateIP(ip net.IP) bool {
	if ip4 := ip.To4(); ip4 != nil {
		for _, n := range privateNetworks {
			if n.Contains(ip4) {
				return true
			}
		}
		return false
	}
	for _, n := range privateNetworks6 {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func (h *OAuthHandler) fetchCIMDMetadata(ctx context.Context, clientIDURL string) (*cimdMetadata, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, clientIDURL, nil)
	if err != nil {
		return nil, errors.Wrap(err, "creating request")
	}

	resp, err := h.cimdClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "fetching CIMD")
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Newf("CIMD returned status %d", resp.StatusCode)
	}

	var meta cimdMetadata
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&meta); err != nil {
		return nil, errors.Wrap(err, "decoding CIMD response")
	}
	return &meta, nil
}

func ssrfSafeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	for _, ip := range ips {
		if isPrivateIP(ip.IP) {
			return nil, fmt.Errorf("SSRF protection: connection to private IP %s blocked", ip.IP)
		}
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no IP addresses found for %s", host)
	}
	return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
}

var consentPageTemplate = template.Must(template.New("consent").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Authorization Request</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; max-width: 480px; margin: 60px auto; padding: 0 20px; color: #333; }
  h1 { font-size: 1.4em; }
  .info { background: #f5f5f5; padding: 16px; border-radius: 8px; margin: 20px 0; }
  .info dt { font-weight: 600; margin-top: 8px; }
  .info dd { margin: 4px 0 0 0; word-break: break-all; }
  button { background: #2563eb; color: #fff; border: none; padding: 12px 32px; border-radius: 6px; font-size: 1em; cursor: pointer; }
  button:hover { background: #1d4ed8; }
</style>
</head>
<body>
<h1>Authorization Request</h1>
<div class="info">
  <dl>
    <dt>Application</dt>
    <dd>{{.ClientName}}</dd>
    <dt>Redirect URI</dt>
    <dd>{{.RedirectURI}}</dd>
    <dt>Requested Scope</dt>
    <dd>{{.Scope}}</dd>
  </dl>
</div>
<form method="POST" action="/consent">
  <input type="hidden" name="google_state" value="{{.GoogleState}}">
  <input type="hidden" name="csrf_token" value="{{.CSRFToken}}">
  <button type="submit">Approve</button>
</form>
</body>
</html>`))
