package cmd

import (
	"os"
	"path/filepath"

	"fast-https/config"
)

// anchorPortableLayout 让归档布局（解压即用）可以从任意目录执行。
//
// 归档的四个基址（./config、./logs、./state、站点根）都按工作目录解析。
// 把安装目录加进 PATH 后在别处执行时，start 会因为找不到
// ./config/fast-https.json 直接失败。这里只在当前目录没有配置、而可执行文件
// 旁边有时切到安装目录；包布局的绝对路径、桌面端显式设置的工作目录都不受影响。
func anchorPortableLayout() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	cwd, err := os.Getwd()
	if err != nil {
		return
	}
	if dir, ok := portableLayoutDir(cwd, filepath.Dir(exe)); ok {
		_ = os.Chdir(dir)
	}
}

// portableLayoutDir 判断是否该把工作目录切到可执行文件所在目录。
func portableLayoutDir(cwd, exeDir string) (string, bool) {
	if filepath.IsAbs(config.CONFIG_FILE_PATH) {
		return "", false
	}
	if _, err := os.Stat(filepath.Join(cwd, config.CONFIG_FILE_PATH)); err == nil {
		return "", false
	}
	if _, err := os.Stat(filepath.Join(exeDir, config.CONFIG_FILE_PATH)); err != nil {
		return "", false
	}
	return exeDir, true
}
