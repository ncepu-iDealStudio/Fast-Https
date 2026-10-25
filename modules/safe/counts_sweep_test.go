package safe

import "testing"

// 起止反序的 IP 段必须报错：旧实现按字符串相等终止循环，nextIP 永远
// 追不上终点，配置加载时直接死循环
func TestParseIPRangeReversedFails(t *testing.T) {
	if _, err := parseIPRange("10.0.0.5-10.0.0.1"); err == nil {
		t.Error("reversed IP range should be rejected")
	}
}

// 超大 IP 段必须报错，不能展开成数十亿个字符串
func TestParseIPRangeTooLargeFails(t *testing.T) {
	if _, err := parseIPRange("0.0.0.0-255.255.255.255"); err == nil {
		t.Error("oversized IP range should be rejected")
	}
}

func TestParseIPRangeValid(t *testing.T) {
	ips, err := parseIPRange("10.0.0.1-10.0.0.3")
	if err != nil {
		t.Fatalf("parseIPRange() error: %v", err)
	}
	want := []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"}
	if len(ips) != len(want) {
		t.Fatalf("got %v, want %v", ips, want)
	}
	for i := range want {
		if ips[i] != want[i] {
			t.Errorf("ips[%d] = %s, want %s", i, ips[i], want[i])
		}
	}
}

// 不活跃 IP 的限流条目会被清扫回收，map 不再无界增长
func TestCountLimitSweepsExpiredEntries(t *testing.T) {
	cl := NewCountLimit(0, 100, 1000)
	cl.gMap["1.1.1.1"] = countIP{curr: 0, expire: 0, count: 1, rate: 100}
	cl.num = 1

	if !cl.Insert("2.2.2.2") {
		t.Fatal("insert should pass")
	}
	if _, ok := cl.gMap["1.1.1.1"]; ok {
		t.Error("expired entry should have been swept")
	}
	if len(cl.gMap) != 1 || cl.num != 1 {
		t.Errorf("len(gMap) = %d, num = %d, want 1/1", len(cl.gMap), cl.num)
	}
}

// 清扫后新条目仍然正常计数
func TestCountLimitStillCountsAfterSweep(t *testing.T) {
	cl := NewCountLimit(0, 2, 1000)
	cl.gMap["1.1.1.1"] = countIP{curr: 0, expire: 0, count: 0, rate: 2}
	cl.num = 1

	// 语义：先比较后自增，rate=2 恰好放行 2 次
	allowed := 0
	for allowed < 10 {
		if !cl.Insert("1.1.1.1") {
			break
		}
		allowed++
	}
	if allowed != 2 {
		t.Errorf("allowed = %d, want 2", allowed)
	}
}

// 新 IP 的首个请求即计数
func TestCountLimitFirstRequestCounts(t *testing.T) {
	cl := NewCountLimit(0, 1, 1000)
	if !cl.Insert("3.3.3.3") {
		t.Fatal("first request should pass")
	}
	if cl.Insert("3.3.3.3") {
		t.Error("second request within window should be rejected at rate=1")
	}
}
