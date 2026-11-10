<#
.SYNOPSIS
  Build release artifacts: goreleaser archives and Linux packages (including the
  desktop deb/rpm/apk), Windows NSIS installers, and leave only the final
  artifacts in dist.

.DESCRIPTION
  Snapshot build by default. Tag the current commit
  first, then use -Release for a real-versioned local build. One NSIS
  installer is built per Windows zip found in dist (amd64 / arm64).

  The desktop console is a separate Go module. Before goreleaser runs, its
  frontend is generated (bindings + vite build) and a .desktop file is
  written, because the Go build embeds frontend/dist and the nfpm packages
  reference the .desktop file. goreleaser then builds the Linux binary with
  CGO_ENABLED=0 and the purego tag (Wails v3.1 Linux backend loads
  GTK4/WebKitGTK at runtime) and produces fast-https-desktop_*_{deb,rpm,apk}.
  The Linux binary is not executed here - GUI behaviour still needs a Linux
  host.

  Both modes only build locally (--skip=publish): no git remote is touched
  and no GITHUB_TOKEN is needed. Add -Publish to actually publish the
  release to the git hosting service (requires GITHUB_TOKEN); in that mode
  NSIS installers are built after publishing, so push them by hand or add
  them to the release later.

  goreleaser release also refuses to run when the current commit has no
  tag: "Tag first" is a hard requirement of -Release, not a script quirk.

.EXAMPLE
  .\build\package.ps1
  .\build\package.ps1 -Release
  .\build\package.ps1 -Release -Publish
  .\build\package.ps1 -SkipBuild -NoInstaller
