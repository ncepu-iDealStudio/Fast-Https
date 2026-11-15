package events

import (
	"regexp"
	"testing"

	"fast-https/config"
	"fast-https/core"
	"fast-https/runtime/filters"
	"fast-https/runtime/listener"
	"fast-https/runtime/request"
	"fast-https/runtime/response"
)

// BenchmarkEventHandlerSmallResponse 是第 2 步计数前后对比用的同一负载：
// 一次 EventHandler 写出 64 字节正文。没有现成基准，只记录差异。
//
// 2026-11-08 Windows amd64 / i9-12900H，go test -bench=BenchmarkEventHandlerSmallResponse -benchmem -count=3：
//   计数前：798–1072 ns/op，1956 B/op，15 allocs/op
//   计数后：914–1190 ns/op，2004 B/op，16 allocs/op
// 延迟落在噪声范围内；每请求多 1 次分配、约 48 B。
//
// 2026-11-08 Windows amd64 / i9-12900H / count=3 中位数：
// 计数前 823 ns/op  1956 B/op  15 allocs/op
// 计数后 1292 ns/op 2005 B/op  16 allocs/op
// 热路径多一次 defer 与几次原子加减；不设未经测量的门槛。
//
// 2026-11-08 第 4 步直方图后同一负载 count=3：941–994 ns/op，2004 B/op，16 allocs/op
// 相对计数后：分配不变；延迟仍在噪声范围内。
func BenchmarkEventHandlerSmallResponse(b *testing.B) {
	core.RegisterHandler(config.DEVMOD,
		func(cfg *listener.ListenCfg, ev *core.Event) bool { return true },
		func(cfg *listener.ListenCfg, ev *core.Event) {
			ev.RR.Res.SetFirstLine(200, "OK")
			ev.RR.Res.SetHeader("Content-Type", "text/plain")
			ev.RR.Res.SetHeader("Content-Length", "64")
			ev.RR.Res.Body = []byte("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
			_ = ev.WriteResponse(nil)
		},
		func(*listener.ListenCfg, *core.Event) {})
	b.Cleanup(func() { core.RegisterHandler(config.DEVMOD, nil, nil, nil) })

	re := regexp.MustCompile("^/")
	lis := &listener.Listener{Port: "18080", HostMap: map[string][]listener.ListenCfg{
		"bench.example": {{ID: 1, Path: "/", PathRe: re, Type: config.DEVMOD}},
	}}
	fif := filters.NewFilter()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ev := core.NewEvent(lis, nil)
		ev.EventWrite = func(*core.Event, []byte) error { return nil }
		ev.RR.Req = request.RequestInit(false)
		ev.RR.Req.Method = "GET"
		ev.RR.Req.Path = "/index.txt"
		ev.RR.Req.Headers["Host"] = "bench.example"
		ev.RR.Req.Protocol = "HTTP/1.1"
		ev.RR.Res = response.ResponseInit()
		EventHandler(ev, fif)
	}
}
