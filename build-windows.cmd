@echo off
rem Copyright (C) 2026 DmShAl (Shepeta Dmitry)
rem SPDX-License-Identifier: GPL-3.0-or-later
setlocal EnableExtensions DisableDelayedExpansion

rem ============================================================
rem wuDict Windows desktop build: [debug|release] [installer] [purego]
rem
rem   debug    (default)  build wudict.exe in the repository root
rem   release             alias for installer: build wudict.exe plus the
rem                       per-user installer in dist\ (needs Inno Setup 6.3+)
rem   installer            build wudict.exe plus the per-user installer
rem   purego              force the pure-Go flavour (see below)
rem
rem Tokens may be written in either order, the way build-android.cmd takes
rem `debug intel` and `intel debug`.
rem
rem FLAVOUR. `make build` builds cgo - mattn SQLite with FTS5 (D4) and the
rem built-in speex .spx decoder - and this script does the same whenever it
rem can find a C compiler. With none it falls back to the pure-Go flavour,
rem which is correct but not the fast one: modernc SQLite, and .spx audio only
rem through an external speexdec. `purego` asks for that flavour deliberately.
rem
rem THE PATHS this script depends on are at the top, in the block below this
rem one: the C compiler and the Inno Setup compiler. Everything else it needs
rem is either on PATH or derived from the repository.
rem
rem A C COMPILER need not be on PATH, and cl.exe can never be used - Go's cgo
rem drives a gcc-style compiler, not MSVC. With GCC_PATH left empty, the search
rem is: %CC%, then gcc.exe on PATH, then GCC_ROOTS. One found by path is handed
rem to Go as an absolute CC, with CXX beside it; gcc locates its own cc1, ld and
rem runtime DLLs relative to itself, so PATH is untouched.
rem
rem This builds the DESKTOP product - upstream's wuDict, port 6888, its own
rem config and library. It is NOT the Android app: that one is
rem build-android.cmd (wuDict2, port 6889, the same code inside an APK).
rem ============================================================

cd /d "%~dp0"

rem ============================================================
rem PATHS - what this machine has to provide. Edit here, not below.
rem ============================================================

