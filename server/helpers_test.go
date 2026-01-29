package server

import (
	"context"
	"testing"
	"time"

	"github.com/cnosuke/mcp-postgresql/config"
	"github.com/stretchr/testify/assert"
)

func TestIsURLStyle(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
		want bool
	}{
		// URL-style DSNs (should return true)
		{
			name: "postgres:// URL",
			dsn:  "postgres://user:pass@localhost/db",
			want: true,
		},
		{
			name: "postgresql:// URL",
			dsn:  "postgresql://user@localhost/db",
			want: true,
		},
		{
			name: "postgres:// with port",
			dsn:  "postgres://user:pass@localhost:5432/db",
			want: true,
		},
		{
			name: "postgres:// with options",
			dsn:  "postgres://user:pass@localhost/db?sslmode=disable",
			want: true,
		},

		// Non-URL-style DSNs (should return false)
		{
			name: "key=value format",
			dsn:  "host=localhost user=postgres",
			want: false,
		},
		{
			name: "empty string",
			dsn:  "",
			want: false,
		},
		{
			name: "short string",
			dsn:  "pg://",
			want: false,
		},
		{
			name: "full key=value format",
			dsn:  "host=localhost port=5432 user=postgres password=secret dbname=test",
			want: false,
		},
		{
			name: "other URL scheme",
			dsn:  "mysql://user:pass@localhost/db",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isURLStyle(tt.dsn)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSanitizeDSNForLog(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
		want string
	}{
		// Key=value format password masking
		{
			name: "key=value with password",
			dsn:  "password=secret dbname=test",
			want: "password=*** dbname=test",
		},
		{
			name: "key=value with password in middle",
			dsn:  "host=localhost password=abc123 user=test",
			want: "host=localhost password=*** user=test",
		},
		{
			name: "key=value with complex password",
			dsn:  "host=localhost password=P@ssw0rd!#$ dbname=test",
			want: "host=localhost password=*** dbname=test",
		},

		// URL format password masking
		{
			name: "URL with password",
			dsn:  "postgres://user:secret@localhost",
			want: "postgres://user:***@localhost",
		},
		{
			name: "URL with password and port",
			dsn:  "postgres://user:secret@localhost:5432/db",
			want: "postgres://user:***@localhost:5432/db",
		},
		{
			name: "postgresql:// URL with password",
			dsn:  "postgresql://admin:password123@host.example.com/mydb",
			want: "postgresql://admin:***@host.example.com/mydb",
		},

		// No password to mask
		{
			name: "key=value without password",
			dsn:  "host=localhost user=postgres",
			want: "host=localhost user=postgres",
		},
		{
			name: "URL without password",
			dsn:  "postgres://user@localhost/db",
			want: "postgres://user@localhost/db",
		},
		{
			name: "empty string",
			dsn:  "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeDSNForLog(tt.dsn)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMapToCSV(t *testing.T) {
	tests := []struct {
		name    string
		data    []map[string]interface{}
		headers []string
		want    string
		wantErr bool
		errMsg  string
	}{
		// Normal cases
		{
			name:    "empty slice",
			data:    []map[string]interface{}{},
			headers: []string{"id", "name"},
			want:    "id,name\n",
			wantErr: false,
		},
		{
			name: "single row",
			data: []map[string]interface{}{
				{"id": 1, "name": "Alice"},
			},
			headers: []string{"id", "name"},
			want:    "id,name\n1,Alice\n",
			wantErr: false,
		},
		{
			name: "multiple rows",
			data: []map[string]interface{}{
				{"id": 1, "name": "Alice"},
				{"id": 2, "name": "Bob"},
				{"id": 3, "name": "Charlie"},
			},
			headers: []string{"id", "name"},
			want:    "id,name\n1,Alice\n2,Bob\n3,Charlie\n",
			wantErr: false,
		},
		{
			name: "special characters - comma",
			data: []map[string]interface{}{
				{"name": "Doe, John", "email": "john@example.com"},
			},
			headers: []string{"name", "email"},
			want:    "name,email\n\"Doe, John\",john@example.com\n",
			wantErr: false,
		},
		{
			name: "special characters - newline",
			data: []map[string]interface{}{
				{"description": "Line1\nLine2", "id": 1},
			},
			headers: []string{"id", "description"},
			want:    "id,description\n1,\"Line1\nLine2\"\n",
			wantErr: false,
		},
		{
			name: "special characters - quotes",
			data: []map[string]interface{}{
				{"title": "Say \"Hello\"", "id": 1},
			},
			headers: []string{"id", "title"},
			want:    "id,title\n1,\"Say \"\"Hello\"\"\"\n",
			wantErr: false,
		},
		{
			name: "nil value",
			data: []map[string]interface{}{
				{"id": 1, "name": nil},
			},
			headers: []string{"id", "name"},
			want:    "id,name\n1,<nil>\n",
			wantErr: false,
		},
		{
			name: "different types",
			data: []map[string]interface{}{
				{"int": 42, "float": 3.14, "bool": true, "string": "test"},
			},
			headers: []string{"int", "float", "bool", "string"},
			want:    "int,float,bool,string\n42,3.14,true,test\n",
			wantErr: false,
		},

		// Error cases
		{
			name: "missing key in map",
			data: []map[string]interface{}{
				{"id": 1},
			},
			headers: []string{"id", "missing_key"},
			wantErr: true,
			errMsg:  "key 'missing_key' not found in map",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MapToCSV(tt.data, tt.headers)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestContextWithTimeout(t *testing.T) {
	tests := []struct {
		name            string
		queryTimeout    int
		expectedTimeout time.Duration
	}{
		{
			name:            "positive timeout",
			queryTimeout:    30,
			expectedTimeout: 30 * time.Second,
		},
		{
			name:            "custom timeout",
			queryTimeout:    60,
			expectedTimeout: 60 * time.Second,
		},
		{
			name:            "zero timeout defaults to 30",
			queryTimeout:    0,
			expectedTimeout: 30 * time.Second,
		},
		{
			name:            "negative timeout defaults to 30",
			queryTimeout:    -1,
			expectedTimeout: 30 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.PostgreSQL.QueryTimeout = tt.queryTimeout

			ctx := context.Background()
			start := time.Now()
			newCtx, cancel := contextWithTimeout(ctx, cfg)
			defer cancel()

			deadline, ok := newCtx.Deadline()
			assert.True(t, ok, "context should have a deadline")

			expectedDeadline := start.Add(tt.expectedTimeout)
			tolerance := 100 * time.Millisecond
			assert.WithinDuration(t, expectedDeadline, deadline, tolerance)
		})
	}
}
