# Fast-Https 安装向导（自绘界面）。
#
# 定位：NSIS 只当安装引擎（fast-https.nsi 的安装段，静默执行），向导界面
# 全部由这里自绘——深空星轨头图、圆角卡片、胶囊按钮、步骤进度页，双击的
# 仍然是单个 setup.exe。交互流程：
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
        install = '立即安装'
        installTo = '安装到'
        browse = '浏览'
        pickDir = '选择安装位置：会在所选目录下创建 Fast-Https 子目录'
        notEmptyTitle = '安装目录不是空的'
        notEmpty = "安装目录不是空的：`n{0}`n`n继续安装会覆盖同名文件；卸载时这个目录会被整体删除。确定继续吗？"
        agree = '安装即同意'
        terms = '用户协议'
        and = '和'
        privacy = '隐私政策'
        installing = '正在安装…'
        phase1 = '正在准备安装环境…'
        phase2 = '正在部署程序与组件…'
        phase3 = '正在注册快捷方式与配置…'
        step1 = '准备'
        step2 = '部署'
        step3 = '收尾'
        unStep1 = '停止服务'
        unStep2 = '清理文件'
        unStep3 = '移除注册'
        workHint = '秩序正在静静落位，请保持此窗口打开'
        unWorkHint = '正在安静收锚，请稍候片刻'
        uacTitle = '需要管理员权限'
        uacBody = '安装到 Program Files 需要管理员授权。即将弹出 UAC 确认框，请选择「是」。'
        uacDenied = '未获得管理员授权，操作已取消。'
        optDesktop = '桌面快捷方式'
        optDesktopTip = '在桌面创建快捷方式'
        optStartMenu = '开始菜单'
        optStartMenuTip = '添加到 Windows 开始菜单'
        optPath = '系统 PATH'
        optPathTip = '加入系统 PATH，可在任意终端直接运行 fast-https'
        customize = '自定义安装路径与选项 ▾'
        customizeOn = '收起自定义选项 ▴'
        starLead = '如果 Fast-Https 帮到了你，欢迎点个 Star'
        starBtn = 'Star'
        doneTitle = '已准备就绪'
        doneSub = '安装位置：'
        doneWhisper = '灯已点亮，静候第一次握手。'
        runApp = '启动 Fast-Https Desktop'
        finish = '完成'
        errTitle = '未能完成安装'
        errSub = '安装程序返回了错误，请重试或查看日志。'
        close = '关闭'
        unBadge = 'UNINSTALL'
        unTitle = ('告别 ' + $Title + '？')
        unTagline = '将为你彻底清理本机文件，不留痕迹'
        unScopeTitle = '以下内容将被一并移除'
        unBody = '• 主程序与桌面管理端本体`n• 本地配置、站点规则、证书与运行日志`n• 状态快照、缓存与桌面快捷方式'
        unKeep = '如果只是想关闭窗口，使用「隐藏到后台托盘」即可，不必卸载。'
        unBtn = '确认卸载'
        unCancel = '再想想'
        uninstalling = '正在停止服务并清理文件…'
        unPhase1 = '正在停止运行中的服务…'
        unPhase2 = '正在清理程序与数据目录…'
        unPhase3 = '正在移除快捷方式与注册项…'
        unDoneTitle = '已安然卸载'
        unDoneSub = '所有程序与本地数据已清理干净，没有残留。'
        unDoneWhisper = '暂且停泊，后会有期。'
        installWhisper = '每一次启程，都是一次新秩序的开始。'
        uninstallWhisper = '把空间干净地还给你，也是体面告别的一部分。'
    }
} else {
    @{
        tagline = 'Local web server & reverse proxy · desktop console'
        install = 'Install Now'
        installTo = 'Install to'
        browse = 'Browse'
        pickDir = 'Choose a location; a Fast-Https subfolder will be created inside it'
        notEmptyTitle = 'Install folder is not empty'
        notEmpty = "The install folder is not empty:`n{0}`n`nInstalling will overwrite files with the same name, and uninstalling removes this folder entirely. Continue?"
        agree = 'By installing you agree to the'
        terms = 'Terms'
        and = 'and'
        privacy = 'Privacy Policy'
        installing = 'Installing…'
        phase1 = 'Preparing environment…'
        phase2 = 'Deploying program files…'
        phase3 = 'Registering shortcuts and entries…'
        step1 = 'Prepare'
        step2 = 'Deploy'
        step3 = 'Finalize'
        unStep1 = 'Stop'
        unStep2 = 'Clean'
        unStep3 = 'Unregister'
        workHint = 'Order is settling into place — please keep this window open'
        unWorkHint = 'Dropping anchor quietly — just a moment'
        uacTitle = 'Administrator permission required'
        uacBody = 'Installing into Program Files needs admin approval. A UAC prompt will appear — please click Yes.'
        uacDenied = 'Administrator permission was denied; the operation was cancelled.'
        optDesktop = 'Desktop Icon'
        optDesktopTip = 'Create a desktop shortcut'
        optStartMenu = 'Start Menu'
        optStartMenuTip = 'Add to the Windows Start menu'
        optPath = 'System PATH'
        optPathTip = 'Add to system PATH to run fast-https from any terminal'
        customize = 'Customize location & options ▾'
        customizeOn = 'Hide options ▴'
        starLead = 'If Fast-Https helps you, a star means a lot'
        starBtn = 'Star'
        doneTitle = 'Ready to Go'
        doneSub = 'Installed to '
        doneWhisper = 'The lamp is lit, awaiting its first handshake.'
        runApp = 'Launch Fast-Https Desktop'
        finish = 'Done'
        errTitle = 'Installation Incomplete'
        errSub = 'The installer reported an error. Please retry or check the logs.'
        close = 'Close'
        unBadge = 'UNINSTALL'
        unTitle = ('Part ways with ' + $Title + '?')
        unTagline = 'Everything installed locally will be removed cleanly'
        unScopeTitle = 'The following will be removed'
        unBody = "• Program and desktop console binaries`n• Local config, sites, certificates and logs`n• Snapshots, caches and shortcuts"
        unKeep = 'To just close the window, use Hide to tray instead.'
        unBtn = 'Uninstall'
        unCancel = 'Keep It'
        uninstalling = 'Stopping services and cleaning up…'
        unPhase1 = 'Stopping running services…'
        unPhase2 = 'Removing program files and data…'
        unPhase3 = 'Cleaning shortcuts and registry…'
        unDoneTitle = 'Cleanly Removed'
        unDoneSub = 'All files and local data have been cleared with nothing left behind.'
        unDoneWhisper = 'Until our paths cross again.'
        installWhisper = 'Every installation is the quiet beginning of a new order.'
        uninstallWhisper = 'Leaving your space spotless is part of a graceful goodbye.'
    }
}

