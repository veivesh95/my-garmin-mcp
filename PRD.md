# PRD: garmin-mcp

| | |
|---|---|
| Owner | Veivesh |
| Status | Draft, v0.1 scaffold |
| Last updated | 2026-11-21 |
| Language | Go (official `modelcontextprotocol/go-sdk`) |

## 1. Problem statement

I run often with a Garmin Forerunner 55, and my training data sits in Garmin Connect. To ask an AI questions like "how did my long runs trend this month?" I currently have to export or copy data by hand. Garmin now offers a free, official Personal API for my own data, but AI assistants can't use it directly. Without a bridge, the data stays siloed and analysis stays manual.

## 2. Goals

1. Ask Claude (desktop, Claude Code, web and mobile) about my Garmin data and get answers grounded in real API responses.
2. Keep the server available 24/7 at zero hosting cost.
3. Keep my data and keys private: no unauthenticated access, no secrets in code or git.
4. Make maintenance boring: key rotation and endpoint changes are a config edit, not a code change.
5. Learn idiomatic Go on a project small enough to finish.

## 3. Non-goals

- **Multi-user or commercial use.** The Personal API is for one user's own data; multi-user apps belong in Garmin's business developer program.
- **Writing to Garmin** (creating workouts, editing activities). Read-only keeps the risk low.
- **Storing or syncing data in a database.** The server proxies live calls; Garmin stays the source of truth.
- **Unofficial / scraped Garmin endpoints.** Only the documented Personal API.
- **Own dashboards or UI.** The AI client is the interface.

## 4. User stories

- As the owner, I want to ask "what were my runs this week?" so that I get a summary without opening Garmin Connect.
- As the owner, I want to drill into one run by ID so that I can discuss pace, heart rate and splits.
- As the owner, I want a single diagnostic tool so that, when something breaks, I can tell whether it's the key, the token or an unconfigured endpoint.
- As the owner, I want the server to refresh its access token on its own so that I never handle the 24-hour token.
- As the owner, I want clear errors when my API key expires so that I know to create a new one.
- As the owner on my phone, I want to use the same server from the Claude mobile app so that it works wherever I am.

## 5. Requirements

### P0 – must have (v0.1)

| ID | Requirement | Acceptance criteria |
|---|---|---|
| P0-1 | Token exchange and caching | Given a valid key, the first call fetches a token and later calls reuse it until 10 min before expiry. On a 401 from a data call, the token is dropped and the call retried once. Covered by `token_test.go`. |
| P0-2 | `check_garmin_connection` tool | Reports `token_ok`, token expiry, any error, and the list of constants still set to placeholders. Works before any data endpoint is configured. |
| P0-3 | `list_activities` tool | Accepts optional `start_date`, `end_date` (YYYY-MM-DD) and `limit` (1–50, default 10). Defaults to the last 30 days. Rejects invalid dates and limits with a clear error. |
| P0-4 | `get_activity` tool | Accepts a numeric `activity_id`. Non-numeric IDs are rejected before any request is sent. |
| P0-5 | Two transports | `MCP_TRANSPORT=stdio` for local clients; `http` serves streamable HTTP. |
| P0-6 | Endpoint auth in HTTP mode | Server refuses to start in HTTP mode without `MCP_BEARER_TOKEN` or `MCP_URL_SECRET` (each ≥ 32 chars). Wrong or missing bearer → 401. |
| P0-7 | Config by constants | Every value I must edit by hand is a constant in `internal/config/config.go`, marked `TODO(manual)`. Unset placeholders produce a startup warning and a tool error, not a crash. |
| P0-8 | Secrets from environment | Garmin key and endpoint secrets come only from env vars. `.env` is git-ignored. |
| P0-9 | Free always-on deployment | Runs on a free VM (Oracle Cloud Always Free or similar) via Docker Compose or systemd, bound to loopback, exposed over HTTPS through Cloudflare Tunnel or Caddy. |

### P1 – nice to have (next)

| ID | Requirement | Notes |
|---|---|---|
| P1-1 | Typed outputs | Replace `RawOutput{Data any}` with structs for the fields that matter (distance, duration, pace, average/max HR, elevation). Smaller responses, better answers. |
| P1-2 | Wellness tools | Sleep, resting HR, HRV, stress, steps, Body Battery, if the Forerunner 55 data and Swagger support them. |
| P1-3 | Pagination | Let `list_activities` page past the limit using the `start` offset. |
| P1-4 | Key-expiry warning | `check_garmin_connection` warns when the key nears expiry (needs the expiry from Garmin or a `GARMIN_API_KEY_EXPIRES` env var). |
| P1-5 | CI | GitHub Actions running `go vet` and `go test`. |

