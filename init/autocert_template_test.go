package init

import (
	"strings"
	"testing"
)

// 本地 CA 的 CN 必须可读：旧实现把写死的邮箱串再拼一次 "fast-https "，
// 浏览器里颁发者显示成 "fast-https fast-https@ncepu.edu.cn"。
func TestLocalCATemplateNaming(t *testing.T) {
	tpl := caTemplate([]byte{1, 2, 3})
	if tpl.Subject.CommonName != "fast-https Local CA" {
		t.Fatalf("CA CN = %q", tpl.Subject.CommonName)
	}
	if strings.Contains(tpl.Subject.CommonName, "@") {
		t.Fatalf("CA CN 不应包含邮箱：%q", tpl.Subject.CommonName)
	}
	if !tpl.IsCA || !tpl.BasicConstraintsValid {
		t.Fatal("本地 CA 必须带 CA 基本约束")
	}
}

// 叶子证书必须带 CN，且 SAN 覆盖全部域名 / IP，否则浏览器证书查看器里
// 「颁发给」是空的，看起来像证书不完整。
func TestLeafTemplateHasSubjectCNAndAllSANs(t *testing.T) {
	tpl := leafTemplate([]string{"localhost", "192.168.1.10", "example.com"})
	if tpl.Subject.CommonName != "localhost" {
		t.Fatalf("叶子 CN = %q, want 主域名", tpl.Subject.CommonName)
	}
	if len(tpl.DNSNames) != 2 || tpl.DNSNames[0] != "localhost" || tpl.DNSNames[1] != "example.com" {
		t.Fatalf("DNSNames = %v", tpl.DNSNames)
	}
	if len(tpl.IPAddresses) != 1 || tpl.IPAddresses[0].String() != "192.168.1.10" {
		t.Fatalf("IPAddresses = %v", tpl.IPAddresses)
	}
	if len(tpl.ExtKeyUsage) == 0 {
		t.Fatal("服务器证书缺少 ExtKeyUsageServerAuth")
	}
}