# ---- 界面（XAML：深空星轨头图 + 干净卡片 + 克制靛蓝/暮红主题） ----
$xaml = @'
<Window xmlns="http://schemas.microsoft.com/winfx/2006/xaml/presentation"
        xmlns:x="http://schemas.microsoft.com/winfx/2006/xaml"
        WindowStyle="None" AllowsTransparency="True" Background="Transparent"
        Width="560" SizeToContent="Height" WindowStartupLocation="CenterScreen"
        ResizeMode="NoResize" ShowInTaskbar="True" FontFamily="Microsoft YaHei UI"
        UseLayoutRounding="True" TextOptions.TextFormattingMode="Display">
  <Window.Resources>
    <!-- 品牌色与桌面端「静谧值班台」保持一致 -->
    <LinearGradientBrush x:Key="HeroInstallFill" StartPoint="0,0" EndPoint="1,1">
      <GradientStop Color="#161D33" Offset="0"/>
      <GradientStop Color="#1E2B4B" Offset="0.55"/>
      <GradientStop Color="#16233D" Offset="1"/>
    </LinearGradientBrush>
    <LinearGradientBrush x:Key="HeroUninstallFill" StartPoint="0,0" EndPoint="1,1">
      <GradientStop Color="#271A21" Offset="0"/>
      <GradientStop Color="#38222B" Offset="0.55"/>
      <GradientStop Color="#24181E" Offset="1"/>
    </LinearGradientBrush>
    <LinearGradientBrush x:Key="PrimaryFill" StartPoint="0,0" EndPoint="1,1">
      <GradientStop Color="#5763DE" Offset="0"/>
      <GradientStop Color="#4551C9" Offset="1"/>
    </LinearGradientBrush>
    <LinearGradientBrush x:Key="DangerFill" StartPoint="0,0" EndPoint="1,1">
      <GradientStop Color="#CE4A40" Offset="0"/>
      <GradientStop Color="#B3372E" Offset="1"/>
    </LinearGradientBrush>
    <SolidColorBrush x:Key="AccentBrush" Color="#4F5BD5"/>
    <SolidColorBrush x:Key="DangerBrush" Color="#C93B2F"/>
    <SolidColorBrush x:Key="InkBrush" Color="#151722"/>
    <SolidColorBrush x:Key="SubInkBrush" Color="#555B6E"/>
    <SolidColorBrush x:Key="MutedBrush" Color="#848CA4"/>
    <SolidColorBrush x:Key="StepIdleBrush" Color="#D4D8E6"/>

    <ControlTemplate x:Key="PrimaryButton" TargetType="Button">
      <Border x:Name="bd" CornerRadius="12" Background="{TemplateBinding Background}">
        <ContentPresenter HorizontalAlignment="Center" VerticalAlignment="Center"/>
      </Border>
      <ControlTemplate.Triggers>
        <Trigger Property="IsMouseOver" Value="True"><Setter TargetName="bd" Property="Opacity" Value="0.92"/></Trigger>
        <Trigger Property="IsPressed" Value="True"><Setter TargetName="bd" Property="Opacity" Value="0.78"/></Trigger>
      </ControlTemplate.Triggers>
    </ControlTemplate>

    <ControlTemplate x:Key="GhostButton" TargetType="Button">
      <Border x:Name="bd" CornerRadius="12" Background="{TemplateBinding Background}"
              BorderBrush="#D5DAE6" BorderThickness="1">
        <ContentPresenter HorizontalAlignment="Center" VerticalAlignment="Center"/>
      </Border>
      <ControlTemplate.Triggers>
        <Trigger Property="IsMouseOver" Value="True">
          <Setter TargetName="bd" Property="Background" Value="#F4F5FA"/>
          <Setter TargetName="bd" Property="BorderBrush" Value="#BFC6DA"/>
        </Trigger>
        <Trigger Property="IsPressed" Value="True">
          <Setter TargetName="bd" Property="Background" Value="#E9ECF5"/>
        </Trigger>
      </ControlTemplate.Triggers>
    </ControlTemplate>

    <ControlTemplate x:Key="QuietButton" TargetType="Button">
      <Border x:Name="bd" Background="{TemplateBinding Background}" CornerRadius="8"
              Padding="{TemplateBinding Padding}">
        <ContentPresenter HorizontalAlignment="{TemplateBinding HorizontalContentAlignment}" VerticalAlignment="Center"/>
      </Border>
      <ControlTemplate.Triggers>
        <Trigger Property="IsMouseOver" Value="True"><Setter TargetName="bd" Property="Background" Value="#F1F3F9"/></Trigger>
        <Trigger Property="IsPressed" Value="True"><Setter TargetName="bd" Property="Background" Value="#E6E9F4"/></Trigger>
      </ControlTemplate.Triggers>
    </ControlTemplate>

    <ControlTemplate x:Key="OptionChipButton" TargetType="Button">
      <Border x:Name="bd" Background="{TemplateBinding Background}"
              BorderBrush="{TemplateBinding BorderBrush}" BorderThickness="1"
              CornerRadius="9" Padding="10,7">
        <ContentPresenter HorizontalAlignment="Center" VerticalAlignment="Center"/>
      </Border>
      <ControlTemplate.Triggers>
        <Trigger Property="IsMouseOver" Value="True">
          <Setter TargetName="bd" Property="BorderBrush" Value="#B8C0E4"/>
        </Trigger>
        <Trigger Property="IsPressed" Value="True">
          <Setter TargetName="bd" Property="Opacity" Value="0.82"/>
        </Trigger>
      </ControlTemplate.Triggers>
    </ControlTemplate>

    <ControlTemplate x:Key="HeroCloseButton" TargetType="Button">
      <Border x:Name="bd" Background="Transparent" CornerRadius="8">
        <ContentPresenter HorizontalAlignment="Center" VerticalAlignment="Center"/>
      </Border>
      <ControlTemplate.Triggers>
        <Trigger Property="IsMouseOver" Value="True"><Setter TargetName="bd" Property="Background" Value="#24FFFFFF"/></Trigger>
        <Trigger Property="IsPressed" Value="True"><Setter TargetName="bd" Property="Background" Value="#38FFFFFF"/></Trigger>
      </ControlTemplate.Triggers>
    </ControlTemplate>

    <Style x:Key="SlimProgress" TargetType="ProgressBar">
      <Setter Property="Template">
        <Setter.Value>
          <ControlTemplate TargetType="ProgressBar">
            <Grid>
              <Border x:Name="PART_Track" CornerRadius="4" Background="#EAECF4"/>
              <Border x:Name="PART_Indicator" CornerRadius="4" HorizontalAlignment="Left"
                      Background="{TemplateBinding Foreground}"/>
            </Grid>
          </ControlTemplate>
        </Setter.Value>
      </Setter>
    </Style>
  </Window.Resources>

  <Border Background="#FFFFFF" CornerRadius="18" Margin="14" BorderBrush="#E4E7F2" BorderThickness="1">
    <Border.Effect><DropShadowEffect BlurRadius="32" ShadowDepth="4" Direction="270" Opacity="0.18"/></Border.Effect>
    <Grid>
      <!-- 顶部深空英雄区：展示品牌、星轨与版本 -->
      <Grid.RowDefinitions>
        <RowDefinition Height="Auto"/>
        <RowDefinition Height="Auto"/>
      </Grid.RowDefinitions>

      <Border x:Name="heroBanner" Grid.Row="0" CornerRadius="17,17,0,0" Background="{StaticResource HeroInstallFill}"
              ClipToBounds="True" Height="154">
        <Grid>
          <!-- 极简精密星轨背景装饰（纯矢量，低透明度零光污染） -->
          <Canvas IsHitTestVisible="False">
            <Ellipse Width="220" Height="220" Canvas.Left="356" Canvas.Top="-68" Stroke="#1CFFFFFF" StrokeThickness="1"/>
            <Ellipse Width="154" Height="154" Canvas.Left="389" Canvas.Top="-35" Stroke="#16FFFFFF" StrokeThickness="1" StrokeDashArray="3 4"/>
            <Ellipse Width="88" Height="88" Canvas.Left="422" Canvas.Top="-2" Stroke="#14FFFFFF" StrokeThickness="1"/>
            <Ellipse Width="5" Height="5" Canvas.Left="387" Canvas.Top="40" Fill="#88A5B4FC"/>
            <Ellipse Width="4" Height="4" Canvas.Left="464" Canvas.Top="78" Fill="#8867E8F9"/>
            <Ellipse Width="180" Height="180" Canvas.Left="-64" Canvas.Top="64" Stroke="#10FFFFFF" StrokeThickness="1"/>
          </Canvas>

          <!-- 拖拽区 + 关闭按钮 -->
          <Grid x:Name="dragBar" Height="46" VerticalAlignment="Top" Background="Transparent"/>
          <Button x:Name="btnClose" Width="30" Height="30" HorizontalAlignment="Right" VerticalAlignment="Top"
                  Margin="0,12,12,0" Cursor="Hand" Template="{StaticResource HeroCloseButton}"
                  Background="Transparent" BorderThickness="0">
            <Path Data="M0,0 L8,8 M8,0 L0,8" Stroke="#CBD5E1" StrokeThickness="1.4"
                  StrokeStartLineCap="Round" StrokeEndLineCap="Round" Stretch="None"
                  HorizontalAlignment="Center" VerticalAlignment="Center"/>
          </Button>

          <!-- 品牌主体（左侧 Logo 玻璃方框 + 右侧 Kicker 胶囊 / 标题 / 标语） -->
          <StackPanel Orientation="Horizontal" VerticalAlignment="Center" Margin="34,10,34,0">
            <Border Width="70" Height="70" CornerRadius="16" Background="#16FFFFFF" BorderBrush="#28FFFFFF" BorderThickness="1">
              <Image x:Name="imgLogo" Width="48" Height="48" RenderOptions.BitmapScalingMode="HighQuality"/>
            </Border>
            <StackPanel VerticalAlignment="Center" Margin="18,0,0,0">
              <Border x:Name="badgeVer" HorizontalAlignment="Left" Background="#20FFFFFF" BorderBrush="#30FFFFFF" BorderThickness="1"
                      CornerRadius="9" Padding="8,2" Margin="0,0,0,6">
                <TextBlock x:Name="txtBadge" FontSize="9.5" FontWeight="SemiBold" Foreground="#CBD5E1" Text="DESKTOP"/>
              </Border>
              <TextBlock x:Name="txtTitle" FontSize="21" FontWeight="SemiBold" Foreground="#F8FAFC"/>
              <TextBlock x:Name="txtTagline" FontSize="12" Foreground="#B4C0D6" Margin="0,5,0,0" MaxWidth="370" TextWrapping="Wrap"/>
            </StackPanel>
          </StackPanel>
        </Grid>
      </Border>

      <!-- 下方白色内容区 -->
      <Grid Grid.Row="1">
        <!-- 欢迎 / 卸载确认 -->
        <StackPanel x:Name="screenWelcome" Margin="34,26,34,22">
          <!-- 卸载模式：清晰的影响范围卡片 -->
          <Border x:Name="panelUnScope" Visibility="Collapsed" Background="#FDF6F6" BorderBrush="#F2D7D5"
                  BorderThickness="1" CornerRadius="12" Padding="16,12" Margin="0,0,0,12">
            <StackPanel>
              <TextBlock x:Name="txtUnScopeTitle" FontSize="12" FontWeight="SemiBold" Foreground="#A9342C" Margin="0,0,0,6"/>
              <TextBlock x:Name="txtBody" FontSize="12" Foreground="#5D4648" LineHeight="20" TextWrapping="Wrap"/>
            </StackPanel>
          </Border>
          <TextBlock x:Name="txtBody2" FontSize="11.5" Foreground="#7A8296" HorizontalAlignment="Center"
                     TextAlignment="Center" TextWrapping="Wrap" MaxWidth="440" Margin="0,0,0,16" Visibility="Collapsed"/>

          <!-- 主次操作按钮行 -->
          <StackPanel x:Name="rowActions" Orientation="Horizontal" HorizontalAlignment="Center">
            <Button x:Name="btnSecondary" Width="136" Height="44" Cursor="Hand" Margin="0,0,12,0"
                    Template="{StaticResource GhostButton}" Background="Transparent" BorderThickness="0"
                    Foreground="#555B6E" FontSize="13.5" Content="" Visibility="Collapsed"/>
            <Button x:Name="btnPrimary" Width="228" Height="44" Cursor="Hand"
                    Template="{StaticResource PrimaryButton}" Background="{StaticResource PrimaryFill}"
                    BorderThickness="0" Foreground="#FFFFFF" FontSize="14" FontWeight="SemiBold" Content="Install"/>
          </StackPanel>

          <!-- 自定义安装：默认收起，点开后展开路径与选项 -->
          <Button x:Name="lnkCustomize" HorizontalAlignment="Center" Margin="0,12,0,0" Cursor="Hand"
                  Template="{StaticResource QuietButton}" Background="Transparent" BorderThickness="0"
                  Foreground="#7B8399" FontSize="11.5" Padding="10,5">
            <Run x:Name="runCustomize" Text="自定义安装 ▾"/>
          </Button>
          <Border x:Name="panelCustom" Visibility="Collapsed" Background="#F6F7FB" BorderBrush="#E8EBF5"
                  BorderThickness="1" CornerRadius="12" Padding="16,14" Margin="0,10,0,0">
            <StackPanel>
              <Grid x:Name="rowPath">
                <Grid.ColumnDefinitions>
                  <ColumnDefinition Width="Auto"/>
                  <ColumnDefinition Width="*"/>
                  <ColumnDefinition Width="Auto"/>
                </Grid.ColumnDefinitions>
                <TextBlock x:Name="txtInstallTo" Grid.Column="0" FontSize="11.5" Foreground="#555B6E" VerticalAlignment="Center" Margin="0,0,10,0"/>
                <TextBox x:Name="txtPath" Grid.Column="1" Height="32" FontSize="11.5" Foreground="#151722"
                         BorderThickness="0" VerticalContentAlignment="Center">
                  <TextBox.Template>
                    <ControlTemplate TargetType="TextBox">
                      <Border x:Name="bd" CornerRadius="8" Background="#FFFFFF" BorderBrush="#D5DAE6" BorderThickness="1">
                        <ScrollViewer x:Name="PART_ContentHost" Margin="9,0" VerticalAlignment="Center"/>
                      </Border>
                      <ControlTemplate.Triggers>
                        <Trigger Property="IsKeyboardFocused" Value="True">
                          <Setter TargetName="bd" Property="BorderBrush" Value="#4F5BD5"/>
                        </Trigger>
                      </ControlTemplate.Triggers>
                    </ControlTemplate>
                  </TextBox.Template>
                </TextBox>
                <Button x:Name="btnBrowse" Grid.Column="2" Width="34" Height="32" Margin="8,0,0,0" Cursor="Hand" FontSize="12"
                        Template="{StaticResource GhostButton}" Background="#FFFFFF" BorderThickness="0"
                        Foreground="#555B6E" Content="…"/>
              </Grid>
              <Grid x:Name="rowOptions" Margin="0,12,0,0">
                <Grid.ColumnDefinitions>
                  <ColumnDefinition Width="*"/>
                  <ColumnDefinition Width="8"/>
                  <ColumnDefinition Width="*"/>
                  <ColumnDefinition Width="8"/>
                  <ColumnDefinition Width="*"/>
                </Grid.ColumnDefinitions>
                <!-- 横向三联微选芯片：选中显示细对勾，未选变为极淡 + 号，低调不抢主按钮 -->
                <Button x:Name="chkDesktop" Grid.Column="0" Cursor="Hand"
                        Template="{StaticResource OptionChipButton}" Background="#FFFFFF" BorderBrush="#D5DBF2">
                  <StackPanel Orientation="Horizontal" VerticalAlignment="Center">
                    <Path x:Name="icoDesktop" Width="11" Height="11" Stretch="Uniform" Margin="0,0,6,0"
                          Data="M1.5,5.8 L4.4,8.6 L9.5,2.4" Stroke="#4F5BD5" StrokeThickness="1.7"
                          StrokeStartLineCap="Round" StrokeEndLineCap="Round" StrokeLineJoin="Round"/>
                    <TextBlock x:Name="lblDesktop" FontSize="11.5" Foreground="#151722" VerticalAlignment="Center"/>
                  </StackPanel>
                </Button>

                <Button x:Name="chkStartMenu" Grid.Column="2" Cursor="Hand"
                        Template="{StaticResource OptionChipButton}" Background="#FFFFFF" BorderBrush="#D5DBF2">
                  <StackPanel Orientation="Horizontal" VerticalAlignment="Center">
                    <Path x:Name="icoStartMenu" Width="11" Height="11" Stretch="Uniform" Margin="0,0,6,0"
                          Data="M1.5,5.8 L4.4,8.6 L9.5,2.4" Stroke="#4F5BD5" StrokeThickness="1.7"
                          StrokeStartLineCap="Round" StrokeEndLineCap="Round" StrokeLineJoin="Round"/>
                    <TextBlock x:Name="lblStartMenu" FontSize="11.5" Foreground="#151722" VerticalAlignment="Center"/>
                  </StackPanel>
                </Button>

                <Button x:Name="chkPath" Grid.Column="4" Cursor="Hand"
                        Template="{StaticResource OptionChipButton}" Background="#FFFFFF" BorderBrush="#D5DBF2">
                  <StackPanel Orientation="Horizontal" VerticalAlignment="Center">
                    <Path x:Name="icoPath" Width="11" Height="11" Stretch="Uniform" Margin="0,0,6,0"
                          Data="M1.5,5.8 L4.4,8.6 L9.5,2.4" Stroke="#4F5BD5" StrokeThickness="1.7"
                          StrokeStartLineCap="Round" StrokeEndLineCap="Round" StrokeLineJoin="Round"/>
                    <TextBlock x:Name="lblPath" FontSize="11.5" Foreground="#151722" VerticalAlignment="Center"/>
                  </StackPanel>
                </Button>
              </Grid>
            </StackPanel>
          </Border>

          <!-- 开源 Star 引导 -->
          <StackPanel Orientation="Horizontal" HorizontalAlignment="Center" Margin="0,14,0,0" x:Name="rowGithub">
            <TextBlock FontSize="11" Foreground="#8A92A6" VerticalAlignment="Center"><Run x:Name="runGithubLead" Text=""/></TextBlock>
            <Button x:Name="btnStar" Height="24" MinWidth="64" Cursor="Hand" Margin="8,0,0,0" Padding="10,0"
                    BorderThickness="0" Foreground="#4F5BD5" FontSize="10.5" FontWeight="SemiBold" Content="">
              <Button.Background>
                <SolidColorBrush Color="#EEF0FB"/>
              </Button.Background>
              <Button.Template>
                <ControlTemplate TargetType="Button">
                  <Border x:Name="bd" CornerRadius="12" Background="{TemplateBinding Background}"
                          BorderBrush="#D8DDF4" BorderThickness="1" Padding="{TemplateBinding Padding}">
                    <StackPanel Orientation="Horizontal" HorizontalAlignment="Center" VerticalAlignment="Center">
                      <TextBlock Text="★" FontSize="10" Margin="0,0,4,0" Foreground="#D98A2B"/>
                      <ContentPresenter VerticalAlignment="Center"/>
                    </StackPanel>
                  </Border>
                  <ControlTemplate.Triggers>
                    <Trigger Property="IsMouseOver" Value="True"><Setter TargetName="bd" Property="Background" Value="#E3E7F9"/></Trigger>
                    <Trigger Property="IsPressed" Value="True"><Setter TargetName="bd" Property="Opacity" Value="0.8"/></Trigger>
                  </ControlTemplate.Triggers>
                </ControlTemplate>
              </Button.Template>
            </Button>
          </StackPanel>

          <!-- 底部极淡哲学眉批 -->
          <TextBlock x:Name="txtWelcomeWhisper" FontSize="10.5" Foreground="#A5ACBE"
                     HorizontalAlignment="Center" Margin="0,14,0,0"/>
        </StackPanel>

        <!-- 进度页 -->
        <StackPanel x:Name="screenWorking" Visibility="Collapsed" Margin="38,28,38,30">
          <!-- 三段式步骤指示 -->
          <Grid Width="310" HorizontalAlignment="Center" Margin="0,0,0,22">
            <Border Height="2" Background="#EAECF4" VerticalAlignment="Top" Margin="26,6,26,0"/>
            <Grid>
              <Grid.ColumnDefinitions>
                <ColumnDefinition Width="*"/>
                <ColumnDefinition Width="*"/>
                <ColumnDefinition Width="*"/>
              </Grid.ColumnDefinitions>
              <StackPanel Grid.Column="0" HorizontalAlignment="Center">
                <Ellipse x:Name="dotStep1" Width="12" Height="12" Fill="#4F5BD5"/>
                <TextBlock x:Name="txtStep1" FontSize="10.5" Foreground="#151722" FontWeight="SemiBold" Margin="0,6,0,0" HorizontalAlignment="Center"/>
              </StackPanel>
              <StackPanel Grid.Column="1" HorizontalAlignment="Center">
                <Ellipse x:Name="dotStep2" Width="12" Height="12" Fill="#D4D8E6"/>
                <TextBlock x:Name="txtStep2" FontSize="10.5" Foreground="#848CA4" Margin="0,6,0,0" HorizontalAlignment="Center"/>
              </StackPanel>
              <StackPanel Grid.Column="2" HorizontalAlignment="Center">
                <Ellipse x:Name="dotStep3" Width="12" Height="12" Fill="#D4D8E6"/>
                <TextBlock x:Name="txtStep3" FontSize="10.5" Foreground="#848CA4" Margin="0,6,0,0" HorizontalAlignment="Center"/>
              </StackPanel>
            </Grid>
          </Grid>

          <!-- 静谧星轨包裹图标 -->
          <Grid Width="72" Height="72" HorizontalAlignment="Center">
            <Ellipse Stroke="#E5E8F4" StrokeThickness="1.2" Fill="#F7F8FC"/>
            <Ellipse Stroke="#C7CEEA" StrokeThickness="1" StrokeDashArray="3 4" Margin="5"/>
            <Image x:Name="imgLogo2" Width="40" Height="40" RenderOptions.BitmapScalingMode="HighQuality"/>
          </Grid>

          <Grid Width="360" HorizontalAlignment="Center" Margin="0,20,0,0">
            <TextBlock x:Name="txtPhase" FontSize="13" FontWeight="SemiBold" Foreground="#23283A" HorizontalAlignment="Left"/>
            <TextBlock x:Name="txtPct" FontSize="12.5" FontWeight="SemiBold" Foreground="#4F5BD5" HorizontalAlignment="Right" Text="0%"/>
          </Grid>
          <ProgressBar x:Name="barWork" Style="{StaticResource SlimProgress}" Width="360" Height="8" Margin="0,10,0,0"
                       Minimum="0" Maximum="100" Value="0" Foreground="#4F5BD5" Background="#EAECF4"/>
          <TextBlock x:Name="txtWorkHint" FontSize="11" Foreground="#949CB0" HorizontalAlignment="Center" Margin="0,14,0,0"/>
        </StackPanel>

        <!-- 完成 / 错误页 -->
        <StackPanel x:Name="screenDone" Visibility="Collapsed" Margin="38,28,38,26">
          <Grid Width="72" Height="72" HorizontalAlignment="Center">
            <Ellipse x:Name="doneRing" Fill="#F1FAF5" Opacity="0.7"/>
            <Ellipse x:Name="doneCircle" Width="56" Height="56" Fill="#E3F5EB"/>
            <Path x:Name="doneCheck" Width="26" Height="26"
                  Data="M5,13 L11,19 L21,7" Stroke="#138050" StrokeThickness="2.8"
                  StrokeStartLineCap="Round" StrokeEndLineCap="Round" StrokeLineJoin="Round"/>
          </Grid>

          <TextBlock x:Name="txtDoneTitle" FontSize="20" FontWeight="SemiBold" Foreground="#151722"
                     HorizontalAlignment="Center" Margin="0,16,0,0"/>
          <TextBlock x:Name="txtDoneSub" FontSize="12" Foreground="#686E82" HorizontalAlignment="Center"
                     Margin="0,8,0,0" TextAlignment="Center" TextWrapping="Wrap" MaxWidth="420"/>
          <TextBlock x:Name="txtDoneWhisper" FontSize="11" Foreground="#9AA2B5" HorizontalAlignment="Center"
                     Margin="0,6,0,0" TextAlignment="Center"/>

          <Button x:Name="btnRun" Width="236" Height="44" Cursor="Hand" HorizontalAlignment="Center" Margin="0,20,0,0"
                  Template="{StaticResource PrimaryButton}" Background="{StaticResource PrimaryFill}"
                  BorderThickness="0" Foreground="#FFFFFF" FontSize="14" FontWeight="SemiBold" Content=""/>
          <Button x:Name="btnFinish" Width="236" Height="40" Cursor="Hand" HorizontalAlignment="Center" Margin="0,10,0,0"
                  Template="{StaticResource GhostButton}" Background="Transparent" BorderThickness="0"
                  Foreground="#555B6E" FontSize="12.5" Content=""/>
        </StackPanel>
      </Grid>
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

