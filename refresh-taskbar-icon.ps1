# Copyright (C) 2026 DmShAl (Shepeta Dmitry)
# SPDX-License-Identifier: GPL-3.0-or-later
param(
    [Parameter(Position = 0)]
    [string] $IconName = 'icon2_transparent_light'
)

$ErrorActionPreference = 'Stop'
$validIcons = @(
    'icon1', 'icon1_transparent', 'icon1_transparent_light',
    'icon2', 'icon2_transparent', 'icon2_transparent_light'
)
if ($IconName -notin $validIcons) {
    Write-Error "Unknown icon '$IconName'. Choose one of: $($validIcons -join ', ')"
    exit 2
}

$sourceIcon = Join-Path $PSScriptRoot "packaging\windows\icons\$IconName.ico"
if (-not (Test-Path -LiteralPath $sourceIcon -PathType Leaf)) {
    Write-Error "Icon file not found: $sourceIcon"
    exit 2
}

$pinnedDir = Join-Path $env:APPDATA 'Microsoft\Internet Explorer\Quick Launch\User Pinned\TaskBar'
if (-not (Test-Path -LiteralPath $pinnedDir -PathType Container)) {
    Write-Error "Pinned taskbar shortcuts folder not found: $pinnedDir"
    exit 1
}

$shell = New-Object -ComObject WScript.Shell
$matchingLinks = @(
    Get-ChildItem -LiteralPath $pinnedDir -Filter '*.lnk' -File | Where-Object {
        $shortcut = $shell.CreateShortcut($_.FullName)
        [IO.Path]::GetFileName($shortcut.TargetPath) -ieq 'DictShelf.exe'
    }
)
if ($matchingLinks.Count -eq 0) {
    Write-Error 'No pinned DictShelf.exe shortcut was found.'
    exit 1
}

$targetExe = $shell.CreateShortcut($matchingLinks[0].FullName).TargetPath
$portableData = Join-Path (Split-Path -Parent $targetExe) 'Data'
if ((Test-Path -LiteralPath (Join-Path $portableData 'dictshelf.toml') -PathType Leaf) -and
    (Test-Path -LiteralPath (Join-Path $portableData 'Dictionaries') -PathType Container)) {
    $dataRoot = $portableData
} else {
    $dataRoot = Join-Path $env:USERPROFILE '.dmshlabs\dictshelf'
}

$darkName = $IconName
$lightName = $IconName
if ($IconName -match '^(icon[12])_transparent_light$') {
    $family = $Matches[1]
    $darkName = "${family}_transparent"
} elseif ($IconName -match '^(icon[12])_transparent$') {
    $family = $Matches[1]
    $lightName = "${family}_transparent_light"
}

$iconsDir = Join-Path $dataRoot 'icons'
New-Item -ItemType Directory -Path $iconsDir -Force | Out-Null
$darkPath = Join-Path $iconsDir 'taskbar-dark.ico'
$lightPath = Join-Path $iconsDir 'taskbar-light.ico'
Copy-Item -LiteralPath (Join-Path $PSScriptRoot "packaging\windows\icons\$darkName.ico") -Destination $darkPath -Force
Copy-Item -LiteralPath (Join-Path $PSScriptRoot "packaging\windows\icons\$lightName.ico") -Destination $lightPath -Force
if ($IconName -match '_transparent_light$') {
    $iconPath = $lightPath
} else {
    $iconPath = $darkPath
}

Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public static class DictShelfShellNotify {
    [DllImport("shell32.dll", CharSet = CharSet.Unicode)]
    public static extern void SHChangeNotify(uint eventId, uint flags, string path, IntPtr secondPath);
}
'@

foreach ($link in $matchingLinks) {
    $shortcut = $shell.CreateShortcut($link.FullName)
    $shortcut.IconLocation = "$iconPath,0"
    $shortcut.Save()
    [DictShelfShellNotify]::SHChangeNotify(0x00002000, 0x00001005, $link.FullName, [IntPtr]::Zero)
    Write-Host "Updated $($link.Name)"
}
[DictShelfShellNotify]::SHChangeNotify(0x08000000, 0, $null, [IntPtr]::Zero)
Get-ChildItem -LiteralPath $iconsDir -Filter 'taskbar-*.ico' -File |
    Where-Object { $_.Name -notin @('taskbar-dark.ico', 'taskbar-light.ico') } |
    ForEach-Object { Remove-Item -LiteralPath $_.FullName -Force }
Write-Host "Taskbar icon: $iconPath"
