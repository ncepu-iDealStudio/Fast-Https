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
    [string]$Dir = "",           # 安装目录（卸载向导用：进度轮询与 PATH 操作目标）
    [switch]$Uninstall,
    [switch]$SelfTest,           # 只加载界面与命名元素后退出，供无头自检
    [string]$PreviewPng = "",    # 把界面无头渲染成 PNG（调样式用）
    [string]$PreviewScreen = "welcome",  # welcome | working | done | error
    [switch]$PreviewExpand,      # 预览时展开「自定义安装」区（看展开态排版）
    [string]$AutoInstall = "",   # 无头端到端测试：跳过界面直接按指定目录安装
    [string]$PathMode = ""       # 无头环境操作：add | remove（把 $Dir 加进/移出系统 PATH）
)

$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName PresentationFramework, WindowsBase, PresentationCore, System.Windows.Forms

# 诊断：参数绑定快照。只写诊断文件不进 stdout（向导以无窗口模式运行，
# stdout 没人看，写文件才可在排障时拿到）。
if ($env:TEMP) {
    "Self=[$Self] Icon=[$Icon] Version=[$Version] Uninstall=$Uninstall AutoInstall=[$AutoInstall] PathMode=[$PathMode] Dir=[$Dir]" |
        Out-File (Join-Path $env:TEMP "fast-https-wizard-diag.txt") -ErrorAction SilentlyContinue
}

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
        uacTitle = '需要管理员权限'
        uacBody = '安装到 Program Files 需要管理员授权。即将弹出 UAC 确认框，请选择「是」。'
        uacDenied = '未获得管理员授权，安装已取消。'
        optDesktop = '创建桌面快捷方式'
        optStartMenu = '添加到开始菜单'
        optPath = '加入系统 PATH（可在任意终端直接运行 fast-https）'
        customize = '自定义安装 ▾'
        customizeOn = '收起 ▴'
        starLead = '如果 Fast-Https 帮到了你，请在仓库点个 Star——这对我们意义重大'
        starBtn = 'Star'
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
        uacTitle = 'Administrator permission required'
        uacBody = 'Installing into Program Files needs admin approval. A UAC prompt will appear — please click Yes.'
        uacDenied = 'Administrator permission was denied; installation cancelled.'
        optDesktop = 'Create desktop shortcut'
        optStartMenu = 'Add to Start menu'
        optPath = 'Add to system PATH (run fast-https from any terminal)'
        customize = 'Customize install ▾'
        customizeOn = 'Collapse ▴'
        starLead = 'If Fast-Https helps you, a GitHub star means a lot'
        starBtn = 'Star'
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
        Width="560" SizeToContent="Height" WindowStartupLocation="CenterScreen"
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
        <!-- 自定义安装：默认收起，点「自定义安装」展开路径与选项 -->
        <Button x:Name="lnkCustomize" HorizontalAlignment="Center" Margin="0,16,0,0" Cursor="Hand"
                BorderThickness="0" Background="Transparent" Foreground="#8A93A0" FontSize="11">
          <Button.Template>
            <ControlTemplate TargetType="Button">
              <Border Background="{TemplateBinding Background}">
                <ContentPresenter HorizontalAlignment="Center" VerticalAlignment="Center"/>
              </Border>
            </ControlTemplate>
          </Button.Template>
          <Run x:Name="runCustomize" Text="自定义安装 ▾"/>
        </Button>
        <StackPanel x:Name="panelCustom" Visibility="Collapsed" HorizontalAlignment="Center">
          <StackPanel Orientation="Horizontal" HorizontalAlignment="Center" Margin="0,12,0,0" x:Name="rowPath">
            <TextBlock x:Name="txtInstallTo" FontSize="11.5" Foreground="#666C75" VerticalAlignment="Center"/>
            <TextBox x:Name="txtPath" Width="290" Height="30" Margin="8,0,0,0" FontSize="11.5" Foreground="#16181D"
                     BorderThickness="0" VerticalContentAlignment="Center" Padding="10,0">
              <TextBox.Background><SolidColorBrush Color="#F2F4F8"/></TextBox.Background>
            </TextBox>
            <Button x:Name="btnBrowse" Height="30" Margin="6,0,0,0" Padding="10,0" Cursor="Hand" FontSize="11.5"
                    Background="Transparent" BorderBrush="#D7DBE1" BorderThickness="1" Content="…"/>
          </StackPanel>
          <StackPanel x:Name="rowOptions" HorizontalAlignment="Center" Margin="0,10,0,0">
            <!-- 圆点选择行：PowerShell 侧管状态（optDesktop 等三个 $script: 布尔），
                 按钮文字带 ●/○ 前缀。不用 CheckBox 模板——ControlTemplate.Triggers
                 在 XamlReader::Parse 下间歇性解析失败（实测），按钮 Click 是本脚本
                 唯一验证过可靠的事件通道 -->
            <Button x:Name="chkDesktop" HorizontalAlignment="Left" Margin="0,0,0,6" Cursor="Hand"
                    BorderThickness="0" Background="Transparent" Foreground="#666C75" FontSize="12"
                    HorizontalContentAlignment="Left">
              <Button.Template>
                <ControlTemplate TargetType="Button">
                  <Border Background="{TemplateBinding Background}">
                    <ContentPresenter HorizontalAlignment="Left" VerticalAlignment="Center"/>
                  </Border>
                </ControlTemplate>
              </Button.Template>
            </Button>
            <Button x:Name="chkStartMenu" HorizontalAlignment="Left" Margin="0,0,0,6" Cursor="Hand"
                    BorderThickness="0" Background="Transparent" Foreground="#666C75" FontSize="12"
                    HorizontalContentAlignment="Left">
              <Button.Template>
                <ControlTemplate TargetType="Button">
                  <Border Background="{TemplateBinding Background}">
                    <ContentPresenter HorizontalAlignment="Left" VerticalAlignment="Center"/>
                  </Border>
                </ControlTemplate>
              </Button.Template>
            </Button>
            <Button x:Name="chkPath" HorizontalAlignment="Left" Cursor="Hand"
                    BorderThickness="0" Background="Transparent" Foreground="#666C75" FontSize="12"
                    HorizontalContentAlignment="Left">
              <Button.Template>
                <ControlTemplate TargetType="Button">
                  <Border Background="{TemplateBinding Background}">
                    <ContentPresenter HorizontalAlignment="Left" VerticalAlignment="Center"/>
                  </Border>
                </ControlTemplate>
              </Button.Template>
            </Button>
          </StackPanel>
        </StackPanel>
        <!-- 开源推荐：Star 引导（文字后跟一颗小按钮，中性色） -->
        <StackPanel Orientation="Horizontal" HorizontalAlignment="Center" Margin="0,10,0,0" x:Name="rowGithub">
          <TextBlock FontSize="10.5" Foreground="#949AA3" VerticalAlignment="Center"><Run x:Name="runGithubLead" Text=""/></TextBlock>
          <Button x:Name="btnStar" Height="24" MinWidth="64" Cursor="Hand" Margin="8,0,0,0" Padding="10,0"
                  BorderThickness="0" Foreground="#FFFFFF" FontSize="10.5" FontWeight="SemiBold" Content="">
            <Button.Background>
              <SolidColorBrush Color="#5A626E"/>
            </Button.Background>
            <Button.Template>
              <ControlTemplate TargetType="Button">
                <Border x:Name="bd" CornerRadius="12" Background="{TemplateBinding Background}" Padding="{TemplateBinding Padding}">
                  <StackPanel Orientation="Horizontal" HorizontalAlignment="Center" VerticalAlignment="Center">
                    <TextBlock Text="★" FontSize="10" Margin="0,0,4,0" Foreground="#FFF7DF"/>
                    <ContentPresenter VerticalAlignment="Center"/>
                  </StackPanel>
                </Border>
                <ControlTemplate.Triggers>
                  <Trigger Property="IsMouseOver" Value="True"><Setter TargetName="bd" Property="Opacity" Value="0.85"/></Trigger>
                  <Trigger Property="IsPressed" Value="True"><Setter TargetName="bd" Property="Opacity" Value="0.7"/></Trigger>
                </ControlTemplate.Triggers>
              </ControlTemplate>
            </Button.Template>
          </Button>
        </StackPanel>
      </StackPanel>

      <!-- 进度 -->
      <StackPanel x:Name="screenWorking" VerticalAlignment="Center" Visibility="Collapsed">
        <Image x:Name="imgLogo2" Width="64" Height="64" HorizontalAlignment="Center" RenderOptions.BitmapScalingMode="HighQuality"/>
        <TextBlock x:Name="txtPhase" FontSize="13" Foreground="#666C75" HorizontalAlignment="Center" Margin="0,20,0,0"/>
        <ProgressBar x:Name="barWork" Width="280" Height="6" Margin="0,18,0,0" Minimum="0" Maximum="100"/>
        <TextBlock x:Name="txtPct" FontSize="11" Foreground="#949AA3" HorizontalAlignment="Center" Margin="0,8,0,0"/>
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
$rowAgree = N 'rowAgree'; $txtAgree = N 'txtAgree'
$lnkCustomize = N 'lnkCustomize'; $runCustomize = N 'runCustomize'; $panelCustom = N 'panelCustom'
$rowGithub = N 'rowGithub'; $runGithubLead = N 'runGithubLead'; $btnStar = N 'btnStar'
$rowOptions = N 'rowOptions'; $chkDesktop = N 'chkDesktop'; $chkStartMenu = N 'chkStartMenu'; $chkPath = N 'chkPath'
$txtPct = N 'txtPct'
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