$heroBanner = N 'heroBanner'; $badgeVer = N 'badgeVer'; $txtBadge = N 'txtBadge'
$dragBar = N 'dragBar'; $btnClose = N 'btnClose'
$screenWelcome = N 'screenWelcome'; $screenWorking = N 'screenWorking'; $screenDone = N 'screenDone'
$imgLogo = N 'imgLogo'; $imgLogo2 = N 'imgLogo2'
$txtTitle = N 'txtTitle'; $txtTagline = N 'txtTagline'
$panelUnScope = N 'panelUnScope'; $txtUnScopeTitle = N 'txtUnScopeTitle'
$txtBody = N 'txtBody'; $txtBody2 = N 'txtBody2'
$btnPrimary = N 'btnPrimary'; $btnSecondary = N 'btnSecondary'
$rowPath = N 'rowPath'; $txtInstallTo = N 'txtInstallTo'; $txtPath = N 'txtPath'; $btnBrowse = N 'btnBrowse'
$lnkCustomize = N 'lnkCustomize'; $runCustomize = N 'runCustomize'; $panelCustom = N 'panelCustom'
$rowGithub = N 'rowGithub'; $runGithubLead = N 'runGithubLead'; $btnStar = N 'btnStar'
$rowOptions = N 'rowOptions'; $chkDesktop = N 'chkDesktop'; $chkStartMenu = N 'chkStartMenu'; $chkPath = N 'chkPath'
$lblDesktop = N 'lblDesktop'; $icoDesktop = N 'icoDesktop'
$lblStartMenu = N 'lblStartMenu'; $icoStartMenu = N 'icoStartMenu'
$lblPath = N 'lblPath'; $icoPath = N 'icoPath'
$txtWelcomeWhisper = N 'txtWelcomeWhisper'
$dotStep1 = N 'dotStep1'; $dotStep2 = N 'dotStep2'; $dotStep3 = N 'dotStep3'
$txtStep1 = N 'txtStep1'; $txtStep2 = N 'txtStep2'; $txtStep3 = N 'txtStep3'
$txtPct = N 'txtPct'
$txtPhase = N 'txtPhase'; $barWork = N 'barWork'; $txtWorkHint = N 'txtWorkHint'
$doneRing = N 'doneRing'; $doneCircle = N 'doneCircle'; $doneCheck = N 'doneCheck'
$txtDoneTitle = N 'txtDoneTitle'; $txtDoneSub = N 'txtDoneSub'; $txtDoneWhisper = N 'txtDoneWhisper'
$btnRun = N 'btnRun'; $btnFinish = N 'btnFinish'

