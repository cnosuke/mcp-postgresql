package server

import (
	"context"
	"encoding/csv"
	"fmt"
	"strings"
	"time"

	"github.com/cnosuke/mcp-postgresql/config"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterAllTools - Register all tools with the server
func RegisterAllTools(mcpServer *server.MCPServer, cfg *config.Config) error {
	// Schema Tools
	listDatabaseTool := mcp.NewTool(
		"list_database",
		mcp.WithDescription("List all databases in the PostgreSQL server"),
		mcp.WithString("dsn",
			mcp.Description("PostgreSQL DSN (Data Source Name) string. Supports both key=value format and URL format (postgres://...). If provided, this overrides the configuration."),
		),
	)

	listSchemaTool := mcp.NewTool(
		"list_schema",
		mcp.WithDescription("List all schemas in the current PostgreSQL database (excluding system schemas)"),
		mcp.WithString("dsn",
			mcp.Description("PostgreSQL DSN (Data Source Name) string. Supports both key=value format and URL format (postgres://...). If provided, this overrides the configuration."),
		),
	)

	listTableTool := mcp.NewTool(
		"list_table",
		mcp.WithDescription("List all tables in the specified schema (default: public)"),
		mcp.WithString("schema",
			mcp.Description("Schema name to list tables from (default: public)"),
		),
		mcp.WithString("dsn",
			mcp.Description("PostgreSQL DSN (Data Source Name) string. Supports both key=value format and URL format (postgres://...). If provided, this overrides the configuration."),
		),
	)

	descTableTool := mcp.NewTool(
		"desc_table",
		mcp.WithDescription("Describe the structure of a table including columns, constraints, and indexes"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("The name of the table to describe"),
		),
		mcp.WithString("schema",
			mcp.Description("Schema name where the table is located (default: public)"),
		),
		mcp.WithString("dsn",
			mcp.Description("PostgreSQL DSN (Data Source Name) string. Supports both key=value format and URL format (postgres://...). If provided, this overrides the configuration."),
		),
	)

	createTableTool := mcp.NewTool(
		"create_table",
		mcp.WithDescription(`Create a new table in the PostgreSQL server.
Note: PostgreSQL uses separate COMMENT ON statements for comments:
  COMMENT ON TABLE tablename IS 'description';
  COMMENT ON COLUMN tablename.columnname IS 'description';`),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("The SQL query to create the table"),
		),
		mcp.WithString("dsn",
			mcp.Description("PostgreSQL DSN (Data Source Name) string. Supports both key=value format and URL format (postgres://...). If provided, this overrides the configuration."),
		),
	)

	alterTableTool := mcp.NewTool(
		"alter_table",
		mcp.WithDescription(`Alter an existing table in the PostgreSQL server.
Note: Use COMMENT ON statements to update column comments. DO NOT drop table or existing columns!`),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("The SQL query to alter the table"),
		),
		mcp.WithString("dsn",
			mcp.Description("PostgreSQL DSN (Data Source Name) string. Supports both key=value format and URL format (postgres://...). If provided, this overrides the configuration."),
		),
	)

	// Data Tools
	readQueryTool := mcp.NewTool(
		"read_query",
		mcp.WithDescription("Execute a read-only SQL query (SELECT only). Make sure you have knowledge of the table structure before writing WHERE conditions. Call `desc_table` first if necessary"),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("The SQL query to execute (SELECT statements only)"),
		),
		mcp.WithString("dsn",
			mcp.Description("PostgreSQL DSN (Data Source Name) string. Supports both key=value format and URL format (postgres://...). If provided, this overrides the configuration."),
		),
	)

	writeQueryTool := mcp.NewTool(
		"write_query",
		mcp.WithDescription("Execute an INSERT SQL query. Supports RETURNING clause to return inserted data. Make sure you have knowledge of the table structure before executing the query."),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("The SQL query to execute (INSERT statement, optionally with RETURNING clause)"),
		),
		mcp.WithString("dsn",
			mcp.Description("PostgreSQL DSN (Data Source Name) string. Supports both key=value format and URL format (postgres://...). If provided, this overrides the configuration."),
		),
	)

	updateQueryTool := mcp.NewTool(
		"update_query",
		mcp.WithDescription("Execute an UPDATE SQL query. Supports RETURNING clause to return updated data. Make sure there is always a WHERE condition. Call `desc_table` first if necessary"),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("The SQL query to execute (UPDATE statement, optionally with RETURNING clause)"),
		),
		mcp.WithString("dsn",
			mcp.Description("PostgreSQL DSN (Data Source Name) string. Supports both key=value format and URL format (postgres://...). If provided, this overrides the configuration."),
		),
	)

	deleteQueryTool := mcp.NewTool(
		"delete_query",
		mcp.WithDescription("Execute a DELETE SQL query. Supports RETURNING clause to return deleted data. Make sure there is always a WHERE condition. Call `desc_table` first if necessary"),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("The SQL query to execute (DELETE statement, optionally with RETURNING clause)"),
		),
		mcp.WithString("dsn",
			mcp.Description("PostgreSQL DSN (Data Source Name) string. Supports both key=value format and URL format (postgres://...). If provided, this overrides the configuration."),
		),
	)

	// Register handlers for each tool
	mcpServer.AddTool(listDatabaseTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		dsn := request.GetString("dsn", "")
		result, err := handleListDatabase(ctx, cfg, dsn)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(result), nil
	})

	mcpServer.AddTool(listSchemaTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		dsn := request.GetString("dsn", "")
		result, err := handleListSchema(ctx, cfg, dsn)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(result), nil
	})

	mcpServer.AddTool(listTableTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		schema := request.GetString("schema", "")
		dsn := request.GetString("dsn", "")
		result, err := handleListTable(ctx, cfg, schema, dsn)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(result), nil
	})

	mcpServer.AddTool(descTableTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := request.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		schema := request.GetString("schema", "")
		dsn := request.GetString("dsn", "")
		result, err := handleDescTable(ctx, cfg, name, schema, dsn)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(result), nil
	})

	if !cfg.PostgreSQL.ReadOnly {
		mcpServer.AddTool(createTableTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			query, err := request.RequireString("query")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			dsn := request.GetString("dsn", "")
			result, err := HandleExecContext(ctx, cfg, query, dsn)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(result), nil
		})
	}

	if !cfg.PostgreSQL.ReadOnly {
		mcpServer.AddTool(alterTableTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			query, err := request.RequireString("query")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			dsn := request.GetString("dsn", "")
			result, err := HandleExecContext(ctx, cfg, query, dsn)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(result), nil
		})
	}

	mcpServer.AddTool(readQueryTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, err := request.RequireString("query")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		dsn := request.GetString("dsn", "")
		result, err := handleReadQuery(ctx, cfg, query, dsn)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(result), nil
	})

	if !cfg.PostgreSQL.ReadOnly {
		mcpServer.AddTool(writeQueryTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			query, err := request.RequireString("query")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			dsn := request.GetString("dsn", "")
			result, err := handleWriteQuery(ctx, cfg, query, "INSERT", dsn)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(result), nil
		})
	}

	if !cfg.PostgreSQL.ReadOnly {
		mcpServer.AddTool(updateQueryTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			query, err := request.RequireString("query")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			dsn := request.GetString("dsn", "")
			result, err := handleWriteQuery(ctx, cfg, query, "UPDATE", dsn)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(result), nil
		})
	}

	if !cfg.PostgreSQL.ReadOnly {
		mcpServer.AddTool(deleteQueryTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			query, err := request.RequireString("query")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			dsn := request.GetString("dsn", "")
			result, err := handleWriteQuery(ctx, cfg, query, "DELETE", dsn)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(result), nil
		})
	}

	return nil
}

