@echo off
echo ============================================
echo   Siigo Web - Production Build
echo ============================================
echo.

:: Build frontend
echo [1/3] Building React frontend...
cd frontend
call npm run build
if errorlevel 1 (
    echo ERROR: Frontend build failed!
    pause
    exit /b 1
)
cd ..
echo      Frontend built OK
echo.

:: Build Go binary (hidden console for Windows GUI mode)
echo [2/3] Building Go binary (GUI mode, no console)...
go build -ldflags "-s -w -H windowsgui" -o siigo-web.exe .
if errorlevel 1 (
    echo ERROR: Go build failed!
    pause
    exit /b 1
)
echo      Binary built OK
echo.

:: Show result
echo [3/3] Build complete!
echo.
for %%A in (siigo-web.exe) do echo      Size: %%~zA bytes (%%~zA)
echo      File: %cd%\siigo-web.exe
echo.
echo   The .exe includes:
echo     - Embedded React frontend
echo     - System tray icon (runs in background)
echo     - Auto-opens browser on start
echo     - Kills previous instance automatically
echo.
echo   To create installer, run Inno Setup on installer.iss
echo.
pause