# 资源刷子：统一取用
$script:inkBrush     = $window.FindResource('InkBrush')
$script:subInkBrush  = $window.FindResource('SubInkBrush')
$script:mutedBrush   = $window.FindResource('MutedBrush')
$script:accentBrush  = $window.FindResource('AccentBrush')
$script:dangerBrush  = $window.FindResource('DangerBrush')
$script:stepIdleBrush = $window.FindResource('StepIdleBrush')
$script:primaryFill  = $window.FindResource('PrimaryFill')
$script:dangerFill   = $window.FindResource('DangerFill')
$script:primaryTpl   = $window.FindResource('PrimaryButton')
$script:ghostTpl     = $window.FindResource('GhostButton')

# 品牌图（没有就退回窗口标题）
if ($Icon -and (Test-Path $Icon)) {
    $iconPath = (Resolve-Path $Icon).Path
    $imgLogo.Source = [System.Windows.Media.Imaging.BitmapFrame]::Create([Uri]$iconPath)
    $imgLogo2.Source = $imgLogo.Source
}
if ($Version) { $window.Title = "$Title $Version" } else { $window.Title = $Title }

function Update-StepDots([int]$step, [bool]$isUninstall) {
    $active = if ($isUninstall) { $script:dangerBrush } else { $script:accentBrush }
    $dots = @($dotStep1, $dotStep2, $dotStep3)
    $txts = @($txtStep1, $txtStep2, $txtStep3)
    for ($i = 0; $i -lt 3; $i++) {
        if ($i -le $step) {
            $dots[$i].Fill = $active
            $txts[$i].Foreground = $script:inkBrush
            $txts[$i].FontWeight = if ($i -eq $step) { 'SemiBold' } else { 'Normal' }
        } else {
            $dots[$i].Fill = $script:stepIdleBrush
            $txts[$i].Foreground = $script:mutedBrush
            $txts[$i].FontWeight = 'Normal'
        }
    }
}

