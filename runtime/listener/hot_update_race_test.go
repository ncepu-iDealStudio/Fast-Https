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
