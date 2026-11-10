# Fast-Https 安装向导（自绘界面）。
#
# 定位：NSIS 只当安装引擎（fast-https.nsi 的安装段，静默执行），向导界面
# 全部由这里自绘——圆角卡片、品牌图、胶囊按钮、自定义进度页，双击的仍然
# 是单个 setup.exe。交互流程：
#   setup.exe（交互）→ 本脚本弹自绘向导 → 点「安装」→ 静默调起
#   setup.exe /S /D=<目录>（引擎模式，跑安装段）→ 向导显示进度与完成页
# 卸载同理：uninstall.exe（交互）→ 本脚本 --uninstall 弹自绘确认 →
#   静默调起 uninstall.exe /S。
#
# 运行环境：Windows 自带的 PowerShell 5.1 + WPF（.NET Framework），
# 零额外依赖。脚本必须带 UTF-8 BOM（PowerShell 5.1 否则按 ANSI 解析）。
param(
    [string]$Self = "",          # 引擎可执行文件（setup.exe / uninstall.exe 自身）
    [string]$Icon = "",          # 品牌图（png，随包提供）
    [string]$Title = "Fast-Https",
    [string]$Version = "",
    [switch]$Uninstall,
    [switch]$SelfTest,           # 只加载界面与命名元素后退出，供无头自检
    [string]$PreviewPng = "",    # 把界面无头渲染成 PNG（调样式用）
    [string]$PreviewScreen = "welcome"  # welcome | working | done | error
)

$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName PresentationFramework, WindowsBase, PresentationCore, System.Windows.Forms

# ---- 文案（中英，按系统语言选择） ----
$zh = $PSUICulture -like 'zh*'
$T = if ($zh) {
    @{
        tagline = '本地网站服务与反向代理 · 桌面管理端'
        install = '安装'
        installTo = '安装到'
        browse = '浏览'
        agree = '安装即同意'
        terms = '用户协议'
        and = '和'
        privacy = '隐私政策'
        installing = '正在安装…'
        phase1 = '正在准备安装…'
        phase2 = '正在复制程序文件…'
        phase3 = '正在注册组件与快捷方式…'
        doneTitle = '安装完成'
        doneSub = '已安装到'
        runApp = '运行 Fast-Https Desktop'
        finish = '完成'
        errTitle = '安装失败'
        errSub = '安装程序返回了错误，请重试或查看日志。'
        close = '关闭'
        unTitle = ('完全卸载 ' + $Title + '？')
        unBody = '将删除程序、配置、站点、证书、日志与缓存，不留残留。'
        unKeep = '如果只是想关掉窗口，用「隐藏到后台托盘」即可，不必卸载。'
        unBtn = '卸载'
        unCancel = '取消'
        uninstalling = '正在卸载…'
        unDoneTitle = '卸载完成'
        unDoneSub = '没有残留，所有数据已清除。'
    }
} else {
    @{
        tagline = 'Local web server & reverse proxy · desktop console'
        install = 'Install'
        installTo = 'Install to'
        browse = 'Browse'
        agree = 'By installing you agree to the'
        terms = 'Terms'
        and = 'and'
        privacy = 'Privacy Policy'
        installing = 'Installing…'
        phase1 = 'Preparing…'
        phase2 = 'Copying program files…'
        phase3 = 'Registering components and shortcuts…'
        doneTitle = 'Installation complete'
        doneSub = 'Installed to'
        runApp = 'Run Fast-Https Desktop'
        finish = 'Done'
        errTitle = 'Installation failed'
        errSub = 'The installer reported an error. Please retry or check the logs.'
        close = 'Close'
        unTitle = ('Completely uninstall ' + $Title + '?')
        unBody = 'Removes the program, config, sites, certificates, logs and caches — nothing is left behind.'
        unKeep = 'To just close the window, use Hide to tray instead.'
        unBtn = 'Uninstall'
        unCancel = 'Cancel'
        uninstalling = 'Uninstalling…'
        unDoneTitle = 'Uninstall complete'
        unDoneSub = 'No leftovers — all data has been removed.'
    }
}

