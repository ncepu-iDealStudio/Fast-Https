package view

import "testing"

// StopServer 依赖主程序 CLI；找不到可执行文件时必须把原因带出去，
// 不能假装「已停止」。
func TestStopServerReportsMissingBinary(t *testing.T) {
	dir := writeSiteWorkspace(t)
	if _, err := StopServer(dir); err == nil {
		t.Fatal("主程序缺失时 StopServer 应返回错误")
	}
}
