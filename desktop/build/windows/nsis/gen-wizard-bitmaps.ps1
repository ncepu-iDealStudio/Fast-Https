# 从应用图标生成安装向导的品牌位图（wizard-side.bmp / wizard-head.bmp）。
# 换图标或改品牌样式后跑一次，提交新的 bmp：
#   powershell -File gen-wizard-bitmaps.ps1
# 两个位图供桌面端与主程序的 NSIS 安装向导共用（MUI_WELCOMEFINISHPAGE_BITMAP
# 164x314 侧边图、MUI_HEADERIMAGE_BITMAP 150x57 页眉图）。
param(
    [string]$IconPath = (Join-Path $PSScriptRoot "..\..\appicon.png"),
    [string]$OutDir = $PSScriptRoot
)

Add-Type -AssemblyName System.Drawing
$icon = [System.Drawing.Image]::FromFile((Resolve-Path $IconPath))

function New-BrandBitmap([int]$w, [int]$h, [bool]$dark) {
    $bmp = New-Object System.Drawing.Bitmap($w, $h)
    $g = [System.Drawing.Graphics]::FromImage($bmp)
    $g.SmoothingMode = 'AntiAlias'
    $g.TextRenderingHint = 'AntiAlias'
    if ($dark) {
        $rect = New-Object System.Drawing.Rectangle(0, 0, $w, $h)
        $c1 = [System.Drawing.Color]::FromArgb(255, 35, 40, 47)
        $c2 = [System.Drawing.Color]::FromArgb(255, 22, 26, 31)
        $brush = New-Object System.Drawing.Drawing2D.LinearGradientBrush($rect, $c1, $c2, 90)
    } else {
        $brush = New-Object System.Drawing.SolidBrush([System.Drawing.Color]::FromArgb(255, 245, 247, 250))
    }
    $g.FillRectangle($brush, 0, 0, $w, $h)
    return @($bmp, $g)
}

# 侧边图 164x314：石墨渐变 + 图标 + 字标
$r = New-BrandBitmap 164 314 $true
$bmp = $r[0]; $g = $r[1]
$g.DrawImage($icon, 34, 62, 96, 96)
$fmt = New-Object System.Drawing.StringFormat
$fmt.Alignment = 'Center'
$font1 = New-Object System.Drawing.Font('Segoe UI', 15, [System.Drawing.FontStyle]::Bold)
$font2 = New-Object System.Drawing.Font('Segoe UI', 8, [System.Drawing.FontStyle]::Regular)
$g.DrawString('fast-https', $font1, [System.Drawing.Brushes]::White, (New-Object System.Drawing.RectangleF(0, 172, 164, 30)), $fmt)
$g.DrawString('D E S K T O P', $font2, (New-Object System.Drawing.SolidBrush([System.Drawing.Color]::FromArgb(255, 154, 163, 174))), (New-Object System.Drawing.RectangleF(0, 202, 164, 20)), $fmt)
$bmp.Save((Join-Path $OutDir "wizard-side.bmp"), [System.Drawing.Imaging.ImageFormat]::Bmp)

# 页眉图 150x57：浅底 + 图标 + 字标
$r = New-BrandBitmap 150 57 $false
$bmp2 = $r[0]; $g2 = $r[1]
$g2.DrawImage($icon, 8, 8, 40, 40)
$font3 = New-Object System.Drawing.Font('Segoe UI', 11, [System.Drawing.FontStyle]::Bold)
$font4 = New-Object System.Drawing.Font('Segoe UI', 7, [System.Drawing.FontStyle]::Regular)
$g2.DrawString('fast-https', $font3, (New-Object System.Drawing.SolidBrush([System.Drawing.Color]::FromArgb(255, 22, 24, 29))), 56, 12)
$g2.DrawString('DESKTOP', $font4, (New-Object System.Drawing.SolidBrush([System.Drawing.Color]::FromArgb(255, 120, 128, 138))), 57, 32)
$bmp2.Save((Join-Path $OutDir "wizard-head.bmp"), [System.Drawing.Imaging.ImageFormat]::Bmp)

$g.Dispose(); $g2.Dispose(); $icon.Dispose()
Write-Host "wizard-side.bmp / wizard-head.bmp generated in $OutDir"
