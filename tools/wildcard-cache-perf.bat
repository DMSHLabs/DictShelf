@echo off
rem Copyright (C) 2026 DmShAl (Shepeta Dmitry)
rem SPDX-License-Identifier: GPL-3.0-or-later
setlocal EnableExtensions DisableDelayedExpansion

rem ============================================================
rem Real-dictionary Wildcards cache performance test.
rem Edit the settings below, then run this file from cmd or Explorer.
rem Result caches and the prepared library are isolated in temporary folders.
rem ============================================================

rem Root folder containing the dictionaries to test.
set "DICT_ROOT=D:\Dictionaries\wuDict\Dictionaries\Big"

rem One or more case-insensitive path fragments, separated by semicolons.
set "DICT_SELECTORS=Longman Dictionary of Contemporary English 6.mdx;Oxford Advanced Learner's Dictionary 10th.mdx;wiktionary_en_all_nopic_2026-08.zim"

rem Upper Wildcards mask and lower filter mask.
set "TOP_MASK=*d*g*"
set "LOWER_MASK=**home**"
set "LOWER_FTS_QUERY=h"

rem 1 enables FTS5 and trigram result indexes; 0 disables them.
set "CACHE_INDEXES=0"

rem Leave blank to inherit CACHE_INDEXES, or set each index to 0 or 1.
set "CACHE_FTS="
set "CACHE_TRIGRAM="

rem 1 measures only upper-cache creation; useful for very broad masks.
set "UPPER_ONLY=0"

rem 1 stops after both lower searches, without building the Combined cache.
set "SKIP_COMBINE=0"

rem Go test settings. Keep COUNT at 1 for a normal timing run.
set "GO_TEST_TAGS=sqlite_fts5"
set "GO_TEST_COUNT=1"
set "VERBOSE=1"

if not exist "%DICT_ROOT%\" (
  echo Dictionary folder not found: "%DICT_ROOT%"
  exit /b 2
)
if "%DICT_SELECTORS%"=="" (
  echo Set DICT_SELECTORS to at least one dictionary filename fragment.
  exit /b 2
)
if not "%CACHE_INDEXES%"=="0" if not "%CACHE_INDEXES%"=="1" (
  echo CACHE_INDEXES must be 0 or 1.
  exit /b 2
)
if "%CACHE_FTS%"=="" set "CACHE_FTS=%CACHE_INDEXES%"
if "%CACHE_TRIGRAM%"=="" set "CACHE_TRIGRAM=%CACHE_INDEXES%"
if not "%CACHE_FTS%"=="0" if not "%CACHE_FTS%"=="1" (
  echo CACHE_FTS must be 0 or 1.
  exit /b 2
)
if not "%CACHE_TRIGRAM%"=="0" if not "%CACHE_TRIGRAM%"=="1" (
  echo CACHE_TRIGRAM must be 0 or 1.
  exit /b 2
)

set "WUDICT_WILDCARD_BENCH=1"
set "WUDICT_WILDCARD_BENCH_DIR=%DICT_ROOT%"
set "WUDICT_WILDCARD_BENCH_DICTS=%DICT_SELECTORS%"
set "WUDICT_WILDCARD_BENCH_TOP=%TOP_MASK%"
set "WUDICT_WILDCARD_BENCH_WITHIN=%LOWER_MASK%"
set "WUDICT_WILDCARD_BENCH_INDEXES=%CACHE_INDEXES%"
set "WUDICT_WILDCARD_BENCH_FTS=%CACHE_FTS%"
set "WUDICT_WILDCARD_BENCH_TRIGRAM=%CACHE_TRIGRAM%"
set "WUDICT_WILDCARD_BENCH_UPPER_ONLY=%UPPER_ONLY%"
set "WUDICT_WILDCARD_BENCH_WITHIN_FTS=%LOWER_FTS_QUERY%"
set "WUDICT_WILDCARD_BENCH_SKIP_COMBINE=%SKIP_COMBINE%"

pushd "%~dp0.."
if errorlevel 1 (
  echo Could not enter the repository root.
  exit /b 2
)

echo Wildcards cache performance test
echo   Dictionaries: %DICT_SELECTORS%
echo   Upper mask:   %TOP_MASK%
echo   Lower mask:   %LOWER_MASK%
echo   Lower FTS:    %LOWER_FTS_QUERY% (prefix)
echo   FTS:          %CACHE_FTS%
echo   Trigram:      %CACHE_TRIGRAM%
echo   Upper only:   %UPPER_ONLY%
echo   Skip combine: %SKIP_COMBINE%
echo.

if "%VERBOSE%"=="0" (
  go test -tags "%GO_TEST_TAGS%" ./internal/server -run "^TestWildcardCachePerformance$" -count=%GO_TEST_COUNT%
) else (
  go test -tags "%GO_TEST_TAGS%" ./internal/server -run "^TestWildcardCachePerformance$" -count=%GO_TEST_COUNT% -v
)
set "RESULT=%ERRORLEVEL%"

popd
exit /b %RESULT%
