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
rem DictShelf Android FOSS build: [debug|release|all] [intel|ABI] [--channel release|pre-release|beta]
rem
rem   intel  also ship the x86_64 Go/JNI bridge, so the same APK runs on the
rem          x86_64 emulator as well as on arm64 hardware. Debug only:
rem          release APKs default to arm64 (D52).
rem   all    build arm64-v8a, armeabi-v7a and universal release APKs.
rem ============================================================

cd /d "%~dp0"

rem Optional private settings; kept out of Git. Do not echo signing passwords.
if exist "%~dp0build-android.local.bat" (
    call "%~dp0build-android.local.bat"
    if errorlevel 1 (
        echo ERROR: Could not load build-android.local.bat.
        exit /b 1
    )
)

set "BUILD_TYPE=%~1"
set "ABI_TOKEN=%~2"
set "CHANNEL=release"
set "CHANNEL_SPECIFIED="
set "CHANNEL_ARGS="
if /i "%~1"=="--channel" (
    set "BUILD_TYPE=debug"
    set "ABI_TOKEN="
    set "CHANNEL=%~2"
    set "CHANNEL_SPECIFIED=1"
) else if /i "%~2"=="--channel" (
    set "ABI_TOKEN="
    set "CHANNEL=%~3"
    set "CHANNEL_SPECIFIED=1"
) else if /i "%~3"=="--channel" (
    set "CHANNEL=%~4"
    set "CHANNEL_SPECIFIED=1"
)
if defined CHANNEL_SPECIFIED if /i not "%CHANNEL%"=="release" if /i not "%CHANNEL%"=="pre-release" if /i not "%CHANNEL%"=="beta" (
    echo ERROR: unknown update channel "%CHANNEL%".
    echo Usage: build-android.cmd [debug^|release^|all] [intel^|ABI] [--channel release^|pre-release^|beta]
    exit /b 1
)
if defined CHANNEL_SPECIFIED set "CHANNEL_ARGS=--channel %CHANNEL%"

if /i "%BUILD_TYPE%"=="all" (
    set "ALL_APK_DIR=android\app\build\outputs\apk\all"
    if not exist "android\app\build\outputs\apk\all" mkdir "android\app\build\outputs\apk\all"
    if errorlevel 1 exit /b 1
    for %%F in (
        "android\app\build\outputs\apk\all\dictshelf-android-arm64-v8a-foss.apk"
        "android\app\build\outputs\apk\all\dictshelf-android-arm64-v8a-foss-unsigned.apk"
        "android\app\build\outputs\apk\all\dictshelf-android-armeabi-v7a-foss.apk"
        "android\app\build\outputs\apk\all\dictshelf-android-armeabi-v7a-foss-unsigned.apk"
        "android\app\build\outputs\apk\all\dictshelf-android-universal-foss.apk"
        "android\app\build\outputs\apk\all\dictshelf-android-universal-foss-unsigned.apk"
    ) do if exist "%%~fF" del /q "%%~fF"
    call "%~f0" release arm64-v8a %CHANNEL_ARGS%
    if errorlevel 1 exit /b 1
    call :stage_all_apk arm64-v8a
    if errorlevel 1 exit /b 1
    call "%~f0" release armeabi-v7a %CHANNEL_ARGS%
    if errorlevel 1 exit /b 1
    call :stage_all_apk armeabi-v7a
    if errorlevel 1 exit /b 1
    call "%~f0" release universal %CHANNEL_ARGS%
    if errorlevel 1 exit /b 1
    call :stage_all_apk universal
    if errorlevel 1 exit /b 1
    echo.
    echo All three APKs are saved under:
    echo   %CD%\android\app\build\outputs\apk\all
    exit /b 0
)

rem The abi token is accepted in either position: `debug intel` and
rem `intel debug` mean the same thing, and a bare `intel` means a debug build.
if /i "%BUILD_TYPE%"=="intel" (
    set "ABI_TOKEN=intel"
    set "BUILD_TYPE=%~2"
)

