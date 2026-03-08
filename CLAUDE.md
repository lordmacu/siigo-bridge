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
- **Z49**: tipo@0(letter), codigo@1, numDoc@4, nombreTercero@15, desc zones@72-128 and @129-192
- **Z09**: nit@16, cuenta@29, fecha@42, desc@93, D/C@143
- **Z06**: tipo@0, codigo@2, nombre@31
- **Z03**: empresa@0, cuenta@3(9), activa@12, nombre@25(70)
- **Z27**: empresa@0(5), codigo@5(6), nombre@11(50), nit@61(13), fecha@122(8)
- **Z11**: tipo@0, codigo@1(3), nit@21(13), cuenta@29(13), fecha@55(8), desc@93(50), D/C@143
- **Z08A**: nit@5(8), tipoPersona@16(2), nombre@18(60), dir@194(56), email@323(70)
- **Z25**: empresa@0(3), cuenta@3(9), nit@12(13), BCD@140 (NOT @25 — 115 bytes ASCII keys before BCD)
- **Z28**: empresa@0(3), cuenta@3(9), BCD@38 (NOT @12 — 26 bytes ASCII keys before BCD)
- **ZDANE**: codigo@0(5), nombre@5(40)

### 4. Data directory
Siigo data lives in `C:\DEMOS01\` (configured in `C:\Siigo\FILEPATH.TXT`).
Files use Windows-1252 encoding — decode with `golang.org/x/text/encoding/charmap`.

### 5. Packed decimal (BCD) decoder
BCD decoder in `siigo-common/parsers/bcd.go` (`DecodePacked`, `ExtractPacked`).
Integrated into Z25 and Z28 parsers for saldo/debito/credito fields.
Sign nibble: C=positive, D=negative, F=unsigned positive.
**WARNING**: BCD fields often start far after ASCII data ends (Z25: @140, Z28: @38). Always verify offset visually in hex dump — look for where 0x30-0x39 (ASCII) stops and 0x00-0x0F (binary) begins.

### 6. Finearom Laravel backend
Located at `C:\laragon\www\finearom\backend` (Laragon, port 8000).
PHP: `C:\laragon\bin\php\php-8.3.21-Win32-vs16-x64\php.exe`
- `SiigoSyncController` handles all sync endpoints
- `POST /api/siigo/login` → Sanctum token
- `POST /api/siigo/sync` → generic `{table, action, key, data}` (primary endpoint)
- Also: `/api/siigo/bulk`, `/api/siigo/webhook`, `/api/siigo/status`, per-table GETs
- Sync user: `siigo-sync@finearom.com` / `siigo123`
- Production URL: `https://ordenes.finearom.co/api`
- Connection verified end-to-end on 2026-03-08 (all 4 tables OK)

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
2. Identify ASCII vs BCD boundaries (BCD can start far from end of text — e.g. Z25 has 140 bytes of ASCII keys before BCD)
3. Write diagnostic script testing offsets on 15+ **distributed** records (not just the first ones)
4. Create parser in `siigo-common/parsers/` following existing patterns (dual-mode EXTFH/Binary)
5. **MANDATORY: Validate with real data** — add validation to `siigo-sync/cmd/validate_all/main.go`
6. Run validation: `cd siigo-sync && go run ./cmd/validate_all/`
7. **Acceptance criteria**: 0 empty key fields, valid dates (1990-2030), reasonable BCD values (<10^9), coherent distributed samples
8. Add to peek tool: `siigo-sync/cmd/peek/main.go`
9. Build: `cd siigo-web && go build -o /tmp/test.exe ./...` (exe may be locked, use temp path)
10. If syncing: add to detector, config, and Laravel endpoint

## How to Validate Parsers
```bash
cd siigo-sync && go run ./cmd/validate_all/
```
This runs all parsers against real ISAM data and checks:
- Key fields not empty (empresa, nombre, codigo, NIT)
- Dates in valid range (1990-2030)
- BCD values reasonable (not ASCII garbage like 3030303...)
- 5 distributed samples per parser showing coherent data
- Type codes are recognizable letters (F/G/L/P, D/C)

Also available: `cd siigo-sync && go run ./cmd/peek/` for quick previews.

### Common parsing pitfalls
- **BCD at wrong offset**: COBOL files have repeated ASCII key data (38-140 bytes) before BCD starts. Always check hex dump visually.
- **Z49 has no dates/amounts**: Z49 is a document INDEX (headers only). Use Z09 for accounting detail lines.
- **Truncated names**: Usually offset is +2 too high. Try subtracting 2.
- **Values like 3030303.xx**: BCD decoder is reading ASCII '0' bytes (0x30). Offset is in ASCII zone, not BCD.

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
| POST | `/api/v1/auth` | Get JWT token (api_key OR username+password) |
| GET | `/api/v1/stats` | Stats summary |
| GET | `/api/v1/clients` | List clients (paginated) |
| GET | `/api/v1/clients/{key}` | Client detail by NIT |
| GET | `/api/v1/products` | List products |
| GET | `/api/v1/products/{key}` | Product detail |
| GET | `/api/v1/movements` | List movements |
| GET | `/api/v1/movements/{key}` | Movement detail |
| GET | `/api/v1/cartera` | List cartera |
| GET | `/api/v1/cartera/{key}` | Cartera detail |

### OData endpoints (for Power BI / BI tools)
| Method | Path | Description |
|--------|------|-------------|
| GET | `/odata` | Service document |
| GET | `/odata/$metadata` | CSDL XML schema (entity types, properties) |
| GET | `/odata/{table}` | Query with `$top`, `$skip`, `$filter`, `$orderby`, `$select`, `$count` |
| GET | `/odata/{table}('key')` | Single entity by key |
| GET | `/odata/{table}/$count` | Count only |