function Apply-WorkingTheme([bool]$isUninstall) {
    if ($isUninstall) {
        $txtStep1.Text = $T.unStep1; $txtStep2.Text = $T.unStep2; $txtStep3.Text = $T.unStep3
        $barWork.Foreground = $script:dangerBrush
        $txtPct.Foreground = $script:dangerBrush
        $txtWorkHint.Text = $T.unWorkHint
    } else {
        $txtStep1.Text = $T.step1; $txtStep2.Text = $T.step2; $txtStep3.Text = $T.step3
        $barWork.Foreground = $script:accentBrush
        $txtPct.Foreground = $script:accentBrush
        $txtWorkHint.Text = $T.workHint
    }
    Update-StepDots 0 $isUninstall
}

# ---- 圆点选项状态与渲染（预览块与主流程共用，定义在两者之前）----
$script:optMap = @{}

$script:geoCheck = [System.Windows.Media.Geometry]::Parse('M1.5,5.8 L4.4,8.6 L9.5,2.4')
$script:geoPlus  = [System.Windows.Media.Geometry]::Parse('M5.5,2 L5.5,9 M2,5.5 L9,5.5')

function Paint-Chip($btn, $lbl, $ico, [string]$title, [string]$tip, [bool]$on) {
    $lbl.Text = $title
    $btn.ToolTip = $tip
    if ($on) {
        $btn.Background = [System.Windows.Media.Brushes]::White
        $btn.BorderBrush = New-Object System.Windows.Media.SolidColorBrush([System.Windows.Media.Color]::FromRgb(0xCE, 0xD5, 0xEE))
        $lbl.Foreground = $script:inkBrush
        $lbl.FontWeight = 'Medium'
        $ico.Data = $script:geoCheck
        $ico.Stroke = $script:accentBrush
    } else {
        $btn.Background = [System.Windows.Media.Brushes]::Transparent
        $btn.BorderBrush = New-Object System.Windows.Media.SolidColorBrush([System.Windows.Media.Color]::FromRgb(0xE1, 0xE4, 0xEF))
        $lbl.Foreground = $script:mutedBrush
        $lbl.FontWeight = 'Normal'
        $ico.Data = $script:geoPlus
        $ico.Stroke = $script:mutedBrush
    }
}

