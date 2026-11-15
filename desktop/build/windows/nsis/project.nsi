; Fast-Https Desktop — NSIS 安装包
;
; 由 `wails3 task windows:create:nsis:installer` 调用，会传入
; -DARG_WAILS_AMD64_BINARY=<路径> 或 -DARG_WAILS_ARM64_BINARY=<路径>。
; 手工编译：
;   makensis -DARG_WAILS_AMD64_BINARY="..\..\bin\fast-https-desktop.exe" project.nsi
;
; 这里显式定义版本与版权；wails_tools.nsh 用 !ifndef 保护，不会覆盖。
Unicode true

!define INFO_PRODUCTVERSION "1.7.0"
!define INFO_COPYRIGHT "GPL-2.0, NCEPU BJ"
!include "wails_tools.nsh"

; 桌面端与主程序同目录分发，卸载时除了自己还有这两个进程可能在跑。
; 三个进程都必须在删文件前停掉：服务占着 exe、日志与 pid 文件时
; RMDir /r 删不掉它们，就是卸载后最常见的残留。
!define SERVER_EXE "fast-https.exe"
!define MONITOR_EXE "fast-https-monitor.exe"

VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"
VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME}"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

ManifestDPIAware true

!include "MUI.nsh"
!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
!define MUI_FINISHPAGE_NOAUTOCLOSE
!define MUI_ABORTWARNING

; ---- 定制向导：品牌图（由 build/appicon.png 生成，见 gen-wizard-bitmaps.ps1）----
!define MUI_WELCOMEFINISHPAGE_BITMAP "wizard-side.bmp"
!define MUI_HEADERIMAGE
!define MUI_HEADERIMAGE_BITMAP "wizard-head.bmp"
; 完成页给「运行」勾选框，装完即开
!define MUI_FINISHPAGE_RUN "$INSTDIR\${PRODUCT_EXECUTABLE}"
; 卸载确认页明确告知清理范围（文案见下方 LangString，按语言切换）
!define MUI_UNCONFIRMPAGE_TEXT_TOP "$(UNINST_CONFIRM_TOP)"
!define MUI_UNCONFIRMPAGE_TEXT_LOCATION "$(UNINST_CONFIRM_BODY)"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
; 快捷方式与开始菜单由用户勾选，不强制创建
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "SimpChinese"
!insertmacro MUI_LANGUAGE "English"

; ---- 定制向导文案：覆盖 MUI 语言文件里的默认文本（同名 LangString 后定义生效）----
LangString MUI_TEXT_WELCOME_INFO_TITLE ${LANG_SIMPCHINESE} "欢迎安装 Fast-Https Desktop"
LangString MUI_TEXT_WELCOME_INFO_TITLE ${LANG_ENGLISH} "Welcome to Fast-Https Desktop"
LangString MUI_TEXT_WELCOME_INFO_TEXT ${LANG_SIMPCHINESE} "本向导将把 Fast-Https 桌面管理端安装到这台电脑。$\r$\n$\r$\n管理端负责查看运行状态、管理站点与证书、修改配置；fast-https.exe 服务由它启停，独立于管理端运行，关掉管理端服务照常在跑。$\r$\n$\r$\n$_CLICK"
LangString MUI_TEXT_WELCOME_INFO_TEXT ${LANG_ENGLISH} "This wizard installs the Fast-Https desktop console on this computer.$\r$\n$\r$\nThe console manages service status, sites, certificates and configuration. The fast-https.exe service runs independently and stays up when the console closes.$\r$\n$\r$\n$_CLICK"
LangString MUI_TEXT_FINISH_INFO_TITLE ${LANG_SIMPCHINESE} "安装完成"
LangString MUI_TEXT_FINISH_INFO_TITLE ${LANG_ENGLISH} "Installation complete"
LangString MUI_TEXT_FINISH_INFO_TEXT ${LANG_SIMPCHINESE} "Fast-Https Desktop 已就绪。$\r$\n第一次用可以先点左下角「帮助文档」，五分钟把站点跑起来。"
LangString MUI_TEXT_FINISH_INFO_TEXT ${LANG_ENGLISH} "Fast-Https Desktop is ready.$\r$\nFor a five-minute walkthrough, open Help at the bottom-left of the app."
LangString MUI_TEXT_FINISH_RUN ${LANG_SIMPCHINESE} "运行 Fast-Https Desktop(&R)"
LangString MUI_TEXT_FINISH_RUN ${LANG_ENGLISH} "Run Fast-Https Desktop(&R)"
LangString MUI_TEXT_ABORTWARNING ${LANG_SIMPCHINESE} "安装尚未完成，确定要退出吗？"
LangString MUI_TEXT_ABORTWARNING ${LANG_ENGLISH} "Setup is incomplete. Quit anyway?"
LangString UNINST_CONFIRM_TOP ${LANG_SIMPCHINESE} "完全卸载 Fast-Https Desktop"
LangString UNINST_CONFIRM_TOP ${LANG_ENGLISH} "Completely uninstall Fast-Https Desktop"
LangString UNINST_CONFIRM_BODY ${LANG_SIMPCHINESE} "卸载会删除程序、配置、站点、证书、日志、状态与缓存，不留残留。$\r$\n$\r$\n如果只是想关掉窗口，请改用「隐藏到后台托盘」，不必卸载。"
LangString UNINST_CONFIRM_BODY ${LANG_ENGLISH} "Uninstalling removes the program along with its config, sites, certificates, logs, state and caches — nothing is left behind.$\r$\n$\r$\nIf you only want to close the window, use Hide to tray instead."

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe"
!if "${WAILS_INSTALL_SCOPE}" == "user"
    InstallDir "$LOCALAPPDATA\Programs\${INFO_PRODUCTNAME}"
