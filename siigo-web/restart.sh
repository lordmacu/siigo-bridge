#!/bin/bash
# Kill all siigo-web processes and free port 3210, then start fresh

echo "Matando procesos siigo-web..."
taskkill //F //IM siigo-web.exe 2>/dev/null
taskkill //F //IM siigo-web-new.exe 2>/dev/null
taskkill //F //IM siigo-web-v2.exe 2>/dev/null
taskkill //F //IM siigo-web-v3.exe 2>/dev/null
taskkill //F //IM siigo-web-v4.exe 2>/dev/null
taskkill //F //IM siigo-web-v5.exe 2>/dev/null
taskkill //F //IM siigo-web-v6.exe 2>/dev/null
taskkill //F //IM siigo-web-v7.exe 2>/dev/null
taskkill //F //IM siigo-web-v8.exe 2>/dev/null
taskkill //F //IM siigo-web-v9.exe 2>/dev/null

# Kill anything on port 3210
PID=$(netstat -ano 2>/dev/null | grep ":3210.*LISTENING" | awk '{print $5}' | head -1)
if [ -n "$PID" ] && [ "$PID" != "0" ]; then
  echo "Matando PID $PID en puerto 3210..."
  taskkill //F //PID "$PID" 2>/dev/null
fi

sleep 1

echo "Iniciando siigo-web-v9..."
cd "$(dirname "$0")"
./siigo-web-v9.exe
