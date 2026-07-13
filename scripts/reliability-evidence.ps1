[CmdletBinding()]
param(
    [string]$OutputDirectory = "artifacts/reliability",
    [string]$SoakSource,
    [TimeSpan]$SoakDuration = ([TimeSpan]::FromHours(8)),
    [int]$SoakOverlaps = 3
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$output = Join-Path $root $OutputDirectory
New-Item -ItemType Directory -Force -Path $output | Out-Null

$started = [DateTimeOffset]::UtcNow
$steps = [System.Collections.Generic.List[object]]::new()

function Invoke-EvidenceStep {
    param([string]$Name, [string[]]$Arguments)

    $log = Join-Path $output ($Name + ".log")
    $stepStarted = [DateTimeOffset]::UtcNow
    & go @Arguments 2>&1 | Tee-Object -FilePath $log
    $code = $LASTEXITCODE
    $steps.Add([ordered]@{
        name = $Name
        command = "go " + ($Arguments -join " ")
        startedUtc = $stepStarted.ToString("O")
        durationMs = [int64]([DateTimeOffset]::UtcNow - $stepStarted).TotalMilliseconds
        exitCode = $code
        log = [IO.Path]::GetFileName($log)
    })
    if ($code -ne 0) {
        throw "Reliability evidence step '$Name' failed with exit code $code."
    }
}

$failure = $null
try {
    Push-Location $root
    Invoke-EvidenceStep "tests" @("test", "./...", "-count=1")
    Invoke-EvidenceStep "vet" @("vet", "./...")
    Invoke-EvidenceStep "build" @("build", "./...")

    if ($SoakSource) {
        $env:PENGUINCTRL_MEDIA_SOAK_SOURCE = (Resolve-Path $SoakSource).Path
        $env:PENGUINCTRL_MEDIA_SOAK_DURATION = $SoakDuration.ToString("c")
        $env:PENGUINCTRL_MEDIA_SOAK_OVERLAPS = $SoakOverlaps.ToString()
        $timeout = [Math]::Ceiling($SoakDuration.TotalHours + 1).ToString() + "h"
        Invoke-EvidenceStep "media-soak" @("test", "./media", "-run", "MultiHourSoak", "-count=1", "-timeout", $timeout)
    }
}
catch {
    $failure = $_.Exception.Message
}
finally {
    Pop-Location
}

$commit = (& git -C $root rev-parse HEAD).Trim()
$dirty = -not [string]::IsNullOrWhiteSpace((& git -C $root status --porcelain) -join "")
$evidence = [ordered]@{
    schemaVersion = 1
    result = $(if ($failure) { "failed" } else { "passed" })
    failure = $failure
    commit = $commit
    dirtyWorktree = $dirty
    startedUtc = $started.ToString("O")
    completedUtc = [DateTimeOffset]::UtcNow.ToString("O")
    machine = [ordered]@{
        os = [Environment]::OSVersion.VersionString
        architecture = [Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()
        name = [Environment]::MachineName
    }
    goVersion = (& go version)
    soak = [ordered]@{
        executed = [bool]$SoakSource
        sourceName = $(if ($SoakSource) { [IO.Path]::GetFileName($SoakSource) } else { $null })
        duration = $SoakDuration.ToString("c")
        overlaps = $SoakOverlaps
    }
    steps = $steps
}
$evidence | ConvertTo-Json -Depth 8 | Set-Content -Encoding utf8 (Join-Path $output "evidence.json")

if ($failure) {
    throw $failure
}
