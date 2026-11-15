<#
.SYNOPSIS
  打 tag 前同步项目里的版本信息。

.DESCRIPTION
  git tag 只改 git 引用，不会改代码里的版本号。本脚本一次性更新所有
  版本字段与生成物：主程序 CURRENT_VERSION、goreleaser 快照版本、
  Windows NSIS 默认值、桌面端各版本字段与前端兜底文本、README 版本徽章，
  并重新生成 man 页与 docs/usage.html。

  可以重复执行：匹配的是「任意 x.y.z」而不是某个具体旧版本。

.EXAMPLE
  .\build\bump-version.ps1 -Version 1.6.1
#>
param(
    [Parameter(Mandatory = $true)][string]$Version
)

$ErrorActionPreference = "Stop"

if ($Version -notmatch '^\d+\.\d+\.\d+$') {
    throw "版本号必须是 x.y.z（例如 1.6.1），实际：$Version"
}
$version4 = "$Version.0"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

# 按正则替换，保留文件原有的 UTF-8 BOM 状态（NSIS 脚本需要 BOM）。
function Update-TextFile {
    param(
        [string]$Relative,
        [string]$Pattern,
        [string]$Replacement,
        [int]$Min = 1
    )
    $path = Join-Path $root $Relative
    if (-not (Test-Path -LiteralPath $path)) { throw "找不到文件：$Relative" }
    $bytes = [System.IO.File]::ReadAllBytes($path)
    $bom = $bytes.Length -ge 3 -and $bytes[0] -eq 0xEF -and $bytes[1] -eq 0xBB -and $bytes[2] -eq 0xBF
    $text = [System.Text.Encoding]::UTF8.GetString($bytes)
    if ($bom) { $text = $text.Substring(1) }
    $found = [regex]::Matches($text, $Pattern)
    if ($found.Count -lt $Min) {
        throw "$Relative 只匹配到 $($found.Count) 处（预期 >= $Min）：$Pattern"
    }
    $newText = [regex]::Replace($text, $Pattern, $Replacement)
    if ($newText -ne $text) {
        $enc = New-Object System.Text.UTF8Encoding($bom)
        [System.IO.File]::WriteAllText($path, $newText, $enc)
    }
    return $found.Count
}

$edits = @(
    @{ File = 'README.md'; Pattern = 'version-\d+\.\d+\.\d+-0E7490'; Replacement = 'version-' + $Version + '-0E7490' },
    @{ File = 'README.en.md'; Pattern = 'version-\d+\.\d+\.\d+-0E7490'; Replacement = 'version-' + $Version + '-0E7490' },
    @{ File = 'config/consts.go'; Pattern = 'CURRENT_VERSION string = "V\d+\.\d+\.\d+"'; Replacement = 'CURRENT_VERSION string = "V' + $Version + '"' },
    @{ File = 'build/goreleaser.yaml'; Pattern = 'version_template: "\d+\.\d+\.\d+"'; Replacement = 'version_template: "' + $Version + '"' },
    @{ File = 'build/installer/windows/fast-https.nsi'; Pattern = '!define APP_VERSION "\d+\.\d+\.\d+"'; Replacement = '!define APP_VERSION "' + $Version + '"' },
    @{ File = 'build/installer/windows/fast-https.nsi'; Pattern = '!define APP_VERSION4 "\d+\.\d+\.\d+\.0"'; Replacement = '!define APP_VERSION4 "' + $version4 + '"' },
    @{ File = 'desktop/internal/view/state.go'; Pattern = 'desktopVersion = "\d+\.\d+\.\d+"'; Replacement = 'desktopVersion = "' + $Version + '"' },
    @{ File = 'desktop/build/config.yml'; Pattern = 'version: "\d+\.\d+\.\d+"'; Replacement = 'version: "' + $Version + '"' },
    # Linux 桌面包已移除（桌面端只发布 Windows），不再有 desktop/build/linux/nfpm/nfpm.yaml
    @{ File = 'desktop/build/windows/nsis/project.nsi'; Pattern = '!define INFO_PRODUCTVERSION "\d+\.\d+\.\d+"'; Replacement = '!define INFO_PRODUCTVERSION "' + $Version + '"' },
    @{ File = 'desktop/build/windows/info.json'; Pattern = '"(file_version|product_version|FileVersion|ProductVersion)": "\d+\.\d+\.\d+"'; Replacement = '"$1": "' + $Version + '"'; Min = 4 },
    # 版本号只在侧边栏显示一次：顶部状态行与账户菜单不再重复
    @{ File = 'desktop/frontend/index.html'; Pattern = 'data-bind="version">\d+\.\d+\.\d+<'; Replacement = 'data-bind="version">' + $Version + '<'; Min = 1 }
)

foreach ($e in $edits) {
    $min = 1
    if ($e.ContainsKey('Min')) { $min = [int]$e.Min }
    $n = Update-TextFile -Relative $e.File -Pattern $e.Pattern -Replacement $e.Replacement -Min $min
    Write-Host ("==> {0} ({1} 处)" -f $e.File, $n)
}

Write-Host "==> go run ./tools/manpage"
& go run ./tools/manpage
if ($LASTEXITCODE -ne 0) { throw "man 页生成失败" }
Write-Host "==> go run ./tools/docgen"
& go run ./tools/docgen
if ($LASTEXITCODE -ne 0) { throw "usage.html 生成失败" }

Write-Host ""
Write-Host "版本信息已统一为 $Version。提交这些改动后再打 tag："
Write-Host "  git commit -am 'chore(release): 版本信息同步为 $Version'"
Write-Host "  git tag -a v$Version -m 'Fast-Https $Version'"
