@echo off
rem Copyright (C) 2026 DmShAl (Shepeta Dmitry)
rem SPDX-License-Identifier: GPL-3.0-or-later
setlocal EnableExtensions DisableDelayedExpansion

set "SHOW_HELP="
for %%A in (%*) do (
    if /i "%%~A"=="-?" set "SHOW_HELP=1"
    if /i "%%~A"=="--?" set "SHOW_HELP=1"
    if /i "%%~A"=="-h" set "SHOW_HELP=1"
    if /i "%%~A"=="--h" set "SHOW_HELP=1"
    if /i "%%~A"=="-help" set "SHOW_HELP=1"
    if /i "%%~A"=="--help" set "SHOW_HELP=1"
)
if defined SHOW_HELP goto :show_help

rem ============================================================
rem DictShelf Windows builds: [cli|server|installer] [--channel release|pre-release|beta] [--update-icon]
rem
rem   (no args)             listenerless Windows WebView app: DictShelf.exe
rem   cli                  console CLI: DictShelf-cli.exe (does not start a server)
rem   server               console HTTP server: DictShelf-server.exe
rem   installer            package the existing DictShelf.exe from the default build
rem                        under dist\ (needs Inno Setup 6.3+)
rem   --update-icon        recompile packaging\windows\dictshelf.ico into the executable
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
rem This builds the DictShelf desktop product, separate from upstream's wuDict.
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

rem ISCC.exe must be Inno Setup 6.3 or newer: packaging\windows\dictshelf.iss
rem uses directives older releases do not support. The installer script
rem searches the uninstall registry and PATH; set ISCC_PATH in the environment
rem or build-windows.local.bat when it cannot locate the compiler.

rem ------------------------------------------------------------
rem Arguments
rem ------------------------------------------------------------

set "BUILD_TYPE=exe"
set "WINDOW_MODE=gui"
set "WINDOW_LDFLAGS=-H windowsgui"
set "OUTPUT_EXE=DictShelf.exe"
set "DEFAULT_SERVE=1"
set "CHANNEL=release"
set "CHANNEL_SPECIFIED="
set "NEED_CHANNEL="
set "UPDATE_ICON="

if not "%~1"=="" for %%A in (%*) do (
    if defined NEED_CHANNEL (
        set "CHANNEL=%%A"
        set "CHANNEL_SPECIFIED=1"
        set "NEED_CHANNEL="
    ) else if /i "%%A"=="--channel" (
        if defined CHANNEL_SPECIFIED (
            echo ERROR: specify --channel only once.
            exit /b 1
        )
        set "NEED_CHANNEL=1"
    ) else if /i "%%A"=="--update-icon" (
        if defined UPDATE_ICON (
            echo ERROR: specify --update-icon only once.
            exit /b 1
        )
        set "UPDATE_ICON=1"
    ) else if /i "%%A"=="installer" (
        set "BUILD_TYPE=installer"
    ) else if /i "%%A"=="cli" (
        set "BUILD_TYPE=cli"
        set "WINDOW_MODE=console"
        set "WINDOW_LDFLAGS="
        set "OUTPUT_EXE=DictShelf-cli.exe"
        set "DEFAULT_SERVE=0"
    ) else if /i "%%A"=="server" (
        set "BUILD_TYPE=server"
        set "WINDOW_MODE=console"
        set "WINDOW_LDFLAGS="
        set "OUTPUT_EXE=DictShelf-server.exe"
        set "DEFAULT_SERVE=1"
    ) else (
        echo ERROR: unknown option "%%A".
        echo Usage: build-windows.cmd [cli^|server^|installer] [--channel release^|pre-release^|beta] [--update-icon]
        exit /b 1
    )
)
if defined NEED_CHANNEL (
    echo ERROR: --channel needs a value: release, pre-release or beta.
    exit /b 1
)
if defined CHANNEL_SPECIFIED if /i not "%CHANNEL%"=="release" if /i not "%CHANNEL%"=="pre-release" if /i not "%CHANNEL%"=="beta" (
    echo ERROR: unknown update channel "%CHANNEL%".
    echo Usage: build-windows.cmd [cli^|server^|installer] [--channel release^|pre-release^|beta] [--update-icon]
    exit /b 1
)
if /i "%BUILD_TYPE%"=="installer" if defined UPDATE_ICON (
    echo ERROR: --update-icon applies to executable builds, not installer packaging.
    exit /b 1
)