if not defined BUILD_TYPE set "BUILD_TYPE=debug"
if /i "%BUILD_TYPE%"=="debug" (
    set "BUILD_TYPE=debug"
    set "GRADLE_TASK=assembleFossDebug"
    set "APK_SUFFIX=-debug"
) else if /i "%BUILD_TYPE%"=="release" (
    set "BUILD_TYPE=release"
    set "GRADLE_TASK=assembleFossRelease"
    set "APK_SUFFIX=-unsigned"
    if defined KEYSTORE (
        set "APK_SUFFIX="
    ) else (
        echo No KEYSTORE configured: building an unsigned release APK.
    )
) else (
    echo ERROR: unknown build type "%BUILD_TYPE%".
    echo Usage: build-android.cmd [debug^|release^|all] [intel^|arm64-v8a^|armeabi-v7a^|universal] [--channel release^|pre-release^|beta]
    exit /b 1
)

if /i "%ABI_TOKEN%"=="x86_64" set "ABI_TOKEN=intel"

set "EMU_X86="
set "ANDROID_ABIS=arm64-v8a"
set "APK_ABI=arm64-v8a"
if defined ABI_TOKEN (
    if /i "%ABI_TOKEN%"=="arm64-v8a" (
        set "ANDROID_ABIS=arm64-v8a"
        set "APK_ABI=arm64-v8a"
    ) else if /i "%ABI_TOKEN%"=="armeabi-v7a" (
        set "ANDROID_ABIS=armeabi-v7a"
        set "APK_ABI=armeabi-v7a"
    ) else if /i "%ABI_TOKEN%"=="universal" (
        set "ANDROID_ABIS=arm64-v8a,armeabi-v7a"
        set "APK_ABI=universal"
    ) else if /i "%ABI_TOKEN%"=="intel" (
        set "EMU_X86=1"
    ) else (
        echo ERROR: unknown option "%ABI_TOKEN%".
        echo Usage: build-android.cmd [debug^|release^|all] [intel^|arm64-v8a^|armeabi-v7a^|universal] [--channel release^|pre-release^|beta]
        exit /b 1
    )
    if defined EMU_X86 (
    if /i "%BUILD_TYPE%"=="release" (
        echo ERROR: intel is debug-only. The x86_64 emulator ABI must not reach a
        echo   release APK - build one with: build-android.cmd release
        exit /b 1
    )
    )
)

rem The ABI token is spelled the way Android spells it, so the APK's name and
rem the lib/<abi>/ path inside it say the same thing. build.gradle composes the
rem same string; this file only predicts the path to check the .so is in there.
if defined EMU_X86 set "APK_ABI=arm64-v8a-x86_64"
if defined EMU_X86 set "ANDROID_ABIS=arm64-v8a,x86_64"

echo.
echo ============================================================
echo DictShelf Android %APK_ABI% build
echo ============================================================
echo.

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
    echo ERROR: git.exe was not found in PATH.
    exit /b 1
)

where java >nul 2>&1
if errorlevel 1 (
    echo ERROR: java.exe was not found in PATH.
    exit /b 1
)

where adb >nul 2>&1
if errorlevel 1 (
    echo WARNING: adb.exe was not found in PATH.
    echo APK will still be built, but install command will not work.
    echo.
)

rem ------------------------------------------------------------
rem Android SDK
rem ------------------------------------------------------------

if not defined ANDROID_HOME (
    set "ANDROID_HOME=%LOCALAPPDATA%\Android\Sdk"
)

if not exist "%ANDROID_HOME%" (
    echo ERROR: Android SDK directory does not exist:
    echo   %ANDROID_HOME%
    exit /b 1
)

echo Android SDK:
echo   %ANDROID_HOME%
echo.

rem ------------------------------------------------------------
rem Android NDK
rem ------------------------------------------------------------

set "NDK=%ANDROID_HOME%\ndk\30.0.16248370"

if not exist "%NDK%" (
    echo ERROR: Android NDK directory does not exist:
    echo   %NDK%
    exit /b 1
)

echo Android NDK:
echo   %NDK%
echo.

rem ------------------------------------------------------------
rem NDK LLVM toolchain
rem ------------------------------------------------------------

