package server

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestStatusWriter(t *testing.T) {
	t.Run("captures status code", func(t *testing.T) {
		rec := httptest.NewRecorder()
		sw := &statusWriter{ResponseWriter: rec, code: http.StatusOK}
		sw.WriteHeader(http.StatusNotFound)
		assert.Equal(t, http.StatusNotFound, sw.code)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("default status is 200", func(t *testing.T) {
		rec := httptest.NewRecorder()
		sw := &statusWriter{ResponseWriter: rec, code: http.StatusOK}
		sw.Write([]byte("hello"))
		assert.Equal(t, http.StatusOK, sw.code)
	})

	t.Run("flush support", func(t *testing.T) {
		rec := httptest.NewRecorder()
		sw := &statusWriter{ResponseWriter: rec, code: http.StatusOK}
		sw.Flush()
		assert.True(t, rec.Flushed)
	})
}

func TestPeekJSONRPCRequest(t *testing.T) {
	t.Run("nil body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Body = nil
		info, err := peekJSONRPCRequest(req)
		assert.NoError(t, err)
		assert.Nil(t, info)
	})

	t.Run("empty body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte{}))
		info, err := peekJSONRPCRequest(req)
		assert.NoError(t, err)
		assert.Nil(t, info)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte("not json")))
		info, err := peekJSONRPCRequest(req)
		assert.NoError(t, err)
		assert.Nil(t, info)

		restored, _ := io.ReadAll(req.Body)
		assert.Equal(t, "not json", string(restored))
	})

	t.Run("extracts rpc method", func(t *testing.T) {
		body := `{"jsonrpc":"2.0","method":"initialize","id":1}`
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(body)))
		info, err := peekJSONRPCRequest(req)
		require.NoError(t, err)
		assert.Equal(t, "initialize", info.Method)
		assert.Empty(t, info.Tool)
	})

	t.Run("extracts tool name from tools/call", func(t *testing.T) {
		body := `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"read_query","arguments":{"query":"SELECT 1"}}}`
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(body)))
		info, err := peekJSONRPCRequest(req)
		require.NoError(t, err)
		assert.Equal(t, "tools/call", info.Method)
		assert.Equal(t, "read_query", info.Tool)
		assert.Greater(t, info.ParamsSize, 0)
	})

	t.Run("body is restored after peek", func(t *testing.T) {
		body := `{"jsonrpc":"2.0","method":"initialize","id":1}`
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(body)))
		_, err := peekJSONRPCRequest(req)
		require.NoError(t, err)

		restored, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		assert.Equal(t, body, string(restored))
	})
}

func TestWithRequestLogging(t *testing.T) {
	t.Run("passes through to next handler", func(t *testing.T) {
		called := false
		inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		})

		handler := withRequestLogging(inner)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		handler.ServeHTTP(rec, req)

		assert.True(t, called)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "ok", rec.Body.String())
	})

	t.Run("captures non-200 status", func(t *testing.T) {
		inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})

		handler := withRequestLogging(inner)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})

	t.Run("body available to inner handler after peek", func(t *testing.T) {
		body := `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"list_table"}}`
		var innerBody []byte
		inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			innerBody, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
		})

		handler := withRequestLogging(inner)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader([]byte(body)))
		handler.ServeHTTP(rec, req)

		assert.Equal(t, body, string(innerBody))
	})
}