rem Packaging reuses the desktop binary built by the default invocation. This
rem keeps the installer's version and executable identical to release artifact 1.
if /i "%BUILD_TYPE%"=="installer" (
    if not exist "DictShelf.exe" (
        echo ERROR: DictShelf.exe is missing. Build the desktop executable first:
        echo   build-windows.cmd
        exit /b 1
    )
    set "VERSION="
    for /f "tokens=2" %%V in ('DictShelf.exe --version') do if not defined VERSION set "VERSION=%%V"
    call :check_channel
    if errorlevel 1 exit /b 1
    call :print_version
    call :find_iscc
    if errorlevel 1 exit /b 1
    call :make_installer
    if errorlevel 1 exit /b 1
    echo.
    echo BUILD SUCCESSFUL
    call :print_version
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
rem DictShelf version
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
if defined DICTSHELF_VERSION set "VERSION=%DICTSHELF_VERSION%"

if defined CHANNEL_SPECIFIED (
    call :stamp_channel
    if errorlevel 1 exit /b 1
)

call :print_version
if defined CHANNEL_SPECIFIED (
    echo Update channel:
    echo   %CHANNEL%
)
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
echo DictShelf Windows %FLAVOUR% build
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

rem Recompile the PE icon resource only when explicitly requested. The checked-in
rem .syso keeps ordinary builds and clean CI builds self-contained.
if defined UPDATE_ICON call :update_embedded_icon
if errorlevel 1 exit /b 1
if not defined UPDATE_ICON if not exist "dictshelf_windows_amd64.syso" (
    echo ERROR: the embedded icon resource is missing.
    echo   Restore dictshelf_windows_amd64.syso or run build-windows.cmd --update-icon.
    exit /b 1
)

rem Version information is generated for every executable build from the same
rem version that is stamped into the Go binary.
call :update_version_info
if errorlevel 1 exit /b 1

rem taskkill may return before Windows releases the image file. Wait for the
rem process to disappear before deleting the old executable or invoking Go.
call :stop_running_output_executable
if errorlevel 1 exit /b 1

rem Only replace the current executable after the old process has exited.
if exist "%OUTPUT_EXE%" (
    set "FILE_WAIT=0"
    call :wait_for_output_file_unlock
    if errorlevel 1 exit /b 1
)
if exist "%OUTPUT_EXE%~" del /F /Q "%OUTPUT_EXE%~"

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
call :print_version

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
    echo   DictShelf.exe                   open the app without a console window
) else if /i "%BUILD_TYPE%"=="cli" (
    echo   %OUTPUT_EXE%                   run CLI commands; without arguments shows help
) else (
    echo   %OUTPUT_EXE%                   run the HTTP server
)
echo   %OUTPUT_EXE% --help            every command and flag
echo.

endlocal
exit /b 0

:show_help
echo DictShelf Windows build
echo.
echo Usage:
echo   build-windows.cmd [cli^|server^|installer] [--channel release^|pre-release^|beta] [--update-icon]
echo.
echo Help keys: -?, --?, -h, --h, -help, --help
echo.
echo Build targets:
echo   ^(no target^)  Build the desktop DictShelf.exe application. This is the default.
echo   cli          Build DictShelf-cli.exe, the console command-line application.
echo   server       Build DictShelf-server.exe, the console HTTP server.
echo   installer    Package the existing DictShelf.exe with Inno Setup 6.3 or newer.
echo.
echo Update channels:
echo   --channel release^|pre-release^|beta
echo   The channel is stamped into the version shown by the application.
echo.
echo Icon:
echo   --update-icon  Rebuild the embedded Windows icon before building the executable.
echo.
echo Version:
echo   Set DICTSHELF_VERSION to override the version read from Git, for example:
echo     set "DICTSHELF_VERSION=dictshelf-v1.02.01"
echo     build-windows.cmd --channel beta
endlocal
exit /b 0