Tables: `clients`, `products`, `movements`, `cartera`. Protected by same JWT as v1.

**$filter operators**: `eq`, `ne`, `gt`, `ge`, `lt`, `le`, `contains()`, `startswith()`
**Example**: `/odata/clients?$top=100&$filter=sync_status eq 'synced'&$orderby=nombre&$count=true`

**Power BI connection**: Get Data → OData Feed → URL: `http://host:3210/odata` → Header `Authorization: Bearer {token}`

### v1 Auth (dual method)
`POST /api/v1/auth` accepts two authentication methods:
- **API Key**: `{"api_key": "your-key"}` — uses configured api_key
- **Credentials**: `{"username": "user", "password": "pass"}` — checks root user (config.json) and app_users table

Both return a JWT valid for 24h. Response includes `method` ("api_key" or "credentials") and `user`.

## User Management (app_users)

Multi-user system with roles and per-module permissions.

### Architecture
- **Root user**: defined in `config.json` (`auth.username` / `auth.password`), always has full access, cannot be deleted
- **App users**: stored in SQLite `app_users` table, managed from web UI (/users page)
- Login checks root user first, then app_users table
- Token stores username, role, and permissions; checked on every request

### Roles
| Role | Access | Can manage users |
|------|--------|-----------------|
| `root` | All modules | Yes (implicit) |
| `admin` | All modules | Yes |
| `editor` | Assigned modules only | No |
| `viewer` | Assigned modules only (read-only) | No |

### Modules (permission keys)
`dashboard`, `clients`, `products`, `movements`, `cartera`, `field-mappings`, `errors`, `logs`, `explorer`, `config`, `users`

### API endpoints
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/users` | List all users (admin/root only) |
| POST | `/api/users` | Create user (admin/root only) |
| PUT | `/api/users/{id}` | Update user role/perms/active/password |
| DELETE | `/api/users/{id}` | Delete user |

### Frontend
- Users page with table, create/edit modals (toggle switches for permissions)
- Sidebar filters nav items based on user permissions
- Routes guarded: unauthorized modules redirect to Dashboard
- Username and role shown in sidebar footer

## Record Edit/Delete

Optional feature to edit or delete individual records from data pages.

### How it works
- **Disabled by default** — must be enabled in Config → Advanced → "Edicion de Registros"
- Config flag: `allow_edit_delete` in config.json
- When enabled, Edit/Delete buttons appear on each row in data tables
- Edited records are marked as `sync_status=pending`, `sync_action=edit` for re-sync
- Deleted records are permanently removed
- Protected fields (hash, sync_status, etc.) cannot be edited

### API endpoints
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/allow-edit-delete` | Get current flag |
| POST | `/api/allow-edit-delete` | Set flag (true/false) |
| GET | `/api/record?table=X&id=N` | Get single record |
| PUT | `/api/record?table=X&id=N` | Update fields |
| DELETE | `/api/record?table=X&id=N` | Delete record |

## Telegram Bot (siigo-common/telegram/)

Integrated Telegram bot for notifications and remote control.

### Notifications (automatic, individually toggleable)
- Server start/restart (includes local + Cloudflare tunnel URLs) — **enabled by default**
- Sync cycle results (adds, edits, errors) — disabled by default
- Sync errors per table — disabled by default
- Login failures — disabled by default
- Changes detected per table — disabled by default
- DB cleared — disabled by default
- Max retries exhausted — disabled by default

Each notification type can be enabled/disabled independently from Config → Telegram → "Tipos de Notificacion" (toggle switches). Config fields: `telegram.notify_server_start`, `telegram.notify_sync_complete`, etc. (`*bool` pointers, nil = default).

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
- Notification toggles: `telegram.notify_server_start`, `notify_sync_complete`, `notify_sync_errors`, `notify_login_failed`, `notify_changes`, `notify_db_cleared`, `notify_max_retries`
- Configurable from web UI (Config page → Telegram tab)

## Dual Sync Loops (independent)

The sync system runs **two independent loops** to isolate ISAM reading from API sending:

| Loop | What it does | Default interval | Config key |
|------|-------------|-----------------|------------|
| **Detect** (ISAM → SQLite) | Reads ISAM files, detects changes, updates SQLite | 60s | `sync.interval_seconds` |
| **Send** (SQLite → API) | Sends pending records to Finearom API | 30s | `sync.send_interval_seconds` |

**Why**: If the API is down or slow, ISAM detection continues uninterrupted. Records stay in SQLite as `pending` until successfully sent.

## Send Behavior
- **Sending is disabled by default** for all modules (`send_enabled` all false)
- User must explicitly enable sending per table from Data pages toggle
- Send loop skips entirely when no module has sending enabled (avoids unnecessary API login)
- Circuit breaker: auto-pauses sending after consecutive failures, resume via `/send-resume` bot command or UI

## SQL Explorer
- Interactive SQL query tool at `/explorer` page
- SQL syntax highlighting in textarea (overlay technique: `<pre>` behind transparent textarea)
- Token types: keywords (purple), functions (blue), strings (yellow), numbers (green), tables (cyan), columns (orange)
- Autocomplete for table/column names
- Paginated results with export

## Data Table Styling
- Syntax-like coloring on table cells by field type:
  - Keys (amber), Names (cyan), Dates (violet), Codes (emerald), Types (orange), Descriptions (gray italic), Values (green)
- CSS classes: `col-key`, `col-name`, `col-date`, `col-code`, `col-type`, `col-desc`, `col-value`

## Key Documentation
- `docs/09-PARSING-PROCESS.md` — Complete parsing methodology with verified offsets
- `docs/07-SYNC-MAP-FINEAROM.md` — Siigo↔Finearom field mapping
