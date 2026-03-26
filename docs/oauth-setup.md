# Google Workspace OAuth Authentication Setup

This guide explains how to configure Google Workspace OAuth 2.0 authentication for the Streamable HTTP transport. When enabled, the MCP server acts as an OAuth Authorization Server proxy, wrapping Google OAuth to authenticate users.

## Overview

```
Claude Code/Desktop  <-->  MCP Server (AS Proxy + RS)  <-->  Google OAuth
                                |
                                +-- /.well-known/oauth-protected-resource
                                +-- /.well-known/oauth-authorization-server
                                +-- /authorize -> consent -> Google OAuth
                                +-- /callback  <- Google OAuth callback
                                +-- /token     (code -> JWT exchange)
                                +-- /mcp       (protected, Bearer JWT)
```

The server supports two types of OAuth clients:

- **CIMD clients** (e.g., Claude Code): `client_id` is an HTTPS URL pointing to a client metadata document
- **Preregistered clients** (e.g., Claude Desktop App): `client_id` is defined in the config file

## Prerequisites

### 1. Public HTTPS URL

The MCP server must be accessible via a public HTTPS URL. For local development, you can use:

- [ngrok](https://ngrok.com/): `ngrok http 8080`
- [Cloudflare Tunnel](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/)

The public URL will be used as the `issuer` in the OAuth configuration.

### 2. Google Cloud Console Setup

1. Go to [Google Cloud Console](https://console.cloud.google.com/)
2. Create a new project (or select an existing one)
3. Navigate to **APIs & Services** > **Credentials**
4. Click **Create Credentials** > **OAuth client ID**
5. Select **Web application** as the application type
6. Set the **Authorized redirect URIs** to: `{your-public-url}/callback`
   - Example: `https://mcp.example.com/callback`
7. Note the **Client ID** and **Client Secret**

If you haven't configured the OAuth consent screen yet:
1. Navigate to **APIs & Services** > **OAuth consent screen**
2. Choose **Internal** (for Google Workspace) or **External**
3. Fill in the required fields (app name, support email)
4. Add scopes: `openid`, `email`, `profile`

## Configuration

Add the `oauth` section to your `config.yml`:

```yaml
http:
  host: '0.0.0.0'
  port: 8080
  endpoint: '/mcp'

oauth:
  enabled: true
  issuer: 'https://mcp.example.com'
  signing_key: ''  # Generate with: openssl rand -hex 32
  token_expiry: 3600  # seconds (default: 1 hour)

  google:
    client_id: 'your-google-client-id.apps.googleusercontent.com'
    client_secret: 'your-google-client-secret'
    allowed_domains: []  # e.g., ['example.com'] to restrict to a Google Workspace domain
    allowed_emails: []   # e.g., ['user@gmail.com'] to allow specific accounts

  clients: []  # Preregistered clients (see below)
```

### Generate a Signing Key

The signing key is used to sign JWT access tokens (HMAC-SHA256). It must be at least 32 bytes:

```bash
openssl rand -hex 32
```

### Access Restrictions

Control who can authenticate:

| Configuration | Behavior |
|---|---|
| Both `allowed_domains` and `allowed_emails` empty | Any Google account can authenticate |
| `allowed_domains: ['example.com']` | Only `example.com` Google Workspace users |
| `allowed_emails: ['user@gmail.com']` | Only specific email addresses |
| Both set | Either domain match OR email match grants access |

All accounts must have a verified email address (`email_verified: true`).

### Preregistered Clients

For clients that don't support CIMD (e.g., some OAuth-capable desktop apps), register them in the config:

```yaml
oauth:
  clients:
    - client_id: 'my-desktop-app'
      client_name: 'My Desktop App'
      redirect_uris:
        - 'http://localhost:3000/callback'
```

### Environment Variables

| Variable | Description | Default |
|---|---|---|
| `OAUTH_ENABLED` | Enable OAuth authentication | `false` |
| `OAUTH_ISSUER` | Public HTTPS URL of the server | (empty) |
| `OAUTH_SIGNING_KEY` | JWT signing key (>= 32 bytes) | (empty) |
| `OAUTH_TOKEN_EXPIRY` | Token expiry in seconds | `3600` |
| `GOOGLE_CLIENT_ID` | Google OAuth client ID | (empty) |
| `GOOGLE_CLIENT_SECRET` | Google OAuth client secret | (empty) |
| `GOOGLE_ALLOWED_DOMAINS` | Comma-separated allowed domains | (empty) |
| `GOOGLE_ALLOWED_EMAILS` | Comma-separated allowed emails | (empty) |

## Running with Docker

```bash
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

Generate a signing key beforehand and reuse it across restarts:
```bash
openssl rand -hex 32
```

When using a reverse proxy (nginx, Caddy, etc.) in front of Docker, ensure:
- The proxy terminates TLS and forwards to `http://localhost:8080`
- The `OAUTH_ISSUER` matches the public URL that clients will use
- The Google OAuth redirect URI (`{issuer}/callback`) is registered in Google Cloud Console

## Connecting Clients

### Claude Code

```bash
claude mcp add mcp-postgresql --transport http https://mcp.example.com/mcp
```

Claude Code supports CIMD and will automatically discover OAuth endpoints via the `.well-known` metadata.

### Claude Desktop App

Add to your `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "postgresql": {
      "url": "https://mcp.example.com/mcp"
    }
  }
}
```

## OAuth Flow

When a client connects without a token:

1. Server responds with `401` + `WWW-Authenticate: Bearer resource_metadata=...`
2. Client discovers OAuth endpoints via `/.well-known/oauth-protected-resource`
3. Client fetches AS metadata from `/.well-known/oauth-authorization-server`
4. Client redirects user to `/authorize` with PKCE challenge
5. Server shows a local consent screen (application name, redirect URI, scope)
6. User approves, server redirects to Google for authentication
7. Google authenticates user and redirects back to `/callback`
8. Server validates Google ID token, issues an authorization code
9. Client exchanges the code for a JWT access token at `/token`
10. Client uses the JWT to access `/mcp`

## Endpoints

| Endpoint | Auth | Description |
|---|---|---|
| `/.well-known/oauth-protected-resource` | No | Protected Resource Metadata (RFC 9728) |
| `/.well-known/oauth-authorization-server` | No | Authorization Server Metadata (RFC 8414) |
| `/authorize` | No | Authorization endpoint |
| `/consent` | No | Consent form submission |
| `/callback` | No | Google OAuth callback |
| `/token` | No | Token exchange (CORS enabled) |
| `/mcp` | JWT | MCP endpoint (OAuth protected) |
| `/health` | No | Health check |

## Security Notes

- **PKCE (S256)** is mandatory for all authorization requests
- **JWT access tokens** are bound to the resource URL (`aud` claim) and client ID
- **Authorization codes** are single-use with a 5-minute TTL
- **Consent records** expire after 10 minutes
- **Google ID tokens** are fully validated (signature via JWKS, issuer, expiry, audience)
- **CIMD fetches** have SSRF protection (private IP blocking, no redirects, 5s timeout)
- **Signing key** must be at least 32 bytes of cryptographic randomness
- **Refresh tokens** are not supported; clients must re-authenticate when the token expires

## Limitations

- **Single instance only**: The in-memory store does not support horizontal scaling
- **No refresh tokens**: Clients re-authenticate after token expiry
- **No Dynamic Client Registration (DCR)**: Use CIMD or preregistered clients
- **No token revocation**: JWT tokens are short-lived (configurable via `token_expiry`)
- **Consent is not persisted**: Users must re-consent after server restart
