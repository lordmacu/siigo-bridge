# Siigo Middleware - Project Instructions

## What This Is
Middleware between **Siigo Pyme** (Colombian COBOL accounting software, ISAM files) and **Finearom** (Laravel B2B platform for aromas/fragrances). We read Siigo's ISAM files, detect changes, and sync to Finearom's REST API.

## Project Structure
```
siigo/
├── siigo-common/     # SHARED Go module (isam/ + parsers/) - SINGLE SOURCE OF TRUTH
├── siigo-sync/       # CLI sync service (polling + API client)
├── siigo-app/        # Desktop app (Wails + Go)
└── docs/             # Documentation (9 MD files)
```

## Critical Rules

### 1. Never duplicate isam/ or parsers/
Both `siigo-sync` and `siigo-app` import from `siigo-common` via:
```
require siigo-common v0.0.0
replace siigo-common => ../siigo-common
```
**NEVER** create isam/ or parsers/ packages inside siigo-sync or siigo-app.

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

## How to Add a New Parser
1. Hex dump: `cd siigo-sync && go run ./cmd/hexdump/ 'C:\DEMOS01\ZXXX'`
2. Write diagnostic script to test offsets on 15+ records
3. Create parser in `siigo-common/parsers/` following existing patterns
4. Add to peek tool: `siigo-sync/cmd/peek/main.go`
5. Build both: `cd siigo-sync && go build ./...` then `cd siigo-app && go build ./...`
6. If syncing: add to detector.go, api/client.go, config, and Laravel endpoint

## How to Validate Parsers
```bash
cd siigo-sync && go run ./cmd/peek/
```
Check: no truncated names, valid dates (2010-2030), recognizable type codes, non-empty fields.

## Key Documentation
- `docs/09-PARSING-PROCESS.md` — Complete parsing methodology with verified offsets
- `docs/07-SYNC-MAP-FINEAROM.md` — Siigo↔Finearom field mapping
