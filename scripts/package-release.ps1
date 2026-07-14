[CmdletBinding()]
param(
    [Parameter(Mandatory)][ValidatePattern('^v?\d+\.\d+\.\d+([-.][0-9A-Za-z.]+)?$')][string]$Version,
    [string]$OutputDirectory = "artifacts/release",
    [string]$CertificatePath,
    [string]$CertificatePassword = $env:CUSUS_SIGNING_PASSWORD,
    [string]$ReliabilityEvidence,
    [switch]$RequireSigning,
    [switch]$AllowDirty
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$output = [IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))
$version = $Version.TrimStart('v')
$commit = (& git -C $root rev-parse HEAD).Trim()
$commitTime = [DateTimeOffset]::FromUnixTimeSeconds([int64](& git -C $root show -s --format=%ct HEAD))
if (-not $AllowDirty -and -not [string]::IsNullOrWhiteSpace(((& git -C $root status --porcelain --untracked-files=no) -join ""))) {
    throw "Release builds require a clean worktree."
}
if ($RequireSigning -and -not $CertificatePath) {
    throw "Official release packaging requires -CertificatePath."
}
if ($ReliabilityEvidence) {
    $evidence = Get-Content -Raw $ReliabilityEvidence | ConvertFrom-Json
    if ($evidence.result -ne "passed" -or $evidence.commit -ne $commit) {
        throw "Reliability evidence must be passing and match commit $commit."
    }
}

New-Item -ItemType Directory -Force -Path $output | Out-Null
$packageName = "CuSus-v$version-windows-amd64"
$stage = Join-Path $output $packageName
if (Test-Path -LiteralPath $stage) { Remove-Item -LiteralPath $stage -Recurse -Force }
New-Item -ItemType Directory -Force -Path $stage | Out-Null

$windresCommand = Get-Command windres.exe -ErrorAction SilentlyContinue
if (-not $windresCommand) { throw "windres.exe is required to embed the pinned Windows manifests and resources." }
$mainResource = Join-Path $root "resource_windows_amd64.syso"
$supervisorResource = Join-Path $root "cmd/cusus-supervisor/resource_windows_amd64.syso"
# TODO(macro): Generate both resource/version inputs from $version so Explorer
# metadata, assembly identity, buildinfo, and release-manifest.json cannot drift.
Push-Location (Join-Path $root "build/windows")
try {
    & $windresCommand.Source --input-format=rc --output-format=coff --target=pe-x86-64 cusus.rc $mainResource
    if ($LASTEXITCODE -ne 0) { throw "CuSus Windows resource compilation failed." }
    & $windresCommand.Source --input-format=rc --output-format=coff --target=pe-x86-64 supervisor.rc $supervisorResource
    if ($LASTEXITCODE -ne 0) { throw "Supervisor Windows resource compilation failed." }
}
finally { Pop-Location }

$buildTime = $commitTime.UtcDateTime.ToString("yyyy-MM-ddTHH:mm:ssZ")
$ldflags = "-s -w -buildid= -X github.com/syspoe/cusus/internal/buildinfo.Version=v$version -X github.com/syspoe/cusus/internal/buildinfo.Commit=$commit -X github.com/syspoe/cusus/internal/buildinfo.BuildTime=$buildTime"
Push-Location $root
try {
    $env:SOURCE_DATE_EPOCH = $commitTime.ToUnixTimeSeconds().ToString()
    & go build -trimpath -buildvcs=false -ldflags $ldflags -o (Join-Path $stage "cusus.exe") .
    if ($LASTEXITCODE -ne 0) { throw "CuSus build failed." }
    & go build -trimpath -buildvcs=false -ldflags $ldflags -o (Join-Path $stage "cusus-supervisor.exe") ./cmd/cusus-supervisor
    if ($LASTEXITCODE -ne 0) { throw "Supervisor build failed." }
}
finally {
    Pop-Location
    Remove-Item -LiteralPath $mainResource, $supervisorResource -Force -ErrorAction SilentlyContinue
}

