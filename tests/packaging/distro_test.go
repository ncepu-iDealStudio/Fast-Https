package packaging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// distroDir 是容器内实机验证脚本的目录，由 run.sh 统一挂载到容器的 /scripts。
const distroDir = "distro"

// TestDistroScriptsStopService 守两条最容易踩的约定：
//  1. 每个验证脚本都 source common.sh —— 那里注册了 trap stop_service EXIT。
//     fast-https start 留下后台守护进程占着 80/443，不 stop 的话下一次测试
//     bind 失败、容器也退不干净。
//  2. run.sh 必须用 --rm 跑容器，跑完即销毁，不在本机留状态。
func TestDistroScriptsStopService(t *testing.T) {
	root := filepath.Join("..", "..")
	dir := filepath.Join(root, "tests", "packaging", distroDir)

	common, err := os.ReadFile(filepath.Join(dir, "common.sh"))
	if err != nil {
		t.Fatalf("读取 common.sh 失败: %v", err)
	}
	if !strings.Contains(string(common), "trap stop_service EXIT") {
		t.Error("common.sh 必须注册 trap stop_service EXIT：否则启动的服务会一直占着 80/443")
	}

	scripts, err := filepath.Glob(filepath.Join(dir, "*.sh"))
	if err != nil {
		t.Fatalf("glob 失败: %v", err)
	}
	if len(scripts) < 4 {
		t.Fatalf("%s 下的脚本数量异常（%d 个），路径可能写错了", distroDir, len(scripts))
	}

	verified := 0
	for _, path := range scripts {
		name := filepath.Base(path)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("读取 %s 失败: %v", name, err)
		}
		body := string(data)

		switch name {
		case "common.sh":
			continue
		case "run.sh":
			if !strings.Contains(body, "docker run --rm") {
				t.Error("run.sh 必须用 docker run --rm：容器不留残余")
			}
			for _, target := range []string{"deb.sh", "rpm.sh", "apk.sh", "upgrade.sh"} {
				if !strings.Contains(body, target) {
					t.Errorf("run.sh 未映射 %s，新增脚本后要同步入口", target)
				}
			}
			continue
		}

		verified++
		if !strings.Contains(body, ". /scripts/common.sh") {
			t.Errorf("%s 未 source /scripts/common.sh，退出时不会 stop 服务", name)
		}
	}

	if verified == 0 {
		t.Fatal("没有找到需要 source common.sh 的验证脚本")
	}
}
