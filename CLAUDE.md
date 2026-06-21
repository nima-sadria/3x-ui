# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

### Backend (Go)

```sh
# Build the binary (embeds the frontend dist/ at compile time)
cd /root/3x-ui-mieru/3x-ui-mieru
go build -o x-ui .

# Run all Go tests
go test ./...

# Run a single package's tests
go test ./internal/sub/...
go test ./internal/xray/...

# Run a specific test by name
go test -run TestFoo ./internal/sub/...

# Run with race detector (used for concurrency-sensitive packages)
go test -race ./...

# Run the binary (uses /etc/x-ui/x-ui.db by default)
XUI_DEBUG=true ./x-ui
```

### Frontend (React + Vite)

```sh
cd /root/3x-ui-mieru/3x-ui-mieru/frontend

# Install dependencies
npm install

# Development server (proxies API to localhost:2053 by default)
npm run dev

# Full production build (runs openapi codegen first, then Vite)
npm run build

# TypeScript type check
npm run typecheck

# Lint
npm run lint

# Run frontend unit tests (Vitest)
npm test

# Watch mode
npm run test:watch

# Regenerate API types from the Go openapi spec (two steps)
# Step 1: regenerate Zod schemas from Go structs
cd .. && go run ./tools/openapigen
# Step 2: regenerate TypeScript API client from the generated spec
cd frontend && node --experimental-strip-types scripts/build-openapi.mjs
# Or run both at once:
npm run gen
```

### Full build pipeline

The Go binary embeds the Vite-built frontend at compile time via `//go:embed all:dist`.  
Running `go build` without first running `npm run build` will use whatever `dist/` is already on disk.

```sh
cd frontend && npm run build && cd .. && go build -o x-ui .
```

### Environment variables

| Variable | Effect |
|---|---|
| `XUI_DEBUG=true` | Enables debug logging and Gin debug mode |
| `XUI_LOG_LEVEL` | `debug\|info\|notice\|warning\|error` |
| `XUI_PORT` | Override the web panel port at startup |
| `XUI_DB_FOLDER` | Override the database folder (default `/etc/x-ui`) |
| `XUI_DB_TYPE=postgres` | Switch backend to PostgreSQL; requires `XUI_DB_DSN` |
| `XUI_DB_DSN` | PostgreSQL DSN |
| `XUI_DB_CACHE_MB` | SQLite page-cache size in MiB (default 32) |
| `XUI_DB_MMAP_MB` | SQLite mmap size in MiB (default 256) |
| `XUI_DB_SYNCHRONOUS` | SQLite synchronous mode (`FULL\|NORMAL\|OFF\|EXTRA`) |
| `XUI_BIN_FOLDER` | Directory containing the `xray` binary (default `bin/`) |
| `XUI_SKIP_HSTS` | Disable HSTS headers when running HTTPS |

---

## Architecture

### Repository layout

```
main.go                      ← entry point; CLI flags, DB init, starts web.Server
internal/
  config/        ← env-var config (version, paths, log level)
  database/
    db.go        ← InitDB, AutoMigrate, seeders, migrations
    model/       ← GORM model structs (Inbound, Client, Node, Host, …)
  xray/          ← Xray process management and gRPC API client
  mtproto/       ← standalone mtg sidecar manager (the precedent for external provider lifecycle)
  sub/           ← subscription link generation (V2Ray raw, Clash, JSON)
  eventbus/      ← in-process pub/sub (Telegram/email notifications, xray crash events)
  logger/        ← structured logger
  util/          ← crypto, random, JSON helpers, net utils
  web/
    web.go       ← web.Server: Gin engine, cron scheduler, background jobs
    controller/  ← HTTP handlers (thin; delegates to services)
    service/     ← business logic
    job/         ← scheduled background jobs (cron)
    runtime/     ← local vs. remote node routing (LocalDeps, Manager)
    middleware/  ← auth, CSRF, body limit, GZIP
    session/     ← cookie session and CSRF token
    websocket/   ← real-time push hub
    global/      ← shared in-memory state (hash storage)
    locale/      ← i18n (go-i18n)
frontend/        ← Vite + React 19 + Ant Design 6 + TypeScript
  src/
    api/         ← Axios client, React Query hooks, websocket bridge
    generated/   ← auto-generated Zod schemas + TypeScript types (do not edit by hand)
    pages/       ← route-level components (inbounds, clients, groups, hosts, nodes, settings, xray, sub)
    components/  ← shared UI components
    models/      ← TypeScript model interfaces (hand-written; parallels Go models)
    schemas/     ← hand-written Zod schemas for form validation
tools/
  openapigen/    ← Go tool that walks struct tags and emits Zod + TypeScript schemas
```

### Request lifecycle

