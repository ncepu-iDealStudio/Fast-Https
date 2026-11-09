; Fast-Https Desktop — NSIS 安装包
;
; 由 `wails3 task windows:create:nsis:installer` 调用，会传入
; -DARG_WAILS_AMD64_BINARY=<路径> 或 -DARG_WAILS_ARM64_BINARY=<路径>。
; 手工编译：
;   makensis -DARG_WAILS_AMD64_BINARY="..\..\bin\fast-https-desktop.exe" project.nsi
;
; 这里显式定义版本与版权；wails_tools.nsh 用 !ifndef 保护，不会覆盖。
Unicode true

!define INFO_PRODUCTVERSION "1.6.2"
!define INFO_COPYRIGHT "GPL-2.0, NCEPU BJ"
!include "wails_tools.nsh"

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
SectionEnd

Section "uninstall"
    !insertmacro wails.setShellContext

    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"
    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}\${INFO_PRODUCTNAME}.lnk"
    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    RMDir "$SMPROGRAMS\${INFO_PRODUCTNAME}"

    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}"
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