:update_embedded_icon
set "WINDRES_PATH="
for /f "delims=" %%W in ('where windres 2^>nul') do if not defined WINDRES_PATH set "WINDRES_PATH=%%W"
if not defined WINDRES_PATH if defined GCC_PATH for %%D in ("%GCC_PATH%") do if exist "%%~dpDwindres.exe" set "WINDRES_PATH=%%~dpDwindres.exe"
if not defined WINDRES_PATH if defined CC_PATH for %%D in ("%CC_PATH%") do if exist "%%~dpDwindres.exe" set "WINDRES_PATH=%%~dpDwindres.exe"
if not defined WINDRES_PATH (
    echo ERROR: windres.exe was not found. It is needed to embed the current app icon.
    echo   Add windres.exe to PATH or install it beside the configured GCC compiler.
    exit /b 1
)
echo Updating the embedded Windows icon...
"%WINDRES_PATH%" -F pe-x86-64 -i packaging\windows\dictshelf.rc -o dictshelf_windows_amd64.syso
if errorlevel 1 (
    echo ERROR: Could not compile packaging\windows\dictshelf.ico into the Windows executable resource.
    exit /b 1
)
exit /b 0

:update_version_info
set "WINDRES_PATH="
for /f "delims=" %%W in ('where windres 2^>nul') do if not defined WINDRES_PATH set "WINDRES_PATH=%%W"
if not defined WINDRES_PATH if defined GCC_PATH for %%D in ("%GCC_PATH%") do if exist "%%~dpDwindres.exe" set "WINDRES_PATH=%%~dpDwindres.exe"
if not defined WINDRES_PATH if defined CC_PATH for %%D in ("%CC_PATH%") do if exist "%%~dpDwindres.exe" set "WINDRES_PATH=%%~dpDwindres.exe"
if not defined WINDRES_PATH (
    echo ERROR: windres.exe was not found. It is needed to embed Windows version information.
    exit /b 1
)
echo Embedding Windows version information for %VERSION%...
powershell -NoProfile -ExecutionPolicy Bypass -File "tools\make-windows-version-info.ps1" -Windres "%WINDRES_PATH%" -Version "%VERSION%" -OriginalFilename "%OUTPUT_EXE%"
if errorlevel 1 (
    echo ERROR: Could not generate Windows version information.
    exit /b 1
)
exit /b 0

:stop_running_output_executable
tasklist /FI "IMAGENAME eq %OUTPUT_EXE%" /NH 2>nul | findstr /i /c:"%OUTPUT_EXE%" >nul
if errorlevel 1 exit /b 0

echo Stopping %OUTPUT_EXE%...
taskkill /IM "%OUTPUT_EXE%" /F >nul 2>&1
set "STOP_WAIT=0"
:wait_for_output_executable
tasklist /FI "IMAGENAME eq %OUTPUT_EXE%" /NH 2>nul | findstr /i /c:"%OUTPUT_EXE%" >nul
if errorlevel 1 (
    echo %OUTPUT_EXE% has exited.
    exit /b 0
)
set /a STOP_WAIT+=1
if %STOP_WAIT% GEQ 60 (
    echo ERROR: %OUTPUT_EXE% did not exit after 60 seconds.
    echo   Close it manually, then run the build again.
    exit /b 1
)
ping -n 2 -w 1000 127.0.0.1 >nul
goto :wait_for_output_executable

