package main

import (
	"testing"
	"time"
)

func TestTransitionTaskRejectsSkip(t *testing.T) {
	dir := t.TempDir()
	st := mustStore(t, dir)
	defer st.Close()
	if _, err := st.InsertEvent(lineEvent{Type: "5xx_ratio", Message: "up", Time: "2026-11-14T00:00:00.000Z"}, "abababababababababababababababab", time.Now()); err != nil {
		t.Fatal(err)
	}
	ok, err := st.TransitionTask("t_abababababababababababababababab", "PENDING", "COMPLETED", time.Now())
	if err != nil || !ok {
		t.Fatalf("pending to completed ok=%v err=%v", ok, err)
	}
	ok, err = st.TransitionTask("t_abababababababababababababababab", "COMPLETED", "EXECUTING", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("completed task accepted a new execution")
	}
}
