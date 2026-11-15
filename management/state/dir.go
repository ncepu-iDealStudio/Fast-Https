// Package state 是管理核心的落盘底座：状态目录、跨进程锁、原子写 JSON。
//
// 不执行 Shell，不用 flock：锁文件用 O_EXCL 创建，持有者死亡后可接管。
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	lockName       = "lock"
	versionsName   = "config-versions"
	operationsName = "operations"
	requestsName   = "requests.json"
	lockRetryTimes = 5
)

// ErrLocked 表示状态目录已被别的活着的进程持有。
var ErrLocked = errors.New("state directory is locked")

// Dir 指向一个状态目录。Alive 可替换，便于测试注入「进程已死」。
type Dir struct {
	Root  string
	Now   func() time.Time
	Alive func(pid int, startKey string) bool
	PID   int
}

type lockRecord struct {
	PID      int    `json:"pid"`
	StartKey string `json:"start_key"`
	At       string `json:"acquired_at"`
}

// Release 解开本进程持有的锁。
type Release func()

func (d Dir) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

func (d Dir) alive(pid int, startKey string) bool {
	if d.Alive != nil {
		return d.Alive(pid, startKey)
	}
	return DefaultAlive(pid, startKey)
}

func (d Dir) pid() int {
	if d.PID != 0 {
		return d.PID
	}
	return os.Getpid()
}

// DefaultAlive 报告 pid 是否仍是当初那一个进程。
func DefaultAlive(pid int, startKey string) bool {
	if !processRunning(pid) {
		return false
	}
	if startKey == "" {
		return true
	}
	cur := processStartKey(pid)
	if cur == "" {
		return true
	}
	return cur == startKey
}

// StartKey 是进程启动标识，用来识别 PID 复用。拿不到时为空。
func StartKey(pid int) string { return processStartKey(pid) }

func (d Dir) CurrentPID() int                  { return d.pid() }
func (d Dir) CurrentTime() time.Time           { return d.now() }
func (d Dir) IsAlive(pid int, key string) bool { return d.alive(pid, key) }

func (d Dir) LockPath() string      { return filepath.Join(d.Root, lockName) }
func (d Dir) VersionsDir() string   { return filepath.Join(d.Root, versionsName) }
func (d Dir) OperationsDir() string { return filepath.Join(d.Root, operationsName) }
func (d Dir) RequestsPath() string  { return filepath.Join(d.Root, requestsName) }
func (d Dir) RecycleDir(opID string) string {
	return filepath.Join(d.Root, "recycle", opID)
}

// Ensure 建好状态目录及其子目录，权限 0700（Windows 尽力而为）。
func (d Dir) Ensure() error {
	for _, p := range []string{d.Root, d.VersionsDir(), d.OperationsDir()} {
		if err := os.MkdirAll(p, 0o700); err != nil {
			return err
		}
		_ = os.Chmod(p, 0o700)
	}
	return nil
}

// Lock 以 O_EXCL 占用锁文件。持有者已死则接管。
func (d Dir) Lock() (Release, error) {
	if err := d.Ensure(); err != nil {
		return nil, err
	}
	rec := lockRecord{
		PID:      d.pid(),
		StartKey: processStartKey(d.pid()),
		At:       d.now().UTC().Format(time.RFC3339Nano),
	}
	body, err := json.Marshal(rec)
	if err != nil {
		return nil, err
	}
	path := d.LockPath()
	for i := 0; i < lockRetryTimes; i++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_, werr := f.Write(body)
			cerr := f.Close()
			if werr != nil {
				_ = os.Remove(path)
				return nil, werr
			}
			if cerr != nil {
				return nil, cerr
			}
			return d.releaseFn(rec.PID), nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
		var held lockRecord
		if rerr := readJSON(path, &held); rerr != nil {
			_ = os.Remove(path)
			continue
		}
		if held.PID == rec.PID && d.alive(held.PID, held.StartKey) {
			// 本进程已持有：嵌套获取返回空释放，避免 Snapshot/Begin 在事务持锁时死锁。
			return func() {}, nil
		}
		if d.alive(held.PID, held.StartKey) {
			return nil, fmt.Errorf("%w by pid %d", ErrLocked, held.PID)
		}
		_ = os.Remove(path)
	}
	return nil, fmt.Errorf("%w: could not acquire", ErrLocked)
}

func (d Dir) releaseFn(pid int) Release {
	return func() {
		path := d.LockPath()
		var held lockRecord
		if err := readJSON(path, &held); err != nil {
			return
		}
		if held.PID != pid {
			return
		}
		_ = os.Remove(path)
	}
}