// handleListDatabase lists all databases in PostgreSQL
func handleListDatabase(ctx context.Context, cfg *config.Config, toolDSN string) (string, error) {
	query := "SELECT datname FROM pg_database WHERE datistemplate = false ORDER BY datname"
	return HandleQueryContext(ctx, cfg, query, toolDSN)
}

// handleListSchema lists all user schemas (excluding system schemas)
func handleListSchema(ctx context.Context, cfg *config.Config, toolDSN string) (string, error) {
	query := `SELECT schema_name FROM information_schema.schemata
WHERE schema_name NOT LIKE 'pg_%' AND schema_name != 'information_schema'
ORDER BY schema_name`
	return HandleQueryContext(ctx, cfg, query, toolDSN)
}

// handleListTable lists all tables in the specified schema
func handleListTable(ctx context.Context, cfg *config.Config, schema, toolDSN string) (string, error) {
	queryCtx, cancel := contextWithTimeout(ctx, cfg)
	defer cancel()

	db, err := GetDBContext(queryCtx, cfg, toolDSN)
	if err != nil {
		return "", err
	}

	if schema == "" {
		schema = "public"
	}

	query := "SELECT tablename FROM pg_tables WHERE schemaname = $1 ORDER BY tablename"
	rows, err := db.QueryxContext(queryCtx, query, schema)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var tablename string
		if err := rows.Scan(&tablename); err != nil {
			return "", err
		}
		result = append(result, map[string]interface{}{"tablename": tablename})
	}

	if err := rows.Err(); err != nil {
		return "", err
	}

	return MapToCSV(result, []string{"tablename"})
}

