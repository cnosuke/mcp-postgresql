package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateReadOnlyQuery(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		wantErr bool
		errMsg  string
	}{
		// Allowed cases
		{
			name:    "simple SELECT",
			query:   "SELECT * FROM users",
			wantErr: false,
		},
		{
			name:    "SELECT with newline",
			query:   "SELECT\n* FROM users",
			wantErr: false,
		},
		{
			name:    "SELECT with tab",
			query:   "SELECT\t* FROM users",
			wantErr: false,
		},
		{
			name:    "SELECT with leading whitespace",
			query:   "  SELECT * FROM users",
			wantErr: false,
		},
		{
			name:    "CTE SELECT",
			query:   "WITH cte AS (SELECT 1) SELECT * FROM cte",
			wantErr: false,
		},
		{
			name:    "SELECT with INTO clause after FROM",
			query:   "SELECT * FROM users INTO @var",
			wantErr: false,
		},

		// Rejected cases - write operations
		{
			name:    "INSERT statement",
			query:   "INSERT INTO users VALUES (1)",
			wantErr: true,
			errMsg:  "only SELECT queries are allowed",
		},
		{
			name:    "UPDATE statement",
			query:   "UPDATE users SET name = 'x'",
			wantErr: true,
			errMsg:  "only SELECT queries are allowed",
		},
		{
			name:    "DELETE statement",
			query:   "DELETE FROM users",
			wantErr: true,
			errMsg:  "only SELECT queries are allowed",
		},

		// Rejected cases - dangerous operations
		{
			name:    "DROP TABLE",
			query:   "DROP TABLE users",
			wantErr: true,
			errMsg:  "only SELECT queries are allowed",
		},
		{
			name:    "TRUNCATE",
			query:   "TRUNCATE users",
			wantErr: true,
			errMsg:  "only SELECT queries are allowed",
		},
		{
			name:    "CREATE TABLE",
			query:   "CREATE TABLE test (id int)",
			wantErr: true,
			errMsg:  "only SELECT queries are allowed",
		},
		{
			name:    "ALTER TABLE",
			query:   "ALTER TABLE users ADD col int",
			wantErr: true,
			errMsg:  "only SELECT queries are allowed",
		},
		{
			name:    "GRANT",
			query:   "GRANT SELECT ON users TO public",
			wantErr: true,
			errMsg:  "only SELECT queries are allowed",
		},
		{
			name:    "COPY",
			query:   "COPY users TO '/tmp/x'",
			wantErr: true,
			errMsg:  "only SELECT queries are allowed",
		},

		// Rejected cases - SELECT INTO (creates new table)
		{
			name:    "SELECT INTO new table",
			query:   "SELECT * INTO new_table FROM users",
			wantErr: true,
			errMsg:  "SELECT INTO is not allowed",
		},

		// Rejected cases - dangerous functions
		{
			name:    "DO block",
			query:   "DO $$ BEGIN PERFORM 1; END $$",
			wantErr: true,
			errMsg:  "only SELECT queries are allowed",
		},
		{
			name:    "CALL procedure",
			query:   "CALL my_procedure()",
			wantErr: true,
			errMsg:  "only SELECT queries are allowed",
		},
		{
			name:    "EXECUTE",
			query:   "EXECUTE my_statement",
			wantErr: true,
			errMsg:  "only SELECT queries are allowed",
		},

		// Rejected cases - write in CTE
		{
			name:    "CTE with INSERT",
			query:   "WITH cte AS (SELECT 1) INSERT INTO users SELECT * FROM cte",
			wantErr: true,
			errMsg:  "SELECT INTO is not allowed",
		},

		// Dangerous operations within SELECT
		{
			name:    "SELECT with DROP in comment simulation",
			query:   "SELECT 'DROP ' FROM users",
			wantErr: true,
			errMsg:  "dangerous operation",
		},
		{
			name:    "SELECT containing ALTER",
			query:   "SELECT * FROM users WHERE name LIKE 'ALTER TABLE'",
			wantErr: true,
			errMsg:  "dangerous operation",
		},
		{
			name:    "SELECT with DO block",
			query:   "SELECT DO $$ BEGIN PERFORM 1; END $$ FROM users",
			wantErr: true,
			errMsg:  "operation \"DO $$\" is not allowed",
		},
		{
			name:    "SELECT with CALL",
			query:   "SELECT CALL my_func() FROM users",
			wantErr: true,
			errMsg:  "operation \"CALL\" is not allowed",
		},
		{
			name:    "SELECT with EXECUTE",
			query:   "SELECT EXECUTE my_stmt FROM users",
			wantErr: true,
			errMsg:  "operation \"EXECUTE\" is not allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateReadOnlyQuery(tt.query)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateWriteQuery(t *testing.T) {
	tests := []struct {
		name         string
		query        string
		expectedType string
		wantErr      bool
		errMsg       string
	}{
		// Allowed cases - INSERT
		{
			name:         "simple INSERT",
			query:        "INSERT INTO users VALUES (1)",
			expectedType: "INSERT",
			wantErr:      false,
		},
		{
			name:         "INSERT with trailing semicolon",
			query:        "INSERT INTO users VALUES (1);",
			expectedType: "INSERT",
			wantErr:      false,
		},
		{
			name:         "INSERT with newline",
			query:        "INSERT\nINTO users VALUES (1)",
			expectedType: "INSERT",
			wantErr:      false,
		},
		{
			name:         "INSERT with tab",
			query:        "INSERT\tINTO users VALUES (1)",
			expectedType: "INSERT",
			wantErr:      false,
		},
		{
			name:         "CTE INSERT",
			query:        "WITH cte AS (SELECT 1) INSERT INTO users SELECT * FROM cte",
			expectedType: "INSERT",
			wantErr:      false,
		},

		// Allowed cases - UPDATE
		{
			name:         "simple UPDATE",
			query:        "UPDATE users SET name = 'x' WHERE id = 1",
			expectedType: "UPDATE",
			wantErr:      false,
		},
		{
			name:         "UPDATE with trailing semicolon",
			query:        "UPDATE users SET name = 'x';",
			expectedType: "UPDATE",
			wantErr:      false,
		},

		// Allowed cases - DELETE
		{
			name:         "simple DELETE",
			query:        "DELETE FROM users WHERE id = 1",
			expectedType: "DELETE",
			wantErr:      false,
		},
		{
			name:         "DELETE with trailing semicolon",
			query:        "DELETE FROM users WHERE id = 1;",
			expectedType: "DELETE",
			wantErr:      false,
		},

		// Rejected cases - multiple statements
		{
			name:         "multiple statements with INSERT",
			query:        "INSERT INTO users VALUES (1); DROP TABLE users",
			expectedType: "INSERT",
			wantErr:      true,
			errMsg:       "multiple statements are not allowed",
		},
		{
			name:         "embedded semicolon in INSERT",
			query:        "INSERT INTO users VALUES (1); --",
			expectedType: "INSERT",
			wantErr:      true,
			errMsg:       "multiple statements are not allowed",
		},

		// Rejected cases - type mismatch
		{
			name:         "UPDATE instead of INSERT",
			query:        "UPDATE users SET x = 1",
			expectedType: "INSERT",
			wantErr:      true,
			errMsg:       "expected INSERT statement",
		},
		{
			name:         "INSERT instead of DELETE",
			query:        "INSERT INTO users VALUES (1)",
			expectedType: "DELETE",
			wantErr:      true,
			errMsg:       "expected DELETE statement",
		},
		{
			name:         "SELECT instead of INSERT",
			query:        "SELECT * FROM users",
			expectedType: "INSERT",
			wantErr:      true,
			errMsg:       "expected INSERT statement",
		},

		// Rejected cases - dangerous operations
		{
			name:         "DROP TABLE",
			query:        "DROP TABLE users",
			expectedType: "INSERT",
			wantErr:      true,
			errMsg:       "dangerous operation",
		},
		{
			name:         "TRUNCATE",
			query:        "TRUNCATE users",
			expectedType: "DELETE",
			wantErr:      true,
			errMsg:       "dangerous operation",
		},
		{
			name:         "COPY",
			query:        "COPY users TO '/tmp/x'",
			expectedType: "INSERT",
			wantErr:      true,
			errMsg:       "dangerous operation",
		},
		{
			name:         "GRANT",
			query:        "GRANT SELECT ON users TO public",
			expectedType: "INSERT",
			wantErr:      true,
			errMsg:       "dangerous operation",
		},
		{
			name:         "REVOKE",
			query:        "REVOKE SELECT ON users FROM public",
			expectedType: "INSERT",
			wantErr:      true,
			errMsg:       "dangerous operation",
		},
		{
			name:         "ALTER TABLE",
			query:        "ALTER TABLE users ADD COLUMN new_col INT",
			expectedType: "UPDATE",
			wantErr:      true,
			errMsg:       "dangerous operation",
		},

		// Rejected cases - CTE type mismatch
		{
			name:         "CTE with wrong expected type",
			query:        "WITH cte AS (SELECT 1) INSERT INTO users SELECT * FROM cte",
			expectedType: "DELETE",
			wantErr:      true,
			errMsg:       "CTE does not contain DELETE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWriteQuery(tt.query, tt.expectedType)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
