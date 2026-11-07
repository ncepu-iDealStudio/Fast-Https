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
  !define APP_VERSION "1.6.1"
!endif
!ifndef APP_VERSION4
  !define APP_VERSION4 "1.6.1.0"
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
!define APP_PUBLISHER "NCEPU BJ"
!define APP_URL "https://gitee.com/ncepu-bj/fast-https"
!define UNINST_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}"

Name "${APP_NAME} ${APP_VERSION} (${APP_ARCH})"
OutFile "${OUT_FILE}"
InstallDir "$PROGRAMFILES64\${APP_NAME}"
InstallDirRegKey HKLM "Software\${APP_NAME}" "InstallDir"
RequestExecutionLevel admin
SetCompressor /SOLID lzma
ShowInstDetails show
ShowUninstDetails show

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

; ------------------------------------------------------------ 安装

Section "Fast-Https 核心文件（必选）" SEC_CORE
  SectionIn RO

  ; 升级时先停掉在跑的桌面端与服务进程，否则文件被占用会替换失败
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

  WriteUninstaller "$INSTDIR\uninstall.exe"

  WriteRegStr HKLM "Software\${APP_NAME}" "InstallDir" "$INSTDIR"
  WriteRegStr HKLM "Software\${APP_NAME}" "Version" "${APP_VERSION}"

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

Section "创建桌面快捷方式" SEC_DESKTOP
  ${If} ${FileExists} "$INSTDIR\${DESKTOP_EXE}"
    CreateShortcut "$DESKTOP\Fast-Https 桌面管理端.lnk" "$INSTDIR\${DESKTOP_EXE}"
  ${Else}
    CreateShortcut "$DESKTOP\Fast-Https 安装目录.lnk" "$INSTDIR"
  ${EndIf}
SectionEnd

Section "添加到 Windows 开始菜单" SEC_STARTMENU
  CreateDirectory "$SMPROGRAMS\${APP_NAME}"
  CreateShortcut "$SMPROGRAMS\${APP_NAME}\使用文档.lnk" "$INSTDIR\docs\usage.html"
  CreateShortcut "$SMPROGRAMS\${APP_NAME}\安装目录.lnk" "$INSTDIR"
  ${If} ${FileExists} "$INSTDIR\${DESKTOP_EXE}"
    CreateShortcut "$SMPROGRAMS\${APP_NAME}\桌面管理端.lnk" "$INSTDIR\${DESKTOP_EXE}"
  ${EndIf}
  CreateShortcut "$SMPROGRAMS\${APP_NAME}\卸载.lnk" "$INSTDIR\uninstall.exe"
SectionEnd

; ------------------------------------------------------------ 卸载

Section "Uninstall"
  nsExec::ExecToLog 'taskkill /F /IM ${DESKTOP_EXE}'
  nsExec::ExecToLog 'taskkill /F /IM ${APP_EXE}'

  DeleteRegKey HKLM "${UNINST_KEY}"
  DeleteRegKey HKLM "Software\${APP_NAME}"

  Delete "$DESKTOP\Fast-Https 桌面管理端.lnk"
  Delete "$DESKTOP\Fast-Https 安装目录.lnk"
  Delete "$SMPROGRAMS\${APP_NAME}\*.lnk"
  RMDir "$SMPROGRAMS\${APP_NAME}"

  RMDir /r "$INSTDIR"
SectionEnd

; ------------------------------------------------------------ 描述

!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
  !insertmacro MUI_DESCRIPTION_TEXT ${SEC_CORE} "程序本体、默认配置、默认站点与使用文档。"
  !insertmacro MUI_DESCRIPTION_TEXT ${SEC_DESKTOP} "在桌面创建图形入口，指向桌面管理端 fast-https-desktop.exe（没有它时退化为安装目录快捷方式），可取消。"
  !insertmacro MUI_DESCRIPTION_TEXT ${SEC_STARTMENU} "在 Windows 开始菜单创建程序分组，可取消。"
!insertmacro MUI_FUNCTION_DESCRIPTION_END
