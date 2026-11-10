; Fast-Https Windows 安装包（NSIS 3.x）
;
; 一般由 build/package.ps1 调用，传入以下宏：
;   /DAPP_VERSION   显示用版本号（由 build/package.ps1 传入）
;   /DAPP_VERSION4  VIProductVersion 用的四段版本号（由 build/package.ps1 传入）
;   /DAPP_ARCH      amd64 / arm64
;   /DSTAGE_DIR     解压好的发行目录（fast-https.exe、config、httpdoc…）
;   /DOUT_FILE      输出的安装包路径
;
; 桌面管理端 fast-https-desktop.exe 由 desktop/ 独立构建，若一并放进 STAGE_DIR，
; 组件页会给它创建快捷方式；没有时不会创建死链。

Unicode true
!include "MUI2.nsh"
!include "LogicLib.nsh"

!ifndef APP_VERSION
  !define APP_VERSION "1.7.0"
!endif
!ifndef APP_VERSION4
  !define APP_VERSION4 "1.7.0.0"
!endif
!ifndef APP_ARCH
  !define APP_ARCH "amd64"
!endif
!ifndef STAGE_DIR
  !error "STAGE_DIR 未定义：请用 /DSTAGE_DIR=<解压目录> 指定发行文件所在目录"
!endif
!ifndef OUT_FILE
  !define OUT_FILE "fast-https-setup.exe"
!endif

!define APP_NAME "Fast-Https"
!define APP_EXE "fast-https.exe"
!define DESKTOP_EXE "fast-https-desktop.exe"
!define MONITOR_EXE "fast-https-monitor.exe"
!define APP_PUBLISHER "NCEPU BJ"
!define APP_URL "https://gitee.com/ncepu-bj/fast-https"
!define UNINST_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}"

Name "${APP_NAME} ${APP_VERSION} (${APP_ARCH})"
OutFile "${OUT_FILE}"
InstallDir "$PROGRAMFILES64\${APP_NAME}"
InstallDirRegKey HKLM "Software\${APP_NAME}" "InstallDir"
; 按需提权：本进程以普通用户启动（双击即可，不弹 UAC），自绘向导负责
; 界面；点「安装」后向导用 RunAs 重新拉起本程序 /S（那时才弹一次 UAC）
; 执行真正的安装段。静默模式（/S）由调用方保证管理员身份。
; 注意：RequestExecutionLevel admin 会让双击立即弹 UAC，用户取消就
; exit 2 什么都没有——曾表现为「安装包打完装不上」。
RequestExecutionLevel user
SetCompressor /SOLID lzma
; 引擎界面彻底不可见：交互模式的界面 100% 由自绘向导负责（Section 宿主
; 拉起 wizard.ps1 后 Quit），nsExec 的 ExecToLog 输出进 DetailView，而
; DetailView 在 nevershow 下不会创建窗口；静默模式本来就无窗口。
ShowInstDetails nevershow
ShowUninstDetails nevershow

VIProductVersion "${APP_VERSION4}"
VIAddVersionKey "ProductName" "${APP_NAME}"
VIAddVersionKey "FileDescription" "${APP_NAME} 安装程序"
VIAddVersionKey "FileVersion" "${APP_VERSION}"
VIAddVersionKey "ProductVersion" "${APP_VERSION} (${APP_ARCH})"
VIAddVersionKey "CompanyName" "${APP_PUBLISHER}"
VIAddVersionKey "LegalCopyright" "GPL-2.0"

!define MUI_ABORTWARNING
!define MUI_FINISHPAGE_NOAUTOCLOSE
!define MUI_UNFINISHPAGE_NOAUTOCLOSE

; ---- 定制向导：品牌图与桌面端安装向导同一套（由应用图标生成，见
; desktop/build/windows/nsis/gen-wizard-bitmaps.ps1）----
!define MUI_WELCOMEFINISHPAGE_BITMAP "..\..\..\desktop\build\windows\nsis\wizard-side.bmp"
!define MUI_HEADERIMAGE
!define MUI_HEADERIMAGE_BITMAP "..\..\..\desktop\build\windows\nsis\wizard-head.bmp"

; 安装流程不出 MUI 页面：交互模式由隐藏的向导宿主段弹自绘向导
; （wizard.ps1，圆角卡片 + 品牌图 + 胶囊按钮），安装本体在 /S 静默
; 模式下作为引擎执行。MUI 只保留卸载页与多语言文案。
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "SimpChinese"
!insertmacro MUI_LANGUAGE "English"

; ------------------------------------------------------------ 向导宿主
; 交互模式在 .onInit 里把界面交给自绘向导后退出：用 Exec（ShellExecute，
; 不等待、不进 job 对象）拉起向导，主进程 Quit 不会连带杀掉它——曾用
; Section+nsExec 方案，引擎「正在安装」窗口（#32770）会与自绘向导同屏；
; 曾用 .onInit+nsExec 方案，nsExec 等待子进程且子进程随主进程退出被杀，
; 向导弹不出来。Exec+Quit 是两个问题的合解。静默模式（/S）直接走安装段。

