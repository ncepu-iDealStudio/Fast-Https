//go:build !windows

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fast-https/config"
)

// TestWaitDaemonStartedReady 覆盖正常路径：子进程写下 ready 后父进程认为启动成功。
func TestWaitDaemonStartedReady(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe: %v", err)
	}
	daemonReadyPipe = r
	t.Cleanup(func() { daemonReadyPipe = nil })

	if _, err := w.WriteString(daemonReadyMsg + "\n"); err != nil {
		t.Fatalf("write ready: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close write end: %v", err)
	}

	if err := waitDaemonStarted(12345, time.Second); err != nil {
		t.Fatalf("ready signal should be reported as started, got %v", err)
	}
}

// TestWaitDaemonStartedChildExited 覆盖子进程启动期死掉的路径：
// 写端关闭等于子进程退出，父进程必须报错并把 daemon.log 的尾部带出来，
// 而不是凭 pid 文件已经写好就宣布启动成功。
func TestWaitDaemonStartedChildExited(t *testing.T) {
	dir := t.TempDir()
	errText := "Error listen: listen tcp 0.0.0.0:443: bind: address already in use"
	if err := os.WriteFile(filepath.Join(dir, daemonLogName), []byte("Listen 0.0.0.0:443\n"+errText+"\n"), 0o644); err != nil {
		t.Fatalf("write daemon log: %v", err)
	}

	prev := config.Current()
	config.Store(&config.Fast_Https{LogRoot: dir}, nil)
	t.Cleanup(func() { config.Store(prev, nil) })

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe: %v", err)
	}
	daemonReadyPipe = r
	t.Cleanup(func() { daemonReadyPipe = nil })
	_ = w.Close() // 子进程没写 ready 就退出了

	err = waitDaemonStarted(4321, time.Second)
	if err == nil {
		t.Fatal("child exiting before ready must fail the start")
	}
	if !strings.Contains(err.Error(), "failed to start") {
		t.Fatalf("error should say the start failed, got %v", err)
	}
	if !strings.Contains(err.Error(), "address already in use") {
		t.Fatalf("error should carry the child's own message, got %v", err)
	}
}

// TestWaitDaemonStartedTimeout 覆盖子进程还活着但迟迟没有报告就绪：
// 只告警不失败，避免把只是启动慢的实例判成失败。
func TestWaitDaemonStartedTimeout(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe: %v", err)
	}
	daemonReadyPipe = r
	t.Cleanup(func() {
		daemonReadyPipe = nil
		_ = w.Close()
	})

	if err := waitDaemonStarted(5678, 20*time.Millisecond); err != nil {
		t.Fatalf("slow start should only warn, got %v", err)
	}
}

func TestTailFileKeepsLastLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.log")
	if err := os.WriteFile(path, []byte("1\n2\n3\n4\n5\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	got := tailFile(path, 2)
	if got != "4\n5" {
		t.Fatalf("tail = %q, want %q", got, "4\n5")
	}
}

func TestTailFileMissing(t *testing.T) {
	got := tailFile(filepath.Join(t.TempDir(), "absent.log"), 5)
	if !strings.Contains(got, "read") {
		t.Fatalf("missing file should be reported, got %q", got)
	}
}