set "ANDROID_API=26"
set "NDK_BIN=%NDK%\toolchains\llvm\prebuilt\windows-x86_64\bin"

if not exist "%NDK_BIN%\clang.exe" (
    echo ERROR: clang.exe was not found:
    echo   %NDK_BIN%\clang.exe
    exit /b 1
)

if not exist "%NDK_BIN%\clang++.exe" (
    echo ERROR: clang++.exe was not found:
    echo   %NDK_BIN%\clang++.exe
    exit /b 1
)

echo NDK toolchain:
echo   %NDK_BIN%
echo.

rem ------------------------------------------------------------
rem DictShelf version
rem Same idea as Makefile:
rem   git describe --tags --always --dirty
rem ------------------------------------------------------------

set "VERSION="

for /f "delims=" %%V in ('git describe --tags --always --dirty 2^>nul') do (
    set "VERSION=%%V"
)

if not defined VERSION (
    set "VERSION=dev"
)

rem Set DICTSHELF_VERSION to build the same named release as CI, for example:
rem   set DICTSHELF_VERSION=dictshelf-v1.02.01
rem Without an override, use the version derived from this checkout's Git history.
if defined DICTSHELF_VERSION set "VERSION=%DICTSHELF_VERSION%"

rem An explicitly selected channel stamps a classifier into preview builds.
rem GitHub still decides whether a published release is marked Pre-release.
if defined CHANNEL_SPECIFIED (
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
)

