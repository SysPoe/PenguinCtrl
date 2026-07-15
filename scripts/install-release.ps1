[CmdletBinding()]
param(
    [string]$InstallDirectory = (Join-Path $env:LOCALAPPDATA "Programs\CuSus"),
    [switch]$Rollback
)

$ErrorActionPreference = "Stop"
$install = [IO.Path]::GetFullPath($InstallDirectory)
$previous = $install + ".previous"

if ($Rollback) {
    if (-not (Test-Path -LiteralPath $previous -PathType Container)) {
        throw "No previous CuSus installation is available at $previous"
    }
    $failed = $install + ".failed-" + [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
    if (Test-Path -LiteralPath $install) {
        Move-Item -LiteralPath $install -Destination $failed
    }
    Move-Item -LiteralPath $previous -Destination $install
    Write-Host "Rolled back CuSus to $install"
    return
}

$source = [IO.Path]::GetFullPath($PSScriptRoot)
if (-not (Test-Path -LiteralPath (Join-Path $source "cusus.exe"))) {
    throw "cusus.exe is missing from release package $source"
}
$incoming = $install + ".incoming-" + [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
New-Item -ItemType Directory -Force -Path $incoming | Out-Null
$published = $false
try {
    Copy-Item -Path (Join-Path $source "*") -Destination $incoming -Recurse -Force

    if (Test-Path -LiteralPath $previous) {
        Remove-Item -LiteralPath $previous -Recurse -Force
    }
    if (Test-Path -LiteralPath $install) {
        Move-Item -LiteralPath $install -Destination $previous
    }
    Move-Item -LiteralPath $incoming -Destination $install
    $published = $true
}
finally {
    if (-not $published -and (Test-Path -LiteralPath $incoming)) {
        Remove-Item -LiteralPath $incoming -Recurse -Force
    }
}
Write-Host "Installed CuSus to $install. Previous version: $previous"
