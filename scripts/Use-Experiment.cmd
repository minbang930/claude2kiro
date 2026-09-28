@echo off
setlocal
if "%~1"=="" (
  echo Usage: scripts\Use-Experiment.cmd ^<branch^>
  exit /b 2
)
where pwsh.exe >nul 2>nul
if %errorlevel%==0 (
  pwsh.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0Use-Experiment.ps1" %*
) else (
  powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0Use-Experiment.ps1" %*
)
exit /b %errorlevel%
