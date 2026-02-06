# CLAUDE.md - mcp-postgresql

## Project Overview

MCP (Model Context Protocol) server implementation for PostgreSQL.
Provides database access tools (schema inspection, CRUD queries) via stdio transport.

## Tech Stack

- **Language**: Go 1.24
- **MCP**: [mark3labs/mcp-go](https://github.com/mark3labs/mcp-go) v0.43+
- **DB Driver**: jackc/pgx/v5 (via stdlib) + jmoiron/sqlx
- **CLI**: urfave/cli/v2
- **Config**: jinzhu/configor (YAML + env vars)
- **Error Handling**: cockroachdb/errors
- **Logging**: uber-go/zap (to file, suppressed on stdout to avoid MCP stdio interference)
- **Testing**: stretchr/testify

## Project Structure

```
main.go           # CLI entrypoint (urfave/cli)
config/
  config.go       # Config struct (YAML + env vars)
config.yml        # Default config file
server/
  server.go       # DBManager, MCP server setup, DB connection pooling
  tools.go        # MCP tool definitions, handlers, SQL validation
  *_test.go       # Unit tests (no DB required)
logger/
  logger.go       # zap logger initialization
Makefile          # Build, test, lint, docker commands
Dockerfile        # Multi-stage build (distroless)
```

## Commands

```bash
# Build
make                          # Build binary to bin/mcp-postgresql

# Test
go test -v ./...              # Run all tests (no DB dependency)

# Lint
golangci-lint run             # Static analysis

# Run
bin/mcp-postgresql server -c config.yml
```

## Architecture Notes

- **stdio transport**: MCP communication uses stdin/stdout. Logger output MUST go to a file (not stdout/stderr) to avoid corrupting the MCP protocol.
- **Read-only mode**: When `read_only: true`, write tools (create_table, alter_table, write_query, update_query, delete_query) are not registered, and DB sessions are set to read-only.
- **DSN resolution**: Supports both key=value (`host=localhost port=5432 ...`) and URL format (`postgres://user:pass@host/db`). Tool-level DSN parameter overrides config.
- **Connection pooling**: `DBManager` uses double-checked locking pattern for thread-safe lazy connection creation, keyed by DSN string.
- **SQL validation**: `validateReadOnlyQuery` (allowlist approach for SELECT) and `validateWriteQuery` (statement type verification + dangerous operation blocking) protect against injection.
- **Query results**: Returned as CSV format via `MapToCSV`.

## Configuration

Config is loaded from YAML file with env var overrides:

| YAML Key                  | Env Var                 | Default     |
|---------------------------|-------------------------|-------------|
| `log`                     | `LOG_PATH`              | `""`        |
| `debug`                   | `DEBUG`                 | `false`     |
| `postgresql.host`         | `POSTGRES_HOST`         | `localhost` |
| `postgresql.port`         | `POSTGRES_PORT`         | `5432`      |
| `postgresql.user`         | `POSTGRES_USER`         | `postgres`  |
| `postgresql.password`     | `POSTGRES_PASSWORD`     | `""`        |
| `postgresql.database`     | `POSTGRES_DATABASE`     | `postgres`  |
| `postgresql.schema`       | `POSTGRES_SCHEMA`       | `public`    |
| `postgresql.sslmode`      | `POSTGRES_SSLMODE`      | `disable`   |
| `postgresql.dsn`          | `POSTGRES_DSN`          | `""`        |
| `postgresql.read_only`    | `POSTGRES_READ_ONLY`    | `false`     |
| `postgresql.query_timeout`| `POSTGRES_QUERY_TIMEOUT` | `30`       |

## MCP Tools Provided

| Tool            | Description                        | Write? |
|-----------------|------------------------------------|--------|
| `list_database` | List all databases                 | No     |
| `list_schema`   | List user schemas                  | No     |
| `list_table`    | List tables in a schema            | No     |
| `desc_table`    | Describe table structure           | No     |
| `create_table`  | Create a new table                 | Yes    |
| `alter_table`   | Alter an existing table            | Yes    |
| `read_query`    | Execute SELECT queries             | No     |
| `write_query`   | Execute INSERT queries             | Yes    |
| `update_query`  | Execute UPDATE queries             | Yes    |
| `delete_query`  | Execute DELETE queries             | Yes    |
