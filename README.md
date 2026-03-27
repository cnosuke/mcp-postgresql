# mcp-postgresql

A Model Context Protocol (MCP) server implementation for PostgreSQL.

## Features

- List databases, schemas, and tables
- Describe table structures (columns, constraints, indexes, comments)
- Execute read-only queries (SELECT)
- Execute write queries (INSERT, UPDATE, DELETE) with RETURNING support
- Create and alter tables
- Support for PostgreSQL schemas
- Read-only mode for safe operation
- Connection via DSN or individual parameters
- URL-style DSN support (`postgres://`, `postgresql://`)
- Connection presets for secure multi-database access (LLM never sees passwords)
- **Dual transport**: stdio and Streamable HTTP (MCP spec 2025-11-25)
- Bearer token authentication and Origin validation for HTTP transport
- **Google Workspace OAuth 2.0** authentication ([setup guide](docs/oauth-setup.md))

## Installation

### From Source

```bash
git clone https://github.com/cnosuke/mcp-postgresql.git
cd mcp-postgresql
make
```

### Docker

```bash
docker pull cnosuke/mcp-postgresql
```

## Configuration

### Configuration File (config.yml)

```yaml
log: 'mcp-postgresql.log'  # Log file path (empty for no file logging; HTTP mode always outputs to console)
log_level: 'info'          # debug, info, warn, error

postgresql:
  host: 'localhost'
  user: 'postgres'
  password: ''
  port: 5432
  database: 'postgres'    # Default database
  schema: 'public'        # Default schema for search_path
  sslmode: 'disable'      # disable, allow, prefer, require, verify-ca, verify-full
  dsn: ''                 # Direct DSN (overrides above settings)
  read_only: false        # Enable read-only mode (disables write tools)

http:
  host: '127.0.0.1'        # Bind address (default: 127.0.0.1)
  port: 8080               # Listen port (default: 8080)
  endpoint: '/mcp'          # MCP endpoint path (default: /mcp)
  auth_token: ''            # Bearer token for authentication (empty = no auth)
  allowed_origins: []       # Allowed Origin headers (empty = allow all)

presets:
  production:
    host: 'prod-db.example.com'
    user: 'app_user'
    password: 'prod_password'
    port: 5432
    database: 'production_db'
    sslmode: 'require'
    read_only: true
  staging:
    dsn: 'postgres://staging_user:pass@staging-db:5432/staging_db'
    read_only: false
```

### Connection Presets

Presets allow managing multiple database connections without exposing passwords to the LLM. Each preset defines a named connection profile in the config file.

Preset fields: `host`, `port`, `user`, `password`, `database`, `schema`, `sslmode`, `dsn`, `read_only`, `query_timeout`

- When `dsn` is set in a preset, it takes priority over individual fields
- `query_timeout: 0` falls back to the global `postgresql.query_timeout` value
- `read_only: true` presets block all write operations at runtime
- Passwords are never exposed through the `list_preset` tool

### Environment Variables

All configuration options can be overridden via environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `LOG_PATH` | Log file path | (empty) |
| `LOG_LEVEL` | Log level (debug, info, warn, error) | info |
| `POSTGRES_HOST` | PostgreSQL host | localhost |
| `POSTGRES_PORT` | PostgreSQL port | 5432 |
| `POSTGRES_USER` | PostgreSQL user | postgres |
| `POSTGRES_PASSWORD` | PostgreSQL password | (empty) |
| `POSTGRES_DATABASE` | Database name | postgres |
| `POSTGRES_SCHEMA` | Default schema | public |
| `POSTGRES_SSLMODE` | SSL mode | disable |
| `POSTGRES_DSN` | Direct DSN connection string | (empty) |
| `POSTGRES_READ_ONLY` | Read-only mode | false |
| `POSTGRES_QUERY_TIMEOUT` | Query timeout in seconds | 30 |
| `HTTP_HOST` | HTTP server bind address | 127.0.0.1 |
| `HTTP_PORT` | HTTP server port | 8080 |
| `HTTP_ENDPOINT` | MCP endpoint path | /mcp |
| `HTTP_AUTH_TOKEN` | Bearer token for authentication | (empty) |
| `HTTP_ALLOWED_ORIGINS` | Allowed Origin headers | (empty) |
| `OAUTH_ENABLED` | Enable OAuth authentication | false |
| `OAUTH_ISSUER` | Public HTTPS URL of the server | (empty) |
| `OAUTH_SIGNING_KEY` | JWT signing key (>= 32 bytes) | (empty) |
| `OAUTH_TOKEN_EXPIRY` | Token expiry in seconds | 3600 |
| `GOOGLE_CLIENT_ID` | Google OAuth client ID | (empty) |
| `GOOGLE_CLIENT_SECRET` | Google OAuth client secret | (empty) |
| `GOOGLE_ALLOWED_DOMAINS` | Allowed Google Workspace domains | (empty) |
| `GOOGLE_ALLOWED_EMAILS` | Allowed email addresses | (empty) |

See [OAuth setup guide](docs/oauth-setup.md) for details.

### DSN Formats

The server supports two DSN formats:

**Key-Value Format:**
```
host=localhost port=5432 user=postgres password=secret dbname=mydb sslmode=disable
```

**URL Format:**
```
postgres://postgres:secret@localhost:5432/mydb?sslmode=disable
postgresql://postgres:secret@localhost:5432/mydb?sslmode=disable
```

## Usage

### Stdio Transport (default)

