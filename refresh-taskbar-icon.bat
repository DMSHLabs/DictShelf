@echo off
rem Copyright (C) 2026 DmShAl (Shepeta Dmitry)
rem SPDX-License-Identifier: GPL-3.0-or-later
setlocal
set "SCRIPT_DIR=%~dp0"
powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%SCRIPT_DIR%refresh-taskbar-icon.ps1" %*
set "RESULT=%ERRORLEVEL%"
if not "%RESULT%"=="0" pause
exit /b %RESULT%
