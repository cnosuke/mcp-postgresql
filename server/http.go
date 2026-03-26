package server

import (
	"context"
	"fmt"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/cnosuke/mcp-postgresql/config"
	"github.com/cockroachdb/errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"
)

// RunHTTP starts the MCP server with Streamable HTTP transport
func RunHTTP(cfg *config.Config, name, version, revision string) error {
	zap.S().Infow("starting MCP PostgreSQL Server (HTTP mode)")

	mcpServer, err := NewMCPServer(cfg, name, version, revision)
	if err != nil {
		return err
	}

	cop := &http.CrossOriginProtection{}
	for _, origin := range cfg.HTTP.AllowedOrigins {
		if err := cop.AddTrustedOrigin(origin); err != nil {
			return fmt.Errorf("invalid allowed origin %q: %w", origin, err)
		}
	}

	httpHandler := mcp.NewStreamableHTTPHandler(
		func(_ *http.Request) *mcp.Server { return mcpServer },
		&mcp.StreamableHTTPOptions{
			CrossOriginProtection: cop,
		},
	)

	handler := withRequestLogging(
		withOriginValidation(
			withAuthMiddleware(httpHandler, cfg.HTTP.AuthToken),
			cfg.HTTP.AllowedOrigins,
		),
	)

	mux := http.NewServeMux()
	mux.Handle(cfg.HTTP.Endpoint, handler)
	mux.HandleFunc("/health", handleHealth)

	addr := fmt.Sprintf("%s:%d", cfg.HTTP.Host, cfg.HTTP.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		zap.S().Infow("HTTP server listening",
			"addr", addr,
			"endpoint", cfg.HTTP.Endpoint,
		)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- errors.Wrap(err, "HTTP server error")
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		zap.S().Infow("shutting down HTTP server")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return errors.Wrap(err, "HTTP server shutdown error")
	}

	zap.S().Infow("HTTP server stopped")
	return nil
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}` + "\n"))
}
