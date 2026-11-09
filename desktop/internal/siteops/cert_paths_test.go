package siteops

import (
	"strings"
	"testing"
)

// ssl 站点在表单里不填证书路径时，应写成主程序 CertInit 会生成的路径，
// 而不是留空让 reload 因「ssl listen requires ssl_certificate」失败。
func TestAddSiteSSLWithoutCertUsesGeneratedPaths(t *testing.T) {
	dir := writeSiteWorkspace(t)
	if _, err := AddSiteFile(dir, SiteInput{
		Name: "secure.local", Listen: "443 ssl", Type: "local", Root: "./httpdoc/root",
	}); err != nil {
		t.Fatal(err)
	}
	m := readNewSite(t, dir, "secure.local.json")
	if m["ssl_certificate"] != "./cert/secure.local.pem" {
		t.Fatalf("ssl 站点留空应使用自动生成的证书：%v", m["ssl_certificate"])
	}
	if m["ssl_certificate_key"] != "./cert/secure.local-key.pem" {
		t.Fatalf("私钥路径 = %v", m["ssl_certificate_key"])
	}
}

// 纯 HTTP 站点不该凭空带出证书字段。
func TestAddSiteHTTPWithoutCertLeavesFieldsEmpty(t *testing.T) {
	dir := writeSiteWorkspace(t)
	if _, err := AddSiteFile(dir, SiteInput{
		Name: "plain.local", Listen: "8080", Type: "local", Root: "./httpdoc/root",
	}); err != nil {
		t.Fatal(err)
	}
	m := readNewSite(t, dir, "plain.local.json")
	if _, ok := m["ssl_certificate"]; ok {
		t.Fatalf("纯 HTTP 站点不应写证书字段：%v", m)
	}
}

// 只填证书不填私钥（或反之）会让 TLS 启动失败，表单阶段就拦下。
func TestAddSiteRejectsHalfFilledCertPair(t *testing.T) {
	dir := writeSiteWorkspace(t)
	_, err := AddSiteFile(dir, SiteInput{
		Name: "half.local", Listen: "443 ssl", Type: "local", Root: "./httpdoc/root",
		Cert: "./cert/half.pem",
	})
	if err == nil || !strings.Contains(err.Error(), "同时") {
		t.Fatalf("只填证书路径应被拒绝：%v", err)
	}
}
