package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{
			name:   "valid bearer token",
			header: "Bearer my-secret-token",
			want:   "my-secret-token",
		},
		{
			name:   "empty header",
			header: "",
			want:   "",
		},
		{
			name:   "non-bearer scheme",
			header: "Basic dXNlcjpwYXNz",
			want:   "",
		},
		{
			name:   "bearer lowercase",
			header: "bearer my-token",
			want:   "my-token",
		},
		{
			name:   "bearer mixed case",
			header: "BEARER my-token",
			want:   "my-token",
		},
		{
			name:   "only prefix without token",
			header: "Bearer ",
			want:   "",
		},
		{
			name:   "malformed - no space",
			header: "Bearertoken",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				r.Header.Set("Authorization", tt.header)
			}
			got := extractBearerToken(r)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestWithAuthMiddleware(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("auth disabled (empty token)", func(t *testing.T) {
		handler := withAuthMiddleware(okHandler, "")
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("valid token", func(t *testing.T) {
		handler := withAuthMiddleware(okHandler, "secret")
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer secret")
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("invalid token", func(t *testing.T) {
		handler := withAuthMiddleware(okHandler, "secret")
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer wrong")
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("no authorization header", func(t *testing.T) {
		handler := withAuthMiddleware(okHandler, "secret")
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

func TestWithOriginValidation(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("empty allowed origins (all permitted)", func(t *testing.T) {
		handler := withOriginValidation(okHandler, nil)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Origin", "https://evil.example.com")
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("allowed origin", func(t *testing.T) {
		handler := withOriginValidation(okHandler, []string{"https://example.com"})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Origin", "https://example.com")
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("disallowed origin", func(t *testing.T) {
		handler := withOriginValidation(okHandler, []string{"https://example.com"})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Origin", "https://evil.example.com")
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("no origin header (non-browser client)", func(t *testing.T) {
		handler := withOriginValidation(okHandler, []string{"https://example.com"})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("origin with trailing slash", func(t *testing.T) {
		handler := withOriginValidation(okHandler, []string{"https://example.com"})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Origin", "https://example.com/")
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})
}
