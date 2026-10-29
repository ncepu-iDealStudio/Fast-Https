; Fast-Https Windows 安装包（NSIS 3.x）
;
; 一般由 scripts/package.ps1 调用，传入以下宏：
;   /DAPP_VERSION   显示用版本号，例如 1.4.0
;   /DAPP_VERSION4  VIProductVersion 用的四段版本号，例如 1.4.0.0
;   /DAPP_ARCH      amd64 / arm64
;   /DSTAGE_DIR     解压好的发行目录（fast-https.exe、config、httpdoc…）
;   /DOUT_FILE      输出的安装包路径
;
; 手工编译示例：
;   makensis /DAPP_VERSION=1.4.0 /DAPP_VERSION4=1.4.0.0 /DAPP_ARCH=amd64 ^
;            /DSTAGE_DIR=dist\.stage\amd64 ^
;            /DOUT_FILE=dist\fast-https-1.4.0-windows-amd64-setup.exe ^
;            installer\windows\fast-https.nsi

Unicode true
!include "MUI2.nsh"
!include "LogicLib.nsh"

!ifndef APP_VERSION
  !define APP_VERSION "1.4.0"
!endif
!ifndef APP_VERSION4
  !define APP_VERSION4 "1.4.0.0"
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
!define APP_PUBLISHER "NCEPU BJ"
!define APP_URL "https://gitee.com/ncepu-bj/fast-https"
!define UNINST_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}"
!define RUN_KEY "Software\Microsoft\Windows\CurrentVersion\Run"

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

  ; 升级时先停掉在跑的托盘与服务进程，否则文件被占用会替换失败
  nsExec::ExecToLog 'taskkill /F /IM monitor.exe'
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
  WriteRegStr HKLM "${UNINST_KEY}" "DisplayIcon" "$INSTDIR\monitor.exe"
  WriteRegStr HKLM "${UNINST_KEY}" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegDWORD HKLM "${UNINST_KEY}" "NoModify" 1
  WriteRegDWORD HKLM "${UNINST_KEY}" "NoRepair" 1

  CreateDirectory "$SMPROGRAMS\${APP_NAME}"
  CreateShortcut "$SMPROGRAMS\${APP_NAME}\使用文档.lnk" "$INSTDIR\docs\usage.html"
  CreateShortcut "$SMPROGRAMS\${APP_NAME}\安装目录.lnk" "$INSTDIR"
  ${If} ${FileExists} "$INSTDIR\monitor.exe"
    CreateShortcut "$SMPROGRAMS\${APP_NAME}\托盘程序.lnk" "$INSTDIR\monitor.exe"
  ${EndIf}
  CreateShortcut "$SMPROGRAMS\${APP_NAME}\卸载.lnk" "$INSTDIR\uninstall.exe"
SectionEnd

Section "开机自动启动托盘（monitor.exe）" SEC_AUTOSTART
  ${If} ${FileExists} "$INSTDIR\monitor.exe"
    WriteRegStr HKCU "${RUN_KEY}" "${APP_NAME}" '"$INSTDIR\monitor.exe"'
  ${Else}
    DetailPrint "当前架构没有托盘程序，跳过开机自启"
  ${EndIf}
SectionEnd

; ------------------------------------------------------------ 卸载

Section "Uninstall"
  nsExec::ExecToLog 'taskkill /F /IM monitor.exe'
  nsExec::ExecToLog 'taskkill /F /IM ${APP_EXE}'

  DeleteRegValue HKCU "${RUN_KEY}" "${APP_NAME}"
  DeleteRegKey HKLM "${UNINST_KEY}"
  DeleteRegKey HKLM "Software\${APP_NAME}"

  Delete "$SMPROGRAMS\${APP_NAME}\*.lnk"
  RMDir "$SMPROGRAMS\${APP_NAME}"

  RMDir /r "$INSTDIR"
SectionEnd

; ------------------------------------------------------------ 描述

!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
  !insertmacro MUI_DESCRIPTION_TEXT ${SEC_CORE} "程序本体、默认配置、默认站点与使用文档。"
  !insertmacro MUI_DESCRIPTION_TEXT ${SEC_AUTOSTART} "登录系统后自动运行托盘程序（仅 amd64 包提供 monitor.exe）。"
!insertmacro MUI_FUNCTION_DESCRIPTION_END