:wait_for_output_file_unlock
del /F /Q "%OUTPUT_EXE%" >nul 2>&1
if not exist "%OUTPUT_EXE%" exit /b 0
set /a FILE_WAIT+=1
if %FILE_WAIT% GEQ 60 (
    echo ERROR: Windows still holds %OUTPUT_EXE% after 60 seconds.
    echo   Close any process using the file, then run the build again.
    exit /b 1
)
ping -n 2 -w 1000 127.0.0.1 >nul
goto :wait_for_output_file_unlock

:print_version
set "DISPLAY_VERSION=dictshelf-%VERSION%"
if /i "%VERSION:~0,1%"=="v" set "DISPLAY_VERSION=dictshelf-%VERSION%"
for /f "tokens=2 delims=v" %%V in ("%VERSION%") do set "DISPLAY_VERSION=dictshelf-v%%V"
echo DictShelf version:
echo   %DISPLAY_VERSION%
echo.
exit /b 0

:stamp_channel
echo(%VERSION%| findstr /i /c:"beta" >nul
if /i "%CHANNEL%"=="beta" (
    if errorlevel 1 set "VERSION=%VERSION%-beta.1"
) else if /i "%CHANNEL%"=="pre-release" (
    if not errorlevel 1 (
        echo ERROR: a Beta version cannot be built in the Pre-release channel.
        exit /b 1
    )
    echo(%VERSION%| findstr /i /c:"-pre" >nul
    if errorlevel 1 set "VERSION=%VERSION%-pre"
) else if not errorlevel 1 (
    echo ERROR: a Beta version cannot be built in the Release channel.
    exit /b 1
)
exit /b 0

:check_channel
if not defined CHANNEL_SPECIFIED exit /b 0
echo(%VERSION%| findstr /i /c:"beta" >nul
if /i "%CHANNEL%"=="beta" (
    if errorlevel 1 (
        echo ERROR: the existing DictShelf.exe is not stamped as Beta.
        echo   Rebuild it with: build-windows.cmd --channel beta
        exit /b 1
    )
) else if not errorlevel 1 (
    echo ERROR: the existing DictShelf.exe is stamped as Beta, not %CHANNEL%.
    exit /b 1
)
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
    echo   A plain `build-windows.cmd` still produces DictShelf.exe.
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
    echo   A plain `build-windows.cmd` still produces DictShelf.exe.
    exit /b 1
)

echo Inno Setup:
echo   %ISCC%
echo.
exit /b 0

rem ============================================================
rem :make_installer - compiles the per-user installer
rem
rem Called with no arguments: DictShelf.exe and dist\ in the repository root are
rem the installer script's own defaults, which is the invocation
rem `make win-installer` and CI both use - one less thing that can drift. It
rem reads the product name and version out of the binary (`DictShelf.exe
rem --version`), so nothing about the identity is restated here.
rem ============================================================

:make_installer
echo ============================================================
echo Building the per-user installer
echo ============================================================
echo.

powershell -NoProfile -ExecutionPolicy Bypass -File tools\make-installer.ps1 -Exe .\DictShelf.exe %ISCC_ARG%

if errorlevel 1 (
    echo.
    echo ============================================================
    echo ERROR: Inno Setup failed.
    echo ============================================================
    echo   A compiler older than 6.3 cannot read packaging\windows\dictshelf.iss
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

for %%F in ("dist\dictshelf-windows-x64-setup-*.exe") do (
    if exist "%%~fF" set "SETUP=%%~fF"
)

if not defined SETUP (
    echo ERROR: iscc reported success but no setup executable was found:
    echo   dist\dictshelf-windows-x64-setup-*.exe
    exit /b 1
)

rem Both v1.2.3 and dictshelf-v1.2.3 tags provide the numeric
rem installer version. Unversioned development builds still read 0.0.0.
echo %VERSION% | findstr /r /c:"^v[0-9]" /c:"^[A-Za-z0-9_-]*-v[0-9]" >nul
if errorlevel 1 (
    echo.
    echo Note: the installer's file name and numeric version read 0.0.0 because
    echo   "%VERSION%" has no release version number.
    echo   Everything shown inside the wizard still carries the real version.
)

exit /b 0
