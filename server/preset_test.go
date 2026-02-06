package server

import (
	"context"
	"strings"
	"testing"

	"github.com/cnosuke/mcp-postgresql/config"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveConnectionParams(t *testing.T) {
	baseCfg := &config.Config{}
	baseCfg.PostgreSQL.Host = "default-host"
	baseCfg.PostgreSQL.Port = 5432
	baseCfg.PostgreSQL.User = "default-user"
	baseCfg.PostgreSQL.Password = "default-pass"
	baseCfg.PostgreSQL.Database = "default-db"
	baseCfg.PostgreSQL.SSLMode = "disable"
	baseCfg.PostgreSQL.Schema = "public"
	baseCfg.PostgreSQL.ReadOnly = false
	baseCfg.PostgreSQL.QueryTimeout = 30
	baseCfg.Presets = map[string]config.PresetConfig{
		"production": {
			Host:         "prod-host",
			User:         "prod-user",
			Password:     "prod-pass",
			Port:         5433,
			Database:     "prod-db",
			Schema:       "prod_schema",
			SSLMode:      "require",
			ReadOnly:     true,
			QueryTimeout: 60,
		},
		"staging": {
			Host:         "staging-host",
			User:         "staging-user",
			Password:     "staging-pass",
			Port:         5432,
			Database:     "staging-db",
			Schema:       "public",
			SSLMode:      "disable",
			ReadOnly:     false,
			QueryTimeout: 0,
		},
		"dsn-only": {
			DSN:      "host=custom-host port=5432 user=custom-user password=custom-pass dbname=custom-db sslmode=disable",
			ReadOnly: false,
		},
		"url-style": {
			DSN:      "postgres://url-user:url-pass@url-host:5432/url-db?sslmode=disable",
			ReadOnly: true,
		},
	}

	m := &DBManager{
		connections: make(map[string]*sqlx.DB),
	}

	tests := []struct {
		name       string
		toolDSN    string
		toolPreset string
		wantErr    bool
		errMsg     string
		checkFn    func(t *testing.T, params *connectionParams)
	}{
		{
			name:       "both dsn and preset specified",
			toolDSN:    "host=localhost",
			toolPreset: "production",
			wantErr:    true,
			errMsg:     "cannot specify both",
		},
		{
			name:       "non-existent preset",
			toolDSN:    "",
			toolPreset: "nonexistent",
			wantErr:    true,
			errMsg:     "not found",
		},
		{
			name:       "preset with fields",
			toolDSN:    "",
			toolPreset: "production",
			wantErr:    false,
			checkFn: func(t *testing.T, params *connectionParams) {
				assert.Contains(t, params.dsn, "host=prod-host")
				assert.Contains(t, params.dsn, "port=5433")
				assert.Contains(t, params.dsn, "user=prod-user")
				assert.Contains(t, params.dsn, "dbname=prod-db")
				assert.Contains(t, params.dsn, "sslmode=require")
				assert.Contains(t, params.dsn, "search_path=prod_schema")
				assert.True(t, params.readOnly)
				assert.Equal(t, 60, params.queryTimeout)
			},
		},
		{
			name:       "preset with read_only=false",
			toolDSN:    "",
			toolPreset: "staging",
			wantErr:    false,
			checkFn: func(t *testing.T, params *connectionParams) {
				assert.Contains(t, params.dsn, "host=staging-host")
				assert.False(t, params.readOnly)
			},
		},
		{
			name:       "preset with query_timeout=0 falls back to global",
			toolDSN:    "",
			toolPreset: "staging",
			wantErr:    false,
			checkFn: func(t *testing.T, params *connectionParams) {
				assert.Equal(t, 30, params.queryTimeout)
			},
		},
		{
			name:       "preset with DSN field",
			toolDSN:    "",
			toolPreset: "dsn-only",
			wantErr:    false,
			checkFn: func(t *testing.T, params *connectionParams) {
				assert.Contains(t, params.dsn, "host=custom-host")
				assert.False(t, params.readOnly)
			},
		},
		{
			name:       "preset with URL-style DSN",
			toolDSN:    "",
			toolPreset: "url-style",
			wantErr:    false,
			checkFn: func(t *testing.T, params *connectionParams) {
				// URL-style DSN gets converted to native format
				assert.NotContains(t, params.dsn, "postgres://")
				assert.True(t, params.readOnly)
			},
		},
		{
			name:       "neither dsn nor preset specified falls back to default",
			toolDSN:    "",
			toolPreset: "",
			wantErr:    false,
			checkFn: func(t *testing.T, params *connectionParams) {
				assert.Contains(t, params.dsn, "host=default-host")
				assert.False(t, params.readOnly)
				assert.Equal(t, 30, params.queryTimeout)
			},
		},
		{
			name:       "dsn specified directly",
			toolDSN:    "host=direct-host port=5432 user=direct-user password=pass dbname=direct-db sslmode=disable",
			toolPreset: "",
			wantErr:    false,
			checkFn: func(t *testing.T, params *connectionParams) {
				assert.Contains(t, params.dsn, "host=direct-host")
				assert.False(t, params.readOnly)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params, err := m.resolveConnectionParams(baseCfg, tt.toolDSN, tt.toolPreset)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				require.NotNil(t, params)
				if tt.checkFn != nil {
					tt.checkFn(t, params)
				}
			}
		})
	}
}