### P2 – future considerations

- **OAuth 2.1 for the HTTP endpoint** instead of a URL secret, so claude.ai connectors authenticate properly. Keep auth in middleware so this can slot in.
- **Response caching** for past dates, which never change, to cut Garmin calls.
- **FIT file download and parsing** for lap/split detail.
- **Training insights tools** (weekly mileage, pace trends) computed server-side.

## 6. Architecture

```
Claude client ──HTTPS──> Cloudflare Tunnel / Caddy ──> garmin-mcp (127.0.0.1:8080)
                                                          │
                                         auth middleware (bearer) or secret path
                                                          │
                                                   MCP tools (internal/tools)
                                                          │
                                   garmin.Client ── TokenSource (cached, mutex)
                                                          │
                                            Garmin Connect Personal API
```

Key decisions:

- **Go + official SDK.** Single static binary, low memory on a free VM, and typed tool schemas generated from structs.
- **Stateless proxy.** No database; tokens live only in memory.
- **Two auth routes.** Bearer header for clients that support headers; secret URL path for claude.ai / mobile custom connectors, which, as far as I know, don't let you set a custom header. The URL secret is weaker (URLs can end up in logs) and is the main reason OAuth is on the P2 list.
- **Placeholders don't crash.** The server is usable for diagnostics before every endpoint is known.

## 7. Configuration reference

| Constant / variable | Where | Must edit? |
|---|---|---|
| `GarminAPIBaseURL` | config.go | Yes, from Swagger |
| `ActivitiesListPath`, `Param*` | config.go | Yes, from Swagger |
| `ActivityDetailPath` | config.go | Yes, from Swagger |
| `AuthSchemeData` | config.go | Confirm |
| `TokenFieldAccessToken`, `TokenFieldExpiresIn` | config.go | Confirm with one curl |
| Limits, timeouts, paths | config.go | Optional |
| `GARMIN_API_KEY` | .env | Yes |
| `MCP_BEARER_TOKEN` / `MCP_URL_SECRET` | .env | At least one in HTTP mode |
| `MCP_TRANSPORT`, `LISTEN_ADDR` | .env | Optional |

## 8. Security and privacy

- Health data is personal. The endpoint is never exposed without auth, and the container is published only on loopback.
- Secrets are compared in constant time; never logged; never committed.
- Activity IDs are validated as digits before being placed in a URL path.
- Garmin responses are capped at 5 MB.
- The Garmin key expires (30 days to one year). Rotation = new key in Garmin Connect, update `.env`, restart.
- Garmin's Personal API terms apply; re-read them before adding features.

## 9. Success metrics

Personal project, so simple and checkable:

- **Works end to end:** `check_garmin_connection` returns `token_ok: true` and `unconfigured_constants: []` from the hosted server.
- **Useful:** at least 3 real training questions per week answered from live data within the first month.
- **Reliable:** no manual restarts needed over 30 days (Docker `restart: unless-stopped`).
- **Cost:** $0 per month.

## 10. Open questions

| Question | Blocking? | Answer from |
|---|---|---|
| Exact base URL, paths and query names for activities | Yes, for P0-3/4 | Swagger docs |
| Token response field names and whether expiry is included | No, defaults exist | One curl call |
| Which wellness endpoints exist and work with a Forerunner 55 | No, P1-2 | Swagger + testing |
| Rate limits on the Personal API | No | Garmin docs / testing |
| Does the go-sdk HTTP handler need options to accept requests via a tunnel (Host / origin checks)? | Maybe | First deployment |
| Does Oracle reclaim idle Always Free VMs in my region? | No | Oracle docs; fallback: another free VM |

## 11. Phases

1. **v0.1 – local:** fill in constants, `go test`, run over stdio in Claude Desktop or Claude Code.
2. **v0.2 – hosted:** free VM, Docker Compose, Cloudflare Tunnel, bearer route for Claude Code.
3. **v0.3 – mobile:** secret-URL connector in claude.ai, used from the Claude app.
4. **v0.4+:** P1 items, starting with typed outputs.
