package metrics

import (
	"os"
	"testing"

	"fast-https/config"
)

func TestInitialStateIsStarting(t *testing.T) {
	ResetForTest()

	if got := State(); got != StateStarting {
		t.Fatalf("state = %s, want starting", got)
	}
	if got := State().String(); got != "starting" {
		t.Fatalf("state string = %q, want starting", got)
	}

	// 替身：ServerInit 在 ListenWithCfg 之后调用这三步。
	RecordProcessStart()
	ReplaceListeners(nil)
	MarkRunning()

	if got := State(); got != StateRunning {
		t.Fatalf("after listeners ready, state = %s, want running", got)
	}
	if StartedAt().IsZero() {
		t.Fatal("started_at should be set")
	}
	if PID() != os.Getpid() {
		t.Fatalf("pid = %d, want %d", PID(), os.Getpid())
	}
	if Version() != config.CURRENT_VERSION {
		t.Fatalf("version = %q, want %q", Version(), config.CURRENT_VERSION)
	}
}

func TestReloadDoesNotResetStart(t *testing.T) {
	ResetForTest()
	RecordProcessStart()
	MarkRunning()
	started := StartedAt()
	pid := PID()

	MarkReloading()
	NoteReloadSuccess()
	MarkRunning()

	if !StartedAt().Equal(started) {
		t.Fatal("reload must not change started_at")
	}
	if PID() != pid {
		t.Fatal("reload must not change pid")
	}
	if Generation() != 1 {
		t.Fatalf("generation = %d, want 1", Generation())
	}
}