Function .onInit
  IfSilent oninit_done
  SetOutPath "$PLUGINSDIR"
  File "/oname=wizard.ps1" "wizard.ps1"
  File "/oname=appicon.png" "..\..\..\desktop\build\appicon.png"
  Exec 'powershell -NoProfile -STA -ExecutionPolicy Bypass -File "$PLUGINSDIR\wizard.ps1" --self "$EXEPATH" --icon "$PLUGINSDIR\appicon.png" --title "${APP_NAME}" --version "${APP_VERSION}"'
  Quit
oninit_done:
FunctionEnd

; ------------------------------------------------------------ 向导宿主（已并入 .onInit）
; 历史方案留档：Section+nsExec（引擎窗口同屏）、.onInit+nsExec（子进程被
; job 连杀）。现行为：上方 .onInit + Exec。

Section "-wizard host" SEC_WIZARD
  ; 静默模式不会走到这里（.onInit 的 IfSilent 分支直通安装段）；
  ; 交互模式已在 .onInit 里 Quit。保留空段以防 MUI 依赖 Section 存在。
SectionEnd

; ------------------------------------------------------------ 安装

; 快捷方式开关：默认 1（创建）。自绘向导按用户勾选传 /DSC_DESKTOP=0/1
; 与 /DSC_STARTMENU=0/1；/S 静默用默认值。
!ifndef SC_DESKTOP
  !define SC_DESKTOP "1"
!endif
!ifndef SC_STARTMENU
  !define SC_STARTMENU "1"
!endif

Section "Fast-Https 核心文件（必选）" SEC_CORE
  SectionIn RO

  ; 升级时先停掉在跑的桌面端、Monitor 与服务进程，否则文件被占用会替换失败。
  ; Monitor 先停：它与主程序分开托管，留着它会把自己被替换的那一下记成一条假事件。
  nsExec::ExecToLog 'taskkill /F /IM ${MONITOR_EXE}'
  nsExec::ExecToLog 'taskkill /F /IM ${DESKTOP_EXE}'
  nsExec::ExecToLog 'taskkill /F /IM ${APP_EXE}'

  ; 用户改过的配置装完再放回去，升级不覆盖
  StrCpy $R0 "0"
  ${If} ${FileExists} "$INSTDIR\config\fast-https.json"
    CopyFiles /SILENT "$INSTDIR\config\fast-https.json" "$TEMP\fast-https.config.bak"
    StrCpy $R0 "1"
  ${EndIf}

  SetOutPath "$INSTDIR"
  File /r "${STAGE_DIR}\*.*"

  ${If} $R0 == "1"
    CopyFiles /SILENT "$TEMP\fast-https.config.bak" "$INSTDIR\config\fast-https.json"
    Delete "$TEMP\fast-https.config.bak"
  ${EndIf}

  ; 运行期目录，压缩包里可能只有占位文件
  CreateDirectory "$INSTDIR\logs"
  CreateDirectory "$INSTDIR\config\cert"
  CreateDirectory "$INSTDIR\config\conf.d"

  ; 自绘向导脚本装进安装目录：卸载时 un.onInit 用它弹自绘卸载确认
  File "/oname=wizard.ps1" "wizard.ps1"

  ; 运行期要写 pid、日志、证书与配置备份，而 Program Files 默认对普通
  ; 用户只读：不授权的话 start 写不出 pid 文件，stop 就会当作「没在跑」
  ; 直接成功（表现为「点了停止但服务还在访问」），日志与首证也写不进去。
  ; 用 Users 组的 SID 授权，避免不同语言的组名差异。
  nsExec::ExecToLog 'icacls "$INSTDIR" /grant *S-1-5-32-545:(OI)(CI)M /T /C /Q'

  WriteUninstaller "$INSTDIR\uninstall.exe"

  WriteRegStr HKLM "Software\${APP_NAME}" "InstallDir" "$INSTDIR"
  WriteRegStr HKLM "Software\${APP_NAME}" "Version" "${APP_VERSION}"
  ; 快捷方式开关由自绘向导传入（/DSC_DESKTOP=1 /DSC_STARTMENU=1），缺省都建；
  ; 记进注册表，卸载只回收实际创建过的快捷方式。
  WriteRegStr HKLM "Software\${APP_NAME}" "ShortcutDesktop" "${SC_DESKTOP}"
  WriteRegStr HKLM "Software\${APP_NAME}" "ShortcutStartMenu" "${SC_STARTMENU}"

  ${If} ${SC_DESKTOP} == "0"
    ; 用户取消桌面快捷方式：清掉升级残留的旧图标
    Delete "$DESKTOP\Fast-Https 桌面管理端.lnk"
    Delete "$DESKTOP\Fast-Https 安装目录.lnk"
  ${EndIf}
  ${If} ${SC_DESKTOP} != "0"
    ${If} ${FileExists} "$INSTDIR\${DESKTOP_EXE}"
      CreateShortcut "$DESKTOP\Fast-Https 桌面管理端.lnk" "$INSTDIR\${DESKTOP_EXE}"
    ${Else}
      CreateShortcut "$DESKTOP\Fast-Https 安装目录.lnk" "$INSTDIR"
    ${EndIf}
  ${EndIf}

  ${If} ${SC_STARTMENU} != "0"
    CreateDirectory "$SMPROGRAMS\${APP_NAME}"
    CreateShortcut "$SMPROGRAMS\${APP_NAME}\使用文档.lnk" "$INSTDIR\docs\usage.html"
    CreateShortcut "$SMPROGRAMS\${APP_NAME}\安装目录.lnk" "$INSTDIR"
    ${If} ${FileExists} "$INSTDIR\${DESKTOP_EXE}"
      CreateShortcut "$SMPROGRAMS\${APP_NAME}\桌面管理端.lnk" "$INSTDIR\${DESKTOP_EXE}"
    ${EndIf}
    CreateShortcut "$SMPROGRAMS\${APP_NAME}\卸载.lnk" "$INSTDIR\uninstall.exe"
  ${EndIf}

  WriteRegStr HKLM "${UNINST_KEY}" "DisplayName" "${APP_NAME} ${APP_VERSION} (${APP_ARCH})"
  WriteRegStr HKLM "${UNINST_KEY}" "DisplayVersion" "${APP_VERSION}"
  WriteRegStr HKLM "${UNINST_KEY}" "Publisher" "${APP_PUBLISHER}"
  WriteRegStr HKLM "${UNINST_KEY}" "URLInfoAbout" "${APP_URL}"
  WriteRegStr HKLM "${UNINST_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKLM "${UNINST_KEY}" "DisplayIcon" "$INSTDIR\${APP_EXE}"
  WriteRegStr HKLM "${UNINST_KEY}" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegDWORD HKLM "${UNINST_KEY}" "NoModify" 1
  WriteRegDWORD HKLM "${UNINST_KEY}" "NoRepair" 1
