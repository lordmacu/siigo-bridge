@echo off
setlocal enableextensions

:: ============================================================
::   Siigo Bridge - Windows Service Installer (NSSM)
::   Fully non-interactive. No prompts, no pauses.
::   Output mirrored to logs\install-service.log.
:: ============================================================

:: --- paths (before elevation so log goes to the right place) ---
set "INSTALL_DIR=%~dp0"
set "INSTALL_DIR=%INSTALL_DIR:~0,-1%"
set "LOG_DIR=%INSTALL_DIR%\logs"
set "LOG_FILE=%LOG_DIR%\install-service.log"
if not exist "%LOG_DIR%" mkdir "%LOG_DIR%"

:: --- self-elevate to admin (re-launch silent, wait for it) ---
net session >nul 2>&1
if errorlevel 1 (
    powershell -NoProfile -Command "Start-Process -FilePath '%~f0' -Verb RunAs -WindowStyle Hidden -Wait"
    exit /b %errorlevel%
)

:: --- from here we are elevated; redirect everything to log ---
call :main >> "%LOG_FILE%" 2>&1
exit /b %errorlevel%

:main
echo.
echo ============================================
echo   [%date% %time%] Siigo Bridge Service Installer
echo ============================================

set "EXE_PATH=%INSTALL_DIR%\siigo-web.exe"
set "NSSM_DIR=%INSTALL_DIR%\nssm"
set "NSSM_EXE=%NSSM_DIR%\nssm.exe"
set "SERVICE_NAME=SiigoBridge"
set "NSSM_URL=https://nssm.cc/release/nssm-2.24.zip"

echo   Install dir: %INSTALL_DIR%
echo   Exe        : %EXE_PATH%
echo   Service    : %SERVICE_NAME%

if not exist "%EXE_PATH%" (
    echo ERROR: siigo-web.exe not found at %EXE_PATH%
    exit /b 1
)

:: --- download NSSM if missing ---
if not exist "%NSSM_EXE%" (
    echo [1/6] Downloading NSSM...
    if not exist "%NSSM_DIR%" mkdir "%NSSM_DIR%"
    powershell -NoProfile -Command "$ProgressPreference='SilentlyContinue'; Invoke-WebRequest -Uri '%NSSM_URL%' -OutFile '%NSSM_DIR%\nssm.zip'"
    if errorlevel 1 (
        echo ERROR: failed to download NSSM
        exit /b 1
    )
    powershell -NoProfile -Command "Expand-Archive -Path '%NSSM_DIR%\nssm.zip' -DestinationPath '%NSSM_DIR%\extracted' -Force"
    copy /y "%NSSM_DIR%\extracted\nssm-2.24\win64\nssm.exe" "%NSSM_EXE%" >nul
    rmdir /s /q "%NSSM_DIR%\extracted"
    del "%NSSM_DIR%\nssm.zip"
) else (
    echo [1/6] NSSM already present
)

:: --- remove existing service ---
echo [2/6] Removing old service (if exists)...
sc query %SERVICE_NAME% >nul 2>&1
if not errorlevel 1 (
    "%NSSM_EXE%" stop %SERVICE_NAME% >nul 2>&1
    "%NSSM_EXE%" remove %SERVICE_NAME% confirm >nul 2>&1
)

:: --- kill zombies ---
echo [3/6] Killing zombie processes...
taskkill /F /IM siigo-web.exe >nul 2>&1
taskkill /F /IM cloudflared.exe >nul 2>&1
timeout /t 2 /nobreak >nul

:: --- install ---
echo [4/6] Installing service...
"%NSSM_EXE%" install %SERVICE_NAME% "%EXE_PATH%"
if errorlevel 1 (
    echo ERROR: nssm install failed
    exit /b 1
)

:: --- configure ---
echo [5/6] Configuring service...
"%NSSM_EXE%" set %SERVICE_NAME% AppDirectory "%INSTALL_DIR%" >nul
"%NSSM_EXE%" set %SERVICE_NAME% DisplayName "Siigo Bridge (Finearom)" >nul
"%NSSM_EXE%" set %SERVICE_NAME% Description "Siigo-Finearom middleware. Auto-restart on crash. Launches Cloudflare tunnel + sync loops." >nul
"%NSSM_EXE%" set %SERVICE_NAME% Start SERVICE_AUTO_START >nul
"%NSSM_EXE%" set %SERVICE_NAME% ObjectName LocalSystem >nul
"%NSSM_EXE%" set %SERVICE_NAME% AppExit Default Restart >nul
"%NSSM_EXE%" set %SERVICE_NAME% AppRestartDelay 5000 >nul
"%NSSM_EXE%" set %SERVICE_NAME% AppThrottle 10000 >nul
"%NSSM_EXE%" set %SERVICE_NAME% AppStopMethodSkip 0 >nul
"%NSSM_EXE%" set %SERVICE_NAME% AppStopMethodConsole 15000 >nul
"%NSSM_EXE%" set %SERVICE_NAME% AppStopMethodWindow 5000 >nul
"%NSSM_EXE%" set %SERVICE_NAME% AppStopMethodThreads 5000 >nul
"%NSSM_EXE%" set %SERVICE_NAME% AppKillProcessTree 1 >nul
"%NSSM_EXE%" set %SERVICE_NAME% AppStdout "%LOG_DIR%\service-stdout.log" >nul
"%NSSM_EXE%" set %SERVICE_NAME% AppStderr "%LOG_DIR%\service-stderr.log" >nul
"%NSSM_EXE%" set %SERVICE_NAME% AppRotateFiles 1 >nul
"%NSSM_EXE%" set %SERVICE_NAME% AppRotateOnline 1 >nul
"%NSSM_EXE%" set %SERVICE_NAME% AppRotateBytes 10485760 >nul
"%NSSM_EXE%" set %SERVICE_NAME% AppStdoutCreationDisposition 4 >nul
"%NSSM_EXE%" set %SERVICE_NAME% AppStderrCreationDisposition 4 >nul
sc failure %SERVICE_NAME% reset= 86400 actions= restart/5000/restart/5000/restart/10000 >nul

:: --- start ---
echo [6/6] Starting service...
"%NSSM_EXE%" start %SERVICE_NAME%
if errorlevel 1 (
    echo ERROR: service did not start. See %LOG_DIR%\service-stderr.log
    exit /b 1
)

echo.
echo DONE. Service "%SERVICE_NAME%" running. Local: http://localhost:3210
exit /b 0