// handleDescTable describes a table structure
func handleDescTable(ctx context.Context, cfg *config.Config, name, schema, toolDSN string) (string, error) {
	queryCtx, cancel := contextWithTimeout(ctx, cfg)
	defer cancel()

	db, err := GetDBContext(queryCtx, cfg, toolDSN)
	if err != nil {
		return "", err
	}

	if schema == "" {
		schema = "public"
	}

	var result strings.Builder

	columnQuery := `
SELECT
    c.column_name,
    c.data_type,
    c.character_maximum_length,
    c.numeric_precision,
    c.numeric_scale,
    c.is_nullable,
    c.column_default,
    pgd.description as column_comment
FROM information_schema.columns c
LEFT JOIN pg_catalog.pg_statio_all_tables st
    ON c.table_schema = st.schemaname AND c.table_name = st.relname
LEFT JOIN pg_catalog.pg_description pgd
    ON pgd.objoid = st.relid AND pgd.objsubid = c.ordinal_position
WHERE c.table_schema = $1 AND c.table_name = $2
ORDER BY c.ordinal_position`

	rows, err := db.QueryxContext(queryCtx, columnQuery, schema, name)
	if err != nil {
		return "", err
	}

	result.WriteString("== Columns ==\n")

	columnCount := 0
	for rows.Next() {
		var columnName, dataType, isNullable string
		var charMaxLen, numPrecision, numScale *int
		var columnDefault, columnComment *string

		if err := rows.Scan(&columnName, &dataType, &charMaxLen, &numPrecision, &numScale, &isNullable, &columnDefault, &columnComment); err != nil {
			rows.Close()
			return "", err
		}

		columnCount++

		typeStr := dataType
		if charMaxLen != nil {
			typeStr = fmt.Sprintf("%s(%d)", dataType, *charMaxLen)
		} else if numPrecision != nil && numScale != nil {
			typeStr = fmt.Sprintf("%s(%d,%d)", dataType, *numPrecision, *numScale)
		}

		line := fmt.Sprintf("  %s %s", columnName, typeStr)
		if isNullable == "NO" {
			line += " NOT NULL"
		}
		if columnDefault != nil {
			line += fmt.Sprintf(" DEFAULT %s", *columnDefault)
		}
		if columnComment != nil && *columnComment != "" {
			line += fmt.Sprintf(" -- %s", *columnComment)
		}
		result.WriteString(line + "\n")
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return "", err
	}
	rows.Close()

	if columnCount == 0 {
		return "", fmt.Errorf("table %s.%s does not exist", schema, name)
	}

	constraintQuery := `
SELECT
    tc.constraint_name,
    tc.constraint_type,
    STRING_AGG(kcu.column_name, ', ' ORDER BY kcu.ordinal_position) as columns
FROM information_schema.table_constraints tc
JOIN information_schema.key_column_usage kcu
    ON tc.constraint_name = kcu.constraint_name
    AND tc.table_schema = kcu.table_schema
WHERE tc.table_schema = $1 AND tc.table_name = $2
GROUP BY tc.constraint_name, tc.constraint_type
ORDER BY tc.constraint_type, tc.constraint_name`

	rows, err = db.QueryxContext(queryCtx, constraintQuery, schema, name)
	if err != nil {
		return "", err
	}

	result.WriteString("\n== Constraints ==\n")
	constraintCount := 0
	for rows.Next() {
		var constraintName, constraintType, columns string
		if err := rows.Scan(&constraintName, &constraintType, &columns); err != nil {
			rows.Close()
			return "", err
		}
		constraintCount++
		result.WriteString(fmt.Sprintf("  %s: %s (%s)\n", constraintType, constraintName, columns))
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return "", err
	}
	rows.Close()

	if constraintCount == 0 {
		result.WriteString("  (none)\n")
	}

	indexQuery := `
SELECT indexname, indexdef
FROM pg_indexes
WHERE schemaname = $1 AND tablename = $2
ORDER BY indexname`

	rows, err = db.QueryxContext(queryCtx, indexQuery, schema, name)
	if err != nil {
		return "", err
	}

	result.WriteString("\n== Indexes ==\n")
	indexCount := 0
	for rows.Next() {
		var indexName, indexDef string
		if err := rows.Scan(&indexName, &indexDef); err != nil {
			rows.Close()
			return "", err
		}
		indexCount++
		result.WriteString(fmt.Sprintf("  %s\n    %s\n", indexName, indexDef))
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return "", err
	}
	rows.Close()

	if indexCount == 0 {
		result.WriteString("  (none)\n")
	}

	tableCommentQuery := `
SELECT obj_description(($1 || '.' || $2)::regclass, 'pg_class') as table_comment`

	var tableComment *string
	if err := db.QueryRowxContext(queryCtx, tableCommentQuery, schema, name).Scan(&tableComment); err == nil && tableComment != nil && *tableComment != "" {
		result.WriteString(fmt.Sprintf("\n== Table Comment ==\n  %s\n", *tableComment))
	}

	return result.String(), nil
}