Copy-Item (Join-Path $root "scripts/install-release.ps1") (Join-Path $stage "install.ps1")
Copy-Item (Join-Path $root "docs/release-notes.md") (Join-Path $stage "RELEASE-NOTES.md")

$modules = & go -C $root list -m -f '{{if not .Main}}{{.Path}}|{{.Version}}|{{.Sum}}{{end}}' all
$packages = foreach ($module in ($modules | Where-Object { $_ })) {
    $parts = $module -split '\|', 3
    [ordered]@{
        SPDXID = "SPDXRef-Package-" + ([Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($parts[0]))).Substring(0, 16))
        name = $parts[0]
        versionInfo = $parts[1]
        downloadLocation = "NOASSERTION"
        filesAnalyzed = $false
        externalRefs = @([ordered]@{ referenceCategory = "PACKAGE-MANAGER"; referenceType = "purl"; referenceLocator = "pkg:golang/$($parts[0])@$($parts[1])" })
    }
}
$sbom = [ordered]@{
    spdxVersion = "SPDX-2.3"
    dataLicense = "CC0-1.0"
    SPDXID = "SPDXRef-DOCUMENT"
    name = $packageName
    documentNamespace = "https://cusus.invalid/spdx/$commit"
    creationInfo = [ordered]@{ created = $buildTime; creators = @("Tool: CuSus-package-release") }
    packages = @($packages)
}
$sbom | ConvertTo-Json -Depth 8 | Set-Content -Encoding utf8 (Join-Path $stage "SBOM.spdx.json")

if ($CertificatePath) {
    $signToolCommand = Get-Command signtool.exe -ErrorAction SilentlyContinue
    $signTool = if ($signToolCommand) { $signToolCommand.Source } else {
        Get-ChildItem "${env:ProgramFiles(x86)}\Windows Kits\10\bin" -Filter signtool.exe -Recurse -ErrorAction SilentlyContinue |
            Where-Object { $_.FullName -match '\\x64\\signtool\.exe$' } |
            Sort-Object FullName -Descending |
            Select-Object -First 1 -ExpandProperty FullName
    }
    if (-not $signTool) { throw "signtool.exe was not found in PATH or the Windows 10 SDK." }
    foreach ($binary in @("cusus.exe", "cusus-supervisor.exe")) {
        & $signTool sign /fd SHA256 /td SHA256 /tr "http://timestamp.digicert.com" /f $CertificatePath /p $CertificatePassword (Join-Path $stage $binary)
        if ($LASTEXITCODE -ne 0) { throw "Signing failed for $binary." }
        & $signTool verify /pa /v (Join-Path $stage $binary)
        if ($LASTEXITCODE -ne 0) { throw "Signature verification failed for $binary." }
    }
}

$manifest = [ordered]@{
    schemaVersion = 1
    product = "CuSus"
    version = "v$version"
    commit = $commit
    buildTime = $buildTime
    configSchema = 1
    showSchema = 2
    signed = [bool]$CertificatePath
    supportedOS = @("Windows 10 22H2", "Windows 11 23H2 or newer")
}
$manifest | ConvertTo-Json -Depth 5 | Set-Content -Encoding utf8 (Join-Path $stage "release-manifest.json")

Get-ChildItem -LiteralPath $stage -File | ForEach-Object { $_.LastWriteTimeUtc = $commitTime.UtcDateTime }
$zip = Join-Path $output ($packageName + ".zip")
if (Test-Path -LiteralPath $zip) { Remove-Item -LiteralPath $zip -Force }
Compress-Archive -Path (Join-Path $stage "*") -DestinationPath $zip -CompressionLevel Optimal
$hashes = Get-ChildItem -LiteralPath $stage -File | Sort-Object Name | ForEach-Object {
    "{0}  {1}" -f (Get-FileHash -Algorithm SHA256 $_.FullName).Hash.ToLowerInvariant(), $_.Name
}
$hashes += "{0}  {1}" -f (Get-FileHash -Algorithm SHA256 $zip).Hash.ToLowerInvariant(), (Split-Path -Leaf $zip)
$hashes | Set-Content -Encoding ascii (Join-Path $output "SHA256SUMS.txt")
Write-Host "Release package: $zip"