SectionEnd

; ------------------------------------------------------------ 卸载向导
; 交互卸载同样交给自绘向导（wizard.ps1 --uninstall 弹确认页，然后静默
; 调起 uninstall.exe /S）；向导脚本不在时回落到下面的 MUI 卸载页。

Function un.onInit
  IfSilent un_done
  IfFileExists "$INSTDIR\wizard.ps1" 0 un_done
  nsExec::ExecToLog 'powershell -NoProfile -STA -ExecutionPolicy Bypass -File "$INSTDIR\wizard.ps1" --uninstall --self "$INSTDIR\uninstall.exe" --title "${APP_NAME}"'
  Quit
un_done:
FunctionEnd

; ------------------------------------------------------------ 卸载

Section "Uninstall"
  ; 顺序与安装一致：Monitor 先停，避免它把卸载过程记成一条假的「不可达」
  nsExec::ExecToLog 'taskkill /F /IM ${MONITOR_EXE}'
  nsExec::ExecToLog 'taskkill /F /IM ${DESKTOP_EXE}'
  nsExec::ExecToLog 'taskkill /F /IM ${APP_EXE}'

  DeleteRegKey HKLM "${UNINST_KEY}"

  ; 只回收实际创建过的快捷方式（按安装时写进注册表的开关），
  ; 再通配兜底一次，开始菜单整组删除
  ReadRegStr $R1 HKLM "Software\${APP_NAME}" "ShortcutDesktop"
  ${If} $R1 != "0"
    Delete "$DESKTOP\Fast-Https 桌面管理端.lnk"
    Delete "$DESKTOP\Fast-Https 安装目录.lnk"
  ${EndIf}
  DeleteRegKey HKLM "Software\${APP_NAME}"

  Delete "$DESKTOP\Fast-Https 桌面管理端.lnk"
  Delete "$DESKTOP\Fast-Https 安装目录.lnk"
  Delete "$SMPROGRAMS\${APP_NAME}\*.lnk"
  RMDir "$SMPROGRAMS\${APP_NAME}"

  ; 桌面端的 WebView2 配置目录（主题偏好、localStorage、缓存）也清掉：
  ; 不删的话卸载完 %APPDATA%\fast-https-desktop.exe 还会留着
  RMDir /r "$AppData\${DESKTOP_EXE}"

  RMDir /r "$INSTDIR"
SectionEnd

; ------------------------------------------------------------ 描述
; 安装流程由自绘向导（wizard.ps1）接管，组件页已移除，无组件描述可写。


