package listener

import (
	"sync"
	"testing"
)

// reload 热更新与请求路径读是并发的：updateCfg 整体替换快照，
// Hosts()/Cfgs() 只读快照。-race 下本测试验证读取端永远看到
// 完整的新配置或完整的旧配置，而不是正在被改写的字段。
func TestHotUpdateSnapshotIsRaceFree(t *testing.T) {
	lis := &Listener{
		Port: "19999",
		Cfg: []ListenCfg{
			{ID: 0, Path: "/", ServerName: "old.local:19999"},
		},
		HostMap: map[string][]ListenCfg{
			"old.local:19999": {{ID: 0, Path: "/"}},
		},
	}
	lis.publish()

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)

	// 读端：模拟在途请求经 Hosts()/Cfgs() 路由
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			hosts := lis.Hosts()
			cfgs := lis.Cfgs()
			if len(cfgs) != 1 {
				t.Error("cfg snapshot should always hold exactly one location")
				return
			}
			if len(hosts["old.local:19999"]) != 1 && len(hosts["new.local:19999"]) != 1 {
				t.Error("host snapshot should always hold one of the two server names")
				return
			}
		}
	}()

	// 写端：模拟反复 reload 热更新
	go func() {
		defer wg.Done()
		newer := []ListenCfg{
			{ID: 1, Path: "/", ServerName: "new.local:19999"},
		}
		hostMap := map[string][]ListenCfg{
			"new.local:19999": {{ID: 1, Path: "/"}},
		}
		for i := 0; i < 1000; i++ {
			lis.updateCfg(newer, hostMap)
		}
		close(stop)
	}()

	wg.Wait()

	if lis.Cfgs()[0].ServerName != "new.local:19999" {
		t.Errorf("final cfg = %q, want new config", lis.Cfgs()[0].ServerName)
	}
}

// 在途请求在 reload 之前已经拿到 location 配置指针，reload 只发布新快照、
// 不改旧切片：这条请求按旧配置处理完，不会看到半新半旧的字段。
func TestInFlightRequestKeepsCapturedCfgAcrossReload(t *testing.T) {
	lis := &Listener{
		Port: "19998",
		Cfg:  []ListenCfg{{ID: 0, ServerName: "old.local:19998", StaticRoot: "/old"}},
		HostMap: map[string][]ListenCfg{
			"old.local:19998": {{ID: 0, ServerName: "old.local:19998", StaticRoot: "/old"}},
		},
	}
	lis.publish()

	// 模拟请求路由：从当前快照取出 server 块与命中的 location 配置。
	hosts := lis.Hosts()
	cfgs := hosts["old.local:19998"]
	captured := &cfgs[0]

	lis.updateCfg(
		[]ListenCfg{{ID: 1, ServerName: "new.local:19998", StaticRoot: "/new"}},
		map[string][]ListenCfg{
			"new.local:19998": {{ID: 1, ServerName: "new.local:19998", StaticRoot: "/new"}},
		},
	)

	if captured.ID != 0 || captured.StaticRoot != "/old" {
		t.Fatalf("in-flight cfg changed under reload: %+v", *captured)
	}
	if got := hosts["old.local:19998"]; len(got) != 1 || got[0].StaticRoot != "/old" {
		t.Fatalf("in-flight host map changed under reload: %+v", got)
	}
	if got := lis.Cfgs()[0].StaticRoot; got != "/new" {
		t.Fatalf("new request should see /new, got %q", got)
	}
	if got := lis.Hosts()["new.local:19998"]; len(got) != 1 || got[0].StaticRoot != "/new" {
		t.Fatalf("new host map = %+v", got)
	}
}
