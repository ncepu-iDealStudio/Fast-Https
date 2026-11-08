//go:build unix

package txn

import (
	"context"
	"syscall"
	"testing"
	"time"

	"fast-https/management/operation"
)

func TestApplyIgnoresSIGHUP(t *testing.T) {
	live, cand, eng, _, _ := setupApply(t)
	done := make(chan error, 1)
	go func() {
		_, err := eng.Apply(context.Background(), Input{Candidate: cand, LiveMain: live, RequestID: "hup"})
		done <- err
	}()
	time.Sleep(20 * time.Millisecond)
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGHUP); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("SIGHUP 后事务未完成")
	}
	list, err := operation.Store{Dir: eng.State}.List(operation.ListFilter{Limit: 5})
	if err != nil || len(list) != 1 || list[0].Status != operation.StatusSucceeded {
		t.Fatalf("list = %v err=%v", list, err)
	}
}
