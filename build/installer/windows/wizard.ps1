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
        workHint = '正在处理，请保持此窗口打开'
        uacTitle = '需要管理员权限'
        uacBody = '安装到 Program Files 需要管理员授权。即将弹出 UAC 确认框，请选择「是」。'
        uacDenied = '未获得管理员授权，操作已取消。'
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
        workHint = 'Working — please keep this window open'
        uacTitle = 'Administrator permission required'
        uacBody = 'Installing into Program Files needs admin approval. A UAC prompt will appear — please click Yes.'
        uacDenied = 'Administrator permission was denied; the operation was cancelled.'
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
# 样式集中在 Window.Resources：主按钮 / 次按钮 / 轻按钮 / 细进度条。
# 之前 ProgressBar 用系统默认模板（灰框 + Win8 绿色块），是整个界面
# 最扎眼的老气来源；这里换成 8px 圆角细轨道 + 石墨色指示条。
$xaml = @'
<Window xmlns="http://schemas.microsoft.com/winfx/2006/xaml/presentation"
        xmlns:x="http://schemas.microsoft.com/winfx/2006/xaml"
        WindowStyle="None" AllowsTransparency="True" Background="Transparent"
        Width="560" SizeToContent="Height" WindowStartupLocation="CenterScreen"
        ResizeMode="NoResize" ShowInTaskbar="True" FontFamily="Microsoft YaHei UI"
        UseLayoutRounding="True" TextOptions.TextFormattingMode="Display">
  <Window.Resources>
    <LinearGradientBrush x:Key="PrimaryFill" StartPoint="0,0" EndPoint="1,1">
      <GradientStop Color="#4F5763" Offset="0"/><GradientStop Color="#262C34" Offset="1"/>
    </LinearGradientBrush>
    <LinearGradientBrush x:Key="DangerFill" StartPoint="0,0" EndPoint="1,1">
      <GradientStop Color="#D4574E" Offset="0"/><GradientStop Color="#A9342C" Offset="1"/>
    </LinearGradientBrush>
    <SolidColorBrush x:Key="InkBrush" Color="#15181D"/>
    <SolidColorBrush x:Key="SubInkBrush" Color="#5B626C"/>
    <SolidColorBrush x:Key="MutedBrush" Color="#8A93A0"/>

    <ControlTemplate x:Key="PrimaryButton" TargetType="Button">
      <Border x:Name="bd" CornerRadius="23" Background="{TemplateBinding Background}">
        <ContentPresenter HorizontalAlignment="Center" VerticalAlignment="Center"/>
      </Border>
      <ControlTemplate.Triggers>
        <Trigger Property="IsMouseOver" Value="True"><Setter TargetName="bd" Property="Opacity" Value="0.90"/></Trigger>
        <Trigger Property="IsPressed" Value="True"><Setter TargetName="bd" Property="Opacity" Value="0.74"/></Trigger>
      </ControlTemplate.Triggers>
    </ControlTemplate>

    <ControlTemplate x:Key="GhostButton" TargetType="Button">
      <Border x:Name="bd" CornerRadius="20" Background="{TemplateBinding Background}"
              BorderBrush="#DCE1E7" BorderThickness="1">
        <ContentPresenter HorizontalAlignment="Center" VerticalAlignment="Center"/>
      </Border>
      <ControlTemplate.Triggers>
        <Trigger Property="IsMouseOver" Value="True">
          <Setter TargetName="bd" Property="Background" Value="#F3F5F8"/>
          <Setter TargetName="bd" Property="BorderBrush" Value="#CDD3DB"/>
        </Trigger>
        <Trigger Property="IsPressed" Value="True">
          <Setter TargetName="bd" Property="Background" Value="#E9ECF1"/>
        </Trigger>
      </ControlTemplate.Triggers>
    </ControlTemplate>

    <ControlTemplate x:Key="QuietButton" TargetType="Button">
      <Border x:Name="bd" Background="{TemplateBinding Background}" CornerRadius="8"
              Padding="{TemplateBinding Padding}">
        <ContentPresenter HorizontalAlignment="{TemplateBinding HorizontalContentAlignment}" VerticalAlignment="Center"/>
      </Border>
      <ControlTemplate.Triggers>
        <Trigger Property="IsMouseOver" Value="True"><Setter TargetName="bd" Property="Background" Value="#F1F3F6"/></Trigger>
        <Trigger Property="IsPressed" Value="True"><Setter TargetName="bd" Property="Background" Value="#E7EAEF"/></Trigger>
      </ControlTemplate.Triggers>
    </ControlTemplate>

    <Style x:Key="SlimProgress" TargetType="ProgressBar">
      <Setter Property="Template">
        <Setter.Value>
          <ControlTemplate TargetType="ProgressBar">
            <Grid>
              <Border x:Name="PART_Track" CornerRadius="4" Background="#EDF0F4"/>
              <Border x:Name="PART_Indicator" CornerRadius="4" HorizontalAlignment="Left"
                      Background="{TemplateBinding Foreground}"/>
            </Grid>
          </ControlTemplate>
        </Setter.Value>
      </Setter>
    </Style>
  </Window.Resources>
  <Border Background="#FFFFFF" CornerRadius="16" Margin="14">
    <Border.Effect><DropShadowEffect BlurRadius="30" ShadowDepth="0" Opacity="0.18"/></Border.Effect>
    <Grid>
      <Grid x:Name="dragBar" Height="48" VerticalAlignment="Top" Background="Transparent"/>
      <Button x:Name="btnClose" Width="32" Height="32" HorizontalAlignment="Right" VerticalAlignment="Top"
              Margin="0,10,10,0" Cursor="Hand" Template="{StaticResource QuietButton}"
              Background="Transparent" BorderThickness="0">
        <Path Data="M0,0 L8,8 M8,0 L0,8" Stroke="#9AA1AB" StrokeThickness="1.4"
              StrokeStartLineCap="Round" StrokeEndLineCap="Round" Stretch="None"
              HorizontalAlignment="Center" VerticalAlignment="Center"/>
      </Button>

      <!-- 欢迎 / 卸载确认 -->
      <StackPanel x:Name="screenWelcome" VerticalAlignment="Center" Margin="44,34,44,28">
        <Grid Width="96" Height="96" HorizontalAlignment="Center">
          <Ellipse Fill="#F5F7FA"/>
          <Image x:Name="imgLogo" Width="70" Height="70" RenderOptions.BitmapScalingMode="HighQuality"/>
        </Grid>
        <TextBlock x:Name="txtTitle" FontSize="21" FontWeight="SemiBold" Foreground="#15181D" HorizontalAlignment="Center" Margin="0,16,0,0"/>
        <TextBlock x:Name="txtTagline" FontSize="12" Foreground="#8A93A0" HorizontalAlignment="Center" Margin="0,7,0,0"/>
        <TextBlock x:Name="txtBody" FontSize="12.5" Foreground="#666C75" HorizontalAlignment="Center" TextAlignment="Center" TextWrapping="Wrap" Width="420" Margin="0,14,0,0" Visibility="Collapsed"/>
        <TextBlock x:Name="txtBody2" FontSize="11" Foreground="#949AA3" HorizontalAlignment="Center" TextAlignment="Center" TextWrapping="Wrap" Width="420" Margin="0,8,0,0" Visibility="Collapsed"/>
        <Button x:Name="btnPrimary" Width="208" Height="46" Cursor="Hand" HorizontalAlignment="Center" Margin="0,24,0,0"
                Template="{StaticResource PrimaryButton}" Background="{StaticResource PrimaryFill}"
                BorderThickness="0" Foreground="#FFFFFF" FontSize="14.5" FontWeight="SemiBold" Content="Install"/>
        <Button x:Name="btnSecondary" Width="208" Height="40" Cursor="Hand" HorizontalAlignment="Center" Margin="0,10,0,0"
                Template="{StaticResource GhostButton}" Background="Transparent" BorderThickness="0"
                Foreground="#5B626C" FontSize="12.5" Content="" Visibility="Collapsed"/>
        <!-- 自定义安装：默认收起，点「自定义安装」展开路径与选项 -->
        <Button x:Name="lnkCustomize" HorizontalAlignment="Center" Margin="0,14,0,0" Cursor="Hand"
                Template="{StaticResource QuietButton}" Background="Transparent" BorderThickness="0"
                Foreground="#8A93A0" FontSize="11" Padding="10,5">
          <Run x:Name="runCustomize" Text="自定义安装 ▾"/>
        </Button>
        <Border x:Name="panelCustom" Visibility="Collapsed" Background="#F7F8FA" BorderBrush="#EEF1F5"
                BorderThickness="1" CornerRadius="12" Padding="14,12" Margin="0,12,0,0">
          <StackPanel>
            <StackPanel Orientation="Horizontal" HorizontalAlignment="Center" x:Name="rowPath">
              <TextBlock x:Name="txtInstallTo" FontSize="11.5" Foreground="#666C75" VerticalAlignment="Center"/>
              <TextBox x:Name="txtPath" Width="280" Height="32" Margin="8,0,0,0" FontSize="11.5" Foreground="#15181D"
                       BorderThickness="0" VerticalContentAlignment="Center">
                <TextBox.Template>
                  <ControlTemplate TargetType="TextBox">
                    <Border x:Name="bd" CornerRadius="8" Background="#FFFFFF" BorderBrush="#DFE4EA" BorderThickness="1">
                      <ScrollViewer x:Name="PART_ContentHost" Margin="9,0" VerticalAlignment="Center"/>
                    </Border>
                    <ControlTemplate.Triggers>
                      <Trigger Property="IsKeyboardFocused" Value="True">
                        <Setter TargetName="bd" Property="BorderBrush" Value="#8A93A0"/>
                      </Trigger>
                    </ControlTemplate.Triggers>
                  </ControlTemplate>
                </TextBox.Template>
              </TextBox>
              <Button x:Name="btnBrowse" Width="34" Height="32" Margin="8,0,0,0" Cursor="Hand" FontSize="12"
                      Template="{StaticResource QuietButton}" Background="#FFFFFF" BorderThickness="0"
                      Foreground="#5B626C" Content="…"/>
            </StackPanel>
            <StackPanel x:Name="rowOptions" Margin="0,12,0,0">
              <!-- 圆点选择行：PowerShell 侧管状态（optDesktop 等三个 $script: 布尔），
                   按钮文字带 ●/○ 前缀。不用 CheckBox 模板——ControlTemplate.Triggers
                   在 XamlReader::Parse 下间歇性解析失败（实测），按钮 Click 是本脚本
                   唯一验证过可靠的事件通道 -->
              <Button x:Name="chkDesktop" HorizontalAlignment="Left" Margin="0,0,0,7" Cursor="Hand"
                      Template="{StaticResource QuietButton}" Background="Transparent" BorderThickness="0"
                      Foreground="#5B626C" FontSize="12" Padding="8,3" HorizontalContentAlignment="Left"/>
              <Button x:Name="chkStartMenu" HorizontalAlignment="Left" Margin="0,0,0,7" Cursor="Hand"
                      Template="{StaticResource QuietButton}" Background="Transparent" BorderThickness="0"
                      Foreground="#5B626C" FontSize="12" Padding="8,3" HorizontalContentAlignment="Left"/>
              <Button x:Name="chkPath" HorizontalAlignment="Left" Cursor="Hand"
                      Template="{StaticResource QuietButton}" Background="Transparent" BorderThickness="0"
                      Foreground="#5B626C" FontSize="12" Padding="8,3" HorizontalContentAlignment="Left"/>
            </StackPanel>
          </StackPanel>
        </Border>
        <!-- 开源推荐：Star 引导（文字后跟一颗小按钮，中性色） -->
        <StackPanel Orientation="Horizontal" HorizontalAlignment="Center" Margin="0,14,0,0" x:Name="rowGithub">
          <TextBlock FontSize="10.5" Foreground="#949AA3" VerticalAlignment="Center"><Run x:Name="runGithubLead" Text=""/></TextBlock>
          <Button x:Name="btnStar" Height="24" MinWidth="64" Cursor="Hand" Margin="8,0,0,0" Padding="10,0"
                  BorderThickness="0" Foreground="#FFFFFF" FontSize="10.5" FontWeight="SemiBold" Content="">
            <Button.Background>
              <SolidColorBrush Color="#5B626C"/>
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
      <StackPanel x:Name="screenWorking" VerticalAlignment="Center" Visibility="Collapsed" Margin="44,44,44,38">
        <Grid Width="76" Height="76" HorizontalAlignment="Center">
          <Ellipse Fill="#F5F7FA"/>
          <Image x:Name="imgLogo2" Width="54" Height="54" RenderOptions.BitmapScalingMode="HighQuality"/>
        </Grid>
        <TextBlock x:Name="txtPhase" FontSize="13.5" Foreground="#3C424B" HorizontalAlignment="Center" Margin="0,20,0,0"/>
        <ProgressBar x:Name="barWork" Style="{StaticResource SlimProgress}" Width="320" Height="8" Margin="0,18,0,0"
                     Minimum="0" Maximum="100" Value="0" Foreground="#2F353D" Background="#EDF0F4"/>
        <TextBlock x:Name="txtPct" FontSize="11.5" Foreground="#8A93A0" HorizontalAlignment="Center" Margin="0,9,0,0" Text="0%"/>
        <TextBlock x:Name="txtWorkHint" FontSize="10.5" Foreground="#A9B0BA" HorizontalAlignment="Center" Margin="0,3,0,0"/>
      </StackPanel>

      <!-- 完成 / 错误 -->
      <StackPanel x:Name="screenDone" VerticalAlignment="Center" Visibility="Collapsed" Margin="44,36,44,30">
        <Ellipse x:Name="doneCircle" Width="72" Height="72" HorizontalAlignment="Center">
          <Ellipse.Fill><SolidColorBrush Color="#EAF7F0"/></Ellipse.Fill>
        </Ellipse>
        <Path x:Name="doneCheck" Width="30" Height="30" HorizontalAlignment="Center" Margin="0,-51,0,0"
              Data="M6,15 L12,21 L24,8" Stroke="#16794A" StrokeThickness="3" StrokeStartLineCap="Round" StrokeEndLineCap="Round" StrokeLineJoin="Round"/>
        <TextBlock x:Name="txtDoneTitle" FontSize="20" FontWeight="SemiBold" Foreground="#15181D" HorizontalAlignment="Center" Margin="0,18,0,0"/>
        <TextBlock x:Name="txtDoneSub" FontSize="12" Foreground="#8A93A0" HorizontalAlignment="Center" Margin="0,8,0,0" TextAlignment="Center" TextWrapping="Wrap" MaxWidth="380"/>
        <Button x:Name="btnRun" Width="236" Height="46" Cursor="Hand" HorizontalAlignment="Center" Margin="0,22,0,0"
                Template="{StaticResource PrimaryButton}" Background="{StaticResource PrimaryFill}"
                BorderThickness="0" Foreground="#FFFFFF" FontSize="14" FontWeight="SemiBold" Content=""/>
        <Button x:Name="btnFinish" Width="236" Height="40" Cursor="Hand" HorizontalAlignment="Center" Margin="0,10,0,0"
                Template="{StaticResource GhostButton}" Background="Transparent" BorderThickness="0"
                Foreground="#5B626C" FontSize="12.5" Content=""/>
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
$lnkCustomize = N 'lnkCustomize'; $runCustomize = N 'runCustomize'; $panelCustom = N 'panelCustom'
$rowGithub = N 'rowGithub'; $runGithubLead = N 'runGithubLead'; $btnStar = N 'btnStar'
$rowOptions = N 'rowOptions'; $chkDesktop = N 'chkDesktop'; $chkStartMenu = N 'chkStartMenu'; $chkPath = N 'chkPath'
$txtPct = N 'txtPct'
$txtPhase = N 'txtPhase'; $barWork = N 'barWork'; $txtWorkHint = N 'txtWorkHint'
$doneCircle = N 'doneCircle'; $doneCheck = N 'doneCheck'; $txtDoneTitle = N 'txtDoneTitle'; $txtDoneSub = N 'txtDoneSub'
$btnRun = N 'btnRun'; $btnFinish = N 'btnFinish'