!else
    InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
!endif
ShowInstDetails show

Function .onInit
   !insertmacro wails.checkArchitecture
FunctionEnd

Section "Fast-Https 桌面管理端（必选）" SEC_CORE
    SectionIn RO
    !insertmacro wails.setShellContext

    ; 升级替换前先停进程，否则被占用的文件替换失败、留下新旧混杂的目录
    nsExec::ExecToLog 'taskkill /F /IM ${MONITOR_EXE}'
    nsExec::ExecToLog 'taskkill /F /IM ${PRODUCT_EXECUTABLE}'
    nsExec::ExecToLog 'taskkill /F /IM ${SERVER_EXE}'

    !insertmacro wails.webview2runtime

    SetOutPath $INSTDIR
    !insertmacro wails.files
    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols
    !insertmacro wails.writeUninstaller
SectionEnd

Section "创建桌面快捷方式" SEC_DESKTOP
    CreateShortCut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
SectionEnd

Section "添加到 Windows 开始菜单" SEC_STARTMENU
    CreateDirectory "$SMPROGRAMS\${INFO_PRODUCTNAME}"
    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}\卸载.lnk" "$INSTDIR\uninstall.exe"
SectionEnd

Section "uninstall"
    !insertmacro wails.setShellContext

    ; 先停进程再删文件（顺序与安装一致，Monitor 先停）：
    ; 进程占着 exe、日志与 pid 时 RMDir /r 删不干净，会留一地残留
    nsExec::ExecToLog 'taskkill /F /IM ${MONITOR_EXE}'
    nsExec::ExecToLog 'taskkill /F /IM ${PRODUCT_EXECUTABLE}'
    nsExec::ExecToLog 'taskkill /F /IM ${SERVER_EXE}'

    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"
    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}\*.lnk"
    RMDir "$SMPROGRAMS\${INFO_PRODUCTNAME}"

    ; WebView2 配置目录（主题偏好、localStorage、缓存）
    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}"
    ; 程序目录连同 config / logs / state / pid 一起删
    RMDir /r $INSTDIR

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols
    !insertmacro wails.deleteUninstaller
SectionEnd

!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
  !insertmacro MUI_DESCRIPTION_TEXT ${SEC_CORE} "程序本体，包含 WebView2 运行时引导。"
  !insertmacro MUI_DESCRIPTION_TEXT ${SEC_DESKTOP} "在桌面创建程序入口，可取消。"
  !insertmacro MUI_DESCRIPTION_TEXT ${SEC_STARTMENU} "在 Windows 开始菜单创建程序分组，可取消。"
!insertmacro MUI_FUNCTION_DESCRIPTION_END
