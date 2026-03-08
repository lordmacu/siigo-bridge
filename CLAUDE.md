# Siigo Middleware - Project Instructions

## What This Is
Middleware between **Siigo Pyme** (Colombian COBOL accounting software, ISAM files) and **Finearom** (Laravel B2B platform for aromas/fragrances). We read Siigo's ISAM files, detect changes, and sync to Finearom's REST API.

## Project Structure
```
siigo/
├── siigo-common/     # (listed below with telegram/)
├── siigo-common/     # SHARED Go module (isam/, parsers/, api/, config/, storage/, telegram/)
├── siigo-web/        # Web interface (Go HTTP :3210 + React/Vite) - PRIMARY
├── siigo-sync/       # CLI sync service (polling + API client)
├── siigo-app/        # Desktop app (Wails + Go) - LEGACY, replaced by siigo-web
├── docs/             # Documentation (9 MD files)
└── start.sh          # Build, run & deploy script
```

## Critical Rules

### 1. Never duplicate shared packages
All projects import from `siigo-common` via:
```
require siigo-common v0.0.0
replace siigo-common => ../siigo-common
```
**NEVER** create isam/, parsers/, api/, config/, or storage/ packages inside siigo-sync, siigo-app, or siigo-web.

### 2. EXTFH vs Binary offsets
ISAM records have DIFFERENT offsets depending on the reader:
- **EXTFH** (via cblrtsm.dll): Clean records, no markers
- **Binary fallback**: Includes 2-byte record markers, shifting all offsets

Every parser MUST check `isam.ExtfhAvailable()` and use dual offsets.

### 3. Verified offsets (EXTFH mode) — DO NOT CHANGE without hex dump evidence
- **Z17**: tipoDoc@18, nombre@36 (NOT @20 and @38 — those were wrong)
- **Z06CP**: nombre@46, fecha@38
- **Z49**: tipo@0(letter), codigo@1, numDoc@4, nombreTercero@15
- **Z09**: nit@16, cuenta@29, fecha@42, desc@93, D/C@143
- **Z06**: tipo@0, codigo@2, nombre@31

