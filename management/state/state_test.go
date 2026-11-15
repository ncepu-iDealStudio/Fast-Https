package state

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestAtomicStateWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.json")
	if err := WriteFile(path, []byte(`{"v":1}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".tmp", []byte(`{"v":`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"v":1}`+"\n" {
		t.Fatalf("残留 .tmp 不应被读到：%s", got)
	}
	if json.Valid(got) == false {
		t.Fatal("读到的不是完整 JSON")
	}
	if err := WriteFile(path, []byte(`{"v":2}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err = ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"v":2}`+"\n" {
		t.Fatalf("新内容 = %s", got)
	}
}

func TestLockExclusive(t *testing.T) {
	dir := os.Getenv("S6_LOCK_DIR")
	role := os.Getenv("S6_LOCK_CHILD")
	if role != "" && dir != "" {
		runLockChild(t, dir, role)
		return
	}

	root := t.TempDir()
	hold := startLockChild(t, root, "hold")
	stdout, err := hold.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := hold.Start(); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if line != "LOCKED\n" {
		t.Fatalf("holder 输出 %q", line)
	}

	try := startLockChild(t, root, "try")
	err = try.Run()
	if err == nil {
		t.Fatal("第二个进程不该拿到锁")
	}
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 2 {
		t.Fatalf("竞争失败的退出码 = %v", err)
	}

	_ = hold.Process.Kill()
	_ = hold.Wait()

	try2 := startLockChild(t, root, "try")
	if err := try2.Run(); err != nil {
		t.Fatalf("持有者死亡后应能接管：%v", err)
	}
}

func startLockChild(t *testing.T, dir, role string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestLockExclusive$", "-test.count=1")
	cmd.Env = append(os.Environ(), "S6_LOCK_DIR="+dir, "S6_LOCK_CHILD="+role)
	return cmd
}

func runLockChild(t *testing.T, dir, role string) {
	t.Helper()
	d := Dir{Root: dir}
	switch role {
	case "hold":
		rel, err := d.Lock()
		if err != nil {
			os.Exit(1)
		}
		_ = rel
		os.Stdout.WriteString("LOCKED\n")
		time.Sleep(30 * time.Second)
		os.Exit(0)
	case "try":
		rel, err := d.Lock()
		if errors.Is(err, ErrLocked) {
			os.Exit(2)
		}
		if err != nil {
			os.Exit(1)
		}
		rel()
		os.Exit(0)
	}
	os.Exit(1)
}

func TestLockReentrantSamePID(t *testing.T) {
	d := Dir{Root: t.TempDir(), Alive: func(int, string) bool { return true }, PID: 7}
	r1, err := d.Lock()
	if err != nil {
		t.Fatal(err)
	}
	r2, err := d.Lock()
	if err != nil {
		t.Fatal("同进程嵌套获取应成功")
	}
	r2()
	if _, err := os.Stat(d.LockPath()); err != nil {
		t.Fatal("内层释放不应删掉外层锁")
	}
	r1()
	if _, err := os.Stat(d.LockPath()); !os.IsNotExist(err) {
		t.Fatal("外层释放应去掉锁文件")
	}
}
