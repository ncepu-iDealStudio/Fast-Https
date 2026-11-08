package cmd

import (
	"os"
	"strings"

	"fast-https/config"
	"fast-https/management/state"
)

// resolveStateDir 默认用编译期状态目录。测试可用 FASTHTTPS_STATE_DIR 指到临时目录。
func resolveStateDir() string {
	if v := strings.TrimSpace(os.Getenv("FASTHTTPS_STATE_DIR")); v != "" {
		return toAbs(v)
	}
	return toAbs(config.STATE_DIR)
}

func stateDir() state.Dir {
	return state.Dir{Root: resolveStateDir()}
}
