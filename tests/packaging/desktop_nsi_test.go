package packaging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWindowsUninstallLeavesNoResidue 锁住两个 Windows 安装器的卸载语义，
// 目标是卸载后机器上不留残留：
//
//  1. 卸载段先停掉 Monitor / 桌面端 / 主程序三个进程再删目录——进程还占着
//     exe、日志与 pid 文件时 RMDir /r 删不掉它们，这是最常见的卸载残留；
//  2. 清掉桌面端的 WebView2 配置目录 %APPDATA%\fast-https-desktop.exe
//     （主题偏好、localStorage、缓存），不删的话卸载完还会留一个文件夹；
//  3. 开始菜单整组回收（*.lnk 通配，连用户后加的快捷方式一起删）。
//
// 桌面端安装器（desktop/build/windows/nsis/project.nsi）与主安装器
// （build/installer/windows/fast-https.nsi）都必须满足。
func TestWindowsUninstallLeavesNoResidue(t *testing.T) {
	cases := []struct {
		name    string
		rel     string
		section string // 卸载段起点（大小写随各模板现状）
		kills   []string
		del     string // 删除安装目录的那行（顺序锚点）
		webview string // WebView2 配置目录清理行
	}{
		{
			name:    "桌面端安装器",
			rel:     "desktop/build/windows/nsis/project.nsi",
			section: `Section "uninstall"`,
			kills: []string{
				`taskkill /F /IM ${MONITOR_EXE}`,
				`taskkill /F /IM ${PRODUCT_EXECUTABLE}`,
				`taskkill /F /IM ${SERVER_EXE}`,
			},
			del:     `RMDir /r $INSTDIR`,
			webview: `RMDir /r "$AppData\${PRODUCT_EXECUTABLE}"`,
		},
		{
			name:    "主安装器",
			rel:     "build/installer/windows/fast-https.nsi",
			section: `Section "Uninstall"`,
			kills: []string{
				`taskkill /F /IM ${MONITOR_EXE}`,
				`taskkill /F /IM ${DESKTOP_EXE}`,
				`taskkill /F /IM ${APP_EXE}`,
			},
			del:     `RMDir /r "$INSTDIR"`,
			webview: `RMDir /r "$AppData\${DESKTOP_EXE}"`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			nsi := readPkgFile(t, c.rel)
			idx := strings.Index(nsi, c.section)
			if idx < 0 {
				t.Fatalf("%s 缺少卸载段 %s", c.rel, c.section)
			}
			section := nsi[idx:]

			delIdx := strings.Index(section, c.del)
			if delIdx < 0 {
				t.Fatalf("%s 卸载段没有删除安装目录的 %s", c.rel, c.del)
			}
			for _, kill := range c.kills {
				n := strings.Count(section, kill)
				if n != 1 {
					t.Errorf("%s 卸载段应恰好执行一次 %s，实际 %d 次", c.rel, kill, n)
				}
				at := strings.Index(section, kill)
				if at < 0 || at > delIdx {
					t.Errorf("%s 的 %s 必须在删除目录之前执行，否则被占用的文件删不掉", c.rel, kill)
				}
			}

			if !strings.Contains(section, c.webview) {
				t.Errorf("%s 卸载段应清理 WebView2 配置目录（%s）", c.rel, c.webview)
			}
			if !strings.Contains(section, `Delete "$SMPROGRAMS\`) || !strings.Contains(section, `*.lnk"`) {
				t.Errorf("%s 卸载段应通配回收开始菜单快捷方式（*.lnk）", c.rel)
			}
		})
	}
}

// TestSetupUsesCustomWizard 锁住「自绘向导 + 静默引擎」的接线：
// 交互模式由隐藏的向导宿主段弹 wizard.ps1（自绘 WPF 界面），向导点
// 「安装」后以 /S 静默调起同一 exe 执行安装段（引擎模式）；卸载经
// un.onInit 同样走 wizard.ps1 --uninstall。向导脚本缺失或接线断了，
// 用户就会退回没有任何界面的裸安装。
func TestSetupUsesCustomWizard(t *testing.T) {
	nsi := readPkgFile(t, "build/installer/windows/fast-https.nsi")
	for _, want := range []string{
		`File "/oname=wizard.ps1" "wizard.ps1"`,       // 向导脚本随包提供（安装段装进 $INSTDIR 供卸载用）
		`File "/oname=appicon.png"`,                   // 品牌图随向导提取
		`--self "$EXEPATH"`,                           // 向导知道引擎在哪（自身）
		`IfSilent oninit_done`,                        // /S 引擎模式跳过向导（.onInit 宿主）
		`Exec 'powershell -NoProfile -STA`,            // Exec 不等待不进 job：主进程 Quit 不杀向导
		`--uninstall --self "$INSTDIR\uninstall.exe"`, // 卸载走向导确认页
		`powershell -NoProfile -STA`,                  // WPF 向导需要 STA
		`ShowInstDetails nevershow`,                   // 引擎界面彻底不可见（防「正在安装」窗口同屏）
	} {
		if !strings.Contains(nsi, want) {
			t.Errorf("fast-https.nsi 缺少自绘向导接线：%s", want)
		}
	}
	wizard := readPkgFile(t, "build/installer/windows/wizard.ps1")
	for _, want := range []string{
		"[switch]$SelfTest", // 无头自检：XAML 与命名元素可加载
		"$PreviewPng",       // 无头预览：调样式可截图
		"'/S'",              // 静默引擎契约：安装/卸载都以 /S 调起自身
	} {
		if !strings.Contains(wizard, want) {
			t.Errorf("wizard.ps1 缺少约定项：%s", want)
		}
	}
}