# 资源刷子：选项行选中/未选中、完成页按钮换装都从这里取，避免散落硬编码
$script:inkBrush     = $window.FindResource('InkBrush')
$script:subInkBrush  = $window.FindResource('SubInkBrush')
$script:mutedBrush   = $window.FindResource('MutedBrush')
$script:primaryFill  = $window.FindResource('PrimaryFill')
$script:primaryTpl   = $window.FindResource('PrimaryButton')
$script:ghostTpl     = $window.FindResource('GhostButton')

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
    # 选中项用墨色，未选中用弱化灰：比纯文字更像「可勾选项」
    $chkDesktop.Foreground   = if ($script:optMap.desktop)   { $script:inkBrush } else { $script:mutedBrush }
    $chkStartMenu.Foreground = if ($script:optMap.startmenu) { $script:inkBrush } else { $script:mutedBrush }
    $chkPath.Foreground      = if ($script:optMap.addpath)   { $script:inkBrush } else { $script:mutedBrush }
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
        $btnPrimary.Background = $window.FindResource('DangerFill')
        $rowPath.Visibility = 'Collapsed'; $rowGithub.Visibility = 'Collapsed'
        $lnkCustomize.Visibility = 'Collapsed'; $panelCustom.Visibility = 'Collapsed'
    } else {
        $txtTagline.Text = $T.tagline
        $btnPrimary.Content = $T.install
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
            $barWork.Value = 46; $txtPct.Text = '46%'
            $txtWorkHint.Text = $T.workHint
            Show-Screen $screenWorking
        }
        'done' {
            $txtDoneTitle.Text = $T.doneTitle
            $txtDoneSub.Text = $T.doneSub + ' ' + (Join-Path $env:ProgramFiles $Title)
            $btnRun.Content = $T.runApp
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
            # 错误页只有「关闭」一个动作：升为主按钮，别让它像脚注
            $btnFinish.Template = $script:primaryTpl
            $btnFinish.Background = $script:primaryFill
            $btnFinish.Foreground = [System.Windows.Media.Brushes]::White
            $btnFinish.FontSize = 14; $btnFinish.FontWeight = 'SemiBold'
            $btnFinish.Height = 46; $btnFinish.Width = 236; $btnFinish.Margin = '0,22,0,0'
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

# New-EngineProcess 以管理员身份拉起静默引擎（写操作需要管理员：RunAs
# 触发一次 UAC），立即返回进程对象、不等待——等待交给调用方。之前在本
# 函数里 -Wait 会把 WPF 的 Dispatcher 线程堵死，整个安装期间窗口无法
# 重绘、计时器也不跳，进度条就停在 5% 像个假死程序。
# extra 是向导传给引擎的附加开关（如快捷方式选项 /DSC_DESKTOP=0）。
# 用户拒绝 UAC 时返回 $null（ERROR_CANCELLED=1223）。
function New-EngineProcess([string]$dest, [string[]]$extra = @()) {
    $argList = @('/S')
    if ($dest) { $argList += ('/D=' + $dest) }
    if ($extra) { $argList += $extra }
    try {
        return Start-Process -FilePath $Self -ArgumentList $argList -Verb RunAs -PassThru
    } catch {
        if ($_.Exception.InnerException -and $_.Exception.InnerException.ErrorCode -eq 1223) { return $null }
        if ($_.Exception.ErrorCode -ne $null -and $_.Exception.ErrorCode -eq 1223) { return $null }
        throw
    }
}

# Run-Engine 同步版：只给无头端到端自检（-AutoInstall）用，返回退出码。
function Run-Engine([string]$dest, [string[]]$extra = @()) {
    $p = New-EngineProcess $dest $extra
    if ($null -eq $p) { return 1223 }
    $p.WaitForExit()
    try { return $p.ExitCode } catch { return -1 }
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

# Set-FinishStyle 完成页操作按钮的排版：
# - 有「运行」主按钮时，「完成」是描边次按钮，不跟主按钮抢视觉重心；
# - 没有其它动作（卸载完成 / 安装失败）时，「完成 / 关闭」升为主按钮。
# 顺带修掉卸载完成页的坑：之前「完成」只是普通文字、Click 又只绑在
# 安装分支，用户点了完全没反应。
function Set-FinishStyle([bool]$primary) {
    if ($primary) {
        $btnFinish.Template = $script:primaryTpl
        $btnFinish.Background = $script:primaryFill
        $btnFinish.Foreground = [System.Windows.Media.Brushes]::White
        $btnFinish.FontSize = 14; $btnFinish.FontWeight = 'SemiBold'
        $btnFinish.Width = 236; $btnFinish.Height = 46; $btnFinish.Margin = '0,22,0,0'
    } else {
        $btnFinish.Template = $script:ghostTpl
        $btnFinish.Background = [System.Windows.Media.Brushes]::Transparent
        $btnFinish.Foreground = $script:subInkBrush
        $btnFinish.FontSize = 12.5; $btnFinish.FontWeight = 'Normal'
        $btnFinish.Width = 236; $btnFinish.Height = 40; $btnFinish.Margin = '0,10,0,0'
    }
}

# Begin-Work 平滑进度 + 非阻塞等待引擎退出。
# 进度每 80ms 按时间指数逼近 90%：视觉上一直在走，也不会虚报完成；
# 引擎真正退出后才拉满 100% 并切到结果页。
# 不再用 Get-ChildItem -Recurse 在 UI 线程数文件——一次递归遍历要几百
# 毫秒，既卡顿又让进度长时间停在 5%，是这次要修的核心问题。
function Begin-Work([System.Diagnostics.Process]$proc, [bool]$install, [string]$dest) {
    # 事件回调里读不到函数的局部变量（实测：DispatcherTimer 的 Tick 里
    # 函数参数全是 $null），参数必须落到 $script: 作用域再进回调
    $script:workPct = 2.0
    $script:workClosed = $false
    $script:workProc = $proc
    $script:workInstall = $install
    $script:workDest = $dest
    $barWork.Value = 2; $txtPct.Text = '2%'
    $script:workTimer = [System.Windows.Threading.DispatcherTimer]::new()
    $script:workTimer.Interval = [TimeSpan]::FromMilliseconds(80)
    $script:workTimer.Add_Tick({
        try {
            if (-not $script:workClosed -and $script:workPct -lt 90) {
                # 指数逼近：前段快、后段慢，长安装也不会提前顶死在 90%
                $script:workPct += [Math]::Max(0.08, (90.0 - $script:workPct) * 0.016)
                if ($script:workPct -ge 90) { $script:workPct = 90 }
                $barWork.Value = $script:workPct
                $txtPct.Text = ([int]$script:workPct).ToString() + '%'
                if ($script:workInstall) {
                    $txtPhase.Text = if ($script:workPct -lt 28) { $T.phase1 }
                                     elseif ($script:workPct -lt 74) { $T.phase2 }
                                     else { $T.phase3 }
                }
            }
            if (-not $script:workClosed) {
                $exited = $false
                try { $exited = $script:workProc.HasExited }
                catch { $exited = -not [bool](Get-Process -Id $script:workProc.Id -ErrorAction SilentlyContinue) }
                if ($exited) {
                    $script:workClosed = $true
                    $script:workTimer.Stop()
                    $code = -1
                    try { $code = $script:workProc.ExitCode } catch {
                        # 提权进程偶尔拿不到退出码，按落地结果反推
                        if ($script:workInstall) {
                            if (Test-Path (Join-Path $script:workDest 'fast-https.exe')) { $code = 0 }
                        } elseif (-not (Test-Path $script:workDest)) { $code = 0 }
                    }
                    $barWork.Value = 100; $txtPct.Text = '100%'
                    Show-Result $code $script:workInstall $script:workDest
                }
            }
        } catch { }
    })
    $script:workTimer.Start()
}

function Show-Result([int]$code, [bool]$install, [string]$dest) {
    $ok = ($code -eq 0)
    $cancelled = ($code -eq 1223)
    if ($cancelled) {
        $txtDoneTitle.Text = $T.errTitle
        $txtDoneSub.Text = $T.uacDenied
    } elseif ($ok) {
        if ($install) {
            $txtDoneTitle.Text = $T.doneTitle
            $txtDoneSub.Text = $T.doneSub + ' ' + $dest
            $btnRun.Content = $T.runApp
        } else {
            $txtDoneTitle.Text = $T.unDoneTitle
            $txtDoneSub.Text = $T.unDoneSub
        }
    } else {
        $txtDoneTitle.Text = $T.errTitle
        $txtDoneSub.Text = $T.errSub
    }
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
    # 只有「安装成功」才有运行入口；卸载完成 / 失败时把「完成」升为主按钮
    $primaryOnly = -not ($ok -and $install)
    $btnRun.Visibility = if ($primaryOnly) { 'Collapsed' } else { 'Visible' }
    Set-FinishStyle $primaryOnly
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
    # 卸载是破坏性动作：主按钮换成红色渐变，与安装区分开
    $btnPrimary.Background = $window.FindResource('DangerFill')
    $rowPath.Visibility = 'Collapsed'
    $lnkCustomize.Visibility = 'Collapsed'; $panelCustom.Visibility = 'Collapsed'
    $rowGithub.Visibility = 'Collapsed'
    $txtWorkHint.Text = $T.workHint

    $btnPrimary.Add_Click({
        $txtPhase.Text = $T.uninstalling
        $txtPct.Text = '0%'
        Show-Screen $screenWorking
        $window.UpdateLayout()
        # 卸载进度同样走平滑推进：文件数递减在中途会因为目录被整体删掉
        # 而突然归零，之前进度条会毫无过渡地跳到 95%，看起来像坏了
        $unDir = if ($Dir) { $Dir } else { Split-Path -Parent $Self }
        $code = 1223
        try {
            $proc = New-EngineProcess
        } catch {
            $proc = $null; $code = -1
        }
        if ($null -eq $proc) { Show-Result $code $false $unDir; return }
        Begin-Work $proc $false $unDir
    })
}

# Initialize-WelcomeScreen 安装欢迎屏的文案与事件绑定。
function Initialize-WelcomeScreen {
    $txtTitle.Text = if ($Version) { "$Title $Version" } else { $Title }
    $txtTagline.Text = $T.tagline
    $btnPrimary.Content = $T.install
    $txtInstallTo.Text = $T.installTo
    $txtPath.Text = $defaultDir
    $btnBrowse.Content = '…'
    $txtWorkHint.Text = $T.workHint
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
        $txtPct.Text = '0%'
        Show-Screen $screenWorking
        $window.UpdateLayout()
        $extra = @()
        if (-not $script:optMap.desktop) { $extra += '/DSC_DESKTOP=0' }
        if (-not $script:optMap.startmenu) { $extra += '/DSC_STARTMENU=0' }
        if (-not $script:optMap.addpath) { $extra += '/DSC_PATH=0' }
        # 引擎异步跑：Begin-Work 负责平滑进度与退出检测，UI 线程全程不阻塞
        $code = 1223
        try {
            $proc = New-EngineProcess $dest $extra
        } catch {
            $proc = $null; $code = -1
        }
        if ($null -eq $proc) { Show-Result $code $true $dest; return }
        Begin-Work $proc $true $dest
    })
    $btnRun.Add_Click({
        $exe = Join-Path $txtPath.Text.Trim() 'fast-https-desktop.exe'
        # 必须指定工作目录：向导通常由下载目录/dist 里的 setup.exe 启动，
        # 子进程会继承该目录并一直锁住它（下次打包 goreleaser 清理 dist 会失败）。
        if (Test-Path $exe) { Start-Process $exe -WorkingDirectory $txtPath.Text.Trim() }
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

# 「完成 / 关闭」两种模式共用：之前只在安装分支绑定，卸载完成页的
# 「完成」点了没反应，就是这里漏了卸载分支。
$btnFinish.Add_Click({ $window.Close() })

# ---- 无头自检（放在函数定义之后才能调 Initialize-*）----
# 元素存在 + 两种模式的事件绑定全跑一遍。曾发生过 TextBlock 绑 Click
# 在运行时才崩（向导弹不出来、nsExec 报错退出），只有真实初始化才能暴露。
if ($SelfTest) {
    $names = 'dragBar','btnClose','screenWelcome','screenWorking','screenDone','imgLogo','imgLogo2','txtTitle','txtTagline','txtBody','txtBody2','btnPrimary','btnSecondary','rowPath','txtInstallTo','txtPath','btnBrowse','lnkCustomize','runCustomize','panelCustom','rowGithub','runGithubLead','btnStar','rowOptions','chkDesktop','chkStartMenu','chkPath','txtPhase','barWork','txtPct','txtWorkHint','doneCircle','doneCheck','txtDoneTitle','txtDoneSub','btnRun','btnFinish'
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





