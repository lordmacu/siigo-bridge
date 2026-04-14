@echo off
setlocal

set "INSTALL_DIR=%~dp0"
set "INSTALL_DIR=%INSTALL_DIR:~0,-1%"
set "LOG_DIR=%INSTALL_DIR%\logs"
set "LOG_FILE=%LOG_DIR%\uninstall-service.log"
if not exist "%LOG_DIR%" mkdir "%LOG_DIR%"

:: --- self-elevate silent, wait ---
net session >nul 2>&1
if errorlevel 1 (
    powershell -NoProfile -Command "Start-Process -FilePath '%~f0' -Verb RunAs -WindowStyle Hidden -Wait"
    exit /b %errorlevel%
)

call :main >> "%LOG_FILE%" 2>&1
exit /b %errorlevel%

:main
echo [%date% %time%] Uninstalling SiigoBridge
set "NSSM_EXE=%INSTALL_DIR%\nssm\nssm.exe"
set "SERVICE_NAME=SiigoBridge"

if exist "%NSSM_EXE%" (
    "%NSSM_EXE%" stop %SERVICE_NAME% >nul 2>&1
    "%NSSM_EXE%" remove %SERVICE_NAME% confirm
) else (
    sc stop %SERVICE_NAME% >nul 2>&1
    sc delete %SERVICE_NAME%
)

taskkill /F /IM siigo-web.exe >nul 2>&1
taskkill /F /IM cloudflared.exe >nul 2>&1

echo DONE. Service removed.
exit /b 0
