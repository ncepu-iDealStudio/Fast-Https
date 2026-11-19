//go:build !rpm

package model

import (
	"strings"
	"testing"

	"fast-https/config/internal/testutil"
)

// 归档 / Docker 布局：解压即用，所有路径都相对工作目录。
// 期望值放在 testdata/model/layout_paths_archive.yaml，逐字锁住旧版布局。
func TestArchiveLayoutPaths(t *testing.T) {
	actual := map[string]string{
		"CONFIG_FILE_PATH": CONFIG_FILE_PATH,
		"MIME_FILE_PATH":   MIME_FILE_PATH,
		"CERT_DIR":         CERT_DIR,
		"ROOT_CRT_DIR":     ROOT_CRT_DIR,
		"PID_FILE":         PID_FILE,
		"DEFAULT_LOG_ROOT": DEFAULT_LOG_ROOT,
		"STATE_DIR":        STATE_DIR,
	}

	var cases []struct {
		Name string `yaml:"name"`
		Want string `yaml:"want"`
	}
	testutil.LoadYAML(t, "model/layout_paths_archive.yaml", &cases)
	for _, c := range cases {
		got, ok := actual[c.Name]
		if !ok {
			t.Fatalf("测试数据引用了未知常量 %q", c.Name)
		}
		if got != c.Want {
			t.Errorf("%s = %q, want %q", c.Name, got, c.Want)
		}
	}
}

func TestServerNames(t *testing.T) {
	var data struct {
		Names []struct {
			Name string   `yaml:"name"`
			In   string   `yaml:"in"`
			Want []string `yaml:"want"`
		} `yaml:"names"`
		Primary []struct {
			Name string `yaml:"name"`
			In   string `yaml:"in"`
			Want string `yaml:"want"`
		} `yaml:"primary"`
	}
	testutil.LoadYAML(t, "model/server_names.yaml", &data)

	for _, c := range data.Names {
		t.Run(c.Name, func(t *testing.T) {
			s := Server{ServerName: c.In}
			got := s.ServerNames()
			if len(got) != len(c.Want) {
				t.Fatalf("ServerNames(%q) = %v, want %v", c.In, got, c.Want)
			}
			for i := range got {
				if got[i] != c.Want[i] {
					t.Errorf("ServerNames(%q)[%d] = %q, want %q", c.In, i, got[i], c.Want[i])
				}
			}
		})
	}

	for _, c := range data.Primary {
		t.Run("primary/"+c.Name, func(t *testing.T) {
			s := Server{ServerName: c.In}
			if got := s.PrimaryServerName(); got != c.Want {
				t.Errorf("PrimaryServerName(%q) = %q, want %q", c.In, got, c.Want)
			}
		})
	}
}

func TestSecurityDefaults(t *testing.T) {
	var nilSec *SecurityConfig
	if nilSec.HostCheckMode() != HostCheckOff {
		t.Fatalf("nil security should default to off")
	}
	if !nilSec.NoSniffEnabled() {
		t.Fatal("nosniff should be on without a security section")
	}

	sec := &SecurityConfig{}
	ApplySecurityDefaults(sec)
	if sec.HostCheck != HostCheckOff {
		t.Fatalf("host_check = %q, want off", sec.HostCheck)
	}
	if sec.Headers.NoSniff == nil || !*sec.Headers.NoSniff {
		t.Fatal("nosniff should default to true")
	}

	disabled := false
	explicit := &SecurityConfig{Headers: SecurityHeaders{NoSniff: &disabled}}
	if explicit.NoSniffEnabled() {
		t.Fatal("explicit false must win")
	}
}

func TestSecurityValidate(t *testing.T) {
	for _, mode := range []string{HostCheckOff, HostCheckWarn, HostCheckEnforce, ""} {
		sec := &SecurityConfig{HostCheck: mode}
		if err := ValidateSecurity(sec); err != nil {
			t.Fatalf("mode %q should be accepted: %v", mode, err)
		}
	}
	if err := ValidateSecurity(&SecurityConfig{HostCheck: "strict"}); err == nil {
		t.Fatal("unknown host_check should be rejected")
	} else if !strings.Contains(err.Error(), "host_check") {
		t.Fatalf("error should name the field: %v", err)
	}
}
