<#
.SYNOPSIS
  Build release artifacts: goreleaser archives, Windows NSIS installers,
  and leave only the final artifacts in dist.

.DESCRIPTION
  Snapshot build by default (version 1.4.0-next). Tag first, then use
  -Release for a real release. One installer is built per Windows zip
  found in dist (amd64 / arm64).

  Kept in dist:
    fast-https_Windows_x86_64.zip       fast-https_Windows_arm64.zip
    fast-https_Linux_x86_64.tar.gz      fast-https_Linux_arm64.tar.gz
    fast-https_Darwin_x86_64.tar.gz     fast-https_Darwin_arm64.tar.gz
    fast-https-<version>-windows-<arch>-setup.exe
    checksums.txt

.EXAMPLE
  .\scripts\package.ps1
  .\scripts\package.ps1 -Release
  .\scripts\package.ps1 -SkipBuild -NoInstaller
#>
param(
    [switch]$Release,
    [switch]$SkipBuild,
    [switch]$NoInstaller,
    [string]$Makensis = ""
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

function Resolve-Makensis {
    param([string]$Hint)
    if ($Hint) {
        if (Test-Path $Hint) { return $Hint }
        throw "makensis not found: $Hint"
    }
    $candidates = @(
        "D:\NSIS\makensis.exe",
        (Join-Path ${env:ProgramFiles(x86)} "NSIS\makensis.exe"),
        (Join-Path $env:ProgramFiles "NSIS\makensis.exe")
    )
    foreach ($c in $candidates) {
        if ($c -and (Test-Path $c)) { return $c }
    }
    $onPath = Get-Command makensis.exe -ErrorAction SilentlyContinue
    if ($onPath) { return $onPath.Source }
    throw "makensis.exe not found, pass -Makensis <path>"
}

function Ensure-Utf8Bom {
    # NSIS 3 reads a script as UTF-8 only when it starts with a BOM; without
    # one it falls back to ANSI and fails on non-ASCII text ("Bad text encoding").
    param([string]$Path)
    $bytes = [System.IO.File]::ReadAllBytes($Path)
    if ($bytes.Length -ge 3 -and $bytes[0] -eq 0xEF -and $bytes[1] -eq 0xBB -and $bytes[2] -eq 0xBF) { return }
    $text = [System.Text.Encoding]::UTF8.GetString($bytes)
    [System.IO.File]::WriteAllText($Path, $text, (New-Object System.Text.UTF8Encoding($true)))
}

function ConvertTo-Version4 {
    param([string]$Version)
    $nums = [regex]::Matches($Version, '\d+') | ForEach-Object { $_.Value }
    while ($nums.Count -lt 4) { $nums += "0" }
    return ($nums[0..3] -join ".")
}

# 1. goreleaser
if (-not $SkipBuild) {
    $grArgs = @("release", "--clean")
    if (-not $Release) { $grArgs += "--snapshot" }
    Write-Host "==> goreleaser $($grArgs -join ' ')"
    & goreleaser @grArgs
    if ($LASTEXITCODE -ne 0) { throw "goreleaser failed (exit $LASTEXITCODE)" }
}

$metadataPath = Join-Path $root "dist\metadata.json"
if (-not (Test-Path $metadataPath)) { throw "dist\metadata.json missing; run a build first" }
$metadata = Get-Content $metadataPath -Raw | ConvertFrom-Json
$version = $metadata.version
$version4 = ConvertTo-Version4 $version
Write-Host "==> version $version ($version4)"

# 2. NSIS installers
if (-not $NoInstaller) {
    $makensis = Resolve-Makensis $Makensis
    $zips = Get-ChildItem (Join-Path $root "dist") -Filter "fast-https_Windows_*.zip"
    if (-not $zips) { throw "no Windows zip in dist; cannot build installers" }

    $stageRoot = Join-Path $root "dist\.stage"
    foreach ($zip in $zips) {
        $arch = "unknown"
        if ($zip.Name -match "x86_64") { $arch = "amd64" }
        elseif ($zip.Name -match "arm64") { $arch = "arm64" }

        $stage = Join-Path $stageRoot $arch
        if (Test-Path $stage) { Remove-Item $stage -Recurse -Force }
        New-Item -ItemType Directory -Path $stage -Force | Out-Null
        Expand-Archive -Path $zip.FullName -DestinationPath $stage -Force
        Copy-Item (Join-Path $root "LICENSE") $stage -Force

        $outFile = Join-Path $root "dist\fast-https-$version-windows-$arch-setup.exe"
        if (Test-Path $outFile) { Remove-Item $outFile -Force }

        $nsi = Join-Path $root "installer\windows\fast-https.nsi"
        Ensure-Utf8Bom $nsi

        Write-Host "==> makensis $arch -> $(Split-Path -Leaf $outFile)"
        & $makensis `
            "/DAPP_VERSION=$version" `
            "/DAPP_VERSION4=$version4" `
            "/DAPP_ARCH=$arch" `
            "/DSTAGE_DIR=$stage" `
            "/DOUT_FILE=$outFile" `
            (Join-Path $root "installer\windows\fast-https.nsi")
        if ($LASTEXITCODE -ne 0) { throw "makensis failed (exit $LASTEXITCODE, arch $arch)" }
    }
    Remove-Item $stageRoot -Recurse -Force -ErrorAction SilentlyContinue
}

# 3. keep only artifacts in dist
$keep = '^(fast-https_.*\.(zip|tar\.gz)|fast-https-.*-setup\.exe|checksums\.txt)$'
Get-ChildItem (Join-Path $root "dist") -Force |
    Where-Object { $_.PSIsContainer -or $_.Name -notmatch $keep } |
    Remove-Item -Recurse -Force

Write-Host ""
Write-Host "==> dist:"
Get-ChildItem (Join-Path $root "dist") | Sort-Object Name |
    Format-Table Name, @{ Name = "MB"; Expression = { [math]::Round($_.Length / 1MB, 1) } } -AutoSize
