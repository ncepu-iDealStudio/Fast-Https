package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSetSiteEnabledRoundTrip 验证停用 → 启用一个来回，且内容不被改动。
func TestSetSiteEnabledRoundTrip(t *testing.T) {
	dir := writeSiteWorkspace(t)
	if _, err := addSiteFile(dir, SiteInput{
		Name: "toggle.local", Listen: "8081", Type: "local", Root: "./x",
	}); err != nil {
		t.Fatal(err)
	}
	active := filepath.Join(dir, "config", "conf.d", "toggle.local.json")
	disabled := active + ".disabled"
	before, err := os.ReadFile(active)
	if err != nil {
		t.Fatal(err)
	}

	if msg, err := setSiteEnabled(dir, "conf.d/toggle.local.json", false); err != nil {
		t.Fatalf("停用失败：%v", err)
	} else if !strings.Contains(msg, "已停用") || !strings.Contains(msg, "重载配置") {
		t.Fatalf("停用提示应说明状态与重载：%q", msg)
	}
	if _, err := os.Stat(active); !os.IsNotExist(err) {
		t.Fatal("停用后 .json 应不存在")
	}
	got, err := os.ReadFile(disabled)
	if err != nil {
		t.Fatal("停用后应有 .json.disabled")
	}
	if string(got) != string(before) {
		t.Fatal("启停只改文件名，不应改文件内容")
	}

	if _, err := setSiteEnabled(dir, "conf.d/toggle.local.json.disabled", true); err != nil {
		t.Fatalf("启用失败：%v", err)
	}
	if _, err := os.Stat(disabled); !os.IsNotExist(err) {
		t.Fatal("启用后 .json.disabled 应不存在")
	}
	after, err := os.ReadFile(active)
	if err != nil {
		t.Fatal("启用后应恢复 .json")
	}
	if string(after) != string(before) {
		t.Fatal("往返后内容应完全一致")
	}
}

// TestSetSiteEnabledIsIdempotent 重复停用/启用应直接返回，不当成错误。
func TestSetSiteEnabledIsIdempotent(t *testing.T) {
	dir := writeSiteWorkspace(t)
	if _, err := addSiteFile(dir, SiteInput{
		Name: "idem.local", Listen: "8082", Type: "local", Root: "./x",
	}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := setSiteEnabled(dir, "conf.d/idem.local.json", false); err != nil {
			t.Fatalf("第 %d 次停用：%v", i+1, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "config", "conf.d", "idem.local.json.disabled")); err != nil {
		t.Fatal("重复停用不应把文件弄丢")
	}
}

// TestSetSiteEnabledRejectsInvalidSources 与删除同一套边界：主配置内联站点、
// conf.d 之外、非法后缀、路径穿越，都不能改文件名。
func TestSetSiteEnabledRejectsInvalidSources(t *testing.T) {
	dir := writeSiteWorkspace(t)
	for _, source := range []string{
		"",
		"fast-https.json",
		"other/a.json",
		"conf.d/a.txt",
		"conf.d/../../etc/a.json",
	} {
		if _, err := setSiteEnabled(dir, source, false); err == nil {
			t.Fatalf("非法来源 %q 应被拒绝", source)
		}
	}
}

// TestReadSitesListsDisabledFiles 是最关键的一条：停用后的站点必须仍出现在
// 列表里（带 enabled=false），否则用户停用一次就再也找不回来。
func TestReadSitesListsDisabledFiles(t *testing.T) {
	dir := writeSiteWorkspace(t)
	if _, err := addSiteFile(dir, SiteInput{
		Name: "visible.local", Listen: "8083", Type: "local", Root: "./x",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := setSiteEnabled(dir, "conf.d/visible.local.json", false); err != nil {
		t.Fatal(err)
	}

	sites, _, err := readSites(dir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range sites {
		if s.Name == "visible.local" {
			found = true
			if s.Enabled {
				t.Fatalf("停用站点的 Enabled 应为 false：%+v", s)
			}
			if s.Source != "conf.d/visible.local.json.disabled" {
				t.Fatalf("Source 应指向停用后的文件：%q", s.Source)
			}
		}
	}
	if !found {
		t.Fatal("停用站点从列表里消失了")
	}
}
