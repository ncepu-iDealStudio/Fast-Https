package packaging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPreremoveStopsLeftoverProcesses 锁住卸载语义中最容易漏的一环：
// `systemctl stop` 只停单元 cgroup 里的进程。直接跑 `fast-https`（不带子命令）
// 时进程会自己 daemonize 到 init 下面，单元管不到它——包删掉后二进制成了
// deleted inode，进程却还占着 80/443，重装后 start 只会报端口被占用。
//
// 所以下拉必须留下两条线索：pid 文件与 /proc 扫描，且 TERM 之后要有 KILL 兜底。
func TestPreremoveStopsLeftoverProcesses(t *testing.T) {
	path := filepath.Join("..", "..", "build", "packaging", "linux", "preremove.sh")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	code := stripComments(string(data))

	caseIdx := strings.Index(code, `case "$1" in`)
	if caseIdx < 0 {
		t.Fatal(`preremove.sh 缺少 case "$1" in，无法区分卸载与升级`)
	}
	esacIdx := strings.Index(code[caseIdx:], "esac")
	if esacIdx < 0 {
		t.Fatal(`preremove.sh 的 case "$1" in 没有结束`)
	}
	branch := code[caseIdx : caseIdx+esacIdx]

	if !strings.Contains(branch, "remove|purge|0") {
		t.Error(`preremove.sh 应把 deb 的 remove/purge 与 rpm 的 $1=0 归为同一档卸载条件（"remove|purge|0)"）`)
	}
	// 升级不能停服务：停掉之后没有任何脚本负责重新拉起。
	if strings.Contains(branch, "upgrade") {
		t.Error("preremove.sh 不应对升级分支做任何停止动作")
	}

	for _, want := range []string{"stop_unit", "kill_leftovers"} {
		if !strings.Contains(branch, want) {
			t.Errorf("preremove.sh 的卸载分支应调用 %s：只停 systemd 单元会漏掉手工启动的进程", want)
		}
	}

	if !strings.Contains(code, "/proc/[0-9]*") {
		t.Error("preremove.sh 缺少 /proc 扫描兜底：pid 文件被 purge 删掉后只能靠它找进程")
	}
	if !strings.Contains(code, "/run/fast-https/fast-https.pid") {
		t.Error("preremove.sh 应按 pid 文件 /run/fast-https/fast-https.pid 找进程")
	}
	if !strings.Contains(code, "kill -9") {
		t.Error("preremove.sh 需要有 SIGKILL 兜底：忽略 SIGTERM 的进程否则杀不掉")
	}
	if !strings.Contains(code, " (deleted)") {
		t.Error(`preremove.sh 应认 /proc/PID/exe 的 " (deleted)" 形态：包已删时二进制就是这个样子`)
	}
}