1. Gin engine (`web.go → initRouter`) mounts three controller groups under `{basePath}`:
   - `IndexController` – serves `dist/index.html` for all panel HTML routes
   - `XUIController` – panel-specific HTML pages (login, subpage)
   - `APIController` – `/panel/api/…` (all JSON endpoints)
2. `APIController.checkAPIAuth` accepts **session cookie**, **`Authorization: Bearer <token>`** (hashed API token), or **mTLS client cert**.
3. All mutating requests also pass `CSRFMiddleware`.
4. Controllers call services; services call GORM for DB and the Xray gRPC API where needed.

### Database

- GORM with SQLite (default, single file at `/etc/x-ui/x-ui.db`) or PostgreSQL.
- Schema is managed via `db.AutoMigrate` in `InitDB` — **no migration files**; adding a field to a model struct and re-running the binary applies the migration.
- One-time data migrations ("seeders") are tracked in the `history_of_seeders` table so they run exactly once.
- Key tables: `inbounds`, `clients`, `client_inbounds`, `client_traffics`, `hosts`, `nodes`, `settings`, `api_tokens`, `client_external_links`, `inbound_fallbacks`.

### Inbound and client storage

`Inbound` stores Xray config as JSON text columns (`settings`, `streamSettings`, `sniffing`) alongside structured metadata (`port`, `protocol`, `tag`, `enable`, traffic counters, expiry).

`Client` / `ClientRecord` is the normalized client table — one row per unique `email`. The many-to-many join `ClientInbound` maps clients to inbounds, with an optional `FlowOverride`. This split (introduced when migrating away from clients-embedded-in-inbound-JSON) means traffic, enable state, and quotas live in one place regardless of how many inbounds a client is attached to.

### External provider pattern — MTProto (reference for Mieru)

`internal/mtproto` is the canonical pattern for managing an external process as a 3x-ui "provider" without touching Xray:

- **Process struct** wraps `os/exec.Cmd` — start, stop, signal, log piping.
- **Manager** holds a map of running `Instance` objects keyed by inbound ID. `Reconcile(desired)` diffs wanted vs. running and starts/stops accordingly.
- **MtprotoJob** (in `web/job/`) runs every 10 s via cron: reads enabled mtproto inbounds from DB, calls `Manager.Reconcile`, then drains `CollectTraffic()` deltas and folds them into the standard `InboundService.AddTraffic`.
- Config files are written to `bin/mtproto/mtg-{id}.toml`; an `Instance.fingerprint()` detects config changes so the process is restarted only when something actually changed.
- Traffic comes from scraping mtg's HTTP metrics endpoint, not from the Xray gRPC API.
- MTProto inbounds are stored in the standard `inbounds` table with `protocol = "mtproto"`.

**This is the exact pattern Mieru should follow**: dedicated `internal/mieru/` package, `MierujJob` in `web/job/`, separate DB tables for Mieru-specific config, `mita` subprocess management.

### Background jobs (cron)

All jobs are registered in `web.Server.startTask()` using `robfig/cron`. Job structs are zero-valued and lazily acquire their service dependencies (zero-value structs with method receivers that call `database.GetDB()` internally).

Key cadences: Xray running check 1 s, Xray traffic poll 5 s, mtproto reconcile 10 s, node heartbeat 5 s, node traffic sync 5 s.

### Xray config generation

`internal/web/service/xray.go` builds the full Xray JSON config from the DB (inbounds, routing template from `settings`, etc.) and writes it to `bin/config.json` before starting the Xray process. **Mieru must not appear here.**

### Traffic accounting

`XrayTrafficJob.Run()` → `InboundService.AddTraffic(traffics, clientTraffics)` → accumulates deltas into `inbounds.up/down` and `client_traffics.up/down` → triggers `disableInvalidInbounds` / `disableInvalidClients` when quotas are hit.

### Frontend API codegen pipeline

The Go tool in `tools/openapigen/` walks Go struct tags to emit `internal/web/dist/` Zod schemas and TypeScript types into `frontend/src/generated/`. The frontend script `scripts/build-openapi.mjs` then builds a typed Axios client. Always run `npm run gen` (or `go run ./tools/openapigen && npm run gen:api`) after changing API-facing structs or adding new endpoints, otherwise the frontend types will be stale.

### Authentication modes

- **Session** – cookie-based login; CSRF token enforced on mutations.
- **API token** – `Authorization: Bearer <token>`; tokens are SHA-256-hashed at rest in `api_tokens` table.
- **mTLS** – verified client certificate from the node CA pool acts as full auth; skips session/CSRF checks.

### Settings

All panel settings (port, cert paths, Telegram config, subscription settings, etc.) live as key/value rows in the `settings` table, accessed via `SettingService`. Feature flags for new providers should follow this pattern using `ENABLE_MIERU_PROVIDER` both as an env var (in `config/`) and as a DB setting if runtime toggle is needed.
