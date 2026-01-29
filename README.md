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
log: 'mcp-postgresql.log'  # Log file path (empty for no logging)
debug: false

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
```

### Environment Variables

All configuration options can be overridden via environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `LOG_PATH` | Log file path | (empty) |
| `DEBUG` | Enable debug mode | false |
| `POSTGRES_HOST` | PostgreSQL host | localhost |
| `POSTGRES_PORT` | PostgreSQL port | 5432 |
| `POSTGRES_USER` | PostgreSQL user | postgres |
| `POSTGRES_PASSWORD` | PostgreSQL password | (empty) |
| `POSTGRES_DATABASE` | Database name | postgres |
| `POSTGRES_SCHEMA` | Default schema | public |
| `POSTGRES_SSLMODE` | SSL mode | disable |
| `POSTGRES_DSN` | Direct DSN connection string | (empty) |
| `POSTGRES_READ_ONLY` | Read-only mode | false |

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

### Running the Server

```bash
# With default config
./bin/mcp-postgresql server

# With custom config
./bin/mcp-postgresql server --config=/path/to/config.yml
```

### Docker

```bash
docker run -e POSTGRES_HOST=host.docker.internal \
           -e POSTGRES_USER=postgres \
           -e POSTGRES_PASSWORD=secret \
           -e POSTGRES_DATABASE=mydb \
           cnosuke/mcp-postgresql
```

### Claude Desktop Configuration

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

## Available Tools

### Schema Tools

| Tool | Description | Read-Only Mode |
|------|-------------|----------------|
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

All tools support an optional `dsn` parameter to override the configured connection:

```json
{
  "name": "list_table",
  "arguments": {
    "schema": "my_schema",
    "dsn": "postgres://user:pass@host:5432/db"
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