rem The C compiler behind the cgo flavour: a 64-bit mingw-w64 GCC (cl.exe
rem cannot be driven by Go's cgo). This machine's comes from Qt, and is the one
rem the search below would land on anyway. An inherited GCC_PATH is respected,
rem the way build-android.cmd respects an inherited ANDROID_HOME.
if not defined GCC_PATH set "GCC_PATH=C:\Qt\Tools\mingw1310_64\bin\gcc.exe"

rem Where a mingw-w64 GCC is looked for when GCC_PATH is empty or stale and
rem gcc.exe is not on PATH. Quoted, wildcards allowed; the last directory that
rem matches and really holds bin\gcc.exe wins.
set "GCC_ROOTS="%SystemDrive%\Qt\Tools\mingw*_64" "%SystemDrive%\msys64\mingw64" "%SystemDrive%\mingw64" "%ProgramFiles%\mingw64" "%ProgramFiles%\WinLibs*\mingw64" "%LOCALAPPDATA%\Programs\mingw64""

rem ISCC.exe, the Inno Setup compiler, and it must be 6.3 or newer:
rem packaging\windows\wudict.iss uses x64compatible and
rem PrivilegesRequiredOverridesAllowed, which nothing older reads. This
rem machine's Inno Setup is 7, installed outside the uninstall registry, so the
rem installer script cannot find it on its own - `make-installer.ps1 -Locate`
rem prints nothing - and the path is pinned here. (Inno Setup 5, a copy of
rem which also sits on this machine, cannot read the script at all: it emits no
rem error text a redirect can carry, and either fails or reports a success that
rem wrote no file.) Empty = ask tools\make-installer.ps1, whose own search
rem covers the uninstall registry and PATH. An inherited ISCC_PATH is
rem respected, the way an inherited GCC_PATH is.
if not defined ISCC_PATH set "ISCC_PATH=D:\ProgSoft\InnoSetup7\ISCC.exe"

rem ------------------------------------------------------------
rem Arguments
rem ------------------------------------------------------------

set "BUILD_TYPE="
set "FLAVOUR="
set "MAKE_INSTALLER="

if not "%~1"=="" for %%A in (%*) do (
    if /i "%%A"=="debug" (
        set "BUILD_TYPE=debug"
    ) else if /i "%%A"=="purego" (
        set "FLAVOUR=purego"
    ) else if /i "%%A"=="installer" (
        set "MAKE_INSTALLER=1"
    ) else if /i "%%A"=="release" (
        set "BUILD_TYPE=release"
        set "MAKE_INSTALLER=1"
    ) else (
        echo ERROR: unknown option "%%A".
        echo Usage: build-windows.cmd [debug^|release] [installer] [purego]
        exit /b 1
    )
)

if not defined BUILD_TYPE set "BUILD_TYPE=debug"

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

echo wuDict version:
echo   %VERSION%
echo.

rem ------------------------------------------------------------
rem Flavour: cgo when a C compiler can be found, pure-Go when it cannot
rem ------------------------------------------------------------

set "GO_TAGS=purego"
set "CGO=0"
set "CC_PATH="

if defined FLAVOUR (
    rem A requested flavour is used as asked; no compiler is looked for.
    set "FLAVOUR=purego"
) else (
    call :find_gcc
    if defined CC_PATH (
        set "FLAVOUR=cgo"
    ) else (
        set "FLAVOUR=purego"
        echo WARNING: no C compiler found - gcc.exe is neither on PATH nor in
        echo   any of the mingw-w64 locations this script knows.
        echo   Building the pure-Go flavour instead: correct, but its SQLite is
        echo   the slower modernc driver and .spx audio needs an external speexdec.
        echo   A 64-bit mingw-w64 GCC gets you what `make build` produces:
        echo     winget install BrechtSanders.WinLibs.POSIX.UCRT
        echo   or point this script at a compiler you already have:
        echo     set "CC=C:\Qt\Tools\mingw1310_64\bin\gcc.exe"
        echo.
    )
)

if /i "%FLAVOUR%"=="cgo" (
    set "GO_TAGS=sqlite_fts5"
    set "CGO=1"
)

rem ------------------------------------------------------------
rem Installer: Inno Setup is only needed when requested, and tools\make-installer.ps1
rem is the one place that knows where it lives - `-Locate` is the installer
rem script's own search. Asked BEFORE the Go build, so a missing compiler costs
rem two seconds instead of the length of a full build.
rem ------------------------------------------------------------

if defined MAKE_INSTALLER (
    call :find_iscc
    if errorlevel 1 exit /b 1
)

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

if /i "%FLAVOUR%"=="cgo" (
    echo Compiler:
    if defined CC_PATH (
        echo   %CC_PATH%
    ) else (
        echo   gcc.exe from PATH
    )
)

echo.
echo Output:
echo   %CD%\wudict.exe
echo.

rem A running server holds wudict.exe open, and Windows then refuses to let the
rem linker replace it - the "Access is denied" trap docs\WINDOWS-VERIFY.md
rem records for preview servers. Said here, where it can still be acted on.
tasklist /FI "IMAGENAME eq wudict.exe" 2>nul | findstr /i /c:"wudict.exe" >nul
if not errorlevel 1 (
    echo WARNING: wudict.exe is running. Close it, or the build will fail when
    echo   it tries to replace the file:
    echo     taskkill /F /IM wudict.exe
    echo.
)

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
  -ldflags "-s -w -X github.com/wuweidict/wudict/internal/cli.Version=%VERSION%" ^
  -o wudict.exe ^
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
echo Verifying wudict.exe
echo ============================================================
echo.

if not exist "wudict.exe" (
    echo ERROR: go build finished but wudict.exe was not found.
    exit /b 1
)

rem It has to START, not merely exist. A cgo build links the mingw runtime
rem dynamically unless it is told otherwise, so a missing libwinpthread-1.dll
rem or libgcc_s_seh-1.dll yields a build that reports success and a binary
rem that dies before main. --version is the cheapest run there is: no port,
rem no dictionary, no tray.
set "RAN="
for /f "delims=" %%V in ('wudict.exe --version') do if not defined RAN set "RAN=%%V"

if not defined RAN (
    echo ERROR: wudict.exe was built but does not run.
    if /i "%FLAVOUR%"=="cgo" (
        echo   Check that the mingw runtime DLLs ^(libwinpthread-1.dll,
        echo   libgcc_s_seh-1.dll^) are beside the exe or on PATH.
    )
    exit /b 1
)

echo OK: wudict.exe runs
echo   %RAN%
echo.

rem The build records its tags, its CGO_ENABLED and the whole module graph
rem inside the exe, so the flavour can be read back from the artifact instead
rem of trusted from the flags above. This is what catches a stale file left by
rem an earlier run or a build cache hit answering with the wrong tags.
set "WANT_DEP=modernc.org/sqlite"
if /i "%FLAVOUR%"=="cgo" set "WANT_DEP=github.com/mattn/go-sqlite3"

go version -m wudict.exe | findstr /i /c:"-tags=%GO_TAGS%" >nul
if errorlevel 1 (
    echo ERROR: wudict.exe was not built with -tags %GO_TAGS%.
    exit /b 1
)

go version -m wudict.exe | findstr /i /c:"CGO_ENABLED=%CGO%" >nul
if errorlevel 1 (
    echo ERROR: wudict.exe was not built with CGO_ENABLED=%CGO%.
    exit /b 1
)

go version -m wudict.exe | findstr /i /c:"%WANT_DEP%" >nul
if errorlevel 1 (
    echo ERROR: wudict.exe does not carry %WANT_DEP%.
    exit /b 1
)

echo OK: built as asked - -tags %GO_TAGS%, CGO_ENABLED=%CGO%, %WANT_DEP%
echo.

rem ------------------------------------------------------------
rem Installer (release or installer)
rem ------------------------------------------------------------

if defined MAKE_INSTALLER (
    call :make_installer
    if errorlevel 1 exit /b 1
)

echo.
echo ============================================================
echo BUILD SUCCESSFUL
echo ============================================================
echo.

echo Binary:
echo   %CD%\wudict.exe
echo.

if defined SETUP (
    echo Installer:
    echo   %SETUP%
    echo.
)

echo Run it:
echo   wudict.exe                    open the app from Explorer, browser from cmd
echo   wudict.exe --help             every command and flag
echo.

if /i "%FLAVOUR%"=="purego" (
    echo This build has no built-in .spx decoder: sound inside .mdd media needs
    echo   speexdec.exe on PATH, or rebuild with a C compiler.
    echo.
)

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
    echo   A plain `build-windows.cmd` still produces wudict.exe.
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
    echo ERROR: no Inno Setup 6.3 or newer was found, and `release` builds the
    echo   installer. Install it, then run this script again:
    echo     winget install JRSoftware.InnoSetup
    echo   A plain `build-windows.cmd` still produces wudict.exe.
    exit /b 1
)

echo Inno Setup:
echo   %ISCC%
echo.
exit /b 0

rem ============================================================
rem :make_installer - compiles the per-user installer
rem
rem Called with no arguments: wudict.exe and dist\ in the repository root are
rem the installer script's own defaults, which is the invocation
rem `make win-installer` and CI both use - one less thing that can drift. It
rem reads the product name and version out of the binary (`wudict.exe
rem --version`), so nothing about the identity is restated here.
rem ============================================================

:make_installer
echo ============================================================
echo Building the per-user installer
echo ============================================================
echo.

powershell -NoProfile -ExecutionPolicy Bypass -File tools\make-installer.ps1 %ISCC_ARG%

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
    echo   then either clear ISCC_PATH at the top of this file, so the installer
    echo   script finds the new compiler itself, or point ISCC_PATH at it.
    exit /b 1
)

set "SETUP="

for %%F in ("dist\wudict-windows-x64-setup-*.exe") do (
    if exist "%%~fF" set "SETUP=%%~fF"
)

if not defined SETUP (
    echo ERROR: iscc reported success but no setup executable was found:
    echo   dist\wudict-windows-x64-setup-*.exe
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