function Paint-Options {
    Paint-Chip $chkDesktop   $lblDesktop   $icoDesktop   $T.optDesktop   $T.optDesktopTip   ([bool]$script:optMap.desktop)
    Paint-Chip $chkStartMenu $lblStartMenu $icoStartMenu $T.optStartMenu $T.optStartMenuTip ([bool]$script:optMap.startmenu)
    Paint-Chip $chkPath      $lblPath      $icoPath      $T.optPath      $T.optPathTip      ([bool]$script:optMap.addpath)
}

# 无头预览：把界面渲染成 PNG（不 ShowDialog），用于调样式与截图留档
if ($PreviewPng) {
    $txtTitle.Text = $Title
    $txtBadge.Text = if ($Uninstall) { $T.unBadge } elseif ($Version) { "v$Version" } else { 'DESKTOP' }
    if ($Uninstall) {
        $heroBanner.Background = $window.FindResource('HeroUninstallFill')
        $txtTitle.Text = $T.unTitle
        $txtTagline.Text = $T.unTagline
        $panelUnScope.Visibility = 'Visible'
        $txtUnScopeTitle.Text = $T.unScopeTitle
        $txtBody.Text = $T.unBody
        $txtBody2.Text = $T.unKeep; $txtBody2.Visibility = 'Visible'
        $btnSecondary.Content = $T.unCancel; $btnSecondary.Visibility = 'Visible'
        $btnPrimary.Content = $T.unBtn; $btnPrimary.Width = 176
        $btnPrimary.Background = $script:dangerFill
        $rowPath.Visibility = 'Collapsed'; $rowGithub.Visibility = 'Collapsed'
        $lnkCustomize.Visibility = 'Collapsed'; $panelCustom.Visibility = 'Collapsed'
        $txtWelcomeWhisper.Text = $T.uninstallWhisper
    } else {
        $txtTagline.Text = $T.tagline
        $btnPrimary.Content = $T.install
        $txtInstallTo.Text = $T.installTo
        $txtPath.Text = (Join-Path $env:ProgramFiles $Title)
        $script:optMap = @{ desktop = $true; startmenu = $true; addpath = $true }
        Paint-Options
        $runGithubLead.Text = $T.starLead
        $btnStar.Content = $T.starBtn
        $txtWelcomeWhisper.Text = $T.installWhisper
        if ($PreviewExpand) {
            $panelCustom.Visibility = 'Visible'
            $runCustomize.Text = $T.customizeOn
        } else {
            $runCustomize.Text = $T.customize
        }
    }
    # 预览指定屏：校验进度页与完成页的排版
    switch ($PreviewScreen) {
        'working' {
            Apply-WorkingTheme ([bool]$Uninstall)
            $txtPhase.Text = if ($Uninstall) { $T.unPhase2 } else { $T.phase2 }
            $barWork.Value = 54; $txtPct.Text = '54%'
            Update-StepDots 1 ([bool]$Uninstall)
            Show-Screen $screenWorking
        }
        'done' {
            if ($Uninstall) {
                $txtDoneTitle.Text = $T.unDoneTitle
                $txtDoneSub.Text = $T.unDoneSub
                $txtDoneWhisper.Text = $T.unDoneWhisper
                $btnRun.Visibility = 'Collapsed'
                $btnFinish.Content = $T.finish
                $btnFinish.Template = $script:primaryTpl
                $btnFinish.Background = $script:primaryFill
                $btnFinish.Foreground = [System.Windows.Media.Brushes]::White
                $btnFinish.FontSize = 14; $btnFinish.FontWeight = 'SemiBold'
                $btnFinish.Height = 44; $btnFinish.Width = 220; $btnFinish.Margin = '0,20,0,0'
            } else {
                $txtDoneTitle.Text = $T.doneTitle
                $txtDoneSub.Text = $T.doneSub + (Join-Path $env:ProgramFiles $Title)
                $txtDoneWhisper.Text = $T.doneWhisper
                $btnRun.Content = $T.runApp
                $btnFinish.Content = $T.finish
            }
            Show-Screen $screenDone
        }
        'error' {
            $txtDoneTitle.Text = $T.errTitle
            $txtDoneSub.Text = $T.errSub
            $txtDoneWhisper.Visibility = 'Collapsed'
            $doneRing.Fill = New-Object System.Windows.Media.SolidColorBrush([System.Windows.Media.Color]::FromRgb(0xFD, 0xF2, 0xF2))
            $doneCircle.Fill = New-Object System.Windows.Media.SolidColorBrush([System.Windows.Media.Color]::FromRgb(0xFB, 0xE2, 0xE0))
            $doneCheck.Data = 'M7,7 L19,19 M19,7 L7,19'
            $doneCheck.Stroke = $script:dangerBrush
            $btnRun.Visibility = 'Collapsed'
            $btnFinish.Content = $T.close
            $btnFinish.Template = $script:primaryTpl
            $btnFinish.Background = $script:primaryFill
            $btnFinish.Foreground = [System.Windows.Media.Brushes]::White
            $btnFinish.FontSize = 14; $btnFinish.FontWeight = 'SemiBold'
            $btnFinish.Height = 44; $btnFinish.Width = 220; $btnFinish.Margin = '0,20,0,0'
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

# 无头端到端：与「安装」按钮走完全相同的引擎调用（Run-Engine，含提权）
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

function Run-Engine([string]$dest, [string[]]$extra = @()) {
    $p = New-EngineProcess $dest $extra
    if ($null -eq $p) { return 1223 }
    $p.WaitForExit()
    try { return $p.ExitCode } catch { return -1 }
}

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
    Add-Type -Namespace Win32 -Name NativeMethods -MemberDefinition `
        '[DllImport("user32.dll", SetLastError = true, CharSet = CharSet.Auto)] public static extern IntPtr SendMessageTimeout(IntPtr hWnd, uint Msg, UIntPtr wParam, string lParam, uint fuFlags, uint uTimeout, out UIntPtr lpdwResult);'
    $result = [UIntPtr]::Zero
    [Win32.NativeMethods]::SendMessageTimeout(0xffff, 0x001A, [UIntPtr]::Zero, 'Environment', 2, 5000, [ref]$result) | Out-Null
}

function Set-FinishStyle([bool]$primary) {
    if ($primary) {
        $btnFinish.Template = $script:primaryTpl
        $btnFinish.Background = $script:primaryFill
        $btnFinish.Foreground = [System.Windows.Media.Brushes]::White
        $btnFinish.FontSize = 14; $btnFinish.FontWeight = 'SemiBold'
        $btnFinish.Width = 220; $btnFinish.Height = 44; $btnFinish.Margin = '0,20,0,0'
    } else {
        $btnFinish.Template = $script:ghostTpl
        $btnFinish.Background = [System.Windows.Media.Brushes]::Transparent
        $btnFinish.Foreground = $script:subInkBrush
        $btnFinish.FontSize = 12.5; $btnFinish.FontWeight = 'Normal'
        $btnFinish.Width = 236; $btnFinish.Height = 40; $btnFinish.Margin = '0,10,0,0'
    }
}

function Begin-Work([System.Diagnostics.Process]$proc, [bool]$install, [string]$dest) {
    $script:workPct = 2.0
    $script:workClosed = $false
    $script:workProc = $proc
    $script:workInstall = $install
    $script:workDest = $dest
    $barWork.Value = 2; $txtPct.Text = '2%'
    Update-StepDots 0 (-not $install)
    $script:workTimer = [System.Windows.Threading.DispatcherTimer]::new()
    $script:workTimer.Interval = [TimeSpan]::FromMilliseconds(80)
    $script:workTimer.Add_Tick({
        try {
            if (-not $script:workClosed -and $script:workPct -lt 90) {
                $script:workPct += [Math]::Max(0.08, (90.0 - $script:workPct) * 0.016)
                if ($script:workPct -ge 90) { $script:workPct = 90 }
                $barWork.Value = $script:workPct
                $txtPct.Text = ([int]$script:workPct).ToString() + '%'
                $stepIdx = if ($script:workPct -lt 28) { 0 } elseif ($script:workPct -lt 74) { 1 } else { 2 }
                Update-StepDots $stepIdx (-not $script:workInstall)
                if ($script:workInstall) {
                    $txtPhase.Text = if ($stepIdx -eq 0) { $T.phase1 } elseif ($stepIdx -eq 1) { $T.phase2 } else { $T.phase3 }
                } else {
                    $txtPhase.Text = if ($stepIdx -eq 0) { $T.unPhase1 } elseif ($stepIdx -eq 1) { $T.unPhase2 } else { $T.unPhase3 }
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
                        if ($script:workInstall) {
                            if (Test-Path (Join-Path $script:workDest 'fast-https.exe')) { $code = 0 }
                        } elseif (-not (Test-Path $script:workDest)) { $code = 0 }
                    }
                    $barWork.Value = 100; $txtPct.Text = '100%'
                    Update-StepDots 2 (-not $script:workInstall)
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
        $txtDoneWhisper.Visibility = 'Collapsed'
    } elseif ($ok) {
        $txtDoneWhisper.Visibility = 'Visible'
        if ($install) {
            $txtDoneTitle.Text = $T.doneTitle
            $txtDoneSub.Text = $T.doneSub + $dest
            $txtDoneWhisper.Text = $T.doneWhisper
            $btnRun.Content = $T.runApp
        } else {
            $txtDoneTitle.Text = $T.unDoneTitle
            $txtDoneSub.Text = $T.unDoneSub
            $txtDoneWhisper.Text = $T.unDoneWhisper
        }
    } else {
        $txtDoneTitle.Text = $T.errTitle
        $txtDoneSub.Text = $T.errSub
        $txtDoneWhisper.Visibility = 'Collapsed'
    }
    if ($ok) {
        $doneRing.Fill = New-Object System.Windows.Media.SolidColorBrush([System.Windows.Media.Color]::FromRgb(0xF1, 0xFA, 0xF5))
        $doneCircle.Fill = New-Object System.Windows.Media.SolidColorBrush([System.Windows.Media.Color]::FromRgb(0xE3, 0xF5, 0xEB))
        $doneCheck.Data = 'M5,13 L11,19 L21,7'
        $doneCheck.Stroke = New-Object System.Windows.Media.SolidColorBrush([System.Windows.Media.Color]::FromRgb(0x13, 0x80, 0x50))
        $btnFinish.Content = $T.finish
    } else {
        $doneRing.Fill = New-Object System.Windows.Media.SolidColorBrush([System.Windows.Media.Color]::FromRgb(0xFD, 0xF2, 0xF2))
        $doneCircle.Fill = New-Object System.Windows.Media.SolidColorBrush([System.Windows.Media.Color]::FromRgb(0xFB, 0xE2, 0xE0))
        $doneCheck.Data = 'M7,7 L19,19 M19,7 L7,19'
        $doneCheck.Stroke = $script:dangerBrush
        $btnFinish.Content = $T.close
    }
    $primaryOnly = -not ($ok -and $install)
    $btnRun.Visibility = if ($primaryOnly) { 'Collapsed' } else { 'Visible' }
    Set-FinishStyle $primaryOnly
    Show-Screen $screenDone
}

# ---- 行为 ----
$dragBar.Add_MouseLeftButtonDown({ $window.DragMove() })
$btnClose.Add_Click({ $window.Close() })

$defaultDir = Join-Path $env:ProgramFiles $Title
try {
    $reg = Get-ItemProperty 'HKLM:\Software\Fast-Https' -Name InstallDir -ErrorAction Stop
    $prev = $reg.InstallDir
    $isTemp = $prev -and ($prev -like "$env:TEMP*" -or $prev -like "$env:TMP*" -or $prev -like "*\Temp\*")
    if ($prev -and (Test-Path $prev) -and -not $isTemp) { $defaultDir = $prev }
} catch { }

function Open-Link([string]$url) { Start-Process $url }

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
        $helper = New-Object System.Windows.Interop.WindowInteropHelper($window)
        if ($helper.Handle -eq [IntPtr]::Zero) { return }
        $MONITOR_DEFAULTTONEAREST = 2
        $hMon = [Wizard.NativeDisplays]::MonitorFromWindow($helper.Handle, $MONITOR_DEFAULTTONEAREST)
        if ($hMon -eq [IntPtr]::Zero) { return }
        $mi = New-Object Wizard.NativeDisplays+MONITORINFO
        $mi.cbSize = [System.Runtime.InteropServices.Marshal]::SizeOf($mi)
        if (-not [Wizard.NativeDisplays]::GetMonitorInfo($hMon, [ref]$mi)) { return }
        $waW = $mi.rcWork.Right - $mi.rcWork.Left
        $waH = $mi.rcWork.Bottom - $mi.rcWork.Top
        $dpi = [Wizard.NativeDisplays]::GetDpiForWindow($helper.Handle)
        if ($dpi -eq 0) { $dpi = 96 }
        $scale = $dpi / 96.0
        $wpfW = $window.Width * $scale
        $wpfH = $window.Height * $scale
        $window.Left = $mi.rcWork.Left + (($waW - $wpfW) / 2) / $scale
        $window.Top = $mi.rcWork.Top + (($waH - $wpfH) / 2) / $scale
    } catch { }
}

function Join-ProductSubdir([string]$path) {
    $p = $path.Trim()
    if (-not $p) { return '' }
    $p = $p.TrimEnd('\')
    if ($p -match '^[A-Za-z]:$') { $p = $p + '\' }
    if ((Split-Path -Leaf $p) -ieq $Title) { return $p.TrimEnd('\') }
    return (Join-Path $p $Title)
}

function Test-DirNotEmpty([string]$path) {
    if (-not (Test-Path -LiteralPath $path)) { return $false }
    if (Test-Path -LiteralPath $path -PathType Leaf) { return $true }
    return @(Get-ChildItem -LiteralPath $path -Force -ErrorAction SilentlyContinue).Count -gt 0
}

function Test-ForeignDir([string]$path) {
    if (-not (Test-DirNotEmpty $path)) { return $false }
    if (Test-Path -LiteralPath (Join-Path $path 'fast-https.exe')) { return $false }
    if (Test-Path -LiteralPath (Join-Path $path 'fast-https-desktop.exe')) { return $false }
    return $true
}

# Initialize-UninstallScreen 卸载确认屏的文案与事件绑定。
function Initialize-UninstallScreen {
    $heroBanner.Background = $window.FindResource('HeroUninstallFill')
    $txtTitle.Text = $T.unTitle
    $txtBadge.Text = $T.unBadge
    $txtTagline.Text = $T.unTagline
    $panelUnScope.Visibility = 'Visible'
    $txtUnScopeTitle.Text = $T.unScopeTitle
    $txtBody.Text = $T.unBody
    $txtBody2.Text = $T.unKeep
    $txtBody2.Visibility = 'Visible'
    $btnSecondary.Content = $T.unCancel
    $btnSecondary.Visibility = 'Visible'
    $btnPrimary.Content = $T.unBtn
    $btnPrimary.Width = 176
    $btnPrimary.Background = $script:dangerFill
    $rowPath.Visibility = 'Collapsed'
    $lnkCustomize.Visibility = 'Collapsed'; $panelCustom.Visibility = 'Collapsed'
    $rowGithub.Visibility = 'Collapsed'
    $txtWelcomeWhisper.Text = $T.uninstallWhisper
    Apply-WorkingTheme $true

    $btnSecondary.Add_Click({
        $script:exitCode = 1
        $window.Close()
    })
    $btnPrimary.Add_Click({
        $txtPhase.Text = $T.unPhase1
        $txtPct.Text = '0%'
        Show-Screen $screenWorking
        $window.UpdateLayout()
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
    $heroBanner.Background = $window.FindResource('HeroInstallFill')
    $txtTitle.Text = $Title
    $txtBadge.Text = if ($Version) { "v$Version" } else { 'DESKTOP' }
    $txtTagline.Text = $T.tagline
    $btnPrimary.Content = $T.install
    $txtInstallTo.Text = $T.installTo
    $txtPath.Text = $defaultDir
    $btnBrowse.Content = '…'
    $script:optMap = @{ desktop = $true; startmenu = $true; addpath = $true }
    Paint-Options
    $runGithubLead.Text = $T.starLead
    $btnStar.Content = $T.starBtn
    $txtWelcomeWhisper.Text = $T.installWhisper
    Apply-WorkingTheme $false

    $script:customOpen = $false
    $panelCustom.Visibility = 'Collapsed'
    $runCustomize.Text = $T.customize
    $lnkCustomize.Add_Click({
        "customize clicked: open=$script:customOpen" | Out-File (Join-Path $env:TEMP "fast-https-wizard-diag.txt") -Append
        $script:customOpen = -not $script:customOpen
        $panelCustom.Visibility = if ($script:customOpen) { 'Visible' } else { 'Collapsed' }
        $runCustomize.Text = if ($script:customOpen) { $T.customizeOn } else { $T.customize }
        Set-CenterToCurrentMonitor
    })
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
        $dlg.Description = $T.pickDir
        if ($dlg.ShowDialog() -eq 'OK') {
            $txtPath.Text = Join-ProductSubdir $dlg.SelectedPath
        }
    })

    $btnPrimary.Add_Click({
        $dest = $txtPath.Text.Trim()
        if (-not $dest) { return }
        if (Test-ForeignDir $dest) {
            $ans = [System.Windows.Forms.MessageBox]::Show(
                ($T.notEmpty -f $dest), $T.notEmptyTitle,
                [System.Windows.Forms.MessageBoxButtons]::YesNo,
                [System.Windows.Forms.MessageBoxIcon]::Warning)
            if ($ans -ne [System.Windows.Forms.MessageBoxResult]::Yes) { return }
        }
        $txtPhase.Text = $T.phase1
        $txtPct.Text = '0%'
        Show-Screen $screenWorking
        $window.UpdateLayout()
        $extra = @()
        if (-not $script:optMap.desktop) { $extra += '/DSC_DESKTOP=0' }
        if (-not $script:optMap.startmenu) { $extra += '/DSC_STARTMENU=0' }
        if (-not $script:optMap.addpath) { $extra += '/DSC_PATH=0' }
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
        if (Test-Path $exe) { Start-Process $exe -WorkingDirectory $txtPath.Text.Trim() }
        else { Start-Process $txtPath.Text.Trim() }
        $window.Close()
    })
}

$exitCode = 0

if ($Uninstall) {
    Initialize-UninstallScreen
    $btnClose.Add_Click({ $script:exitCode = 1; $window.Close() })
} else {
    Initialize-WelcomeScreen
}

$btnFinish.Add_Click({ $window.Close() })

# ---- 无头自检 ----
if ($SelfTest) {
    $names = 'dragBar','btnClose','screenWelcome','screenWorking','screenDone','imgLogo','imgLogo2','txtTitle','txtTagline','txtBody','txtBody2','btnPrimary','btnSecondary','rowPath','txtInstallTo','txtPath','btnBrowse','lnkCustomize','runCustomize','panelCustom','rowGithub','runGithubLead','btnStar','rowOptions','chkDesktop','chkStartMenu','chkPath','txtPhase','barWork','txtPct','txtWorkHint','doneCircle','doneCheck','txtDoneTitle','txtDoneSub','btnRun','btnFinish'
    foreach ($n in $names) { if ($null -eq (N $n)) { Write-Error "selftest: missing element $n"; exit 1 } }
    if (-not $Self) { $script:Self = "selftest-fake.exe" }
    Initialize-WelcomeScreen
    Initialize-UninstallScreen
    Write-Host "selftest ok: wizard UI + bindings loaded"
    exit 0
}

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