// validateReadOnlyQuery checks if a query is read-only
func validateReadOnlyQuery(query string) error {
	upperQuery := strings.ToUpper(strings.TrimSpace(query))

	// Only allow SELECT and WITH...SELECT queries (allowlist approach)
	isSelect := strings.HasPrefix(upperQuery, "SELECT ") ||
		strings.HasPrefix(upperQuery, "SELECT\t") ||
		strings.HasPrefix(upperQuery, "SELECT\n")
	isWithSelect := strings.HasPrefix(upperQuery, "WITH ") && strings.Contains(upperQuery, " SELECT ")

	if !isSelect && !isWithSelect {
		return fmt.Errorf("only SELECT queries are allowed in read_query")
	}

	// Detect SELECT INTO (creates a new table)
	if strings.Contains(upperQuery, " INTO ") {
		intoIdx := strings.Index(upperQuery, " INTO ")
		fromIdx := strings.Index(upperQuery, " FROM ")
		// INTO before FROM indicates SELECT INTO
		if fromIdx == -1 || intoIdx < fromIdx {
			return fmt.Errorf("SELECT INTO is not allowed: it creates a new table")
		}
	}

	// Block DML operations that might appear in CTEs
	forbiddenInCTE := []string{"INSERT INTO", "DELETE FROM"}
	for _, forbidden := range forbiddenInCTE {
		if strings.Contains(upperQuery, forbidden) {
			return fmt.Errorf("write operations detected in query")
		}
	}

	if strings.Contains(upperQuery, "UPDATE ") && strings.Contains(upperQuery, " SET ") {
		return fmt.Errorf("write operations detected in query")
	}

	// Block dangerous operations
	dangerous := []string{"DROP ", "TRUNCATE ", "COPY ", "GRANT ", "REVOKE ", "ALTER ", "CREATE "}
	for _, d := range dangerous {
		if strings.Contains(upperQuery, d) {
			return fmt.Errorf("dangerous operation %q is not allowed", strings.TrimSpace(d))
		}
	}

	// Block potentially dangerous function calls
	dangerousFunctions := []string{"DO $$", "CALL ", "EXECUTE "}
	for _, f := range dangerousFunctions {
		if strings.Contains(upperQuery, f) {
			return fmt.Errorf("operation %q is not allowed in read queries", strings.TrimSpace(f))
		}
	}

	return nil
}

// validateWriteQuery checks if a write query is safe to execute
func validateWriteQuery(query, expectedType string) error {
	trimmedQuery := strings.TrimSpace(query)
	upperQuery := strings.ToUpper(trimmedQuery)

	// Detect multiple statements by checking for semicolons
	// Allow trailing semicolon but reject embedded ones
	withoutTrailing := strings.TrimSuffix(trimmedQuery, ";")
	if strings.Contains(withoutTrailing, ";") {
		return fmt.Errorf("multiple statements are not allowed")
	}

	// Block dangerous operations
	dangerous := []string{"DROP ", "TRUNCATE ", "COPY ", "GRANT ", "REVOKE ", "ALTER "}
	for _, d := range dangerous {
		if strings.Contains(upperQuery, d) {
			return fmt.Errorf("dangerous operation %q is not allowed", strings.TrimSpace(d))
		}
	}

	// Verify expected statement type
	if !strings.HasPrefix(upperQuery, expectedType+" ") && !strings.HasPrefix(upperQuery, expectedType+"\t") && !strings.HasPrefix(upperQuery, expectedType+"\n") {
		// Also allow "WITH ... INSERT/UPDATE/DELETE" (CTE)
		if strings.HasPrefix(upperQuery, "WITH ") {
			// Check if the CTE eventually leads to the expected operation
			if !strings.Contains(upperQuery, " "+expectedType+" ") {
				return fmt.Errorf("expected %s statement, but CTE does not contain %s", expectedType, expectedType)
			}
		} else {
			return fmt.Errorf("expected %s statement", expectedType)
		}
	}

	return nil
}

