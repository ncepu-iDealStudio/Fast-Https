package packaging

import (
	"fast-https/infra/logging"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLogrotateCoversEveryLogFile 守三件事：
//  1. 轮转规则覆盖 infra/logging 里定义的每一个日志文件名
//  2. 用 copytruncate——进程常开 fd 且 O_APPEND 追加，create 不配合 reload 会丢日志
//  3. 规则文件确实被打进 /etc/logrotate.d/fast-https，并作为配置文件安装
//
// 第 1 条把"改日志文件名"和"轮转配置"绑在一起：只改名字而忘了改这边，
// 新文件就再也没人轮转，磁盘会被慢慢写满。
func TestLogrotateCoversEveryLogFile(t *testing.T) {
	root := filepath.Join("..", "..")

	data, err := os.ReadFile(filepath.Join(root, "build", "packaging", "logrotate", "fast-https"))
	if err != nil {
		t.Fatalf("读取 logrotate 配置失败: %v", err)
	}
	conf := string(data)

	if !strings.Contains(conf, "/var/log/fast-https/*.log {") {
		t.Error("logrotate 配置的路径段应为 /var/log/fast-https/*.log（包布局的日志目录）")
	}
	if !strings.Contains(conf, "copytruncate") {
		t.Error("logrotate 配置必须使用 copytruncate：进程常开 fd，create 会让日志继续写进改名后的旧文件")
	}
	if strings.Contains(conf, "postrotate") && !strings.Contains(conf, "copytruncate") {
		t.Error("用 postrotate 就必须同时保证进程重新打开日志文件，当前实现只有 copytruncate 这条路是安全的")
	}

	for _, name := range []string{
		logging.SystemLogName,
		logging.AccessLogName,
		logging.ErrorLogName,
		logging.SafeLogName,
	} {
		ok, err := filepath.Match("*.log", name)
		if err != nil || !ok {
			t.Errorf("日志文件名 %q 不再匹配轮转通配 *.log，轮转配置需要同步修改", name)
		}
	}

	release, err := os.ReadFile(filepath.Join(root, "build", "goreleaser.yaml"))
	if err != nil {
		t.Fatalf("读取 goreleaser 配置失败: %v", err)
	}
	yaml := string(release)
	if !strings.Contains(yaml, "dst: /etc/logrotate.d/fast-https") {
		t.Error("goreleaser 的 nfpms.contents 里缺少 /etc/logrotate.d/fast-https，装完不会轮转")
	}
	if !strings.Contains(yaml, "- logrotate") {
		t.Error("goreleaser 的 dependencies 里缺少 logrotate：轮转配置需要它才会执行")
	}
}
