package server

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

// withOriginValidation wraps an http.Handler with Origin header validation.
// Per MCP spec 2025-11-25, servers MUST validate the Origin header and return
// 403 Forbidden if it does not match the allowed origins.
// If allowedOrigins is empty, all origins are permitted (development mode).
// If the Origin header is absent, the request is allowed (non-browser clients).
func withOriginValidation(next http.Handler, allowedOrigins []string) http.Handler {
	if len(allowedOrigins) == 0 {
		return next
	}

	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[strings.TrimRight(o, "/")] = struct{}{}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}

		normalized := strings.TrimRight(origin, "/")
		if _, ok := allowed[normalized]; !ok {
			zap.S().Warnw("rejected request with disallowed origin",
				"origin", origin,
			)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// withAuthMiddleware wraps an http.Handler with Bearer token authentication.
// If authToken is empty, authentication is disabled and the handler is returned as-is.
// Token comparison uses crypto/subtle.ConstantTimeCompare to prevent timing attacks.
func withAuthMiddleware(next http.Handler, authToken string) http.Handler {
	if authToken == "" {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r)
		if token == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		if subtle.ConstantTimeCompare([]byte(token), []byte(authToken)) != 1 {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// extractBearerToken extracts the Bearer token from the Authorization header.
func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}

	const prefix = "Bearer "
	if len(auth) < len(prefix) || !strings.EqualFold(auth[:len(prefix)], prefix) {
		return ""
	}

	return auth[len(prefix):]
}

// statusWriter wraps http.ResponseWriter to capture the status code.
type statusWriter struct {
	http.ResponseWriter
	code        int
	wroteHeader bool
}

func (w *statusWriter) WriteHeader(code int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.wroteHeader = true
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// jsonRPCInfo holds extracted JSON-RPC request metadata.
type jsonRPCInfo struct {
	Method     string
	Tool       string
	ParamsSize int
}

// peekJSONRPCRequest reads the request body, extracts JSON-RPC metadata,
// and restores the body for downstream handlers.
func peekJSONRPCRequest(r *http.Request) (*jsonRPCInfo, error) {
	if r.Body == nil {
		return nil, nil
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		r.Body = io.NopCloser(bytes.NewReader(nil))
		return nil, err
	}
	r.Body = io.NopCloser(bytes.NewReader(body))

	if len(body) == 0 {
		return nil, nil
	}

	// Only parse the first 1MB for metadata extraction to limit memory usage
	parseBody := body
	if len(parseBody) > 1<<20 {
		parseBody = parseBody[:1<<20]
	}

	var req struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(parseBody, &req); err != nil {
		return nil, nil
	}

	info := &jsonRPCInfo{
		Method:     req.Method,
		ParamsSize: len(req.Params),
	}

	if req.Method == "tools/call" && len(req.Params) > 0 {
		var toolCall struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(req.Params, &toolCall); err == nil {
			info.Tool = toolCall.Name
		}
	}

	return info, nil
}

// withRequestLogging wraps an http.Handler with request logging.
func withRequestLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		rpcInfo, _ := peekJSONRPCRequest(r)

		sw := &statusWriter{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(sw, r)

		latency := time.Since(start)
		fields := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"remote_addr", r.RemoteAddr,
			"status", sw.code,
			"latency_ms", latency.Milliseconds(),
		}

		if rpcInfo != nil {
			if rpcInfo.Method != "" {
				fields = append(fields, "rpc_method", rpcInfo.Method)
			}
			if rpcInfo.Tool != "" {
				fields = append(fields, "tool", rpcInfo.Tool)
			}
			if rpcInfo.ParamsSize > 0 {
				fields = append(fields, "params_bytes", rpcInfo.ParamsSize)
			}
		}

		switch {
		case sw.code >= 500:
			zap.S().Errorw("HTTP request", fields...)
		case sw.code >= 400:
			zap.S().Warnw("HTTP request", fields...)
		default:
			zap.S().Infow("HTTP request", fields...)
		}
	})
}