# ---- 界面（XAML：圆角卡片 + 品牌图 + 胶囊按钮） ----
$xaml = @'
<Window xmlns="http://schemas.microsoft.com/winfx/2006/xaml/presentation"
        xmlns:x="http://schemas.microsoft.com/winfx/2006/xaml"
        WindowStyle="None" AllowsTransparency="True" Background="Transparent"
        Width="560" Height="404" WindowStartupLocation="CenterScreen"
        ResizeMode="NoResize" ShowInTaskbar="True" FontFamily="Microsoft YaHei UI"
        UseLayoutRounding="True" TextOptions.TextFormattingMode="Display">
  <Border Background="#FFFFFF" CornerRadius="14" Margin="14">
    <Border.Effect><DropShadowEffect BlurRadius="26" ShadowDepth="0" Opacity="0.20"/></Border.Effect>
    <Grid Margin="2">
      <Grid x:Name="dragBar" Height="44" VerticalAlignment="Top" Background="Transparent"/>
      <Button x:Name="btnClose" Width="30" Height="30" HorizontalAlignment="Right" VerticalAlignment="Top"
              Margin="0,8,8,0" Background="Transparent" BorderThickness="0" Cursor="Hand">
        <Path Data="M0,0 L8,8 M8,0 L0,8" Stroke="#949AA3" StrokeThickness="1.4" StrokeStartLineCap="Round" StrokeEndLineCap="Round"/>
      </Button>

      <!-- 欢迎 / 卸载确认 -->
      <StackPanel x:Name="screenWelcome" VerticalAlignment="Center" Margin="48,40,48,24">
        <Image x:Name="imgLogo" Width="84" Height="84" HorizontalAlignment="Center" RenderOptions.BitmapScalingMode="HighQuality"/>
        <TextBlock x:Name="txtTitle" FontSize="20" FontWeight="SemiBold" Foreground="#16181D" HorizontalAlignment="Center" Margin="0,14,0,0"/>
        <TextBlock x:Name="txtTagline" FontSize="11.5" Foreground="#949AA3" HorizontalAlignment="Center" Margin="0,6,0,0"/>
        <TextBlock x:Name="txtBody" FontSize="12.5" Foreground="#666C75" HorizontalAlignment="Center" TextAlignment="Center" TextWrapping="Wrap" Width="420" Margin="0,14,0,0" Visibility="Collapsed"/>
        <TextBlock x:Name="txtBody2" FontSize="11" Foreground="#949AA3" HorizontalAlignment="Center" TextAlignment="Center" TextWrapping="Wrap" Width="420" Margin="0,8,0,0" Visibility="Collapsed"/>
        <Button x:Name="btnPrimary" Width="148" Height="44" Cursor="Hand" HorizontalAlignment="Center" Margin="0,22,0,0"
                BorderThickness="0" Foreground="#FFFFFF" FontSize="15" FontWeight="SemiBold" Content="Install">
          <Button.Background>
            <LinearGradientBrush StartPoint="0,0" EndPoint="1,1">
              <GradientStop Color="#5A626E" Offset="0"/><GradientStop Color="#2C323B" Offset="1"/>
            </LinearGradientBrush>
          </Button.Background>
          <Button.Template>
            <ControlTemplate TargetType="Button">
              <Border x:Name="bd" CornerRadius="22" Background="{TemplateBinding Background}">
                <ContentPresenter HorizontalAlignment="Center" VerticalAlignment="Center"/>
              </Border>
              <ControlTemplate.Triggers>
                <Trigger Property="IsMouseOver" Value="True"><Setter TargetName="bd" Property="Opacity" Value="0.88"/></Trigger>
                <Trigger Property="IsPressed" Value="True"><Setter TargetName="bd" Property="Opacity" Value="0.72"/></Trigger>
              </ControlTemplate.Triggers>
            </ControlTemplate>
          </Button.Template>
        </Button>
        <Button x:Name="btnSecondary" Width="148" Height="44" Cursor="Hand" HorizontalAlignment="Center" Margin="0,10,0,0"
                Background="Transparent" BorderThickness="0" Foreground="#666C75" FontSize="13" Content="" Visibility="Collapsed"/>
        <StackPanel Orientation="Horizontal" HorizontalAlignment="Center" Margin="0,18,0,0" x:Name="rowPath">
          <TextBlock x:Name="txtInstallTo" FontSize="11.5" Foreground="#666C75" VerticalAlignment="Center"/>
          <TextBox x:Name="txtPath" Width="290" Height="30" Margin="8,0,0,0" FontSize="11.5" Foreground="#16181D"
                   BorderThickness="0" VerticalContentAlignment="Center" Padding="10,0">
            <TextBox.Background><SolidColorBrush Color="#F2F4F8"/></TextBox.Background>
          </TextBox>
          <Button x:Name="btnBrowse" Height="30" Margin="6,0,0,0" Padding="10,0" Cursor="Hand" FontSize="11.5"
                  Background="Transparent" BorderBrush="#D7DBE1" BorderThickness="1" Content="…"/>
        </StackPanel>
        <StackPanel Orientation="Horizontal" HorizontalAlignment="Center" Margin="0,16,0,0" x:Name="rowAgree">
          <TextBlock x:Name="txtAgree" FontSize="10.5" Foreground="#949AA3"/>
          <TextBlock x:Name="lnkTerms" FontSize="10.5" Foreground="#5A626E" TextDecorations="Underline" Cursor="Hand" Margin="4,0,0,0"/>
          <TextBlock x:Name="txtAnd" FontSize="10.5" Foreground="#949AA3" Margin="4,0"/>
          <TextBlock x:Name="lnkPrivacy" FontSize="10.5" Foreground="#5A626E" TextDecorations="Underline" Cursor="Hand"/>
        </StackPanel>
      </StackPanel>

      <!-- 进度 -->
      <StackPanel x:Name="screenWorking" VerticalAlignment="Center" Visibility="Collapsed">
        <Image x:Name="imgLogo2" Width="64" Height="64" HorizontalAlignment="Center" RenderOptions.BitmapScalingMode="HighQuality"/>
        <TextBlock x:Name="txtPhase" FontSize="13" Foreground="#666C75" HorizontalAlignment="Center" Margin="0,20,0,0"/>
        <ProgressBar x:Name="barWork" Width="280" Height="6" Margin="0,18,0,0" IsIndeterminate="True"/>
      </StackPanel>

      <!-- 完成 / 错误 -->
      <StackPanel x:Name="screenDone" VerticalAlignment="Center" Visibility="Collapsed">
        <Ellipse x:Name="doneCircle" Width="72" Height="72" HorizontalAlignment="Center">
          <Ellipse.Fill><SolidColorBrush Color="#EAF7F0"/></Ellipse.Fill>
        </Ellipse>
        <Path x:Name="doneCheck" Width="30" Height="30" HorizontalAlignment="Center" Margin="0,-51,0,0"
              Data="M6,15 L12,21 L24,8" Stroke="#16794A" StrokeThickness="3" StrokeStartLineCap="Round" StrokeEndLineCap="Round" StrokeLineJoin="Round"/>
        <TextBlock x:Name="txtDoneTitle" FontSize="18" FontWeight="SemiBold" Foreground="#16181D" HorizontalAlignment="Center" Margin="0,18,0,0"/>
        <TextBlock x:Name="txtDoneSub" FontSize="11.5" Foreground="#949AA3" HorizontalAlignment="Center" Margin="0,8,0,0" TextAlignment="Center" TextWrapping="Wrap" MaxWidth="380"/>
        <Button x:Name="btnRun" Width="200" Height="42" Cursor="Hand" HorizontalAlignment="Center" Margin="0,22,0,0"
                BorderThickness="0" Foreground="#FFFFFF" FontSize="13.5" FontWeight="SemiBold" Content="">
          <Button.Background>
            <LinearGradientBrush StartPoint="0,0" EndPoint="1,1">
              <GradientStop Color="#5A626E" Offset="0"/><GradientStop Color="#2C323B" Offset="1"/>
            </LinearGradientBrush>
          </Button.Background>
          <Button.Template>
            <ControlTemplate TargetType="Button">
              <Border x:Name="bd" CornerRadius="21" Background="{TemplateBinding Background}">
                <ContentPresenter HorizontalAlignment="Center" VerticalAlignment="Center"/>
              </Border>
              <ControlTemplate.Triggers>
                <Trigger Property="IsMouseOver" Value="True"><Setter TargetName="bd" Property="Opacity" Value="0.88"/></Trigger>
                <Trigger Property="IsPressed" Value="True"><Setter TargetName="bd" Property="Opacity" Value="0.72"/></Trigger>
              </ControlTemplate.Triggers>
            </ControlTemplate>
          </Button.Template>
        </Button>
        <Button x:Name="btnFinish" Width="120" Height="36" Cursor="Hand" HorizontalAlignment="Center" Margin="0,10,0,0"
                Background="Transparent" BorderThickness="0" Foreground="#666C75" FontSize="12.5" Content=""/>
      </StackPanel>
    </Grid>
  </Border>
