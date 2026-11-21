# garmin-mcp

An MCP server in Go that exposes your own Garmin Connect data (via the official Personal API) to Claude and other MCP clients. See `PRD.md` for goals, scope and design decisions.

> **Status:** scaffold. Written against `modelcontextprotocol/go-sdk` v1.7.0 but not yet compiled. Expect to fix small API mismatches on the first `go build`.

## Manual checklist

Everything you need to edit by hand is a constant in `internal/config/config.go`, marked `TODO(manual)`. Secrets go in `.env`, never in code.

1. **Create a Personal API key** in Garmin Connect Web (Settings → Personal API Key). Copy it; it's shown once.
2. **Check the token response shape** once:
   ```sh
   curl -s -X POST 'https://personal.connectapi.garmin.com/personalapitoken-service/personal-api/personal-api-key/token' \
     -H "Authorization: ApiKey $GARMIN_API_KEY"
   ```
   If the fields aren't `access_token` / `expires_in`, update `TokenFieldAccessToken` and `TokenFieldExpiresIn`.
3. **From the Swagger docs**, fill in:
   - `GarminAPIBaseURL`
   - `ActivitiesListPath` and the `Param*` query names
   - `ActivityDetailPath` (use `{id}` for the activity ID)
   - `AuthSchemeData` if data calls don't use `Bearer`
4. Run the `check_garmin_connection` tool. It reports whether the token works and which constants are still placeholders.

## Build and test

```sh
go mod tidy          # fetches the SDK and writes go.sum
go test ./...
go build -o garmin-mcp ./cmd/garmin-mcp
```

## Run locally (stdio)

For Claude Desktop, add to `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "garmin": {
      "command": "/absolute/path/to/garmin-mcp",
      "env": { "GARMIN_API_KEY": "your-key", "MCP_TRANSPORT": "stdio" }
    }
  }
}
```

For Claude Code:

```sh
claude mcp add garmin --env GARMIN_API_KEY=your-key --env MCP_TRANSPORT=stdio -- /absolute/path/to/garmin-mcp
```

## Run hosted (HTTP)

```sh
cp .env.example .env     # fill in GARMIN_API_KEY and one or both secrets
openssl rand -hex 32     # use for MCP_BEARER_TOKEN / MCP_URL_SECRET
docker compose up -d --build
curl http://127.0.0.1:8080/healthz
```

The container is published only on the VM's loopback. Put HTTPS in front with **Cloudflare Tunnel** (no open ports) or **Caddy** with a domain.

Two routes, use whichever your client supports:

| Route | Auth | For |
|---|---|---|
| `https://your-domain/mcp` | `Authorization: Bearer <MCP_BEARER_TOKEN>` | Claude Code, MCP Inspector |
| `https://your-domain/mcp/<MCP_URL_SECRET>` | secret in the URL | claude.ai and Claude mobile custom connectors |

Claude Code over HTTP:

```sh
claude mcp add --transport http garmin https://your-domain/mcp --header "Authorization: Bearer <token>"
```

For claude.ai / mobile: add a custom connector with the secret URL. Treat that URL as a password; rotate `MCP_URL_SECRET` if it leaks.

Without Docker, see `deploy/garmin-mcp.service` for a systemd unit.

## Troubleshooting

- **403 behind the tunnel or proxy:** recent go-sdk versions may include localhost / DNS-rebinding or cross-origin protection in the streamable HTTP handler. Check `mcp.StreamableHTTPOptions` for the relevant option.
- **"garmin rejected the API key":** the key is wrong, deleted or expired. Create a new one and update `.env`.
- **"token response has no ... field":** the error lists the real field names. Update the `TokenField*` constants.
- **Test with MCP Inspector:** `npx @modelcontextprotocol/inspector`, then connect to `http://127.0.0.1:8080/mcp` with the bearer header.

## Layout

```
cmd/garmin-mcp/main.go      entry point, transports, HTTP routes
internal/config/config.go   ALL manual constants + env loading
internal/garmin/            token exchange/cache and HTTP client
internal/auth/              bearer-token middleware
internal/tools/             MCP tool definitions
deploy/                     systemd unit
```
