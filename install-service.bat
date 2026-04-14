@echo off
setlocal enableextensions

:: ============================================================
::   Siigo Bridge - Windows Service Installer (NSSM)
::   Service runs siigo-web.exe which ALREADY launches:
::     - Cloudflare quick tunnel + watchdog
::     - Sync loops (detect + send)
::     - Telegram bot
::   No duplicate instances: kills zombies + removes old service.
:: ============================================================

:: --- self-elevate to admin ---
net session >nul 2>&1
if errorlevel 1 (
    echo Requesting administrator privileges...
    powershell -Command "Start-Process -FilePath '%~f0' -Verb RunAs"
    exit /b
)

:: --- paths (script lives next to siigo-web.exe) ---
set "INSTALL_DIR=%~dp0"
set "INSTALL_DIR=%INSTALL_DIR:~0,-1%"
set "EXE_PATH=%INSTALL_DIR%\siigo-web.exe"
set "LOG_DIR=%INSTALL_DIR%\logs"
set "NSSM_DIR=%INSTALL_DIR%\nssm"
set "NSSM_EXE=%NSSM_DIR%\nssm.exe"
set "SERVICE_NAME=SiigoBridge"
set "NSSM_URL=https://nssm.cc/release/nssm-2.24.zip"

echo ============================================
echo   Siigo Bridge - Service Installer
echo ============================================
echo   Install dir: %INSTALL_DIR%
echo   Exe        : %EXE_PATH%
echo   Service    : %SERVICE_NAME%
echo.

:: --- verify exe exists ---
if not exist "%EXE_PATH%" (
    echo ERROR: siigo-web.exe not found at:
    echo   %EXE_PATH%
    echo Build it first: cd siigo-web ^& build.bat
    pause
    exit /b 1
)

:: --- ensure log dir ---
if not exist "%LOG_DIR%" mkdir "%LOG_DIR%"

:: --- download NSSM if missing ---
if not exist "%NSSM_EXE%" (
    echo [1/6] Downloading NSSM...
    if not exist "%NSSM_DIR%" mkdir "%NSSM_DIR%"
    powershell -NoProfile -Command "$ProgressPreference='SilentlyContinue'; Invoke-WebRequest -Uri '%NSSM_URL%' -OutFile '%NSSM_DIR%\nssm.zip'"
    if errorlevel 1 (
        echo ERROR: Failed to download NSSM from %NSSM_URL%
        pause
        exit /b 1
    )
    powershell -NoProfile -Command "Expand-Archive -Path '%NSSM_DIR%\nssm.zip' -DestinationPath '%NSSM_DIR%\extracted' -Force"
    copy /y "%NSSM_DIR%\extracted\nssm-2.24\win64\nssm.exe" "%NSSM_EXE%" >nul
    rmdir /s /q "%NSSM_DIR%\extracted"
    del "%NSSM_DIR%\nssm.zip"
    echo      NSSM OK
) else (
    echo [1/6] NSSM already present
)
echo.

:: --- stop + remove existing service if present ---
echo [2/6] Removing old service (if exists)...
sc query %SERVICE_NAME% >nul 2>&1
if not errorlevel 1 (
    "%NSSM_EXE%" stop %SERVICE_NAME% >nul 2>&1
    "%NSSM_EXE%" remove %SERVICE_NAME% confirm >nul 2>&1
    echo      Old service removed
) else (
    echo      No previous service
)
echo.

:: --- kill zombie siigo-web.exe + cloudflared.exe ---
echo [3/6] Killing zombie processes...
taskkill /F /IM siigo-web.exe >nul 2>&1
taskkill /F /IM cloudflared.exe >nul 2>&1
timeout /t 2 /nobreak >nul
echo      Zombies cleared
echo.

:: --- install service ---
echo [4/6] Installing service...
"%NSSM_EXE%" install %SERVICE_NAME% "%EXE_PATH%"
if errorlevel 1 (
    echo ERROR: nssm install failed
    pause
    exit /b 1
)

:: --- configure service ---
echo [5/6] Configuring service...
"%NSSM_EXE%" set %SERVICE_NAME% AppDirectory "%INSTALL_DIR%"
"%NSSM_EXE%" set %SERVICE_NAME% DisplayName "Siigo Bridge (Finearom)"
"%NSSM_EXE%" set %SERVICE_NAME% Description "Siigo-Finearom middleware. Auto-restarts on crash. Launches Cloudflare tunnel + sync loops."
"%NSSM_EXE%" set %SERVICE_NAME% Start SERVICE_AUTO_START
"%NSSM_EXE%" set %SERVICE_NAME% ObjectName LocalSystem

:: restart policy: on any exit, restart after 5s; throttle to avoid loops
"%NSSM_EXE%" set %SERVICE_NAME% AppExit Default Restart
"%NSSM_EXE%" set %SERVICE_NAME% AppRestartDelay 5000
"%NSSM_EXE%" set %SERVICE_NAME% AppThrottle 10000

:: graceful shutdown: send Ctrl+C (shutdown.go handles SIGINT), wait 15s, then kill
"%NSSM_EXE%" set %SERVICE_NAME% AppStopMethodSkip 0
"%NSSM_EXE%" set %SERVICE_NAME% AppStopMethodConsole 15000
"%NSSM_EXE%" set %SERVICE_NAME% AppStopMethodWindow 5000
"%NSSM_EXE%" set %SERVICE_NAME% AppStopMethodThreads 5000
"%NSSM_EXE%" set %SERVICE_NAME% AppKillProcessTree 1

:: logs (rotate at 10MB, keep on restart)
"%NSSM_EXE%" set %SERVICE_NAME% AppStdout "%LOG_DIR%\service-stdout.log"
"%NSSM_EXE%" set %SERVICE_NAME% AppStderr "%LOG_DIR%\service-stderr.log"
"%NSSM_EXE%" set %SERVICE_NAME% AppRotateFiles 1
"%NSSM_EXE%" set %SERVICE_NAME% AppRotateOnline 1
"%NSSM_EXE%" set %SERVICE_NAME% AppRotateBytes 10485760
"%NSSM_EXE%" set %SERVICE_NAME% AppStdoutCreationDisposition 4
"%NSSM_EXE%" set %SERVICE_NAME% AppStderrCreationDisposition 4

:: Windows failure actions as belt-and-suspenders (if NSSM itself crashes)
sc failure %SERVICE_NAME% reset= 86400 actions= restart/5000/restart/5000/restart/10000 >nul
echo      Config OK
echo.

:: --- start ---
echo [6/6] Starting service...
"%NSSM_EXE%" start %SERVICE_NAME%
if errorlevel 1 (
    echo ERROR: Service did not start. Check logs:
    echo   %LOG_DIR%\service-stderr.log
    pause
    exit /b 1
)
echo.

echo ============================================
echo   Installed and running.
echo ============================================
echo   Service name : %SERVICE_NAME%
echo   Local URL    : http://localhost:3210
echo   Logs         : %LOG_DIR%\service-*.log
echo   Tunnel log   : %INSTALL_DIR%\cloudflared\quick-tunnel.log
echo.
echo   Control:
echo     sc query %SERVICE_NAME%
echo     nssm\nssm.exe restart %SERVICE_NAME%
echo     nssm\nssm.exe stop %SERVICE_NAME%
echo     nssm\nssm.exe edit %SERVICE_NAME%
echo     uninstall-service.bat
echo.
echo   The service auto-starts at boot and restarts on crash.
echo.
pause
endlocal
