# Copyright (C) 2026 DmShAl (Shepeta Dmitry)
# SPDX-License-Identifier: GPL-3.0-or-later

param(
    [Parameter(Mandatory = $true)][string]$Windres,
    [Parameter(Mandatory = $true)][string]$Version,
    [Parameter(Mandatory = $true)][string]$OriginalFilename
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
# ONE resource object carries BOTH the icons and the version information. The
# Go PE linker accepts a single .rsrc section (cmd/link/internal/ld/pe.go,
# setpersrc), so a second .syso of the same GOOS/GOARCH would fail the link
# with "too many .rsrc sections" - the two cannot be separate files.
$output = Join-Path $root 'dictshelf_windows_amd64.syso'
$iconsRC = Join-Path $root 'packaging/windows/dictshelf.rc'
if (-not (Test-Path -LiteralPath $iconsRC)) { throw "the Windows icon resource source is missing: $iconsRC" }
$iconsText = [IO.File]::ReadAllText($iconsRC)
$versionMatch = [regex]::Match($Version, '(?i)(?:dictshelf-)?v?(\d+)\.(\d+)\.(\d+)')
$numeric = @(0, 0, 0, 0)
if ($versionMatch.Success) {
    for ($i = 0; $i -lt 3; $i++) {
        $number = [int]$versionMatch.Groups[$i + 1].Value
        if ($number -gt 65535) { throw "Version component exceeds Windows VERSIONINFO limit: $number" }
        $numeric[$i] = $number
    }
    $suffixMatch = [regex]::Match($Version, '(?i)(?:beta|pre(?:-release)?)[.-]?(\d+)')
    if ($suffixMatch.Success) { $numeric[3] = [int]$suffixMatch.Groups[1].Value }
}
$numericVersion = $numeric -join ','
$escapedVersion = $Version.Replace('\', '\\').Replace('"', '\"')
$escapedFilename = $OriginalFilename.Replace('\', '\\').Replace('"', '\"')
$buildDate = [DateTime]::UtcNow.ToString('yyyy-MM-dd')
$rc = @"
// Version information appended to packaging/windows/dictshelf.rc by
// tools/make-windows-version-info.ps1; the two compile into one .rsrc.
1 VERSIONINFO
 FILEVERSION $numericVersion
 PRODUCTVERSION $numericVersion
 FILEFLAGSMASK 0x3fL
 FILEOS 0x40004L
 FILETYPE 0x1L
 FILESUBTYPE 0x0L
BEGIN
  BLOCK "StringFileInfo"
  BEGIN
    BLOCK "040904b0"
    BEGIN
      VALUE "CompanyName", "DMSH Labs\0"
      VALUE "FileDescription", "DictShelf dictionary manager\0"
      VALUE "FileVersion", "$escapedVersion\0"
      VALUE "InternalName", "DictShelf\0"
      VALUE "LegalCopyright", "Copyright (C) 2026 DmShAl (Shepeta Dmitry)\0"
      VALUE "OriginalFilename", "$escapedFilename\0"
      VALUE "ProductName", "DictShelf\0"
      VALUE "ProductVersion", "$escapedVersion\0"
      VALUE "BuildDate", "$buildDate\0"
    END
  END
  BLOCK "VarFileInfo"
  BEGIN
    VALUE "Translation", 0x409, 1200
  END
END
"@
$tempRC = Join-Path $root (".dictshelf-version-{0}.rc" -f [guid]::NewGuid())
try {
    # Written beside the icons rc, not in %TEMP%: windres resolves the ICON file
    # paths relative to its working directory, which is what dictshelf.rc assumes.
    [IO.File]::WriteAllText($tempRC, $iconsText + "`r`n" + $rc, [Text.Encoding]::ASCII)
    & $Windres -F pe-x86-64 -i $tempRC -o $output
    if ($LASTEXITCODE -ne 0) { throw "windres failed with exit code $LASTEXITCODE" }
} finally {
    Remove-Item -LiteralPath $tempRC -Force -ErrorAction SilentlyContinue
}
Write-Host "Windows resource (six icons + version $Version ($numericVersion), built $buildDate): $output"
