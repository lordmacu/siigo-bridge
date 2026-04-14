@echo off
setlocal

:: --- self-elevate ---
net session >nul 2>&1
if errorlevel 1 (
    powershell -Command "Start-Process -FilePath '%~f0' -Verb RunAs"
    exit /b
)

set "INSTALL_DIR=%~dp0"
set "INSTALL_DIR=%INSTALL_DIR:~0,-1%"
set "NSSM_EXE=%INSTALL_DIR%\nssm\nssm.exe"
set "SERVICE_NAME=SiigoBridge"

echo Stopping and removing %SERVICE_NAME%...

if exist "%NSSM_EXE%" (
    "%NSSM_EXE%" stop %SERVICE_NAME% >nul 2>&1
    "%NSSM_EXE%" remove %SERVICE_NAME% confirm
) else (
    sc stop %SERVICE_NAME% >nul 2>&1
    sc delete %SERVICE_NAME%
)

taskkill /F /IM siigo-web.exe >nul 2>&1
taskkill /F /IM cloudflared.exe >nul 2>&1

echo Done. Service removed. NSSM folder and logs kept.
pause
endlocal
