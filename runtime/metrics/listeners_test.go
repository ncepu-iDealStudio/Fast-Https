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

// 在途请求数由注册的实时来源填充；未注册来源时保持 0，端口表仍是静态副本。
func TestListenersWithInFlightSource(t *testing.T) {
	ResetForTest()
	t.Cleanup(func() { SetListenerInFlightSource(nil) })

	ReplaceListeners([]ListenerInfo{{Port: "8080"}, {Port: "443", SSL: true, H2: true}})
	if got := listenersWithInFlight(); got[0].InFlightRequests != 0 {
		t.Fatalf("no source, in-flight = %d, want 0", got[0].InFlightRequests)
	}

	SetListenerInFlightSource(func() map[string]int64 {
		return map[string]int64{"8080": 3}
	})
	got := listenersWithInFlight()
	if got[0].InFlightRequests != 3 {
		t.Fatalf("in-flight = %d, want 3", got[0].InFlightRequests)
	}
	if got[1].InFlightRequests != 0 {
		t.Fatalf("unknown port in-flight = %d, want 0", got[1].InFlightRequests)
	}
	if snap := TakeSnapshot(); len(snap.Listeners) != 2 || snap.Listeners[0].InFlightRequests != 3 {
		t.Fatalf("snapshot listeners = %+v", snap.Listeners)
	}
}
