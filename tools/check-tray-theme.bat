@echo off
setlocal

rem Read the Windows theme preference used by the taskbar and Start menu.
set "LIGHT_THEME="
for /f "tokens=3" %%A in ('reg query "HKCU\Software\Microsoft\Windows\CurrentVersion\Themes\Personalize" /v SystemUsesLightTheme 2^>nul ^| findstr /i "SystemUsesLightTheme"') do set "LIGHT_THEME=%%A"

if not defined LIGHT_THEME (
    echo Taskbar theme: unknown
    echo Windows does not expose SystemUsesLightTheme for this user.
    exit /b 1
)

if /i "%LIGHT_THEME%"=="0x0" (
    echo Taskbar theme: dark
) else if /i "%LIGHT_THEME%"=="0x1" (
    echo Taskbar theme: light
) else (
    echo Taskbar theme: unknown ^(%LIGHT_THEME%^)
    exit /b 1
)

echo Registry value: %LIGHT_THEME%
echo This reports the Windows theme preference, not the rendered taskbar color.
endlocal