</Window>
'@

# ---- 加载界面 ----
$window = [System.Windows.Markup.XamlReader]::Parse($xaml)
function N([string]$name) { return $window.FindName($name) }

function Show-Screen($screen) {
    foreach ($s in @($screenWelcome, $screenWorking, $screenDone)) { $s.Visibility = 'Collapsed' }
    $screen.Visibility = 'Visible'
}

$dragBar = N 'dragBar'; $btnClose = N 'btnClose'
$screenWelcome = N 'screenWelcome'; $screenWorking = N 'screenWorking'; $screenDone = N 'screenDone'
$imgLogo = N 'imgLogo'; $imgLogo2 = N 'imgLogo2'
$txtTitle = N 'txtTitle'; $txtTagline = N 'txtTagline'; $txtBody = N 'txtBody'; $txtBody2 = N 'txtBody2'
$btnPrimary = N 'btnPrimary'; $btnSecondary = N 'btnSecondary'
$rowPath = N 'rowPath'; $txtInstallTo = N 'txtInstallTo'; $txtPath = N 'txtPath'; $btnBrowse = N 'btnBrowse'
$rowAgree = N 'rowAgree'; $txtAgree = N 'txtAgree'; $lnkTerms = N 'lnkTerms'; $txtAnd = N 'txtAnd'; $lnkPrivacy = N 'lnkPrivacy'
$txtPhase = N 'txtPhase'; $barWork = N 'barWork'
$doneCircle = N 'doneCircle'; $doneCheck = N 'doneCheck'; $txtDoneTitle = N 'txtDoneTitle'; $txtDoneSub = N 'txtDoneSub'
$btnRun = N 'btnRun'; $btnFinish = N 'btnFinish'

