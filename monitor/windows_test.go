//go:build windows && amd64

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestResolvePath(t *testing.T) {
	abs := `C:\fast-https\fast-https.pid`
	if got := resolvePath(abs); got != abs {
		t.Errorf("绝对路径应原样返回，got %q", got)
	}
	if got := resolvePath("./fast-https.pid"); got != filepath.Join(exeDir(), "fast-https.pid") {
		t.Errorf("相对路径应挂到程序目录，got %q", got)
	}
	if got := resolvePath(""); got != "" {
		t.Errorf("空路径应原样返回，got %q", got)
	}
}

// TestAcquireSingleInstance 验证第二个实例拿不到锁，
// 第一个实例释放后可以再次获取。
func TestAcquireSingleInstance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "monitor.lock")

	first, ok := acquireSingleInstance(path)
	if !ok {
		t.Fatal("首个实例应当获取成功")
	}
	if first == 0 {
		t.Fatal("首个实例应当返回有效句柄")
	}

	second, ok := acquireSingleInstance(path)
	if ok {
		t.Error("第二个实例不应获取成功")
	}
	if second != 0 {
		t.Error("失败的调用不应返回句柄")
	}

	if err := windows.CloseHandle(first); err != nil {
		t.Fatalf("关闭句柄失败: %v", err)
	}

	third, ok := acquireSingleInstance(path)
	if !ok {
		t.Fatal("前一个实例退出后应当可以重新获取")
	}
	if err := windows.CloseHandle(third); err != nil {
		t.Fatalf("关闭句柄失败: %v", err)
	}
}

func TestProcessAliveRejectsInvalidPid(t *testing.T) {
	for _, pid := range []string{"", "0", "-1", "not-a-pid"} {
		if processAlive(pid) {
			t.Errorf("pid %q 不应被判为存活", pid)
		}
	}
}

// setChild 注入/还原被测的子进程句柄。
func setChild(t *testing.T, proc *os.Process) {
	t.Helper()
	childMu.Lock()
	old := child
	child = proc
	childMu.Unlock()
	t.Cleanup(func() {
		childMu.Lock()
		child = old
		childMu.Unlock()
	})
}

// TestServerRunningUsesOwnedChild 覆盖启动瞬间的场景：
// 子进程已经拉起但还没写 pid 文件，此时必须按子进程句柄判定为运行中，
// 否则会把刚启动的进程判成「不是本托盘启动的」，退出时就不再回收。
func TestServerRunningUsesOwnedChild(t *testing.T) {
	cmd := exec.Command("ping", "-n", "30", "127.0.0.1")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		t.Skipf("无法启动测试子进程: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	setChild(t, cmd.Process)

	if !serverRunning() {
		t.Fatal("子进程存活但 pid 文件缺失时，应判定为运行中")
	}
}

// TestShutdownServerTerminatesOwnedChild 覆盖兜底路径：
// stop 命令不生效时，退出托盘仍要终止自己拉起的子进程。
func TestShutdownServerTerminatesOwnedChild(t *testing.T) {
	oldStop, oldKill := stopTimeout, killTimeout
	stopTimeout, killTimeout = 300*time.Millisecond, 2*time.Second
	t.Cleanup(func() { stopTimeout, killTimeout = oldStop, oldKill })

	cmd := exec.Command("ping", "-n", "60", "127.0.0.1")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		t.Skipf("无法启动测试子进程: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })

	setChild(t, cmd.Process)

	done := make(chan struct{})
	go func() {
		shutdownServer()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(stopTimeout + killTimeout + 5*time.Second):
		t.Fatal("shutdownServer 超时未返回")
	}

	if pidAlive(cmd.Process.Pid) {
		t.Errorf("自己拉起的子进程应被终止，pid %d 仍存活", cmd.Process.Pid)
	}
}

// shortenExitTimeouts 缩短退出等待，避免测试白等。
func shortenExitTimeouts(t *testing.T) {
	t.Helper()
	oldStop, oldKill := stopTimeout, killTimeout
	stopTimeout, killTimeout = 300*time.Millisecond, 2*time.Second
	t.Cleanup(func() { stopTimeout, killTimeout = oldStop, oldKill })
}

// startForeignProcess 起一个与 fast-https 无关的进程，充当「不是本托盘拉起的服务」。
func startForeignProcess(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("ping", "-n", "60", "127.0.0.1")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		t.Skipf("无法启动测试子进程: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	return cmd
}

// writePidFile 按主程序的真实格式（JSON）写 pid 文件。
func writePidFile(t *testing.T, path string, pid int) {
	t.Helper()
	if err := os.WriteFile(path, fmt.Appendf(nil, `{"pid":%d}`, pid), 0o644); err != nil {
		t.Fatalf("写入 pid 文件失败: %v", err)
	}
}

// TestGetPidReadsPidFile 覆盖 pid 文件解析：主程序写的是 JSON，
// 按纯数字解析会永远读不到进程号，托盘也就不认外面启动的服务。
func TestGetPidReadsPidFile(t *testing.T) {
	path := pointPidFile(t)

	cases := []struct {
		name    string
		content string
		want    string
	}{
		{"JSON 格式", `{"pid":4321}`, "4321"},
		{"纯数字旧格式", "1234\r\n", "1234"},
		{"空文件", "", ""},
		{"损坏内容", "{not json", ""},
	}
	for _, c := range cases {
		if err := os.WriteFile(path, []byte(c.content), 0o644); err != nil {
			t.Fatalf("写入 pid 文件失败: %v", err)
		}
		if got := getPid(); got != c.want {
			t.Errorf("%s: getPid() = %q, 期望 %q", c.name, got, c.want)
		}
	}
}

// pointPidFile 把 pid 文件重定向到临时目录，并返回该路径。
func pointPidFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fast-https.pid")
	old := pidFilePath
	pidFilePath = func() string { return path }
	t.Cleanup(func() { pidFilePath = old })
	return path
}

// mustFinishWithin 跑一段可能阻塞的逻辑，超过 limit 直接失败。
func mustFinishWithin(t *testing.T, fn func(), limit time.Duration) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(limit):
		t.Fatal("shutdownServer 超时未返回")
	}
}

