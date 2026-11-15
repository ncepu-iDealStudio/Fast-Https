package metrics

import (
	"sync"
	"testing"
)

func TestConcurrentCounters(t *testing.T) {
	ResetForTest()

	const n = 1000
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			ConnAccepted()
			ConnClosed()
		}()
	}
	wg.Wait()

	if got := ConnActive(); got < 0 {
		t.Fatalf("active = %d, want non-negative", got)
	}
	if got := ConnActive(); got != 0 {
		t.Fatalf("active = %d, want 0 after paired accept/close", got)
	}
	if got := ConnAcceptedTotal(); got != n {
		t.Fatalf("accepted_total = %d, want %d", got, n)
	}
	if got := ConnClosedTotal(); got != n {
		t.Fatalf("closed_total = %d, want %d", got, n)
	}
}