func TestHandleListPreset(t *testing.T) {
	t.Run("with presets", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Presets = map[string]config.PresetConfig{
			"production": {
				Host:     "prod-host",
				User:     "prod-user",
				Password: "secret-password",
				Database: "prod-db",
				ReadOnly: true,
			},
			"staging": {
				Host:     "staging-host",
				User:     "staging-user",
				Password: "another-secret",
				Database: "staging-db",
				ReadOnly: false,
			},
		}

		result, err := handleListPreset(cfg)
		require.NoError(t, err)

		// Verify CSV header
		assert.True(t, strings.HasPrefix(result, "preset_name,host,user,database,read_only\n"))

		// Verify password is never exposed
		assert.NotContains(t, result, "secret-password")
		assert.NotContains(t, result, "another-secret")

		// Verify data is present
		assert.Contains(t, result, "production")
		assert.Contains(t, result, "prod-host")
		assert.Contains(t, result, "staging")
		assert.Contains(t, result, "staging-host")

		// Verify sorted order (production before staging)
		prodIdx := strings.Index(result, "production")
		stagingIdx := strings.Index(result, "staging")
		assert.Less(t, prodIdx, stagingIdx)
	})

	t.Run("empty presets", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Presets = map[string]config.PresetConfig{}

		result, err := handleListPreset(cfg)
		require.NoError(t, err)
		assert.Equal(t, "preset_name,host,user,database,read_only\n", result)
	})

	t.Run("nil presets", func(t *testing.T) {
		cfg := &config.Config{}

		result, err := handleListPreset(cfg)
		require.NoError(t, err)
		assert.Equal(t, "preset_name,host,user,database,read_only\n", result)
	})

	t.Run("DSN-only preset extracts host and database", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Presets = map[string]config.PresetConfig{
			"dsn-preset": {
				DSN: "postgres://dsn-user:dsn-pass@dsn-host:5432/dsn-db?sslmode=disable",
			},
		}

		result, err := handleListPreset(cfg)
		require.NoError(t, err)
		assert.Contains(t, result, "dsn-host")
		assert.Contains(t, result, "dsn-db")
		assert.NotContains(t, result, "dsn-pass")
	})

	t.Run("key-value DSN preset extracts host and database", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Presets = map[string]config.PresetConfig{
			"kv-preset": {
				DSN: "host=kv-host port=5432 user=kv-user password=kv-pass dbname=kv-db sslmode=disable",
			},
		}

		result, err := handleListPreset(cfg)
		require.NoError(t, err)
		assert.Contains(t, result, "kv-host")
		assert.Contains(t, result, "kv-db")
		assert.NotContains(t, result, "kv-pass")
	})
}

func TestHandleWriteQueryPresetReadOnly(t *testing.T) {
	cfg := &config.Config{}
	cfg.Presets = map[string]config.PresetConfig{
		"readonly-db": {
			Host:     "ro-host",
			User:     "ro-user",
			Port:     5432,
			Database: "ro-db",
			SSLMode:  "disable",
			ReadOnly: true,
		},
		"writable-db": {
			Host:     "rw-host",
			User:     "rw-user",
			Port:     5432,
			Database: "rw-db",
			SSLMode:  "disable",
			ReadOnly: false,
		},
	}

	t.Run("write_query blocked on read-only preset", func(t *testing.T) {
		_, err := handleWriteQuery(context.Background(), cfg, "INSERT INTO t VALUES(1)", "INSERT", "", "readonly-db")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read-only")
		assert.Contains(t, err.Error(), "readonly-db")
	})

	t.Run("update_query blocked on read-only preset", func(t *testing.T) {
		_, err := handleWriteQuery(context.Background(), cfg, "UPDATE t SET x = 1 WHERE id = 1", "UPDATE", "", "readonly-db")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read-only")
	})

	t.Run("delete_query blocked on read-only preset", func(t *testing.T) {
		_, err := handleWriteQuery(context.Background(), cfg, "DELETE FROM t WHERE id = 1", "DELETE", "", "readonly-db")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read-only")
	})

	t.Run("exec blocked on read-only preset", func(t *testing.T) {
		_, err := handleExecWithPreset(context.Background(), cfg, "CREATE TABLE t (id int)", "", "readonly-db")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read-only")
	})
}

func TestExtractHostDatabaseFromDSN(t *testing.T) {
	tests := []struct {
		name     string
		dsn      string
		wantHost string
		wantDB   string
	}{
		{
			name:     "URL-style DSN",
			dsn:      "postgres://user:pass@myhost:5432/mydb?sslmode=disable",
			wantHost: "myhost",
			wantDB:   "mydb",
		},
		{
			name:     "key-value DSN",
			dsn:      "host=myhost port=5432 user=user password=pass dbname=mydb sslmode=disable",
			wantHost: "myhost",
			wantDB:   "mydb",
		},
		{
			name:     "empty DSN",
			dsn:      "",
			wantHost: "",
			wantDB:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, db := extractHostDatabaseFromDSN(tt.dsn)
			assert.Equal(t, tt.wantHost, host)
			assert.Equal(t, tt.wantDB, db)
		})
	}
}
