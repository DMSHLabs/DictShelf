@echo off
rem Copyright (C) 2026 DmShAl (Shepeta Dmitry)
rem SPDX-License-Identifier: GPL-3.0-or-later
setlocal EnableExtensions DisableDelayedExpansion

rem ============================================================
rem wuDict2 Windows builds: [cli|server|installer]
rem
rem   (no args)             listenerless Windows WebView app: wuDict2.exe
rem   cli                  console CLI: wuDict2-cli.exe (does not start a server)
rem   server               console HTTP server: wuDict2-server.exe
rem   installer            package the existing wuDict2.exe from the default build
rem                        under dist\ (needs Inno Setup 6.3+)
rem
rem The installer mode packages the exact GUI executable built by the default mode.
rem
rem FLAVOUR. Windows desktop builds always use cgo - mattn SQLite with FTS5
rem (D4) and the built-in speex .spx decoder. A compatible 64-bit mingw-w64
rem GCC is required; this script never falls back to purego.
rem
rem Machine-specific compiler overrides belong in the ignored
rem build-windows.local.bat. Otherwise the C compiler is found from CC, PATH or
rem GCC_ROOTS, and Inno Setup is found from the registry or PATH.
rem
rem A C COMPILER need not be on PATH, and cl.exe can never be used - Go's cgo
rem drives a gcc-style compiler, not MSVC. With GCC_PATH left empty, the search
rem is: %CC%, then gcc.exe on PATH, then GCC_ROOTS. One found by path is handed
rem to Go as an absolute CC, with CXX beside it; gcc locates its own cc1, ld and
rem runtime DLLs relative to itself, so PATH is untouched.
rem
rem This builds the wuDict2 desktop product, separate from upstream's wuDict.
rem ============================================================

cd /d "%~dp0"

rem Machine-specific tool paths belong in this ignored local file, never here.
rem GitHub Actions configures its own compiler and must not load local settings.
if not defined GITHUB_ACTIONS if exist "%~dp0build-windows.local.bat" (
    call "%~dp0build-windows.local.bat"
    if errorlevel 1 (
        echo ERROR: Could not load build-windows.local.bat.
        exit /b 1
    )
)

rem ============================================================
rem TOOL PATHS - local overrides load from build-windows.local.bat above.
rem ============================================================