// TestWindowsWizardBranding 锁住安装向导的品牌定制：
// 两个 Windows 安装器都挂同一套品牌位图（侧边图 + 页眉图，由应用图标用
// gen-wizard-bitmaps.ps1 生成）；桌面端向导保留产品化文案（欢迎 / 完成 /
// 卸载确认里明确告知数据清理范围）与「运行」勾选框。
func TestWindowsWizardBranding(t *testing.T) {
	for _, rel := range []string{
		"desktop/build/windows/nsis/project.nsi",
		"build/installer/windows/fast-https.nsi",
	} {
		nsi := readPkgFile(t, rel)
		for _, bmp := range []string{"wizard-side.bmp", "wizard-head.bmp"} {
			if !strings.Contains(nsi, bmp) {
				t.Errorf("%s 应引用向导品牌图 %s", rel, bmp)
			}
		}
		if !strings.Contains(nsi, "MUI_WELCOMEFINISHPAGE_BITMAP") || !strings.Contains(nsi, "MUI_HEADERIMAGE_BITMAP") {
			t.Errorf("%s 应设置 MUI_WELCOMEFINISHPAGE_BITMAP 与 MUI_HEADERIMAGE_BITMAP", rel)
		}
	}

	// 位图必须真实存在（缺图编译时才报错，太晚）
	for _, bmp := range []string{"wizard-side.bmp", "wizard-head.bmp"} {
		if _, err := os.Stat(filepath.Join("..", "..", "desktop", "build", "windows", "nsis", bmp)); err != nil {
			t.Errorf("向导品牌图缺失（用 desktop/build/windows/nsis/gen-wizard-bitmaps.ps1 生成）：%v", err)
		}
	}

	proj := readPkgFile(t, "desktop/build/windows/nsis/project.nsi")
	for _, want := range []string{
		`LangString MUI_TEXT_WELCOME_INFO_TITLE ${LANG_SIMPCHINESE}`,
		`LangString MUI_TEXT_WELCOME_INFO_TITLE ${LANG_ENGLISH}`,
		`LangString MUI_TEXT_FINISH_INFO_TEXT ${LANG_SIMPCHINESE}`,
		`LangString MUI_TEXT_FINISH_INFO_TEXT ${LANG_ENGLISH}`,
		`MUI_FINISHPAGE_RUN "$INSTDIR\${PRODUCT_EXECUTABLE}"`,
		`MUI_UNCONFIRMPAGE_TEXT_TOP "$(UNINST_CONFIRM_TOP)"`,
		`MUI_UNCONFIRMPAGE_TEXT_LOCATION "$(UNINST_CONFIRM_BODY)"`,
	} {
		if !strings.Contains(proj, want) {
			t.Errorf("桌面端安装向导缺少定制项：%s", want)
		}
	}
}

// TestWindowsExeDefinesMatchInstalledNames 锁住 taskkill 目标与实际分发的
// 可执行名一致：宏定义改了名就会杀错进程，残留又回来了。
func TestWindowsExeDefinesMatchInstalledNames(t *testing.T) {
	for _, want := range []struct{ rel, def string }{
		{"desktop/build/windows/nsis/project.nsi", `!define SERVER_EXE "fast-https.exe"`},
		{"desktop/build/windows/nsis/project.nsi", `!define MONITOR_EXE "fast-https-monitor.exe"`},
		{"build/installer/windows/fast-https.nsi", `!define APP_EXE "fast-https.exe"`},
		{"build/installer/windows/fast-https.nsi", `!define DESKTOP_EXE "fast-https-desktop.exe"`},
		{"build/installer/windows/fast-https.nsi", `!define MONITOR_EXE "fast-https-monitor.exe"`},
	} {
		if nsi := readPkgFile(t, want.rel); !strings.Contains(nsi, want.def) {
			t.Errorf("%s 应包含 %s", want.rel, want.def)
		}
	}
	// 桌面端的 PRODUCT_EXECUTABLE 由 wails 工具宏定义为 <项目名>.exe
	if tools := readPkgFile(t, "desktop/build/windows/nsis/wails_tools.nsh"); !strings.Contains(tools, `!define PRODUCT_EXECUTABLE "${INFO_PROJECTNAME}.exe"`) {
		t.Error("wails_tools.nsh 的 PRODUCT_EXECUTABLE 应保持 ${INFO_PROJECTNAME}.exe（taskkill 依赖它）")
	}
}