### 4. Data directory
Siigo data lives in `C:\DEMOS01\` (configured in `C:\Siigo\FILEPATH.TXT`).
Files use Windows-1252 encoding — decode with `golang.org/x/text/encoding/charmap`.

### 5. Packed decimal fields are NOT yet decoded
Monetary amounts in Z17, Z49, Z09, Z06CP are in COBOL packed decimal (BCD).
These fields are currently skipped. Do not attempt to read them as text.

## start.sh — Build, Run & Deploy Script

The `start.sh` script in the project root manages building, running, and deploying.
**Default mode: use `dev` during development, `start` for production.**

### Commands

| Command | What it does |
|---------|-------------|
| `./start.sh dev` | Go server (:3210) + Vite dev (:5173, hot reload). No tunnel. Use during development. |
| `./start.sh start` | Full build (React + Go) + server + Cloudflare tunnel. For production/sharing. |
| `./start.sh restart` | Recompile Go only, restart server. **Tunnel stays alive (same URL)**. |
| `./start.sh restart force` | Restart everything including tunnel. **New URL**. |
| `./start.sh stop` | Stop all services (server, tunnel, vite). |
| `./start.sh status` | Show running state, URLs, and API stats. |
| `./start.sh logs` | Show last 50 server log lines. |

### Workflow
- **Developing**: Run `./start.sh dev`. Edit React files (auto-reload via Vite). For Go changes, run `./start.sh restart`.
- **Sharing/demo**: Run `./start.sh start`. Get a public Cloudflare URL. Use `./start.sh restart` to apply Go changes without losing the URL.
- **Done**: Run `./start.sh stop`.

### Key behavior
- `restart` (without `force`) only restarts the Go server — the Cloudflare tunnel keeps running so the public URL does not change.
- `restart force` kills everything and creates a new tunnel (new URL).
- Config is auto-created at `siigo-web/config.json` with defaults if missing.
- Uses `taskkill` on Windows for reliable process cleanup.

## How to Add a New Parser
1. Hex dump: `cd siigo-sync && go run ./cmd/hexdump/ 'C:\DEMOS01\ZXXX'`
2. Write diagnostic script to test offsets on 15+ records
3. Create parser in `siigo-common/parsers/` following existing patterns
4. Add to peek tool: `siigo-sync/cmd/peek/main.go`
5. Build: `cd siigo-web && go build ./...`
6. If syncing: add to detector, config, and Laravel endpoint

## How to Validate Parsers
```bash
cd siigo-sync && go run ./cmd/peek/
```
Check: no truncated names, valid dates (2010-2030), recognizable type codes, non-empty fields.

## API Publica v1 & Swagger

The public API v1 is documented via OpenAPI/Swagger at `siigo-web/swagger.json` and served at `/api/v1/docs`.

### IMPORTANT: Keep Swagger updated
When adding, modifying, or removing any `/api/v1/*` endpoint in `siigo-web/main.go`:
1. **Update `siigo-web/swagger.json`** to reflect the change (new path, parameters, schemas, responses)
2. Follow the existing patterns: use `$ref` for shared parameters/responses, add appropriate tags
3. Test that `/api/v1/docs` renders correctly after changes

### Current v1 endpoints
| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/auth` | Get JWT token (send api_key) |
| GET | `/api/v1/stats` | Stats summary |
| GET | `/api/v1/clients` | List clients (paginated) |
| GET | `/api/v1/clients/{key}` | Client detail by NIT |
| GET | `/api/v1/products` | List products |
| GET | `/api/v1/products/{key}` | Product detail |
| GET | `/api/v1/movements` | List movements |
| GET | `/api/v1/movements/{key}` | Movement detail |
| GET | `/api/v1/cartera` | List cartera |
| GET | `/api/v1/cartera/{key}` | Cartera detail |

## Telegram Bot (siigo-common/telegram/)

Integrated Telegram bot for notifications and remote control.

### Notifications (automatic)
- Server start/restart (includes local + Cloudflare tunnel URLs)
- Sync cycle results (adds, edits, errors)
- Login failures, max retries exhausted, DB cleared
- Changes detected per table

### Commands (interactive, via Telegram chat)
| Command | Description |
|---------|-------------|
| `/status` | Server status (detect/send loops, uptime, pending, errors) |
| `/stats` | Record counts per module |
| `/errors` | Error summary |
| `/sync` | Trigger manual sync (detect + send) |
| `/pause` | Pause both loops |
| `/resume` | Resume both loops |
| `/retry` | Retry error records |
| `/url` | Show local + Cloudflare tunnel URLs + Swagger |
| `/logs` | Last log entries |
| `/health` | Health check |
| `/exec {pin} {cmd}` | Execute shell command (30s timeout, PIN protected) |
| `/claude {pin}` | Start Claude remote |
| `/help` | List commands |

### Config
- `config.json` → `telegram.enabled`, `telegram.bot_token`, `telegram.chat_id`, `telegram.exec_pin`
- Configurable from web UI (Config page → Telegram Bot section)

## Dual Sync Loops (independent)

The sync system runs **two independent loops** to isolate ISAM reading from API sending:

| Loop | What it does | Default interval | Config key |
|------|-------------|-----------------|------------|
| **Detect** (ISAM → SQLite) | Reads ISAM files, detects changes, updates SQLite | 60s | `sync.interval_seconds` |
| **Send** (SQLite → API) | Sends pending records to Finearom API | 30s | `sync.send_interval_seconds` |

**Why**: If the API is down or slow, ISAM detection continues uninterrupted. Records stay in SQLite as `pending` until successfully sent.

## Key Documentation
- `docs/09-PARSING-PROCESS.md` — Complete parsing methodology with verified offsets
- `docs/07-SYNC-MAP-FINEAROM.md` — Siigo↔Finearom field mapping