#>
param(
    [switch]$Release,
    [switch]$Publish,
    [switch]$SkipBuild,
    [switch]$NoInstaller,
    [string]$Makensis = ""
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

# 构建配置集中在 build/：goreleaser 需要显式指定 -f，产物仍落 $root\dist
$grConfig = "build/goreleaser.yaml"

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

function Prepare-Desktop {
    # goreleaser 的 desktop-linux 构建会 //go:embed all:frontend/dist，
    # nfpm 的 contents 又引用 build/linux/*.desktop，两者都必须在构建前就绪。
    $desktop = Join-Path $root "desktop"
    if (-not (Test-Path (Join-Path $desktop "go.mod"))) { throw "desktop/ 不存在，无法构建桌面端包" }
    if (-not (Get-Command wails3 -ErrorAction SilentlyContinue)) {
        throw "未找到 wails3；先 go install github.com/wailsapp/wails/v3/cmd/wails3@latest"
    }

    # //go:embed all:frontend/dist 要求目录存在，否则生成 bindings 会失败。
    # 先放占位文件，随后 Vite 构建会覆盖成真正的产物。
    $distDir = Join-Path $desktop "frontend\dist"
    New-Item -ItemType Directory -Path $distDir -Force | Out-Null
    $placeholder = Join-Path $distDir "index.html"
    if (-not (Test-Path $placeholder)) {
        [System.IO.File]::WriteAllText($placeholder, "<!doctype html><html><body></body></html>")
    }

    Push-Location $desktop
    try {
        Write-Host "==> desktop: generate bindings"
        & wails3 generate bindings -clean=true -time-type=Date
        if ($LASTEXITCODE -ne 0) { throw "wails3 generate bindings failed (exit $LASTEXITCODE)" }

        # Windows 桌面端 exe 是安装包里的图形入口：wails3 build 会顺带构建前端
        # （dist 供 goreleaser 的 Linux 构建 embed）并生成 .syso（图标 + 版本信息）。
        Write-Host "==> desktop: windows amd64 build"
        & wails3 build
        if ($LASTEXITCODE -ne 0) { throw "wails3 build failed (exit $LASTEXITCODE)" }

        # arm64 安装包需要对应架构的 exe。Windows 后端不依赖 CGO，可直接交叉编译。
        Write-Host "==> desktop: windows arm64 build"
        & wails3 generate syso -arch arm64 -icon build/windows/icon.ico -manifest build/windows/wails.exe.manifest -info build/windows/info.json -out wails_windows_arm64.syso
        if ($LASTEXITCODE -ne 0) { throw "generate syso arm64 failed (exit $LASTEXITCODE)" }
        $env:CGO_ENABLED = "0"
        $env:GOOS = "windows"
        $env:GOARCH = "arm64"
        try {
            & go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui" -o "bin\fast-https-desktop-arm64.exe" .
            if ($LASTEXITCODE -ne 0) { throw "windows arm64 build failed (exit $LASTEXITCODE)" }
        }
        finally {
            $env:GOOS = "windows"
            $env:GOARCH = "amd64"
            $env:CGO_ENABLED = "0"
            Remove-Item -LiteralPath (Join-Path $desktop "wails_windows_arm64.syso") -Force -ErrorAction SilentlyContinue
        }

        Write-Host "==> desktop: generate .desktop"
        & wails3 generate .desktop -name "fast-https-desktop" -exec "fast-https-desktop" -icon "fast-https-desktop" -outputfile (Join-Path $desktop "build\linux\fast-https-desktop.desktop") -categories "Network;System;Utility;"
        if ($LASTEXITCODE -ne 0) { throw "generate .desktop failed (exit $LASTEXITCODE)" }
    }
    finally { Pop-Location }
}

# 1. desktop 前端 & goreleaser（含 Linux 主程序与桌面端的 deb/rpm/apk）
if (-not $SkipBuild) {
    Prepare-Desktop

    $grArgs = @("release", "-f", $grConfig, "--clean")
    if (-not $Release) { $grArgs += "--snapshot" }
    # 本地打包不发布：-Publish 才真正上传，需要 GITHUB_TOKEN
    if (-not $Publish) { $grArgs += "--skip=publish" }
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

        # 桌面管理端 exe 是安装包里的图形入口：fast-https.exe 是 -H=windowsgui
        # 编译的命令行服务端，双击不会出现窗口，快捷方式应指向桌面管理端。
        $desktopExe = Join-Path $root $(if ($arch -eq "arm64") { "desktop\bin\fast-https-desktop-arm64.exe" } else { "desktop\bin\fast-https-desktop.exe" })
        if (Test-Path $desktopExe) {
            $target = Join-Path $stage "fast-https-desktop.exe"
            Copy-Item -LiteralPath $desktopExe -Destination $target -Force
            Write-Host "==> stage $arch : 桌面管理端 $([math]::Round((Get-Item -LiteralPath $target).Length / 1MB, 1)) MB"
        }
        else {
            Write-Warning "未找到 $desktopExe：该安装包不含桌面管理端，快捷方式会退化为安装目录"
        }

        Copy-Item (Join-Path $root "LICENSE") $stage -Force

        $outFile = Join-Path $root "dist\fast-https-$version-windows-$arch-setup.exe"
        if (Test-Path $outFile) { Remove-Item $outFile -Force }

        $nsi = Join-Path $PSScriptRoot "installer\windows\fast-https.nsi"
        Ensure-Utf8Bom $nsi
        # 自绘向导脚本：PowerShell 5.1 只认带 BOM 的 UTF-8
        Ensure-Utf8Bom (Join-Path $PSScriptRoot "installer\windows\wizard.ps1")

        Write-Host "==> makensis $arch -> $(Split-Path -Leaf $outFile)"
        & $makensis `
            "/DAPP_VERSION=$version" `
            "/DAPP_VERSION4=$version4" `
            "/DAPP_ARCH=$arch" `
            "/DSTAGE_DIR=$stage" `
            "/DOUT_FILE=$outFile" `
            $nsi
        if ($LASTEXITCODE -ne 0) { throw "makensis failed (exit $LASTEXITCODE, arch $arch)" }
    }
    Remove-Item $stageRoot -Recurse -Force -ErrorAction SilentlyContinue
}

# 3. keep only artifacts in dist
$keep = '^(fast-https_.*\.(zip|tar\.gz|deb|rpm|apk)|fast-https-.*-setup\.exe|fast-https-desktop_.*\.(deb|rpm|apk)|checksums\.txt)$'
Get-ChildItem (Join-Path $root "dist") -Force |
    Where-Object { $_.PSIsContainer -or $_.Name -notmatch $keep } |
    Remove-Item -Recurse -Force

Write-Host ""
Write-Host "==> dist:"
Get-ChildItem (Join-Path $root "dist") | Sort-Object Name |
    Format-Table Name, @{ Name = "MB"; Expression = { [math]::Round($_.Length / 1MB, 1) } } -AutoSize
