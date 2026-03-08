#!/bin/bash
# =============================================================
# Siigo Web - start | stop | restart | dev | status | logs
#
# Uso:
#   ./start.sh start          Build + servidor + tunnel
#   ./start.sh dev             Go server + Vite dev (hot reload)
#   ./start.sh restart         Recompila Go, reinicia server (tunnel intacto)
#   ./start.sh restart force   Reinicia TODO incluyendo tunnel (nueva URL)
#   ./start.sh stop            Detiene todo
#   ./start.sh status          Muestra estado
#   ./start.sh logs            Ultimos logs del servidor
# =============================================================

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
WEB_DIR="$SCRIPT_DIR/siigo-web"
FRONTEND_DIR="$WEB_DIR/frontend"
CONFIG_FILE="$WEB_DIR/config.json"
PID_FILE="/tmp/siigo-web.pid"
TUNNEL_PID_FILE="/tmp/siigo-tunnel.pid"
VITE_PID_FILE="/tmp/siigo-vite.pid"
AIR_PID_FILE="/tmp/siigo-air.pid"
PORT=3210
VITE_PORT=5173
SIIGO_EXE="siigo-web.exe"
AIR_BIN="$(go env GOPATH)/bin/air"

# --- Telegram Bot (set via env vars or .env file) ---
if [ -f ".env" ]; then
    source .env
fi
TG_BOT_TOKEN="${TG_BOT_TOKEN:-}"
TG_CHAT_ID="${TG_CHAT_ID:-}"

# --- Colores ---
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

