package core

import (
	"fast-https/runtime/listener"
	"sync"
	"testing"
)

// resetServices 在测试结束后把钩子表恢复成测试前的样子。
func resetServices(t *testing.T) {
	t.Helper()
	previous := current()
	t.Cleanup(func() { SetServices(previous) })
}

func TestServicesDefaultAllows(t *testing.T) {
	resetServices(t)
	SetServices(Services{})

	ev := &Event{}
	cfg := &listener.ListenCfg{}
	if ConnRejected(ev) || ListenHandled(ev) || LocationRejected(cfg, ev) || AuthRejected(cfg, ev) {
		t.Fatal("unset hooks should allow the event")
	}
	if WAFRejected(cfg, ev) || RequestHandled(ev) {
		t.Fatal("unset hooks should allow the event")
	}
	RunBeforeListenReload()
	RunAfterListen()
	RunBeforeStop()
	RunAfterStop()
}

func TestServicesUsesConfiguredHooks(t *testing.T) {
	resetServices(t)

	var after int
	SetServices(Services{
		RejectConn:     func(*Event) bool { return true },
		HandleListen:   func(*Event) bool { return true },
		RejectLocation: func(*listener.ListenCfg, *Event) bool { return true },
		RejectAuth:     func(*listener.ListenCfg, *Event) bool { return true },
		AfterListen:    func() { after++ },
	})
	ev := &Event{}
	cfg := &listener.ListenCfg{}
	if !ConnRejected(ev) || !ListenHandled(ev) || !LocationRejected(cfg, ev) || !AuthRejected(cfg, ev) {
		t.Fatal("configured hooks should refuse the event")
	}
	RunAfterListen()
	if after != 1 {
		t.Fatalf("AfterListen calls = %d", after)
	}
}

// TestSetEarlyRequestHandlerSwapsOnlyThatHook 覆盖 reload 挂/撤 ACME 挑战应答：
// 只换 early-request 一个字段，其它钩子与计数不受影响。
func TestSetEarlyRequestHandlerSwapsOnlyThatHook(t *testing.T) {
	resetServices(t)

	var after, rejected int
	SetServices(Services{
		RejectConn:  func(*Event) bool { rejected++; return true },
		AfterListen: func() { after++ },
	})

	called := 0
	SetEarlyRequestHandler(func(*Event) bool { called++; return true })
	if !RequestHandled(&Event{}) || called != 1 {
		t.Fatalf("early-request hook not applied: handled=%v calls=%d", RequestHandled(&Event{}), called)
	}

	// 撤销后回到放行，且不会再把请求挡在路由之前。
	SetEarlyRequestHandler(nil)
	calledBefore := called
	if RequestHandled(&Event{}) {
		t.Fatal("removed early-request hook must not handle the request")
	}
	if called != calledBefore {
		t.Fatal("removed hook should not be called")
	}

	// 其它钩子仍在原位。
	if !ConnRejected(&Event{}) || rejected != 1 {
		t.Fatalf("RejectConn hook lost after partial update: rejected=%d", rejected)
	}
	RunAfterListen()
	if after != 1 {
		t.Fatalf("AfterListen hook lost after partial update: calls=%d", after)
	}
}

// TestSetEarlyRequestHandlerConcurrentWithReads 在 -race 下确认整体替换
// 与请求路径的读取之间不出现撕裂。
func TestSetEarlyRequestHandlerConcurrentWithReads(t *testing.T) {
	resetServices(t)

	SetServices(Services{RejectConn: func(*Event) bool { return false }})
	SetEarlyRequestHandler(func(*Event) bool { return true })

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				RequestHandled(&Event{})
				ConnRejected(&Event{})
			}
		}
	}()

	for i := 0; i < 200; i++ {
		if i%2 == 0 {
			SetEarlyRequestHandler(func(*Event) bool { return true })
			continue
		}
		SetEarlyRequestHandler(nil)
	}
	close(stop)
	wg.Wait()
}