// TestStopServerKillsOwnedChildWhenStopFails 覆盖菜单 Stop 的兜底：
// stop 命令失败时也要把自己拉起的子进程收掉，否则点了 Stop 什么都没发生。
func TestStopServerKillsOwnedChildWhenStopFails(t *testing.T) {
	shortenExitTimeouts(t)

	owned := startForeignProcess(t)
	setChild(t, owned.Process)

	mustFinishWithin(t, stopServer, stopTimeout+killTimeout+5*time.Second)

	if pidAlive(owned.Process.Pid) {
		t.Errorf("stop 失败时子进程应被强制终止，pid %d 仍存活", owned.Process.Pid)
	}
}

// TestRunWithTimeoutKillsHungCommand 覆盖 stop/reload 命令自己卡住的情况：
// 命令不返回时必须被强制终止，否则托盘退出会一直挂着。
func TestRunWithTimeoutKillsHungCommand(t *testing.T) {
	start := time.Now()
	err := runWithTimeout("ping", []string{"-n", "60", "127.0.0.1"}, 300*time.Millisecond)
	if err == nil {
		t.Fatal("超时的命令应返回错误")
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("命令未被及时终止，耗时 %v", elapsed)
	}
}

// TestShutdownServerLeavesForeignServerUntouched 覆盖归属判定的边界：
// pid 文件指向的进程不是本托盘拉起的，退出时只能放着不管，不能误杀别人的服务。
func TestShutdownServerLeavesForeignServerUntouched(t *testing.T) {
	shortenExitTimeouts(t)

	foreign := startForeignProcess(t)
	writePidFile(t, pointPidFile(t), foreign.Process.Pid)

	// 本托盘没有拉起过服务。
	setChild(t, nil)

	mustFinishWithin(t, shutdownServer, stopTimeout+killTimeout+5*time.Second)

	if !pidAlive(foreign.Process.Pid) {
		t.Fatalf("非本托盘拉起的进程不应被终止，pid %d 已消失", foreign.Process.Pid)
	}
}

// TestShutdownServerOnlyKillsOwnedChild 覆盖归属判定的另一侧：
// pid 文件与子进程句柄指向不同进程时，只回收自己拉起的那个，另一个不能动。
func TestShutdownServerOnlyKillsOwnedChild(t *testing.T) {
	shortenExitTimeouts(t)

	foreign := startForeignProcess(t)
	writePidFile(t, pointPidFile(t), foreign.Process.Pid)

	owned := startForeignProcess(t)
	setChild(t, owned.Process)

	mustFinishWithin(t, shutdownServer, stopTimeout+killTimeout+5*time.Second)

	if pidAlive(owned.Process.Pid) {
		t.Errorf("自己拉起的子进程应被终止，pid %d 仍存活", owned.Process.Pid)
	}
	if !pidAlive(foreign.Process.Pid) {
		t.Errorf("pid 文件指向的其他进程不应被终止，pid %d 已消失", foreign.Process.Pid)
	}
}
