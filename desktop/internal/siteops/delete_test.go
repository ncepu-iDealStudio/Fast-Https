package siteops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeleteSiteRemovesFileAndKeepsBackup(t *testing.T) {
	dir := writeSiteWorkspace(t)
	if _, err := AddSiteFile(dir, SiteInput{
		Name: "gone.local", Listen: "8081", Type: "local", Root: "./x",
	}); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "config", "conf.d", "gone.local.json")
	if _, err := os.Stat(target); err != nil {
		t.Fatal("前置条件：站点文件应存在")
	}

	msg, err := DeleteSiteFile(dir, "conf.d/gone.local.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("删除后 .json 应不存在")
	}
	if _, err := os.Stat(target + ".bak"); err != nil {
		t.Fatal("删除应留下 .bak 备份")
	}
	if !strings.Contains(msg, "重载配置") {
		t.Fatalf("提示应提醒重载：%q", msg)
	}
}

// 主配置里内联的站点没有独立文件，不能删。
func TestDeleteSiteRejectsInlineServer(t *testing.T) {
	dir := writeSiteWorkspace(t)
	if _, err := DeleteSiteFile(dir, "fast-https.json"); err == nil {
		t.Fatal("主配置内联站点应拒绝删除")
	}
}

func TestDeleteSiteRejectsOutsideConfD(t *testing.T) {
	dir := writeSiteWorkspace(t)
	for _, source := range []string{
		"other/a.json",     // 不在 conf.d 下
		"conf.d/a.txt",     // 不是 json
		"conf.d/../x.json", // 路径穿越
		"conf.d/a.json/../../b.json",
		"",
	} {
		if _, err := DeleteSiteFile(dir, source); err == nil {
			t.Fatalf("source %q 应被拒绝", source)
		}
	}
}

func TestDeleteSiteMissingFile(t *testing.T) {
	dir := writeSiteWorkspace(t)
	if _, err := DeleteSiteFile(dir, "conf.d/nope.json"); err == nil {
		t.Fatal("不存在的站点文件应报错")
	}
}