// handleReadQuery executes a read-only query after validation
func handleReadQuery(ctx context.Context, cfg *config.Config, query, toolDSN string) (string, error) {
	if err := validateReadOnlyQuery(query); err != nil {
		return "", err
	}
	return HandleQueryContext(ctx, cfg, query, toolDSN)
}

// handleWriteQuery executes a write query with RETURNING support
func handleWriteQuery(ctx context.Context, cfg *config.Config, query, expectedType, toolDSN string) (string, error) {
	if err := validateWriteQuery(query, expectedType); err != nil {
		return "", err
	}

	if strings.Contains(strings.ToUpper(query), "RETURNING") {
		return HandleQueryContext(ctx, cfg, query, toolDSN)
	}

	return HandleExecContext(ctx, cfg, query, toolDSN)
}

// contextWithTimeout creates a context with the configured query timeout
func contextWithTimeout(ctx context.Context, cfg *config.Config) (context.Context, context.CancelFunc) {
	timeout := cfg.PostgreSQL.QueryTimeout
	if timeout <= 0 {
		timeout = 30
	}
	return context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
}

// HandleQueryContext executes a read query with context and returns the result as CSV
func HandleQueryContext(ctx context.Context, cfg *config.Config, query, toolDSN string) (string, error) {
	result, headers, err := DoQueryContext(ctx, cfg, query, toolDSN)
	if err != nil {
		return "", err
	}

	s, err := MapToCSV(result, headers)
	if err != nil {
		return "", err
	}

	return s, nil
}

// HandleQuery executes a read query and returns the result as CSV (legacy wrapper)
func HandleQuery(cfg *config.Config, query, toolDSN string) (string, error) {
	return HandleQueryContext(context.Background(), cfg, query, toolDSN)
}

// DoQueryContext executes a query with context and returns the result rows and headers
func DoQueryContext(ctx context.Context, cfg *config.Config, query, toolDSN string) ([]map[string]interface{}, []string, error) {
	queryCtx, cancel := contextWithTimeout(ctx, cfg)
	defer cancel()

	db, err := GetDBContext(queryCtx, cfg, toolDSN)
	if err != nil {
		return nil, nil, err
	}

	rows, err := db.QueryxContext(queryCtx, query)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}

	result := []map[string]interface{}{}
	for rows.Next() {
		row, err := rows.SliceScan()
		if err != nil {
			return nil, nil, err
		}

		resultRow := map[string]interface{}{}
		for i, col := range cols {
			switch v := row[i].(type) {
			case []byte:
				resultRow[col] = string(v)
			default:
				resultRow[col] = v
			}
		}
		result = append(result, resultRow)
	}

	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	return result, cols, nil
}

// DoQuery executes a query and returns the result rows and headers (legacy wrapper)
func DoQuery(cfg *config.Config, query, toolDSN string) ([]map[string]interface{}, []string, error) {
	return DoQueryContext(context.Background(), cfg, query, toolDSN)
}

// HandleExecContext executes a write query with context and returns the result summary
func HandleExecContext(ctx context.Context, cfg *config.Config, query, toolDSN string) (string, error) {
	queryCtx, cancel := contextWithTimeout(ctx, cfg)
	defer cancel()

	db, err := GetDBContext(queryCtx, cfg, toolDSN)
	if err != nil {
		return "", err
	}

	result, err := db.ExecContext(queryCtx, query)
	if err != nil {
		return "", err
	}

	ra, err := result.RowsAffected()
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%d rows affected", ra), nil
}

// HandleExec executes a write query and returns the result summary (legacy wrapper)
func HandleExec(cfg *config.Config, query, toolDSN string) (string, error) {
	return HandleExecContext(context.Background(), cfg, query, toolDSN)
}

// MapToCSV converts map result to CSV format
func MapToCSV(m []map[string]interface{}, headers []string) (string, error) {
	var csvBuf strings.Builder
	writer := csv.NewWriter(&csvBuf)

	if err := writer.Write(headers); err != nil {
		return "", fmt.Errorf("failed to write headers: %v", err)
	}

	for _, item := range m {
		row := make([]string, len(headers))
		for i, header := range headers {
			value, exists := item[header]
			if !exists {
				return "", fmt.Errorf("key '%s' not found in map", header)
			}
			row[i] = fmt.Sprintf("%v", value)
		}
		if err := writer.Write(row); err != nil {
			return "", fmt.Errorf("failed to write row: %v", err)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return "", fmt.Errorf("error flushing CSV writer: %v", err)
	}

	return csvBuf.String(), nil
}