log()  { echo -e "${GREEN}[✓]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }
err()  { echo -e "${RED}[✗]${NC} $1"; }
info() { echo -e "${CYAN}[i]${NC} $1"; }

# Send Telegram notification
tg_send() {
    local msg="$1"
    if [ -n "$TG_BOT_TOKEN" ] && [ -n "$TG_CHAT_ID" ]; then
        curl -s -X POST "https://api.telegram.org/bot${TG_BOT_TOKEN}/sendMessage" \
            -H "Content-Type: application/json" \
            -d "{\"chat_id\":${TG_CHAT_ID},\"text\":\"${msg}\",\"parse_mode\":\"HTML\"}" > /dev/null 2>&1 &
    fi
}

# -------------------------------------------------------
# Helpers
# -------------------------------------------------------
kill_by_name() {
    local name="$1"
    if command -v taskkill &>/dev/null; then
        taskkill //F //IM "$name" &>/dev/null
    else
        pkill -f "$name" 2>/dev/null
    fi
}

kill_by_pid() {
    local pidfile="$1"
    local label="$2"
    if [ -f "$pidfile" ]; then
        local pid
        pid=$(cat "$pidfile")
        if kill -0 "$pid" 2>/dev/null; then
            kill "$pid" 2>/dev/null
            log "$label detenido (PID $pid)"
            rm -f "$pidfile"
            return 0
        fi
        rm -f "$pidfile"
    fi
    return 1
}

kill_port() {
    local port="$1"
    if command -v netstat &>/dev/null; then
        local pids
        pids=$(netstat -ano 2>/dev/null | grep ":${port} " | grep LISTENING | awk '{print $5}' | sort -u)
        for pid in $pids; do
            if [ -n "$pid" ] && [ "$pid" != "0" ]; then
                taskkill //F //PID "$pid" &>/dev/null && log "Proceso en puerto $port matado (PID $pid)"
            fi
        done
    fi
}

kill_all_processes() {
    # Matar por PID files
    kill_by_pid "$AIR_PID_FILE" "Air" || true
    kill_by_pid "$PID_FILE" "Servidor" || true
    kill_by_pid "$TUNNEL_PID_FILE" "Tunnel" || true
    kill_by_pid "$VITE_PID_FILE" "Vite dev" || true

    # Matar por nombre de proceso
    kill_by_name "siigo-web.exe" &>/dev/null || true
    kill_by_name "siigo-web-new.exe" &>/dev/null || true
    kill_by_name "cloudflared.exe" &>/dev/null || true
    kill_by_name "air.exe" &>/dev/null || true

    # Matar por puerto (lo que sea que este escuchando)
    kill_port $PORT
    kill_port $VITE_PORT

    sleep 1
}

is_running() {
    local pidfile="$1"
    [ -f "$pidfile" ] && kill -0 "$(cat "$pidfile")" 2>/dev/null
}

tunnel_url() {
    grep -o 'https://[a-z0-9-]*\.trycloudflare\.com' /tmp/cloudflared.log 2>/dev/null | head -1
}

ensure_config() {
    if [ ! -f "$CONFIG_FILE" ]; then
        warn "Config no encontrada, creando $CONFIG_FILE..."
        cat > "$CONFIG_FILE" << 'JSONEOF'
{
  "siigo": {
    "data_path": "C:\\DEMOS01\\"
  },
  "finearom": {
    "base_url": "https://ordenes.finearom.co/api",
    "email": "siigo-sync@finearom.com",
    "password": ""
  },
  "sync": {
    "interval_seconds": 60,
    "files": ["Z17", "Z04", "Z49", "Z092024"],
    "state_path": "sync_state.json"
  }
}
JSONEOF
        log "Config creada con valores por defecto."
    fi
}

# -------------------------------------------------------
# build_backend: solo compila Go
# -------------------------------------------------------
build_backend() {
    log "Compilando backend (Go)..."
    cd "$WEB_DIR"
    # Build to temp name to avoid "file in use" on Windows
    rm -f siigo-web-new.exe 2>/dev/null
    go build -o siigo-web-new.exe . 2>&1
    if [ -f siigo-web-new.exe ]; then
        mv -f siigo-web-new.exe siigo-web.exe 2>/dev/null || {
            # If mv fails (exe locked), use the new name directly
            warn "No se pudo reemplazar siigo-web.exe (en uso), usando siigo-web-new.exe"
            SIIGO_EXE="siigo-web-new.exe"
        }
    fi
    log "Backend listo"
}

# -------------------------------------------------------
# build_all: compila frontend + backend
# -------------------------------------------------------
build_all() {
    echo ""
    echo -e "${BOLD}=========================================${NC}"
    echo -e "${BOLD}  Siigo Web - Build${NC}"
    echo -e "${BOLD}=========================================${NC}"
    echo ""

    log "Instalando dependencias del frontend..."
    cd "$FRONTEND_DIR"
    npm install --silent 2>/dev/null
    log "Compilando frontend (React)..."
    npm run build 2>&1 | tail -3
    log "Frontend listo"

    build_backend
}

# -------------------------------------------------------
# start_server: inicia el proceso Go
# -------------------------------------------------------
start_server() {
    # Matar servidor previo (por PID, nombre y puerto)
    kill_by_pid "$PID_FILE" "Servidor" || true
    kill_by_name "siigo-web.exe" &>/dev/null || true
    kill_by_name "siigo-web-new.exe" &>/dev/null || true
    kill_port $PORT
    sleep 1

    # Abrir puerto en firewall de Windows (idempotente, no falla si ya existe)
    if command -v netsh &>/dev/null; then
        netsh advfirewall firewall show rule name="Siigo Middleware" &>/dev/null 2>&1 || \
        netsh advfirewall firewall add rule name="Siigo Middleware" dir=in action=allow protocol=TCP localport=$PORT &>/dev/null 2>&1 && \
        log "Firewall: puerto $PORT abierto" || warn "No se pudo abrir el firewall (requiere admin)"
    fi

    log "Iniciando servidor en puerto $PORT..."
    cd "$WEB_DIR"
    ./$SIIGO_EXE > /tmp/siigo-web.log 2>&1 &
    echo $! > "$PID_FILE"
    sleep 2

    if curl -s "http://localhost:$PORT/api/stats" > /dev/null 2>&1; then
        log "Servidor corriendo en http://localhost:$PORT"
        return 0
    else
        err "El servidor no respondio. Log:"
        cat /tmp/siigo-web.log
        rm -f "$PID_FILE"
        return 1
    fi
}

# -------------------------------------------------------
# start_tunnel: inicia Cloudflare tunnel
# -------------------------------------------------------
start_tunnel() {
    # Matar tunnel previo
    kill_by_pid "$TUNNEL_PID_FILE" "Tunnel" || kill_by_name "cloudflared.exe" || true
    sleep 1

    log "Iniciando Cloudflare Tunnel..."
    cloudflared tunnel --url "http://localhost:$PORT" > /tmp/cloudflared.log 2>&1 &
    echo $! > "$TUNNEL_PID_FILE"
    sleep 5

    local url
    url=$(tunnel_url)
    if [ -n "$url" ]; then
        log "Tunnel activo: $url"
    else
        warn "Tunnel iniciado pero URL no detectada aun."
        info "Revisa: cat /tmp/cloudflared.log"
    fi
}

# -------------------------------------------------------
# show_banner: muestra resumen final
# -------------------------------------------------------
show_banner() {
    local mode="$1"
    local url
    url=$(tunnel_url)

    echo ""
    echo -e "${BOLD}=========================================${NC}"
    echo -e "  ${GREEN}${BOLD}Siigo Web - $mode${NC}"
    echo -e "${BOLD}=========================================${NC}"
    echo ""
    echo -e "  Local:   ${GREEN}http://localhost:$PORT${NC}"

    if [ "$mode" = "DEV" ]; then
        echo -e "  Vite:    ${GREEN}http://localhost:$VITE_PORT${NC}  (hot reload)"
    fi

    if [ -n "$url" ]; then
        echo -e "  Publica: ${GREEN}$url${NC}"
    fi

    # Telegram notification
    if [ -n "$url" ]; then
        tg_send "🟢 <b>Siigo Web - ${mode}</b>%0A%0A🌐 Web: ${url}%0A📄 Swagger: ${url}/api/v1/docs%0A🖥 Local: http://localhost:${PORT}"
    else
        tg_send "🟢 <b>Siigo Web - ${mode}</b>%0A%0A🖥 Local: http://localhost:${PORT}"
    fi

    echo ""
    echo "  Comandos:"
    echo "    ./start.sh stop            Detener todo"
    if [ "$mode" = "DEV" ]; then
        echo "    ./start.sh restart         Recompilar Go (Vite se actualiza solo)"
    else
        echo "    ./start.sh restart         Recompilar + reiniciar (misma URL)"
        echo "    ./start.sh restart force   Reiniciar todo (nueva URL)"
    fi
    echo "    ./start.sh status          Ver estado"
    echo "    ./start.sh logs            Ver logs del servidor"
    echo -e "${BOLD}=========================================${NC}"
}

# =======================================================
# COMMANDS
# =======================================================

# -------------------------------------------------------
# do_start: build completo + servidor + tunnel
# -------------------------------------------------------
do_start() {
    ensure_config

    # Matar todo lo previo antes de arrancar
    if is_running "$PID_FILE" || is_running "$AIR_PID_FILE"; then
        warn "Procesos previos detectados, matando todo..."
    fi
    kill_all_processes

    build_all
    start_server || return 1
    start_tunnel
    show_banner "PRODUCCION"
}

# -------------------------------------------------------
# do_dev: Air (Go live reload) + Vite dev (hot reload)
# -------------------------------------------------------
do_dev() {
    ensure_config

    # Si ya hay algo corriendo, lo matamos todo
    if is_running "$AIR_PID_FILE" || is_running "$PID_FILE"; then
        warn "Servidor previo detectado, matando todo..."
    fi

    echo ""
    echo -e "${BOLD}=========================================${NC}"
    echo -e "${BOLD}  Siigo Web - Dev Mode (auto-reload)${NC}"
    echo -e "${BOLD}=========================================${NC}"
    echo ""

    # Instalar deps si faltan
    cd "$FRONTEND_DIR"
    [ ! -d "node_modules" ] && npm install --silent 2>/dev/null && log "Dependencias instaladas"

    # Matar todos los procesos previos y liberar puertos
    kill_all_processes

    # Iniciar Air (Go live reload) — recompila y reinicia al detectar cambios en .go
    log "Iniciando Air (Go live reload)..."
    cd "$WEB_DIR"
    "$AIR_BIN" > /tmp/siigo-air.log 2>&1 &
    echo $! > "$AIR_PID_FILE"
    sleep 4

    if curl -s "http://localhost:$PORT/api/stats" > /dev/null 2>&1; then
        log "Go server corriendo en http://localhost:$PORT (auto-reload con Air)"
    else
        warn "Go server aun arrancando... revisa: cat /tmp/siigo-air.log"
    fi

    # Iniciar Vite dev server
    log "Iniciando Vite dev server (hot reload)..."
    cd "$FRONTEND_DIR"
    npx vite --port $VITE_PORT > /tmp/siigo-vite.log 2>&1 &
    echo $! > "$VITE_PID_FILE"
    sleep 3

    if curl -s "http://localhost:$VITE_PORT" > /dev/null 2>&1; then
        log "Vite corriendo en http://localhost:$VITE_PORT"
    else
        warn "Vite no respondio aun, puede tardar unos segundos..."
    fi

    # Iniciar Cloudflare Tunnel
    start_tunnel

    show_banner "DEV"
}

# -------------------------------------------------------
# do_stop: detiene todo
# -------------------------------------------------------
do_stop() {
    log "Deteniendo todos los procesos y liberando puertos..."
    kill_all_processes
    log "Todo detenido."
}

# -------------------------------------------------------
# do_restart: reinicia servidor, opcionalmente tunnel
# -------------------------------------------------------
do_restart() {
    local force="$1"

    echo ""

    if [ "$force" = "force" ]; then
        info "Reinicio completo (servidor + tunnel)..."
        do_stop
        sleep 2
        do_start
    else
        info "Reiniciando servidor..."

        # Matar todos los procesos y liberar puertos
        kill_all_processes

        # Recompilar frontend + backend
        log "Compilando frontend (React)..."
        cd "$FRONTEND_DIR"
        npm run build 2>&1 | tail -3
        log "Frontend listo"

        build_backend

        # Solo reiniciar el servidor Go
        start_server || return 1

        # Reiniciar tunnel (fue matado por kill_all_processes)
        start_tunnel

        show_banner "PRODUCCION"
    fi
}

# -------------------------------------------------------
# do_status: muestra estado actual
# -------------------------------------------------------
do_status() {
    echo ""
    echo -e "${BOLD}=========================================${NC}"
    echo -e "${BOLD}  Siigo Web - Estado${NC}"
    echo -e "${BOLD}=========================================${NC}"
    echo ""

    if is_running "$PID_FILE"; then
        log "Servidor: corriendo (PID $(cat "$PID_FILE"))"
        echo -e "     Local: ${GREEN}http://localhost:$PORT${NC}"
    else
        err "Servidor: detenido"
    fi

    if is_running "$TUNNEL_PID_FILE"; then
        local url
        url=$(tunnel_url)
        log "Tunnel: corriendo (PID $(cat "$TUNNEL_PID_FILE"))"
        [ -n "$url" ] && echo -e "     Publica: ${GREEN}$url${NC}"
    else
        err "Tunnel: detenido"
    fi

    if is_running "$AIR_PID_FILE"; then
        log "Air (Go reload): corriendo (PID $(cat "$AIR_PID_FILE"))"
    fi

    if is_running "$VITE_PID_FILE"; then
        log "Vite dev: corriendo (PID $(cat "$VITE_PID_FILE"))"
        echo -e "     Vite: ${GREEN}http://localhost:$VITE_PORT${NC}"
    fi

    if curl -s "http://localhost:$PORT/api/stats" > /dev/null 2>&1; then
        echo ""
        info "Stats:"
        curl -s "http://localhost:$PORT/api/stats" | python3 -m json.tool 2>/dev/null || curl -s "http://localhost:$PORT/api/stats"
    fi
    echo ""
}

# -------------------------------------------------------
# do_logs: muestra logs del servidor
# -------------------------------------------------------
do_logs() {
    if [ -f /tmp/siigo-web.log ]; then
        tail -50 /tmp/siigo-web.log
    else
        warn "No hay logs disponibles."
    fi
}

# =======================================================
# Main
# =======================================================
CMD="${1:-start}"
ARG2="${2:-}"

case "$CMD" in
    start)
        do_start
        ;;
    dev)
        do_dev
        ;;
    stop)
        do_stop
        ;;
    restart)
        do_restart "$ARG2"
        ;;
    status)
        do_status
        ;;
    logs)
        do_logs
        ;;
    *)
        echo ""
        echo "Uso: $0 {start|dev|stop|restart|status|logs}"
        echo ""
        echo "  start            Build completo + servidor + tunnel"
        echo "  dev              Go server + Vite dev (hot reload, sin tunnel)"
        echo "  stop             Detener todo"
        echo "  restart          Recompilar Go, reiniciar server (misma URL)"
        echo "  restart force    Reiniciar TODO incluyendo tunnel (nueva URL)"
        echo "  status           Ver estado actual"
        echo "  logs             Ver ultimos logs del servidor"
        echo ""
        exit 1
        ;;
esac