# 无头自检的元素清单与初始化调用放在函数定义之后（脚本尾部），
# 这里只提前声明：升级版自检见文件末尾的 if ($SelfTest) 块。

# ---- 圆点选项状态与渲染（预览块与主流程共用，定义在两者之前）----
$script:optMap = @{}

function Paint-Options {
    $chkDesktop.Content   = "$(if ($script:optMap.desktop) {'●'} else {'○'})  " + $T.optDesktop
    $chkStartMenu.Content = "$(if ($script:optMap.startmenu) {'●'} else {'○'})  " + $T.optStartMenu
    $chkPath.Content      = "$(if ($script:optMap.addpath) {'●'} else {'○'})  " + $T.optPath
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
        $rowPath.Visibility = 'Collapsed'; $rowGithub.Visibility = 'Collapsed'
        $lnkCustomize.Visibility = 'Collapsed'; $panelCustom.Visibility = 'Collapsed'
    } else {
        $txtTagline.Text = $T.tagline
        $btnPrimary.Content = $T.install + '  ›'
        $txtInstallTo.Text = $T.installTo
        $txtPath.Text = (Join-Path $env:ProgramFiles $Title)
        $script:optMap = @{ desktop = $true; startmenu = $true; addpath = $true }
        Paint-Options
        $runGithubLead.Text = $T.starLead
        $btnStar.Content = $T.starBtn
        if ($PreviewExpand) { $panelCustom.Visibility = 'Visible' }
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

# 无头端到端：与「安装」按钮走完全相同的引擎调用（Run-Engine，含提权），
# 供守卫测试与排障用，不显示界面。已是管理员时 RunAs 不弹窗直接执行。
if ($AutoInstall) {
    $dest = $AutoInstall
    Write-Host ("engine: " + $Self + " /S /D=" + $dest)
    try {
        $code = Run-Engine $dest
    } catch {
        Write-Error ("engine failed to start: " + $_.Exception.Message)
        $code = -1
    }
    Write-Host ("engine exit code: " + $code)
    exit $code
}

# 无头 PATH 操作在函数定义之后执行（见文件尾部的 if ($PathMode) 块）。

# Run-Engine 调起静默引擎（写操作需要管理员：RunAs 触发 UAC，仅弹一次）。
# extra 是向导传给引擎的附加开关（如快捷方式选项 /DSC_DESKTOP=0）。
# 返回引擎退出码；用户拒绝 UAC 时返回 1223（Windows 标准码）。
function Run-Engine([string]$dest, [string[]]$extra = @()) {
    $argList = @('/S')
    if ($dest) { $argList += ('/D=' + $dest) }
    if ($extra) { $argList += $extra }
    try {
        $p = Start-Process -FilePath $Self -ArgumentList $argList -Verb RunAs -PassThru -Wait
        return $p.ExitCode
    } catch {
        # 用户在 UAC 弹窗点了「否」：Win32Exception 里带 1223（ERROR_CANCELLED）
        if ($_.Exception.InnerException -and $_.Exception.InnerException.ErrorCode -eq 1223) { return 1223 }
        if ($_.Exception.ErrorCode -ne $null -and $_.Exception.ErrorCode -eq 1223) { return 1223 }
        throw
    }
}

# Update-MachinePath 把 $dir 加入（mode=add）或移出（mode=remove）系统
# PATH（HKLM 环境，写后广播 WM_SETTINGCHANGE 让新开的终端立刻可见）。
# 幂等：add 时不重复、remove 时不存在即原样返回。
# 调用方保证管理员身份（安装/卸载引擎已提权，PathMode 复用同一令牌）。
function Update-MachinePath([string]$dir, [string]$mode) {
    $key = 'HKLM:\SYSTEM\CurrentControlSet\Control\Session Manager\Environment'
    $cur = (Get-ItemProperty $key -Name Path -ErrorAction Stop).Path
    $parts = $cur -split ';' | Where-Object { $_ -ne '' }
    $has = $parts | Where-Object { $_.TrimEnd('\') -ieq $dir.TrimEnd('\') }
    if ($mode -eq 'add') {
        if ($has) { return }
        $new = ($parts + $dir.TrimEnd('\')) -join ';'
    } else {
        if (-not $has) { return }
        $new = ($parts | Where-Object { $_.TrimEnd('\') -ine $dir.TrimEnd('\') }) -join ';'
    }
    Set-ItemProperty $key -Name Path -Value $new -Type ExpandString
    # 广播环境变更：新开的终端/进程立即拿到新 PATH，不用注销重登
    Add-Type -Namespace Win32 -Name NativeMethods -MemberDefinition `
        '[DllImport("user32.dll", SetLastError = true, CharSet = CharSet.Auto)] public static extern IntPtr SendMessageTimeout(IntPtr hWnd, uint Msg, UIntPtr wParam, string lParam, uint fuFlags, uint uTimeout, out UIntPtr lpdwResult);'
    $result = [UIntPtr]::Zero
    [Win32.NativeMethods]::SendMessageTimeout(0xffff, 0x001A, [UIntPtr]::Zero, 'Environment', 2, 5000, [ref]$result) | Out-Null
}

# Watch-Install 进度轮询：install 模式看目标目录文件数递增（对数曲线
# 5%→92%），uninstall 模式看安装目录文件数递减（从初始计数往 0 走）。
# 都由引擎完成后的 100% 收尾。返回计时器，调用方 Stop。
function Watch-Install([string]$dir, [bool]$uninstall) {
    $barWork.Value = 5; $txtPct.Text = '5%'
    $script:baseCount = 0
    if ($uninstall) {
        $script:baseCount = (Get-ChildItem $dir -Recurse -File -ErrorAction SilentlyContinue | Measure-Object).Count
    }
    $watch = [System.Windows.Threading.DispatcherTimer]::new()
    $watch.Interval = [TimeSpan]::FromMilliseconds(250)
    $watch.Add_Tick({
        try {
            if (Test-Path $dir) {
                $n = (Get-ChildItem $dir -Recurse -File -ErrorAction SilentlyContinue | Measure-Object).Count
                $pct = 5
                if ($uninstall -and $script:baseCount -gt 0) {
                    # 递减：清理掉的占比
                    $done = $script:baseCount - $n
                    $pct = [Math]::Min(92, [int](5 + 87 * ($done / $script:baseCount)))
                } elseif (-not $uninstall) {
                    # 递增：经验值完整安装约 200 个文件，对数曲线避免前快后停
                    $pct = [Math]::Min(92, [int](5 + 87 * [Math]::Log(1 + $n) / [Math]::Log(1 + 200)))
                }
                $barWork.Value = $pct
                $txtPct.Text = "$pct%"
            } elseif ($uninstall) {
                # 目录已被整体删掉：接近完成
                $barWork.Value = 95; $txtPct.Text = '95%'
            }
        } catch { }
    })
    $watch.Start()
    return $watch
}

function Show-Result([int]$code, [bool]$install, [string]$dest) {
    $ok = ($code -eq 0)
    $cancelled = ($code -eq 1223)
    if ($cancelled) {
        $txtDoneTitle.Text = if ($install) { $T.errTitle } else { $T.errTitle }
        $txtDoneSub.Text = $T.uacDenied
    } elseif ($ok) {
        if ($install) {
            $txtDoneTitle.Text = $T.doneTitle
            $txtDoneSub.Text = $T.doneSub + ' ' + $dest
            $btnRun.Content = $T.runApp + '  ›'
        } else {
            $txtDoneTitle.Text = $T.unDoneTitle
            $txtDoneSub.Text = $T.unDoneSub
        }
    } else {
        $txtDoneTitle.Text = $T.errTitle
        $txtDoneSub.Text = $T.errSub
    }
    if (-not ($ok -and $install)) { $btnRun.Visibility = 'Collapsed' } else { $btnRun.Visibility = 'Visible' }
    if ($ok) {
        $doneCircle.Fill = New-Object System.Windows.Media.SolidColorBrush([System.Windows.Media.Color]::FromRgb(0xEA, 0xF7, 0xF0))
        $doneCheck.Data = 'M6,15 L12,21 L24,8'
        $doneCheck.Stroke = [System.Windows.Media.Brushes]::ForestGreen
        $btnFinish.Content = $T.finish
    } else {
        $doneCircle.Fill = [System.Windows.Media.Brushes]::MistyRose
        $doneCheck.Data = 'M8,8 L22,22 M22,8 L8,22'
        $doneCheck.Stroke = [System.Windows.Media.Brushes]::IndianRed
        $btnFinish.Content = $T.close
    }
    Show-Screen $screenDone
}

# ---- 行为 ----
$dragBar.Add_MouseLeftButtonDown({ $window.DragMove() })
$btnClose.Add_Click({ $window.Close() })

# 安装目录默认值：市面标准是 C:\Program Files\<产品名>。
# 只在「上次安装目录真实存在且不在临时目录」时才沿用（升级重装体验），
# 防止测试/卸载残留的 Temp 路径被当成默认值（曾表现为默认装到 %TEMP%）。
$defaultDir = Join-Path $env:ProgramFiles $Title
try {
    $reg = Get-ItemProperty 'HKLM:\Software\Fast-Https' -Name InstallDir -ErrorAction Stop
    $prev = $reg.InstallDir
    $isTemp = $prev -and ($prev -like "$env:TEMP*" -or $prev -like "$env:TMP*" -or $prev -like "*\Temp\*")
    if ($prev -and (Test-Path $prev) -and -not $isTemp) { $defaultDir = $prev }
} catch { }

function Open-Link([string]$url) { Start-Process $url }

# 多显示器支持：窗口居中「相对它当前所在的显示器」。
# 之前用 SystemParameters.WorkArea 居中，那是主屏工作区——扩展屏上
# 展开/收起改变窗口高度一居中就跳回笔记本主屏（用户实测踩到）。
# 这里用 MonitorFromWindow 拿窗口所在屏，取该屏工作区居中；
# WPF 单位与物理像素按当前 DPI 换算。
Add-Type -Namespace Wizard -Name NativeDisplays -MemberDefinition @'
[DllImport("user32.dll")] public static extern IntPtr MonitorFromWindow(IntPtr hwnd, uint dwFlags);
[DllImport("user32.dll", CharSet = CharSet.Auto)] public static extern bool GetMonitorInfo(IntPtr hMonitor, ref MONITORINFO lpmi);
[DllImport("user32.dll")] public static extern uint GetDpiForWindow(IntPtr hwnd);
[StructLayout(LayoutKind.Sequential)] public struct RECT { public int Left, Top, Right, Bottom; }
[StructLayout(LayoutKind.Sequential)] public struct MONITORINFO {
  public int cbSize;
  public RECT rcMonitor;
  public RECT rcWork;
  public uint dwFlags;
}
'@

function Set-CenterToCurrentMonitor {
    try {
        # WindowInteropHelper 拿 Win32 句柄（窗口须已显示；未显示时跳过，
        # 让 WindowStartupLocation=CenterScreen 处理首次定位）
        $helper = New-Object System.Windows.Interop.WindowInteropHelper($window)
        if ($helper.Handle -eq [IntPtr]::Zero) { return }
        $MONITOR_DEFAULTTONEAREST = 2
        $hMon = [Wizard.NativeDisplays]::MonitorFromWindow($helper.Handle, $MONITOR_DEFAULTTONEAREST)
        if ($hMon -eq [IntPtr]::Zero) { return }
        $mi = New-Object Wizard.NativeDisplays+MONITORINFO
        $mi.cbSize = [System.Runtime.InteropServices.Marshal]::SizeOf($mi)
        if (-not [Wizard.NativeDisplays]::GetMonitorInfo($hMon, [ref]$mi)) { return }
        # 该屏工作区的物理像素
        $waW = $mi.rcWork.Right - $mi.rcWork.Left
        $waH = $mi.rcWork.Bottom - $mi.rcWork.Top
        # DPI 换算（Win10 1607+ 有 GetDpiForWindow；失败按 96 处理）
        $dpi = [Wizard.NativeDisplays]::GetDpiForWindow($helper.Handle)
        if ($dpi -eq 0) { $dpi = 96 }
        $scale = $dpi / 96.0
        $wpfW = $window.Width * $scale
        $wpfH = $window.Height * $scale
        $window.Left = $mi.rcWork.Left + (($waW - $wpfW) / 2) / $scale
        $window.Top = $mi.rcWork.Top + (($waH - $wpfH) / 2) / $scale
    } catch { }
}

# Initialize-UninstallScreen 卸载确认屏的文案与事件绑定。
function Initialize-UninstallScreen {
    $txtTitle.Text = $T.unTitle
    $txtTagline.Visibility = 'Collapsed'
    $txtBody.Text = $T.unBody
    $txtBody.Visibility = 'Visible'
    $txtBody2.Text = $T.unKeep
    $txtBody2.Visibility = 'Visible'
    $btnPrimary.Content = $T.unBtn
    $btnPrimary.Background = [System.Windows.Media.Brushes]::IndianRed
    $rowPath.Visibility = 'Collapsed'
    $lnkCustomize.Visibility = 'Collapsed'; $panelCustom.Visibility = 'Collapsed'
    $rowGithub.Visibility = 'Collapsed'

    $btnPrimary.Add_Click({
        $txtPhase.Text = $T.uninstalling
        Show-Screen $screenWorking
        $window.UpdateLayout()
        # 卸载真进度：轮询安装目录剩余文件数（递减），引擎清理完拉满
        $unDir = if ($Dir) { $Dir } else { Split-Path -Parent $Self }
        $watch = Watch-Install $unDir $true
        try {
            $code = Run-Engine $null
        } catch {
            $code = -1
        }
        $watch.Stop()
        $barWork.Value = 100; $txtPct.Text = '100%'
        Show-Result $code $false ''
    })
}

# Initialize-WelcomeScreen 安装欢迎屏的文案与事件绑定。
function Initialize-WelcomeScreen {
    $txtTitle.Text = if ($Version) { "$Title $Version" } else { $Title }
    $txtTagline.Text = $T.tagline
    $btnPrimary.Content = $T.install + '  ›'
    $txtInstallTo.Text = $T.installTo
    $txtPath.Text = $defaultDir
    $btnBrowse.Content = '…'
    $script:optMap = @{ desktop = $true; startmenu = $true; addpath = $true }
    Paint-Options
    $runGithubLead.Text = $T.starLead
    $btnStar.Content = $T.starBtn
    # 默认收起：点「自定义安装」展开路径与选项（窗口随之伸缩）
    $script:customOpen = $false
    $panelCustom.Visibility = 'Collapsed'
    $runCustomize.Text = $T.customize
    $lnkCustomize.Add_Click({
        "customize clicked: open=$script:customOpen" | Out-File (Join-Path $env:TEMP "fast-https-wizard-diag.txt") -Append
        $script:customOpen = -not $script:customOpen
        $panelCustom.Visibility = if ($script:customOpen) { 'Visible' } else { 'Collapsed' }
        $runCustomize.Text = if ($script:customOpen) { $T.customizeOn } else { $T.customize }
        # 窗口高度随内容伸缩；居中相对窗口当前所在显示器（多显示器不跳屏）
        Set-CenterToCurrentMonitor
    })
    # 圆点选项：点击切换状态并重绘（●=选中 ○=未选）
    $chkDesktop.Add_Click({
        $script:optMap.desktop = -not $script:optMap.desktop
        Paint-Options
    })
    $chkStartMenu.Add_Click({
        $script:optMap.startmenu = -not $script:optMap.startmenu
        Paint-Options
    })
    $chkPath.Add_Click({
        $script:optMap.addpath = -not $script:optMap.addpath
        Paint-Options
    })
    $btnStar.Add_Click({ Open-Link 'https://gitee.com/ncepu-bj/fast-https' })
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
        # 真进度：轮询目标目录文件数；引擎退出后拉满
        $watch = Watch-Install $dest $false
        try {
            $extra = @()
            if (-not $script:optMap.desktop) { $extra += '/DSC_DESKTOP=0' }
            if (-not $script:optMap.startmenu) { $extra += '/DSC_STARTMENU=0' }
            if (-not $script:optMap.addpath) { $extra += '/DSC_PATH=0' }
            $code = Run-Engine $dest $extra
        } catch {
            $code = -1
        }
        $watch.Stop()
        $barWork.Value = 100; $txtPct.Text = '100%'
        Show-Result $code $true $dest
    })
    $btnFinish.Add_Click({ $window.Close() })
    $btnRun.Add_Click({
        $exe = Join-Path $txtPath.Text.Trim() 'fast-https-desktop.exe'
        if (Test-Path $exe) { Start-Process $exe }
        else { Start-Process $txtPath.Text.Trim() }
        $window.Close()
    })
}

$exitCode = 0

if ($Uninstall) {
    # ---- 卸载：自绘确认 → 静默卸载 → 完成 ----
    Initialize-UninstallScreen
    $btnClose.Add_Click({ $script:exitCode = 1; $window.Close() })
} else {
    # ---- 安装：欢迎（目录 + 协议）→ 静默安装 → 完成 ----
    Initialize-WelcomeScreen
}

# ---- 无头自检（放在函数定义之后才能调 Initialize-*）----
# 元素存在 + 两种模式的事件绑定全跑一遍。曾发生过 TextBlock 绑 Click
# 在运行时才崩（向导弹不出来、nsExec 报错退出），只有真实初始化才能暴露。
if ($SelfTest) {
    $names = 'dragBar','btnClose','screenWelcome','screenWorking','screenDone','imgLogo','imgLogo2','txtTitle','txtTagline','txtBody','txtBody2','btnPrimary','btnSecondary','rowPath','txtInstallTo','txtPath','btnBrowse','lnkCustomize','runCustomize','panelCustom','rowGithub','runGithubLead','btnStar','rowOptions','chkDesktop','chkStartMenu','chkPath','txtPhase','barWork','txtPct','doneCircle','doneCheck','txtDoneTitle','txtDoneSub','btnRun','btnFinish'
    foreach ($n in $names) { if ($null -eq (N $n)) { Write-Error "selftest: missing element $n"; exit 1 } }
    if (-not $Self) { $script:Self = "selftest-fake.exe" }
    Initialize-WelcomeScreen      # 安装模式文案与事件绑定（崩溃在这里当场爆）
    Initialize-UninstallScreen    # 卸载模式文案与事件绑定
    Write-Host "selftest ok: wizard UI + bindings loaded"
    exit 0
}

# ---- 无头 PATH 操作（函数定义之后才能调 Update-MachinePath）----
# 由已提权的引擎（或测试）调用，把 $Dir 加进/移出系统 PATH。
if ($PathMode -and $Dir) {
    try {
        Update-MachinePath $Dir $PathMode
        Write-Host ("path " + $PathMode + " ok: " + $Dir)
        exit 0
    } catch {
        Write-Error ("path " + $PathMode + " failed: " + $_.Exception.Message)
        exit 1
    }
}

$null = $window.ShowDialog()
exit $exitCode