rem Normally Gradle derives versionCode from Git history. Override it only for
rem testing an update from an older installed build to a newer release APK.
set "VERSION_CODE_OVERRIDE="
if defined DICTSHELF_VERSION_CODE (
    set "VERSION_CODE_OVERRIDE=%DICTSHELF_VERSION_CODE%"
    echo(%DICTSHELF_VERSION_CODE%| findstr /r /x "[1-9][0-9]*" >nul
    if errorlevel 1 (
        echo ERROR: DICTSHELF_VERSION_CODE must be a positive integer.
        exit /b 1
    )
)

call :print_version
echo Update channel:
echo   %CHANNEL%
if defined VERSION_CODE_OVERRIDE echo Android versionCode override: %VERSION_CODE_OVERRIDE%
echo.

rem ------------------------------------------------------------
rem Output path for native Android binary
rem ------------------------------------------------------------

set "JNI_DIR=android\app\src\main\jniLibs\arm64-v8a"
set "ANDROID_BRIDGE=%JNI_DIR%\libwudictbridge.so"
set "JNI_DIR_ARMV7=android\app\src\main\jniLibs\armeabi-v7a"
set "ANDROID_BRIDGE_ARMV7=%JNI_DIR_ARMV7%\libwudictbridge.so"

rem The emulator ABI goes to a source dir that is a default for NO source set.
rem build.gradle looks at it only under -PemuX86=1, so however long this file
rem sits here it can never reach a release APK.
set "EMU_JNI_DIR=android\app\src\emuX86\jniLibs\x86_64"
set "ANDROID_BRIDGE_X86=%EMU_JNI_DIR%\libwudictbridge.so"

if not exist "%JNI_DIR%" (
    mkdir "%JNI_DIR%"
)

if errorlevel 1 (
    echo ERROR: Could not create:
    echo   %JNI_DIR%
    exit /b 1
)

if defined EMU_X86 (
    if not exist "%EMU_JNI_DIR%" mkdir "%EMU_JNI_DIR%"
    if errorlevel 1 (
        echo ERROR: Could not create:
        echo   %EMU_JNI_DIR%
        exit /b 1
    )
)

rem ------------------------------------------------------------
rem Build native Go/JNI bridge
rem
rem Equivalent to Makefile android-go:
rem
rem GOFLAGS:
rem   -tags sqlite_fts5 -trimpath
rem
rem LDFLAGS:
rem   -s -w
rem   -extldflags '-Wl,-z,max-page-size=16384'
rem The app version is passed to Gradle below and reaches Go/JNI through
rem BuildConfig.VERSION_NAME when the native runtime starts.
rem ------------------------------------------------------------

if /i "%ANDROID_ABIS%"=="arm64-v8a" (
    call :build_bridge arm64 aarch64 "%ANDROID_BRIDGE%"
    if errorlevel 1 exit /b 1
)
if /i "%ANDROID_ABIS%"=="armeabi-v7a" (
    call :build_bridge arm armv7a "%ANDROID_BRIDGE_ARMV7%"
    if errorlevel 1 exit /b 1
)
if /i "%ANDROID_ABIS%"=="arm64-v8a,armeabi-v7a" (
    call :build_bridge arm64 aarch64 "%ANDROID_BRIDGE%"
    if errorlevel 1 exit /b 1
    call :build_bridge arm armv7a "%ANDROID_BRIDGE_ARMV7%"
    if errorlevel 1 exit /b 1
)

if defined EMU_X86 (
    call :build_bridge amd64 x86_64 "%ANDROID_BRIDGE_X86%"
    if errorlevel 1 exit /b 1
)

rem ------------------------------------------------------------
rem Build Android APK
rem ------------------------------------------------------------

echo ============================================================
echo Building DictShelf FOSS %BUILD_TYPE% APK
echo ============================================================
echo.

rem -PemuX86=1 is what makes build.gradle look at src/emuX86/jniLibs; without
rem it that directory is invisible to every variant.
set "GRADLE_EXTRA="
set "GRADLE_EXTRA=-PversionName=%VERSION%"
if defined VERSION_CODE_OVERRIDE set "GRADLE_EXTRA=%GRADLE_EXTRA% -PversionCode=%VERSION_CODE_OVERRIDE%"
set "GRADLE_EXTRA=%GRADLE_EXTRA% -PandroidAbis=%ANDROID_ABIS% -PapkAbiName=%APK_ABI%"
if defined EMU_X86 set "GRADLE_EXTRA=%GRADLE_EXTRA% -PemuX86=1"

pushd android

call gradlew.bat %GRADLE_TASK% %GRADLE_EXTRA%

if errorlevel 1 (
    popd
    echo.
    echo ============================================================
    echo ERROR: Gradle build failed.
    echo ============================================================
    exit /b 1
)

popd

rem ------------------------------------------------------------
rem Expected APK
rem ------------------------------------------------------------

set "APK=android\app\build\outputs\apk\foss\%BUILD_TYPE%\dictshelf-android-%APK_ABI%-foss%APK_SUFFIX%.apk"

if not exist "%APK%" (
    echo ERROR: Gradle finished but APK was not found:
    echo   %APK%
    exit /b 1
)

rem ------------------------------------------------------------
rem Verify the in-process Go library inside APK
rem ------------------------------------------------------------

echo.
echo ============================================================
echo Verifying APK
echo ============================================================
echo.

where jar >nul 2>&1

if errorlevel 1 (
    echo WARNING: jar.exe was not found in PATH.
    echo APK content verification skipped.
) else (
    if /i "%ANDROID_ABIS%"=="arm64-v8a" (
        call :verify_abi arm64-v8a
        if errorlevel 1 exit /b 1
    )
    if /i "%ANDROID_ABIS%"=="armeabi-v7a" (
        call :verify_abi armeabi-v7a
        if errorlevel 1 exit /b 1
    )
    if /i "%ANDROID_ABIS%"=="arm64-v8a,armeabi-v7a" (
        call :verify_abi arm64-v8a
        if errorlevel 1 exit /b 1
        call :verify_abi armeabi-v7a
        if errorlevel 1 exit /b 1
    )

    if defined EMU_X86 (
        jar tf "%APK%" | findstr /x /c:"lib/x86_64/libwudictbridge.so" >nul
        if errorlevel 1 (
            echo ERROR: APK does not contain lib/x86_64/libwudictbridge.so
            exit /b 1
        )
        echo OK: APK contains lib/x86_64/libwudictbridge.so
    )
)

echo.
echo ============================================================
echo BUILD SUCCESSFUL
echo ============================================================
echo.
call :print_version

echo APK:
echo   %CD%\%APK%
echo.

if /i "%APK_SUFFIX%"=="-unsigned" (
    echo Sign this release APK with your release key before installing.
) else (
    echo Install:
    echo   adb install -r "%APK%"
)

if defined EMU_X86 (
    echo.
    echo This APK carries arm64-v8a AND x86_64, so it installs on the phone and on
    echo   the x86_64 emulator alike. It is a debug build, which is also what lets
    echo   chrome://inspect attach to its WebView.
)
echo.

endlocal
exit /b 0

:print_version
set "DISPLAY_VERSION=dictshelf-%VERSION%"
if /i "%VERSION:~0,1%"=="v" set "DISPLAY_VERSION=dictshelf-%VERSION%"
for /f "tokens=2 delims=v" %%V in ("%VERSION%") do set "DISPLAY_VERSION=dictshelf-v%%V"
echo DictShelf version:
echo   %DISPLAY_VERSION%
echo.
exit /b 0

:build_bridge
set "GOOS=android"
set "GOARCH=%~1"
set "CGO_ENABLED=1"
set "ANDROID_TRIPLE=%~2-linux-android%ANDROID_API%"
if /i "%~1"=="arm" set "ANDROID_TRIPLE=armv7a-linux-androideabi%ANDROID_API%"
set "CC=%NDK_BIN%\clang.exe --target=%ANDROID_TRIPLE%"
set "CXX=%NDK_BIN%\clang++.exe --target=%ANDROID_TRIPLE%"
if /i "%~1"=="arm" (set "GOARM=7") else (set "GOARM=")

echo Building libwudictbridge.so for android/%~1
go build -buildmode=c-shared -tags sqlite_fts5 -trimpath ^
  -ldflags "-s -w -extldflags=-Wl,-z,max-page-size=16384" ^
  -o "%~3" ./android/nativebridge
if errorlevel 1 exit /b 1
if not exist "%~3" exit /b 1
exit /b 0

:verify_abi
jar tf "%APK%" | findstr /x /c:"lib/%~1/libwudictbridge.so" >nul
if errorlevel 1 (
    echo ERROR: APK does not contain lib/%~1/libwudictbridge.so
    exit /b 1
)
echo OK: APK contains lib/%~1/libwudictbridge.so
exit /b 0

:stage_all_apk
set "STAGE_FOUND="
for %%F in ("android\app\build\outputs\apk\foss\release\dictshelf-android-%~1-foss*.apk") do (
    if exist "%%~fF" (
        copy /y "%%~fF" "%ALL_APK_DIR%\%%~nxF" >nul
        if errorlevel 1 exit /b 1
        set "STAGE_FOUND=1"
    )
)
if not defined STAGE_FOUND (
    echo ERROR: Could not stage the %~1 release APK.
    exit /b 1
)
exit /b 0

:show_help
echo DictShelf Android FOSS build
echo.
echo Usage:
echo   build-android.cmd [debug^|release^|all] [intel^|arm64-v8a^|armeabi-v7a^|universal] [--channel release^|pre-release^|beta]
echo.
echo Help keys: -?, --?, -h, --h, -help, --help
echo.
echo Build types:
echo   debug       Debug APK; default build type. Use intel to include x86_64 for emulators.
echo   release     Release APK; defaults to arm64-v8a.
echo   all         Build arm64-v8a, armeabi-v7a and universal release APKs.
echo.
echo ABI values:
echo   arm64-v8a, armeabi-v7a, universal; intel is an alias for an x86_64 debug build.
echo.
echo Update channels:
echo   --channel release^|pre-release^|beta
echo.
echo Set DICTSHELF_VERSION before running to choose a version, for example:
echo   set "DICTSHELF_VERSION=dictshelf-v1.02.01"
echo For update-flow tests, optionally set DICTSHELF_VERSION_CODE below the downloaded APK's code:
echo   set "DICTSHELF_VERSION_CODE=1112"
echo   build-android.cmd release arm64-v8a
echo.
echo Without DICTSHELF_VERSION, the version is read from Git.
endlocal
exit /b 0
