package metrics

import "testing"

func TestParseStatus(t *testing.T) {
	ResetForTest()

	code, ok := ParseStatus("HTTP/1.1 200 OK")
	if !ok || code != 200 {
		t.Fatalf("200: code=%d ok=%v", code, ok)
	}
	code, ok = ParseStatus("HTTP/1.1 404 Not Found")
	if !ok || code != 404 {
		t.Fatalf("404: code=%d ok=%v", code, ok)
	}
	code, ok = ParseStatus("HTTP/1.1 503")
	if !ok || code != 503 {
		t.Fatalf("503: code=%d ok=%v", code, ok)
	}

	for _, line := range []string{"", "garbage", "HTTP/1.1 abc OK", "HTTP/1.1 99 Low", "HTTP/1.1 1000 Big"} {
		if code, ok := ParseStatus(line); ok {
			t.Fatalf("ParseStatus(%q) = %d, want ok=false", line, code)
		}
	}

	RequestBegin()
	RequestEnd("")
	RequestBegin()
	RequestEnd("garbage")
	if RequestsActive() != 0 {
		t.Fatalf("active = %d, want 0", RequestsActive())
	}
	if ResponsesUnknown() != 2 {
		t.Fatalf("unknown = %d, want 2", ResponsesUnknown())
	}
	if Responses2xx() != 0 {
		t.Fatal("empty/garbage must not fall back into 2xx")
	}
}