```bash
# With default config
./bin/mcp-postgresql server

# With custom config
./bin/mcp-postgresql server --config=/path/to/config.yml
```

### Streamable HTTP Transport

```bash
# Start HTTP server
./bin/mcp-postgresql http --config=/path/to/config.yml
# → http://127.0.0.1:8080/mcp
# → http://127.0.0.1:8080/health (no auth required)
```

The HTTP transport supports:
- **Origin validation** (MCP spec MUST requirement, DNS rebinding prevention)
- **Bearer token authentication** (timing-safe comparison)
- **Google Workspace OAuth 2.0** (PKCE, CIMD, Google ID token validation) — see [OAuth setup guide](docs/oauth-setup.md)
- **Health check endpoint** at `/health` (no authentication required)

### Docker

```bash
# Stdio transport
docker run -e POSTGRES_HOST=host.docker.internal \
           -e POSTGRES_USER=postgres \
           -e POSTGRES_PASSWORD=secret \
           -e POSTGRES_DATABASE=mydb \
           cnosuke/mcp-postgresql

# HTTP transport
docker run -p 8080:8080 \
           -e POSTGRES_HOST=host.docker.internal \
           -e POSTGRES_USER=postgres \
           -e POSTGRES_PASSWORD=secret \
           -e POSTGRES_DATABASE=mydb \
           -e HTTP_AUTH_TOKEN=your-secret-token \
           cnosuke/mcp-postgresql http --config=/app/config.yml

# HTTP transport with OAuth
docker run -p 8080:8080 \
           -e POSTGRES_HOST=host.docker.internal \
           -e POSTGRES_USER=postgres \
           -e POSTGRES_PASSWORD=secret \
           -e POSTGRES_DATABASE=mydb \
           -e OAUTH_ENABLED=true \
           -e OAUTH_ISSUER=https://mcp.example.com \
           -e OAUTH_SIGNING_KEY=your-pre-generated-signing-key \
           -e GOOGLE_CLIENT_ID=your-client-id.apps.googleusercontent.com \
           -e GOOGLE_CLIENT_SECRET=your-client-secret \
           -e GOOGLE_ALLOWED_DOMAINS=example.com \
           cnosuke/mcp-postgresql http --config=/app/config.yml
```

### Client Configuration

#### Claude Desktop (stdio)

Add to your `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "postgresql": {
      "command": "/path/to/mcp-postgresql",
      "args": ["server", "--config=/path/to/config.yml"]
    }
  }
}
```

Or with Docker:

```json
{
  "mcpServers": {
    "postgresql": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm",
        "-e", "POSTGRES_HOST=host.docker.internal",
        "-e", "POSTGRES_USER=postgres",
        "-e", "POSTGRES_PASSWORD=secret",
        "-e", "POSTGRES_DATABASE=mydb",
        "cnosuke/mcp-postgresql"
      ]
    }
  }
}
```

#### Claude Code (HTTP)

```bash
# Without authentication
claude mcp add mcp-postgresql --transport http http://localhost:8080/mcp

# With Bearer token authentication
claude mcp add mcp-postgresql --transport http http://localhost:8080/mcp \
  -H "Authorization: Bearer <token>"
```

## Available Tools

### Schema Tools

| Tool | Description | Read-Only Mode |
|------|-------------|----------------|
| `list_preset` | List configured connection presets | Available |
| `list_database` | List all databases | Available |
| `list_schema` | List all schemas (excluding system schemas) | Available |
| `list_table` | List tables in a schema | Available |
| `desc_table` | Describe table structure | Available |
| `create_table` | Create a new table | Disabled |
| `alter_table` | Alter an existing table | Disabled |

### Data Tools

| Tool | Description | Read-Only Mode |
|------|-------------|----------------|
| `read_query` | Execute SELECT queries | Available |
| `write_query` | Execute INSERT queries (RETURNING supported) | Disabled |
| `update_query` | Execute UPDATE queries (RETURNING supported) | Disabled |
| `delete_query` | Execute DELETE queries (RETURNING supported) | Disabled |

### Tool Parameters

All tools support optional `dsn` and `preset` parameters to override the configured connection. These two parameters cannot be used together.

**Using DSN:**
```json
{
  "name": "list_table",
  "arguments": {
    "schema": "my_schema",
    "dsn": "postgres://user:pass@host:5432/db"
  }
}
```

**Using Preset:**
```json
{
  "name": "read_query",
  "arguments": {
    "preset": "production",
    "query": "SELECT * FROM users LIMIT 10"
  }
}
```

### PostgreSQL-Specific Features

#### Schema Support

The `list_table` and `desc_table` tools support a `schema` parameter:

```json
{
  "name": "list_table",
  "arguments": {
    "schema": "my_schema"
  }
}
```

#### RETURNING Clause

Write operations (`write_query`, `update_query`, `delete_query`) support PostgreSQL's RETURNING clause:

```sql
INSERT INTO users (name, email) VALUES ('John', 'john@example.com') RETURNING id, created_at;
UPDATE users SET status = 'active' WHERE id = 1 RETURNING *;
DELETE FROM users WHERE id = 1 RETURNING *;
```

#### Table Comments

PostgreSQL uses separate `COMMENT ON` statements for table and column comments:

```sql
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL
);

COMMENT ON TABLE users IS 'User account information';
COMMENT ON COLUMN users.name IS 'Full name of the user';
```

## Development

### Build

```bash
make
```

### Test

```bash
make test
```

### Docker Build

```bash
make docker-build
```

## License

MIT License

## Author

[@cnosuke](https://github.com/cnosuke)
