package packaging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPostremovePurgesOnlyOnExplicitRemoval 锁住卸载语义：
//   - apt purge / rpm -e（$1=0）才清配置、证书与日志
//   - 升级（rpm $1=1、deb upgrade）什么都不动
//
// 脚本里的路径写死在 /etc、/var，只能在文本层守：一旦有人给 case 补上 "1) "
// 分支，或把清理逻辑挪出 case，升级就会删掉用户的证书。
func TestPostremovePurgesOnlyOnExplicitRemoval(t *testing.T) {
	path := filepath.Join("..", "..", "build", "packaging", "linux", "postremove.sh")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	code := stripComments(string(data))

	caseIdx := strings.Index(code, `case "$1" in`)
	if caseIdx < 0 {
		t.Fatal(`postremove.sh 缺少 case "$1" in，无法区分卸载与升级`)
	}
	if !strings.Contains(code, "purge|0)") {
		t.Error(`postremove.sh 应把 deb 的 purge 与 rpm 的 $1=0 归为同一档清理条件（"purge|0)"）`)
	}
	if strings.Contains(code, "1)") {
		t.Error("postremove.sh 不应对升级（$1=1）分支做任何清理，升级不能删用户证书")
	}

	for _, target := range []string{
		"/etc/fast-https/cert",
		"/var/log/fast-https",
		"/run/fast-https",
	} {
		idx := strings.Index(code, target)
		if idx < 0 {
			t.Errorf("postremove.sh 的 purge 分支应清理 %s", target)
			continue
		}
		if idx < caseIdx {
			t.Errorf("%s 的删除语句出现在 case 之前，升级时也会执行", target)
		}
	}
}

// stripComments 去掉整行注释，避免注释里提到的路径被当成实际清理动作。
func stripComments(script string) string {
	var b strings.Builder
	for _, line := range strings.Split(script, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}
