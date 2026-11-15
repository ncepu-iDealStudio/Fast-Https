package cli

import (
	"os"
	"path/filepath"
	"strings"

	"fast-https/config"
	"fast-https/management/state"
)

// ResolveStateDir 默认用编译期状态目录。测试可用 FASTHTTPS_STATE_DIR 指到临时目录。
func ResolveStateDir() string {
	if v := strings.TrimSpace(os.Getenv("FASTHTTPS_STATE_DIR")); v != "" {
		return abs(v)
	}
	return abs(config.STATE_DIR)
}

// StateDir 返回状态目录句柄。
func StateDir() state.Dir {
	return state.Dir{Root: ResolveStateDir()}
}

// abs 把路径转成绝对路径，失败时原样返回。
func abs(p string) string {
	if p == "" {
		return p
	}
	if out, err := filepath.Abs(p); err == nil {
		return out
	}
	return p
}
