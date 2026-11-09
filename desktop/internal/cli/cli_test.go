package cli

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEnvelopeVectors 用 testdata/envelopes.jsonl 逐条验证 --json 封套解析。
//
// 桌面端所有命令都靠这层解析，成功/失败/带警告/缺字段的形状必须稳定；
// 加一种响应只需往数据文件里加一行，不必改测试代码。
func TestEnvelopeVectors(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "envelopes.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for line := 1; sc.Scan(); line++ {
		raw := strings.TrimSpace(sc.Text())
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		var c struct {
			Name         string `json:"name"`
			Raw          string `json:"raw"`
			WantOK       bool   `json:"want_ok"`
			WantCommand  string `json:"want_command"`
			WantDataName string `json:"want_data_name"`
			WantError    string `json:"want_error"`
		}
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			t.Fatalf("testdata/envelopes.jsonl:%d: %v", line, err)
		}
		t.Run(c.Name, func(t *testing.T) {
			var env Envelope[struct {
				Name string `json:"name"`
			}]
			if err := json.Unmarshal([]byte(c.Raw), &env); err != nil {
				t.Fatalf("解析封套失败：%v", err)
			}
			if env.OK != c.WantOK || env.Command != c.WantCommand {
				t.Fatalf("ok=%v command=%q，期望 %v / %q", env.OK, env.Command, c.WantOK, c.WantCommand)
			}
			if c.WantDataName != "" && env.Data.Name != c.WantDataName {
				t.Fatalf("data.name=%q，期望 %q", env.Data.Name, c.WantDataName)
			}
			err := env.Err()
			got := ""
			if err != nil {
				got = err.Error()
			}
			if got != c.WantError {
				t.Fatalf("错误文案=%q，期望 %q", got, c.WantError)
			}
		})
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
}