# 品牌图（没有就退回窗口标题）
if ($Icon -and (Test-Path $Icon)) {
    $iconPath = (Resolve-Path $Icon).Path
    $imgLogo.Source = [System.Windows.Media.Imaging.BitmapFrame]::Create([Uri]$iconPath)
    $imgLogo2.Source = $imgLogo.Source
}
if ($Version) { $window.Title = "$Title $Version" } else { $window.Title = $Title }

# 无头自检：界面与命名元素都加载成功即通过
if ($SelfTest) {
    $names = 'dragBar','btnClose','screenWelcome','screenWorking','screenDone','imgLogo','imgLogo2','txtTitle','txtTagline','txtBody','txtBody2','btnPrimary','btnSecondary','rowPath','txtInstallTo','txtPath','btnBrowse','rowAgree','txtAgree','lnkTerms','txtAnd','lnkPrivacy','txtPhase','barWork','doneCircle','doneCheck','txtDoneTitle','txtDoneSub','btnRun','btnFinish'
    foreach ($n in $names) { if ($null -eq (N $n)) { Write-Error "selftest: missing element $n"; exit 1 } }
    Write-Host "selftest ok: wizard UI loaded"
    exit 0
}

# 无头预览：把界面渲染成 PNG（不 ShowDialog），用于调样式与截图留档
if ($PreviewPng) {
    $txtTitle.Text = if ($Version) { "$Title $Version" } else { $Title }
    if ($Uninstall) {
        $txtTitle.Text = $T.unTitle
        $txtTagline.Visibility = 'Collapsed'
        $txtBody.Text = $T.unBody; $txtBody.Visibility = 'Visible'
        $txtBody2.Text = $T.unKeep; $txtBody2.Visibility = 'Visible'
        $btnPrimary.Content = $T.unBtn
        $btnPrimary.Background = [System.Windows.Media.Brushes]::IndianRed
        $rowPath.Visibility = 'Collapsed'; $rowAgree.Visibility = 'Collapsed'
    } else {
        $txtTagline.Text = $T.tagline
        $btnPrimary.Content = $T.install + '  ›'
        $txtInstallTo.Text = $T.installTo
        $txtPath.Text = (Join-Path $env:ProgramFiles $Title)
        $txtAgree.Text = $T.agree; $lnkTerms.Text = $T.terms
        $txtAnd.Text = $T.and; $lnkPrivacy.Text = $T.privacy
    }
    # 预览指定屏：校验进度页与完成页的排版
    switch ($PreviewScreen) {
        'working' {
            $txtPhase.Text = $T.phase2
            Show-Screen $screenWorking
        }
        'done' {
            $txtDoneTitle.Text = $T.doneTitle
            $txtDoneSub.Text = $T.doneSub + ' ' + (Join-Path $env:ProgramFiles $Title)
            $btnRun.Content = $T.runApp + '  ›'
            $btnFinish.Content = $T.finish
            Show-Screen $screenDone
        }
        'error' {
            $txtDoneTitle.Text = $T.errTitle
            $txtDoneSub.Text = $T.errSub
            $doneCircle.Fill = [System.Windows.Media.Brushes]::MistyRose
            $doneCheck.Data = 'M8,8 L22,22 M22,8 L8,22'
            $doneCheck.Stroke = [System.Windows.Media.Brushes]::IndianRed
            $btnRun.Visibility = 'Collapsed'
            $btnFinish.Content = $T.close
            Show-Screen $screenDone
        }
    }
    # 离屏显示以完成布局渲染（ShowInTaskbar 关 + 移出屏幕，用户不可见）
    $window.WindowStartupLocation = 'Manual'
    $window.ShowInTaskbar = $false
    $window.Left = -20000; $window.Top = -20000
    $null = $window.Show()
    $window.UpdateLayout()
    Start-Sleep -Milliseconds 300
    $rtb = New-Object System.Windows.Media.Imaging.RenderTargetBitmap([int]$window.Width, [int]$window.Height, 96, 96, [System.Windows.Media.PixelFormats]::Pbgra32)
    $rtb.Render($window)
    $encoder = New-Object System.Windows.Media.Imaging.PngBitmapEncoder
    $encoder.Frames.Add([System.Windows.Media.Imaging.BitmapFrame]::Create($rtb))
    $fs = [System.IO.File]::Create($PreviewPng)
    $encoder.Save($fs); $fs.Close()
    $window.Close()
    Write-Host "preview saved: $PreviewPng"
    exit 0
}

