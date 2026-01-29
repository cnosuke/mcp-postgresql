package server

import (
	"context"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/cnosuke/mcp-postgresql/config"
	"github.com/cockroachdb/errors"
	"github.com/jmoiron/sqlx"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/xo/dburl"
	"go.uber.org/zap"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// DBManager manages database connections with thread safety
type DBManager struct {
	mu          sync.RWMutex
	connections map[string]*sqlx.DB
	readOnly    bool
}

var (
	dbManager = &DBManager{
		connections: make(map[string]*sqlx.DB),
	}
)

// Run - Execute the MCP server
func Run(cfg *config.Config, name string, version string, revision string) error {
	zap.S().Infow("starting MCP PostgreSQL Server")

	// Set read-only mode in DBManager
	dbManager.SetReadOnly(cfg.PostgreSQL.ReadOnly)

	// Format version string with revision if available
	versionString := version
	if revision != "" && revision != "xxx" {
		versionString = versionString + " (" + revision + ")"
	}

	// Create custom hooks for error handling
	hooks := &server.Hooks{}
	hooks.AddOnError(func(ctx context.Context, id any, method mcp.MCPMethod, message any, err error) {
		zap.S().Errorw("MCP error occurred",
			"id", id,
			"method", method,
			"error", err,
		)
	})

	// Create MCP server with server name and version
	zap.S().Debugw("creating MCP server",
		"name", name,
		"version", versionString,
	)
	mcpServer := server.NewMCPServer(
		name,
		versionString,
		server.WithHooks(hooks),
	)

	// Register all tools
	zap.S().Debugw("registering PostgreSQL tools")
	if err := RegisterAllTools(mcpServer, cfg); err != nil {
		zap.S().Errorw("failed to register tools", "error", err)
		return err
	}

	// Start the server with stdio transport
	zap.S().Infow("starting MCP server")
	err := server.ServeStdio(mcpServer)
	if err != nil {
		zap.S().Errorw("failed to start server", "error", err)
		return errors.Wrap(err, "failed to start server")
	}

	// ServeStdio will block until the server is terminated
	zap.S().Infow("server shutting down")
	return nil
}

// SetReadOnly sets the read-only mode for the DBManager
func (m *DBManager) SetReadOnly(readOnly bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.readOnly = readOnly
}

// GetDB returns a database connection for the given DSN with thread safety
func (m *DBManager) GetDB(ctx context.Context, cfg *config.Config, toolDSN string) (*sqlx.DB, error) {
	dsn, err := m.resolveDSN(cfg, toolDSN)
	if err != nil {
		return nil, err
	}

	// Fast path: check if connection already exists
	m.mu.RLock()
	if db, ok := m.connections[dsn]; ok {
		m.mu.RUnlock()
		return db, nil
	}
	m.mu.RUnlock()

	// Slow path: create new connection
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if db, ok := m.connections[dsn]; ok {
		return db, nil
	}

	db, err := m.createConnection(ctx, cfg, dsn)
	if err != nil {
		return nil, err
	}

	m.connections[dsn] = db
	return db, nil
}

// resolveDSN determines the DSN to use based on config and tool parameter
func (m *DBManager) resolveDSN(cfg *config.Config, toolDSN string) (string, error) {
	dsn := toolDSN
	if dsn == "" {
		dsn = cfg.PostgreSQL.DSN
		if dsn == "" {
			if cfg.PostgreSQL.Host == "" || cfg.PostgreSQL.User == "" {
				return "", fmt.Errorf("PostgreSQL connection information is required. Please provide a valid DSN parameter or configure PostgreSQL connection in config file")
			}

			dbname := cfg.PostgreSQL.Database
			if dbname == "" {
				dbname = "postgres"
			}

			dsn = fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
				cfg.PostgreSQL.Host,
				cfg.PostgreSQL.Port,
				cfg.PostgreSQL.User,
				cfg.PostgreSQL.Password,
				dbname,
				cfg.PostgreSQL.SSLMode)

			if cfg.PostgreSQL.Schema != "" {
				dsn += fmt.Sprintf(" search_path=%s", cfg.PostgreSQL.Schema)
			}
		}
	}

	if isURLStyle(dsn) {
		u, err := dburl.Parse(dsn)
		if err != nil {
			return "", fmt.Errorf("failed to parse database URL: %v", err)
		}
		dsn = u.DSN
		zap.S().Debugw("converted URL-style DSN to native PostgreSQL DSN", "dsn", sanitizeDSNForLog(dsn))
	}

	return dsn, nil
}

// createConnection creates a new database connection with proper configuration
func (m *DBManager) createConnection(ctx context.Context, cfg *config.Config, dsn string) (*sqlx.DB, error) {
	db, err := sqlx.ConnectContext(ctx, "pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to establish database connection: %v", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	// Set session to read-only if configured
	if m.readOnly {
		if _, err := db.ExecContext(ctx, "SET SESSION CHARACTERISTICS AS TRANSACTION READ ONLY"); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to set read-only mode: %v", err)
		}
		zap.S().Debugw("set session to read-only mode")
	}

	return db, nil
}

// Close closes all database connections
func (m *DBManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error
	for dsn, db := range m.connections {
		if err := db.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close connection for %s: %w", dsn, err))
		}
		delete(m.connections, dsn)
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// CloseDB closes all database connections (package-level function)
func CloseDB() error {
	return dbManager.Close()
}

// GetDB - Get database connection (legacy wrapper for compatibility)
func GetDB(cfg *config.Config, toolDSN string) (*sqlx.DB, error) {
	return dbManager.GetDB(context.Background(), cfg, toolDSN)
}

// GetDBContext - Get database connection with context
func GetDBContext(ctx context.Context, cfg *config.Config, toolDSN string) (*sqlx.DB, error) {
	return dbManager.GetDB(ctx, cfg, toolDSN)
}

// isURLStyle checks if the DSN is likely a URL-style connection string
// rather than a native PostgreSQL DSN format
func isURLStyle(dsn string) bool {
	prefixes := []string{"postgres://", "postgresql://"}
	for _, prefix := range prefixes {
		if len(dsn) >= len(prefix) && dsn[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

var (
	dsnPasswordRegex    = regexp.MustCompile(`password=([^ ]+)`)
	dsnURLPasswordRegex = regexp.MustCompile(`://([^:]+):([^@]+)@`)
)

// sanitizeDSNForLog removes sensitive information from DSN for safe logging
func sanitizeDSNForLog(dsn string) string {
	sanitized := dsnPasswordRegex.ReplaceAllString(dsn, "password=***")
	sanitized = dsnURLPasswordRegex.ReplaceAllString(sanitized, "://$1:***@")
	return sanitized
}
