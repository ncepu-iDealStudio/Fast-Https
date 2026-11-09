package siteops

import (
	"strings"
	"testing"
)

func TestParseSiteInputMultiDomain(t *testing.T) {
	cases := []struct {
		name    string
		input   SiteInput
		wantErr bool
		errMsg  string
	}{
		{
			name:  "单域名",
			input: SiteInput{Name: "example.com", Listen: "80", Type: "local", Root: "./x"},
		},
		{
			name:  "多域名空格分隔",
			input: SiteInput{Name: "a.example b.example", Listen: "80", Type: "local", Root: "./x"},
		},
		{
			name:  "多域名多余空白",
			input: SiteInput{Name: "  a.example   b.example  ", Listen: "80", Type: "local", Root: "./x"},
		},
		{
			name:  "三个域名",
			input: SiteInput{Name: "a.com b.com c.com", Listen: "80", Type: "local", Root: "./x"},
		},
		{
			name:    "域名含非法字符",
			input:   SiteInput{Name: "a.example b@example", Listen: "80", Type: "local", Root: "./x"},
			wantErr: true,
			errMsg:  "非法字符",
		},
		{
			name:    "空 server_name",
			input:   SiteInput{Name: "", Listen: "80", Type: "local", Root: "./x"},
			wantErr: true,
			errMsg:  "不能为空",
		},
		{
			name:  "IP 与域名混合",
			input: SiteInput{Name: "example.com 192.168.1.10", Listen: "80", Type: "local", Root: "./x"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := parseSiteInput(c.input)
			if c.wantErr {
				if err == nil {
					t.Fatal("期望报错，实际成功")
				}
				if c.errMsg != "" && !strings.Contains(err.Error(), c.errMsg) {
					t.Fatalf("错误信息 %q 不包含 %q", err.Error(), c.errMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("不应报错：%v", err)
			}
			if p.name != strings.TrimSpace(c.input.Name) {
				t.Errorf("p.name = %q, want %q", p.name, strings.TrimSpace(c.input.Name))
			}
		})
	}
}

func TestSanitizeSiteNameMultiDomain(t *testing.T) {
	// 多域名时文件名用第一个域名
	name := strings.Fields("a.example b.example")[0]
	got := sanitizeSiteName(name)
	if got != "a.example" {
		t.Errorf("sanitizeSiteName(%q) = %q, want a.example", name, got)
	}
}