rem The C compiler behind the cgo flavour: a 64-bit mingw-w64 GCC (cl.exe
rem cannot be driven by Go's cgo). Set GCC_PATH or CC in the environment or in
rem build-windows.local.bat to pin a machine-specific compiler.

rem Where a mingw-w64 GCC is looked for when GCC_PATH is empty or stale and
rem gcc.exe is not on PATH. Quoted, wildcards allowed; the last directory that
rem matches and really holds bin\gcc.exe wins.
set "GCC_ROOTS="%SystemDrive%\Qt\Tools\mingw*_64" "%SystemDrive%\msys64\mingw64" "%SystemDrive%\mingw64" "%ProgramFiles%\mingw64" "%ProgramFiles%\WinLibs*\mingw64" "%LOCALAPPDATA%\Programs\mingw64""

rem ISCC.exe must be Inno Setup 6.3 or newer: packaging\windows\wudict.iss
rem uses directives older releases do not support. The installer script
rem searches the uninstall registry and PATH; set ISCC_PATH in the environment
rem or build-windows.local.bat when it cannot locate the compiler.

rem ------------------------------------------------------------
rem Arguments
rem ------------------------------------------------------------

set "BUILD_TYPE=exe"
set "WINDOW_MODE=gui"
set "WINDOW_LDFLAGS=-H windowsgui"
set "OUTPUT_EXE=wuDict2.exe"
set "DEFAULT_SERVE=1"

if not "%~1"=="" for %%A in (%*) do (
    if /i "%%A"=="installer" (
        set "BUILD_TYPE=installer"
    ) else if /i "%%A"=="cli" (
        set "BUILD_TYPE=cli"
        set "WINDOW_MODE=console"
        set "WINDOW_LDFLAGS="
        set "OUTPUT_EXE=wuDict2-cli.exe"
        set "DEFAULT_SERVE=0"
    ) else if /i "%%A"=="server" (
        set "BUILD_TYPE=server"
        set "WINDOW_MODE=console"
        set "WINDOW_LDFLAGS="
        set "OUTPUT_EXE=wuDict2-server.exe"
        set "DEFAULT_SERVE=1"
    ) else (
        echo ERROR: unknown option "%%A".
        echo Usage: build-windows.cmd [cli^|server^|installer]
        exit /b 1
    )
)

rem Packaging reuses the desktop binary built by the default invocation. This
rem keeps the installer's version and executable identical to release artifact 1.
if /i "%BUILD_TYPE%"=="installer" (
    if not exist "wuDict2.exe" (
        echo ERROR: wuDict2.exe is missing. Build the desktop executable first:
        echo   build-windows.cmd
        exit /b 1
    )
    set "VERSION="
    for /f "tokens=2" %%V in ('wuDict2.exe --version') do if not defined VERSION set "VERSION=%%V"
    call :find_iscc
    if errorlevel 1 exit /b 1
    call :make_installer
    if errorlevel 1 exit /b 1
    echo.
    echo BUILD SUCCESSFUL
    echo Installer:
    echo   %SETUP%
    endlocal
    exit /b 0
)

rem ------------------------------------------------------------
rem Check required tools
rem ------------------------------------------------------------

where go >nul 2>&1
if errorlevel 1 (
    echo ERROR: go.exe was not found in PATH.
    exit /b 1
)

where git >nul 2>&1
if errorlevel 1 (
    echo ERROR: git.exe was not found in PATH ^(the version stamp needs it^).
    exit /b 1
)

rem ------------------------------------------------------------
rem wuDict version
rem Same idea as the Makefile and build-android.cmd:
rem   git describe --tags --always --dirty
rem ------------------------------------------------------------

set "VERSION="

for /f "delims=" %%V in ('git describe --tags --always --dirty 2^>nul') do (
    set "VERSION=%%V"
)

if not defined VERSION (
    set "VERSION=dev"
)

rem CI supplies the release tag so installer metadata matches the published artifact.
if defined WUDICT_VERSION set "VERSION=%WUDICT_VERSION%"

echo wuDict version:
echo   %VERSION%
echo.

rem ------------------------------------------------------------
rem Windows desktop builds require cgo; never silently build purego
rem ------------------------------------------------------------

set "CC_PATH="
call :find_gcc
if not defined CC_PATH (
    echo ERROR: a 64-bit mingw-w64 GCC is required for the Windows cgo build.
    echo   Install one, or set GCC_PATH in build-windows.local.bat or CC in the environment.
    echo   This script does not build the pure-Go flavour.
    exit /b 1
)
"%CC_PATH%" --version >nul 2>&1
if errorlevel 1 (
    echo ERROR: the selected C compiler cannot be run:
    echo   %CC_PATH%
    echo   Check GCC_PATH or CC. The existing executable was not replaced.
    exit /b 1
)

set "FLAVOUR=cgo"
set "GO_TAGS=sqlite_fts5"
set "CGO=1"

rem ------------------------------------------------------------
rem ------------------------------------------------------------
rem Build
rem ------------------------------------------------------------

echo.
echo ============================================================
echo wuDict Windows %FLAVOUR% build
echo ============================================================
echo.

echo Flavour:
echo   %FLAVOUR% ^(tags: %GO_TAGS%, CGO_ENABLED=%CGO%^)
echo Window subsystem:
echo   %WINDOW_MODE%

echo Compiler:
echo   %CC_PATH%

echo.
echo Output:
echo   %CD%\%OUTPUT_EXE%
echo.

rem A running server holds wuDict2.exe open, and Windows then refuses to let the
rem linker replace it - the "Access is denied" trap docs\WINDOWS-VERIFY.md
rem records for preview servers. Said here, where it can still be acted on.
tasklist /FI "IMAGENAME eq %OUTPUT_EXE%" 2>nul | findstr /i /c:"%OUTPUT_EXE%" >nul
if not errorlevel 1 (
    echo WARNING: %OUTPUT_EXE% is running. The build will try to stop it before
    echo   replacing the file; if that fails, close it and run the build again:
    echo     taskkill /F /IM %OUTPUT_EXE%
    echo.
)

rem Only replace the current executable after all prerequisites are present.
taskkill /IM "%OUTPUT_EXE%" /F >nul 2>&1
del /F /Q "%OUTPUT_EXE%"
del /F /Q "%OUTPUT_EXE%~"

rem A compiler found by path is passed as an absolute CC because it is not on
rem PATH, and CXX is derived beside it only when that g++.exe is really there -
rem %CC% may name a bare `clang`, which must be left for Go to resolve.
if defined CC_PATH (
    set "CC=%CC_PATH%"
    for %%D in ("%CC_PATH%") do if exist "%%~dpDg++.exe" set "CXX=%%~dpDg++.exe"
)

set "CGO_ENABLED=%CGO%"

go build ^
  -trimpath ^
  -tags %GO_TAGS% ^
  -ldflags "%WINDOW_LDFLAGS% -s -w -X github.com/wuweidict/wudict/internal/cli.Version=%VERSION% -X github.com/wuweidict/wudict/internal/cli.DefaultServe=%DEFAULT_SERVE%" ^
  -o "%OUTPUT_EXE%" ^
  .

if errorlevel 1 (
    echo.
    echo ============================================================
    echo ERROR: Go build failed.
    echo ============================================================
    exit /b 1
)

rem ------------------------------------------------------------
rem Verify the binary
rem ------------------------------------------------------------

echo.
echo ============================================================
echo Verifying %OUTPUT_EXE%
echo ============================================================
echo.

if not exist "%OUTPUT_EXE%" (
    echo ERROR: go build finished but %OUTPUT_EXE% was not found.
    exit /b 1
)

rem It has to START, not merely exist. A cgo build links the mingw runtime
rem dynamically unless it is told otherwise, so a missing libwinpthread-1.dll
rem or libgcc_s_seh-1.dll yields a build that reports success and a binary
rem that dies before main. --version is the cheapest run there is: no port,
rem no dictionary, no tray.
set "RAN="
for /f "delims=" %%V in ('"%OUTPUT_EXE%" --version') do if not defined RAN set "RAN=%%V"

if not defined RAN (
    echo ERROR: %OUTPUT_EXE% was built but does not run.
    echo   Check that the mingw runtime DLLs ^(libwinpthread-1.dll,
    echo   libgcc_s_seh-1.dll^) are beside the exe or on PATH.
    exit /b 1
)

echo OK: %OUTPUT_EXE% runs
echo   %RAN%
echo.

rem The build records its tags, its CGO_ENABLED and the whole module graph
rem inside the exe, so the flavour can be read back from the artifact instead
rem of trusted from the flags above. This is what catches a stale file left by
rem an earlier run or a build cache hit answering with the wrong tags.
set "WANT_DEP=github.com/mattn/go-sqlite3"

go version -m "%OUTPUT_EXE%" | findstr /i /c:"-tags=%GO_TAGS%" >nul
if errorlevel 1 (
    echo ERROR: %OUTPUT_EXE% was not built with -tags %GO_TAGS%.
    exit /b 1
)

go version -m "%OUTPUT_EXE%" | findstr /i /c:"CGO_ENABLED=%CGO%" >nul
if errorlevel 1 (
    echo ERROR: %OUTPUT_EXE% was not built with CGO_ENABLED=%CGO%.
    exit /b 1
)

go version -m "%OUTPUT_EXE%" | findstr /i /c:"%WANT_DEP%" >nul
if errorlevel 1 (
    echo ERROR: %OUTPUT_EXE% does not carry %WANT_DEP%.
    exit /b 1
)

echo OK: built as asked - -tags %GO_TAGS%, CGO_ENABLED=%CGO%, %WANT_DEP%
echo.

echo.
echo ============================================================
echo BUILD SUCCESSFUL
echo ============================================================
echo.

echo Binary:
echo   %CD%\%OUTPUT_EXE%
echo.

if defined SETUP (
    echo Installer:
    echo   %SETUP%
    echo.
)

echo Run it:
if /i "%WINDOW_MODE%"=="gui" (
    echo   wuDict2.exe                   open the app without a console window
) else if /i "%BUILD_TYPE%"=="cli" (
    echo   %OUTPUT_EXE%                   run CLI commands; without arguments shows help
) else (
    echo   %OUTPUT_EXE%                   run the HTTP server
)
echo   %OUTPUT_EXE% --help            every command and flag
echo.

endlocal
exit /b 0

rem ============================================================
rem :find_gcc - sets CC_PATH when a C compiler the build can use is found
rem
rem %CC% first: an explicit setting outranks any guess, and it is passed on as
rem given (a bare `gcc` or `clang` is left for Go to resolve). Then PATH, which
rem needs nothing set at all. Then the roots a mingw-w64 GCC normally lives
rem under - the Qt Tools one first, since a Qt installation is the usual way
rem a compiler reaches a Windows box without ever being added to PATH.
rem
rem A wrong guess cannot silently win: the candidate has to exist as gcc.exe,
rem and a binary that is not a compiler Go can drive fails the build loudly.
rem ============================================================

:find_gcc
rem GCC_PATH first: a pinned path outranks any search. Then %CC%, passed on as
rem given (a bare `gcc` or `clang` is left for Go to resolve). Then PATH, which
rem needs nothing set at all. Then GCC_ROOTS.
if defined GCC_PATH (
    if exist "%GCC_PATH%" (
        set "CC_PATH=%GCC_PATH%"
        exit /b 0
    )
    echo Note: GCC_PATH points at a file that is not there:
    echo   %GCC_PATH%
    echo   Falling back to the search: gcc.exe on PATH, then GCC_ROOTS.
    echo.
)

if defined CC (
    set "CC_PATH=%CC%"
    exit /b 0
)

where gcc >nul 2>&1
if not errorlevel 1 exit /b 0

rem A pattern that matched nothing comes back as itself, so it is the `if exist`
rem that decides, not the loop having run.
for /d %%D in (%GCC_ROOTS%) do if exist "%%~fD\bin\gcc.exe" set "CC_PATH=%%~fD\bin\gcc.exe"

exit /b 0

rem ============================================================
rem :find_iscc - asks tools\make-installer.ps1 where Inno Setup is
rem
rem No second copy of that search lives here: `-Locate` prints the compiler the
rem installer script itself would use, prints nothing when there is none, and
rem exits 0 either way - so empty output is the answer, not a failure.
rem ============================================================

:find_iscc
where powershell >nul 2>&1
if errorlevel 1 (
    echo ERROR: powershell.exe was not found, and the installer is compiled by
    echo   tools\make-installer.ps1 - the script that knows where Inno Setup is.
    echo   A plain `build-windows.cmd` still produces wuDict2.exe.
    exit /b 1
)

set "ISCC="
set "ISCC_ARG="

if defined ISCC_PATH (
    if exist "%ISCC_PATH%" (
        set "ISCC=%ISCC_PATH%"
        rem Handed to make-installer.ps1 as -Iscc: a pinned compiler is used as
        rem given, which also bypasses that script's own search and its 6.3
        rem floor - the reason ISCC_PATH has to point at 6.3 or newer.
        set "ISCC_ARG=-Iscc "%ISCC_PATH%""
    ) else (
        echo Note: ISCC_PATH points at a file that is not there:
        echo   %ISCC_PATH%
        echo   Falling back to the installer script's own search.
        echo.
    )
)

if not defined ISCC (
    for /f "delims=" %%I in ('powershell -NoProfile -ExecutionPolicy Bypass -File tools\make-installer.ps1 -Locate 2^>nul') do (
        set "ISCC=%%I"
    )
)

if not defined ISCC (
    echo ERROR: no Inno Setup 6.3 or newer was found, and `installer` packages
    echo   the desktop executable. Install it, then run this script again:
    echo     winget install JRSoftware.InnoSetup
    echo   A plain `build-windows.cmd` still produces wuDict2.exe.
    exit /b 1
)

echo Inno Setup:
echo   %ISCC%
echo.
exit /b 0

rem ============================================================
rem :make_installer - compiles the per-user installer
rem
rem Called with no arguments: wuDict2.exe and dist\ in the repository root are
rem the installer script's own defaults, which is the invocation
rem `make win-installer` and CI both use - one less thing that can drift. It
rem reads the product name and version out of the binary (`wuDict2.exe
rem --version`), so nothing about the identity is restated here.
rem ============================================================

:make_installer
echo ============================================================
echo Building the per-user installer
echo ============================================================
echo.

powershell -NoProfile -ExecutionPolicy Bypass -File tools\make-installer.ps1 -Exe .\wuDict2.exe %ISCC_ARG%

if errorlevel 1 (
    echo.
    echo ============================================================
    echo ERROR: Inno Setup failed.
    echo ============================================================
    echo   A compiler older than 6.3 cannot read packaging\windows\wudict.iss
    echo   ^(x64compatible, PrivilegesRequiredOverridesAllowed^). Inno Setup 5
    echo   is one: it writes no error text a redirect can carry, and it either
    echo   fails or reports a success that produced no file - both seen here.
    echo   This installer needs 6.3 or newer:
    echo     winget install JRSoftware.InnoSetup
    echo   then clear ISCC_PATH from the environment and build-windows.local.bat
    echo   so the installer script finds the new compiler itself, or update it.
    exit /b 1
)

set "SETUP="

for %%F in ("dist\wudict2-windows-x64-setup-*.exe") do (
    if exist "%%~fF" set "SETUP=%%~fF"
)

if not defined SETUP (
    echo ERROR: iscc reported success but no setup executable was found:
    echo   dist\wudict2-windows-x64-setup-*.exe
    exit /b 1
)

rem Both v1.2.3 and this fork's wudict2-v1.2.3 tags provide the numeric
rem installer version. Unversioned development builds still read 0.0.0.
echo %VERSION% | findstr /r /c:"^v[0-9]" /c:"^wudict2-v[0-9]" >nul
if errorlevel 1 (
    echo.
    echo Note: the installer's file name and numeric version read 0.0.0 because
    echo   "%VERSION%" has no release version number.
    echo   Everything shown inside the wizard still carries the real version.
)

exit /b 0
