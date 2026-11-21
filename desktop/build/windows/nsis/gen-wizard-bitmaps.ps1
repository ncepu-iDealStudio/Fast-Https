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

function New-RoundedRectPath([float]$x, [float]$y, [float]$w, [float]$h, [float]$r) {
    $d = $r * 2
    $p = New-Object System.Drawing.Drawing2D.GraphicsPath
    $p.AddArc($x, $y, $d, $d, 180, 90)
    $p.AddArc($x + $w - $d, $y, $d, $d, 270, 90)
    $p.AddArc($x + $w - $d, $y + $h - $d, $d, $d, 0, 90)
    $p.AddArc($x, $y + $h - $d, $d, $d, 90, 90)
    $p.CloseFigure()
    return $p
}

# ---- 侧边图 164x314：深空靛蓝渐变 + 极简星轨 + 玻璃图标框 + 字标 ----
$bmp = New-Object System.Drawing.Bitmap(164, 314)
$g = [System.Drawing.Graphics]::FromImage($bmp)
$g.SmoothingMode = 'AntiAlias'
$g.InterpolationMode = 'HighQualityBicubic'
$g.TextRenderingHint = 'ClearTypeGridFit'

$rect = New-Object System.Drawing.Rectangle(0, 0, 164, 314)
$c1 = [System.Drawing.Color]::FromArgb(255, 22, 29, 51)   # #161D33
$c2 = [System.Drawing.Color]::FromArgb(255, 30, 43, 75)   # #1E2B4B
$c3 = [System.Drawing.Color]::FromArgb(255, 20, 31, 56)   # #141F38
$grad = New-Object System.Drawing.Drawing2D.LinearGradientBrush($rect, $c1, $c3, 65)
$blend = New-Object System.Drawing.Drawing2D.ColorBlend
$blend.Colors = @($c1, $c2, $c3)
$blend.Positions = @(0.0, 0.5, 1.0)
$grad.InterpolationColors = $blend
$g.FillRectangle($grad, $rect)

# 极简星轨圆环（低透明度，克制无光污染）
$penRing1 = New-Object System.Drawing.Pen([System.Drawing.Color]::FromArgb(28, 255, 255, 255), 1)
$penRing2 = New-Object System.Drawing.Pen([System.Drawing.Color]::FromArgb(22, 165, 180, 252), 1)
$penRing2.DashPattern = @(3, 4)
$g.DrawEllipse($penRing1, -18, 12, 200, 200)
$g.DrawEllipse($penRing2, 10, 40, 144, 144)
$g.DrawEllipse($penRing1, 58, 218, 150, 150)

# 微小星点
$dotBrush1 = New-Object System.Drawing.SolidBrush([System.Drawing.Color]::FromArgb(145, 165, 180, 252))
$dotBrush2 = New-Object System.Drawing.SolidBrush([System.Drawing.Color]::FromArgb(135, 103, 232, 249))
$g.FillEllipse($dotBrush1, 134, 62, 5, 5)
$g.FillEllipse($dotBrush2, 24, 152, 4, 4)

# 图标圆角玻璃底座
$cardPath = New-RoundedRectPath 42 72 80 80 18
$cardFill = New-Object System.Drawing.SolidBrush([System.Drawing.Color]::FromArgb(26, 255, 255, 255))
$cardEdge = New-Object System.Drawing.Pen([System.Drawing.Color]::FromArgb(46, 255, 255, 255), 1)
$g.FillPath($cardFill, $cardPath)
$g.DrawPath($cardEdge, $cardPath)
$g.DrawImage($icon, 55, 85, 54, 54)

# 品牌字标
$fmt = New-Object System.Drawing.StringFormat
$fmt.Alignment = 'Center'
$font1 = New-Object System.Drawing.Font('Segoe UI', 14.5, [System.Drawing.FontStyle]::Bold)
$font2 = New-Object System.Drawing.Font('Segoe UI', 7.5, [System.Drawing.FontStyle]::Bold)
$font3 = New-Object System.Drawing.Font('Segoe UI', 7.5, [System.Drawing.FontStyle]::Regular)
$g.DrawString('fast-https', $font1, [System.Drawing.Brushes]::White, (New-Object System.Drawing.RectangleF(0, 168, 164, 28)), $fmt)

# DESKTOP 细胶囊
$pillPath = New-RoundedRectPath 48 199 68 18 9
$pillFill = New-Object System.Drawing.SolidBrush([System.Drawing.Color]::FromArgb(30, 139, 149, 246))
$pillEdge = New-Object System.Drawing.Pen([System.Drawing.Color]::FromArgb(65, 139, 149, 246), 1)
$g.FillPath($pillFill, $pillPath)
$g.DrawPath($pillEdge, $pillPath)
$g.DrawString('DESKTOP', $font2, (New-Object System.Drawing.SolidBrush([System.Drawing.Color]::FromArgb(220, 203, 213, 225))), (New-Object System.Drawing.RectangleF(0, 201, 164, 16)), $fmt)

# 底部极淡副标
$g.DrawString('Quiet · Fast · Secure', $font3, (New-Object System.Drawing.SolidBrush([System.Drawing.Color]::FromArgb(130, 180, 192, 214))), (New-Object System.Drawing.RectangleF(0, 280, 164, 18)), $fmt)
$bmp.Save((Join-Path $OutDir "wizard-side.bmp"), [System.Drawing.Imaging.ImageFormat]::Bmp)

# ---- 页眉图 150x57：浅静底 + 细底边 + 圆角图标框 + 字标 ----
$bmp2 = New-Object System.Drawing.Bitmap(150, 57)
$g2 = [System.Drawing.Graphics]::FromImage($bmp2)
$g2.SmoothingMode = 'AntiAlias'
$g2.InterpolationMode = 'HighQualityBicubic'
$g2.TextRenderingHint = 'ClearTypeGridFit'

$g2.FillRectangle((New-Object System.Drawing.SolidBrush([System.Drawing.Color]::FromArgb(255, 244, 245, 249))), 0, 0, 150, 57)
$headCard = New-RoundedRectPath 10 9 38 38 10
$g2.FillPath([System.Drawing.Brushes]::White, $headCard)
$g2.DrawPath((New-Object System.Drawing.Pen([System.Drawing.Color]::FromArgb(255, 213, 218, 230), 1)), $headCard)
$g2.DrawImage($icon, 16, 15, 26, 26)

$fontH1 = New-Object System.Drawing.Font('Segoe UI', 10.5, [System.Drawing.FontStyle]::Bold)
$fontH2 = New-Object System.Drawing.Font('Segoe UI', 7, [System.Drawing.FontStyle]::Bold)
$g2.DrawString('fast-https', $fontH1, (New-Object System.Drawing.SolidBrush([System.Drawing.Color]::FromArgb(255, 21, 23, 34))), 56, 12)
$g2.DrawString('DESKTOP', $fontH2, (New-Object System.Drawing.SolidBrush([System.Drawing.Color]::FromArgb(255, 79, 91, 213))), 57, 32)
$bmp2.Save((Join-Path $OutDir "wizard-head.bmp"), [System.Drawing.Imaging.ImageFormat]::Bmp)

$g.Dispose(); $g2.Dispose(); $icon.Dispose()
Write-Host "wizard-side.bmp / wizard-head.bmp generated in $OutDir"
