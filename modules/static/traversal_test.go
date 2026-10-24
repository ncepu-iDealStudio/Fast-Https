package static

import (
	"path/filepath"
	"testing"

	"fast-https/core"
	"fast-https/runtime/dynlog"
	"fast-https/runtime/listener"
	"fast-https/runtime/request"
	"fast-https/runtime/response"
)

func TestStaticPathInRoot(t *testing.T) {
	root := t.TempDir()

	// 根内路径放行
	if got, ok := staticPathInRoot(root, root+"/a/b.txt"); !ok || got != filepath.Clean(root+"/a/b.txt") {
		t.Errorf("inside path rejected: ok=%v got=%q", ok, got)
	}
	// 根目录本身放行
	if _, ok := staticPathInRoot(root, root); !ok {
		t.Error("root itself rejected")
	}
	// /../ 逃逸拒绝
	if _, ok := staticPathInRoot(root, root+"/../outside.txt"); ok {
		t.Error("../ escape accepted")
	}
	// 多层逃逸拒绝
	if _, ok := staticPathInRoot(root, root+"/a/../../outside.txt"); ok {
		t.Error("nested ../ escape accepted")
	}
	// 完全在根外的路径拒绝
	if _, ok := staticPathInRoot(root, filepath.Join(filepath.Dir(root), "elsewhere.txt")); ok {
		t.Error("outside path accepted")
	}
}

// 带 .. 的请求路径按未找到处理（调用方回 404），不会读根目录外的文件
func TestGetResBytesRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	cfg := &listener.ListenCfg{Path: "/", StaticRoot: dir}
	ev := &core.Event{RR: core.RRcircle{Req: request.RequestInit(false)}}
	ev.RR.Res = response.ResponseInit()

	// StaticEvent 会把 StaticRoot 与请求路径直接拼接，这里复现同一行为
	if got := getResBytes(&dynlog.DynLogger{}, cfg, cfg.StaticRoot+"/../outside.txt", "close", ev); got != -1 {
		t.Errorf("traversal returned %d, want -1", got)
	}
}
