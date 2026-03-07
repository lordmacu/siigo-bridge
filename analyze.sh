#!/bin/bash
# Genera isam_tables.json con el catálogo de tablas ISAM de Siigo
# Uso: ./analyze.sh [DATA_DIR] [OUTPUT_FILE]

DATA_DIR="${1:-C:\\DEMOS01}"
OUTPUT="${2:-siigo-common/isam_tables.json}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/siigo-sync" || exit 1
go run ./cmd/analyze/ "$DATA_DIR" "$SCRIPT_DIR/$OUTPUT"
