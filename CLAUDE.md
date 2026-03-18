# CLAUDE.md - mcp-postgresql

## Project Overview

MCP (Model Context Protocol) server implementation for PostgreSQL.
Provides database access tools (schema inspection, CRUD queries) via stdio and Streamable HTTP transports.

## Tech Stack

- **Language**: Go 1.25
- **MCP**: [modelcontextprotocol/go-sdk](https://github.com/modelcontextprotocol/go-sdk)
- **DB Driver**: jackc/pgx/v5 (via stdlib) + jmoiron/sqlx
- **CLI**: urfave/cli/v3
- **Config**: jinzhu/configor (YAML + env vars)
- **Error Handling**: cockroachdb/errors
- **Logging**: uber-go/zap (to file, suppressed on stdout to avoid MCP stdio interference)
- **Testing**: stretchr/testify

## Project Structure

```
main.go           # CLI entrypoint (urfave/cli) - server/http subcommands
config/
  config.go       # Config struct (YAML + env vars)
config.yml        # Default config file
server/
  server.go       # DBManager, NewMCPServer(), DB connection pooling
  http.go         # RunHTTP() - Streamable HTTP transport
  middleware.go   # Bearer token auth + Origin validation middleware
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

# Run (stdio transport)
bin/mcp-postgresql server -c config.yml

# Run (Streamable HTTP transport)
bin/mcp-postgresql http -c config.yml
```

## Architecture Notes

- **Dual transport**: Supports both stdio (`server` subcommand) and Streamable HTTP (`http` subcommand) transports.
- **stdio transport**: MCP communication uses stdin/stdout. Logger output MUST go to a file (not stdout/stderr) to avoid corrupting the MCP protocol.
- **HTTP transport**: Uses go-sdk `NewStreamableHTTPHandler()` with custom `http.ServeMux` for routing. Includes Origin validation (go-sdk built-in `CrossOriginProtection` + custom middleware) and optional Bearer token authentication with `crypto/subtle.ConstantTimeCompare`. Health check at `/health` (no auth).
- **Read-only mode**: When `read_only: true`, write tools (create_table, alter_table, write_query, update_query, delete_query) are not registered, and DB sessions are set to read-only.
- **DSN resolution**: Priority: Tool DSN param > Tool preset param > cfg.PostgreSQL.DSN > individual fields. Supports both key=value (`host=localhost port=5432 ...`) and URL format (`postgres://user:pass@host/db`).
- **Connection presets**: `presets` map in config defines named connection profiles. Preset DSN resolution: preset DSN field > build from preset fields. Cache key includes read-only flag to avoid mixing connections.
- **Connection pooling**: `DBManager` uses double-checked locking pattern for thread-safe lazy connection creation, keyed by DSN string + read-only flag.
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
| `http.host`               | `HTTP_HOST`              | `127.0.0.1`|
| `http.port`               | `HTTP_PORT`              | `8080`     |
| `http.endpoint`           | `HTTP_ENDPOINT`          | `/mcp`     |
| `http.auth_token`         | `HTTP_AUTH_TOKEN`        | `""`       |
| `http.allowed_origins`    | `HTTP_ALLOWED_ORIGINS`   | `[]`       |
| `presets.<name>.*`        | (YAML only)              | -           |

## MCP Tools Provided

| Tool            | Description                        | Write? |
|-----------------|------------------------------------|--------|
| `list_preset`   | List connection presets (no passwords) | No  |
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