# ---- 行为 ----
$dragBar.Add_MouseLeftButtonDown({ $window.DragMove() })
$btnClose.Add_Click({ $window.Close() })

# 安装目录默认值：优先沿用上次安装位置
$defaultDir = Join-Path $env:ProgramFiles $Title
try {
    $reg = Get-ItemProperty 'HKLM:\Software\Fast-Https' -Name InstallDir -ErrorAction Stop
    if ($reg.InstallDir) { $defaultDir = $reg.InstallDir }
} catch { }

function Open-Link([string]$url) { Start-Process $url }

$exitCode = 0

if ($Uninstall) {
    # ---- 卸载：自绘确认 → 静默卸载 → 完成 ----
    $txtTitle.Text = $T.unTitle
    $txtTagline.Visibility = 'Collapsed'
    $txtBody.Text = $T.unBody
    $txtBody.Visibility = 'Visible'
    $txtBody2.Text = $T.unKeep
    $txtBody2.Visibility = 'Visible'
    $btnPrimary.Content = $T.unBtn
    $btnPrimary.Background = [System.Windows.Media.Brushes]::IndianRed
    $rowPath.Visibility = 'Collapsed'
    $rowAgree.Visibility = 'Collapsed'
    $btnClose.Add_Click({ $script:exitCode = 1; $window.Close() })

    $btnPrimary.Add_Click({
        $txtPhase.Text = $T.uninstalling
        Show-Screen $screenWorking
        $window.UpdateLayout()
        try {
            $p = Start-Process -FilePath $Self -ArgumentList '/S' -PassThru -Wait
            $script:exitCode = $p.ExitCode
        } catch { $script:exitCode = 1 }
        if ($script:exitCode -eq 0) {
            $txtDoneTitle.Text = $T.unDoneTitle
            $txtDoneSub.Text = $T.unDoneSub
            $doneCheck.Data = 'M6,15 L12,21 L24,8'
            $btnRun.Visibility = 'Collapsed'
            $btnFinish.Content = $T.finish
            Show-Screen $screenDone
        } else {
            $txtDoneTitle.Text = $T.errTitle
            $txtDoneSub.Text = $T.errSub
            $doneCircle.Fill = [System.Windows.Media.Brushes]::MistyRose
            $doneCheck.Data = 'M8,8 L22,22 M22,8 L8,22'
            $doneCheck.Stroke = [System.Windows.Media.Brushes]::IndianRed
            $btnRun.Visibility = 'Collapsed'
            $btnFinish.Content = $T.close
            Show-Screen $screenDone
        }
    })
} else {
    # ---- 安装：欢迎（目录 + 静议）→ 静默安装 → 完成 ----
    $txtTitle.Text = if ($Version) { "$Title $Version" } else { $Title }
    $txtTagline.Text = $T.tagline
    $btnPrimary.Content = $T.install + '  ›'
    $txtInstallTo.Text = $T.installTo
    $txtPath.Text = $defaultDir
    $btnBrowse.Content = '…'
    $txtAgree.Text = $T.agree
    $lnkTerms.Text = $T.terms
    $txtAnd.Text = $T.and
    $lnkPrivacy.Text = $T.privacy
    $lnkTerms.Add_Click({ Open-Link 'https://gitee.com/ncepu-bj/fast-https' })
    $lnkPrivacy.Add_Click({ Open-Link 'https://gitee.com/ncepu-bj/fast-https' })
    $btnBrowse.Add_Click({
        $dlg = New-Object System.Windows.Forms.FolderBrowserDialog
        if ($dlg.ShowDialog() -eq 'OK') { $txtPath.Text = $dlg.SelectedPath }
    })

    $btnPrimary.Add_Click({
        $dest = $txtPath.Text.Trim()
        if (-not $dest) { return }
        $txtPhase.Text = $T.phase1
        Show-Screen $screenWorking
        $window.UpdateLayout()
        $timer = [System.Windows.Threading.DispatcherTimer]::new()
        $script:phase = 0
        $timer.Interval = [TimeSpan]::FromMilliseconds(700)
        $timer.Add_Tick({
            $script:phase++
            if ($script:phase -eq 1) { $txtPhase.Text = $T.phase2 }
            elseif ($script:phase -ge 2) { $txtPhase.Text = $T.phase3 }
        })
        $timer.Start()
        try {
            $p = Start-Process -FilePath $Self -ArgumentList '/S', ('/D=' + $dest) -PassThru -Wait
            $script:exitCode = $p.ExitCode
        } catch { $script:exitCode = 1 }
        $timer.Stop()
        if ($script:exitCode -eq 0) {
            $txtDoneTitle.Text = $T.doneTitle
            $txtDoneSub.Text = $T.doneSub + ' ' + $dest
            $btnRun.Content = $T.runApp + '  ›'
            $btnFinish.Content = $T.finish
            Show-Screen $screenDone
        } else {
            $txtDoneTitle.Text = $T.errTitle
            $txtDoneSub.Text = $T.errSub
            $doneCircle.Fill = [System.Windows.Media.Brushes]::MistyRose
            $doneCheck.Data = 'M8,8 L22,22 M22,8 L8,22'
            $doneCheck.Stroke = [System.Windows.Media.Brushes]::IndianRed
            $btnRun.Visibility = 'Collapsed'
            $btnFinish.Content = $T.close
            Show-Screen $screenDone
        }
    })
    $btnFinish.Add_Click({ $window.Close() })
    $btnRun.Add_Click({
        $exe = Join-Path $txtPath.Text.Trim() 'fast-https-desktop.exe'
        if (Test-Path $exe) { Start-Process $exe }
        else { Start-Process $txtPath.Text.Trim() }
        $window.Close()
    })
}

$null = $window.ShowDialog()
exit $exitCode
