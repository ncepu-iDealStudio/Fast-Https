package metrics

import "testing"

func TestReplaceListenersCopies(t *testing.T) {
	ResetForTest()

	src := []ListenerInfo{{Port: "8080"}, {Port: "443", SSL: true, H2: true}}
	ReplaceListeners(src)
	src[0].Port = "mutated"

	got := Listeners()
	if len(got) != 2 || got[0].Port != "8080" {
		t.Fatalf("stored copy was mutated: %+v", got)
	}

	got[0].Port = "other"
	again := Listeners()
	if again[0].Port != "8080" {
		t.Fatalf("Listeners returned a live alias: %+v", again)
	}
}
