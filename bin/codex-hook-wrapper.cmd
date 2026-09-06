@echo off
rem codex-hook-wrapper.cmd - dedicated Codex launcher (Windows).
rem Windows support for the Codex route is NOT declared until the disposable
rem Windows scenario passes; this file ships so the frozen commandWindows
rem identity stays stable from the first release.
rem Observation contract: always exit 0 with empty output.
setlocal
set "CN_PRODUCT=codex"
if not defined PLUGIN_ROOT set "PLUGIN_ROOT=%~dp0.."
for %%F in ("%~dp0claude-notifications-windows-*.exe") do (
    "%%F" %* >nul 2>nul
    exit /b 0
)
exit /b 0
