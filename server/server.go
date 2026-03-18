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
	"github.com/modelcontextprotocol/go-sdk/mcp"
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

// NewMCPServer creates and configures an MCPServer with all tools registered
func NewMCPServer(cfg *config.Config, name, version, revision string) (*mcp.Server, error) {
	// Set read-only mode in DBManager
	dbManager.SetReadOnly(cfg.PostgreSQL.ReadOnly)

	// Format version string with revision if available
	versionString := version
	if revision != "" && revision != "xxx" {
		versionString = versionString + " (" + revision + ")"
	}

	zap.S().Debugw("creating MCP server",
		"name", name,
		"version", versionString,
	)
	mcpServer := mcp.NewServer(
		&mcp.Implementation{Name: name, Version: versionString},
		nil,
	)

	zap.S().Debugw("registering PostgreSQL tools")
	if err := RegisterAllTools(mcpServer, cfg); err != nil {
		zap.S().Errorw("failed to register tools", "error", err)
		return nil, err
	}

	return mcpServer, nil
}

// Run executes the MCP server with stdio transport
func Run(cfg *config.Config, name string, version string, revision string) error {
	zap.S().Infow("starting MCP PostgreSQL Server")

	mcpServer, err := NewMCPServer(cfg, name, version, revision)
	if err != nil {
		return err
	}

	zap.S().Infow("starting MCP server with stdio transport")
	if err := mcpServer.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		zap.S().Errorw("failed to start server", "error", err)
		return errors.Wrap(err, "failed to start server")
	}

	zap.S().Infow("server shutting down")
	return nil
}

// connectionParams holds resolved connection parameters
type connectionParams struct {
	dsn          string
	readOnly     bool
	queryTimeout int
}

// SetReadOnly sets the read-only mode for the DBManager
func (m *DBManager) SetReadOnly(readOnly bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.readOnly = readOnly
}

// GetDB returns a database connection for the given DSN with thread safety
func (m *DBManager) GetDB(ctx context.Context, cfg *config.Config, toolDSN string) (*sqlx.DB, error) {
	db, _, err := m.GetDBWithPreset(ctx, cfg, toolDSN, "")
	return db, err
}

// GetDBWithPreset returns a database connection resolved from DSN or preset
func (m *DBManager) GetDBWithPreset(ctx context.Context, cfg *config.Config, toolDSN, toolPreset string) (*sqlx.DB, *connectionParams, error) {
	params, err := m.resolveConnectionParams(cfg, toolDSN, toolPreset)
	if err != nil {
		return nil, nil, err
	}

	// queryTimeout is intentionally excluded: it's applied per-query via context, not per-connection.
	cacheKey := fmt.Sprintf("%s|ro=%t", params.dsn, params.readOnly)

	// Fast path: check if connection already exists
	m.mu.RLock()
	if db, ok := m.connections[cacheKey]; ok {
		m.mu.RUnlock()
		return db, params, nil
	}
	m.mu.RUnlock()

	// Slow path: create new connection
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if db, ok := m.connections[cacheKey]; ok {
		return db, params, nil
	}

	db, err := m.createConnectionWithReadOnly(ctx, params.dsn, params.readOnly)
	if err != nil {
		return nil, nil, err
	}

	m.connections[cacheKey] = db
	return db, params, nil
}

// resolveConnectionParams resolves DSN and metadata from tool parameters
func (m *DBManager) resolveConnectionParams(cfg *config.Config, toolDSN, toolPreset string) (*connectionParams, error) {
	if toolDSN != "" && toolPreset != "" {
		return nil, fmt.Errorf("cannot specify both 'dsn' and 'preset' parameters")
	}

	if toolPreset != "" {
		preset, ok := cfg.Presets[toolPreset]
		if !ok {
			return nil, fmt.Errorf("preset '%s' not found in configuration", toolPreset)
		}

		dsn, err := m.buildPresetDSN(&preset)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to resolve preset '%s'", toolPreset)
		}

		queryTimeout := preset.QueryTimeout
		if queryTimeout <= 0 {
			queryTimeout = cfg.PostgreSQL.QueryTimeout
		}

		return &connectionParams{
			dsn:          dsn,
			readOnly:     preset.ReadOnly,
			queryTimeout: queryTimeout,
		}, nil
	}

	// Fall back to existing DSN resolution
	dsn, err := m.resolveDSN(cfg, toolDSN)
	if err != nil {
		return nil, err
	}

	return &connectionParams{
		dsn:          dsn,
		readOnly:     cfg.PostgreSQL.ReadOnly,
		queryTimeout: cfg.PostgreSQL.QueryTimeout,
	}, nil
}

// buildPresetDSN builds a DSN string from preset configuration
func (m *DBManager) buildPresetDSN(preset *config.PresetConfig) (string, error) {
	dsn := preset.DSN
	if dsn == "" {
		if preset.Host == "" || preset.User == "" {
			return "", fmt.Errorf("preset requires either 'dsn' or both 'host' and 'user'")
		}

		dbname := preset.Database
		if dbname == "" {
			dbname = "postgres"
		}

		port := preset.Port
		if port == 0 {
			port = 5432
		}

		sslmode := preset.SSLMode
		if sslmode == "" {
			sslmode = "disable"
		}

		dsn = fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
			preset.Host, port, preset.User, preset.Password, dbname, sslmode)

		schema := preset.Schema
		if schema == "" {
			schema = "public"
		}
		dsn += fmt.Sprintf(" search_path=%s", schema)
	}

	if isURLStyle(dsn) {
		u, err := dburl.Parse(dsn)
		if err != nil {
			return "", fmt.Errorf("failed to parse preset database URL: %v", err)
		}
		dsn = u.DSN
		zap.S().Debugw("converted preset URL-style DSN to native PostgreSQL DSN", "dsn", sanitizeDSNForLog(dsn))
	}

	return dsn, nil
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

// createConnectionWithReadOnly creates a new database connection with the specified read-only mode
func (m *DBManager) createConnectionWithReadOnly(ctx context.Context, dsn string, readOnly bool) (*sqlx.DB, error) {
	db, err := sqlx.ConnectContext(ctx, "pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to establish database connection: %v", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	// Set session to read-only if configured
	if readOnly {
		if _, err := db.ExecContext(ctx, "SET SESSION CHARACTERISTICS AS TRANSACTION READ ONLY"); err != nil {
			_ = db.Close()
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
