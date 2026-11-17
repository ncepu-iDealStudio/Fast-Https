// Package tlsconf 只改写配置文件顶层的 tls 段。
//
// 与 acme 段同样的做法：文本级局部替换，注释、缩进、字段顺序原样保留。
// 写入前回读校验；清空时删除整个 tls 成员，不留空对象。
package tlsconf

import (
	"encoding/json"
	"fmt"
	"os"

	"fast-https/config/internal/model"
	"fast-https/config/internal/schema"
	"fast-https/infra/files"
	"fast-https/infra/jsonc"
)

// PreviewTLSSection 返回写入 tls 段之后的完整文件内容，不落盘。
// cert/key 都为空时表示清除，删除整个 tls 成员。
func PreviewTLSSection(path string, c *model.TLSConfig) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var patched []byte
	if c == nil || !c.HasDefaultCertificate() {
		patched, err = deleteTLSMember(raw)
	} else {
		patched, err = jsonc.PatchMember(raw, "tls", func(indent string) ([]byte, error) {
			return marshalTLSSection(c, indent)
		}, jsonc.PatchOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := verifyTLSSection(patched, c); err != nil {
		return nil, fmt.Errorf("%s: refused to write, result does not parse back: %w", path, err)
	}
	return patched, nil
}

// ReadTLSDefaults 读取配置里的 tls 段；没有该段时返回零值。
func ReadTLSDefaults(path string) (model.TLSConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return model.TLSConfig{}, fmt.Errorf("read %s: %w", path, err)
	}
	var probe struct {
		TLS model.TLSConfig `json:"tls"`
	}
	if err := schema.Unmarshal(raw, &probe); err != nil {
		return model.TLSConfig{}, fmt.Errorf("%s: %w", path, err)
	}
	return probe.TLS, nil
}

// WriteTLSSection 把 tls 段写进配置文件：已有该键只替换值，没有则追加；
// 传空证书时删除该键。写入用临时文件 + rename，避免半个文件被读到。
func WriteTLSSection(path string, c *model.TLSConfig) error {
	patched, err := PreviewTLSSection(path, c)
	if err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if stat, err := os.Stat(path); err == nil {
		mode = stat.Mode().Perm()
	}
	return files.WriteAtomic(path, patched, mode)
}

// marshalTLSSection 生成 tls 段的值。字段由 TLSConfig 定义决定，
// 结构体新增字段时这里不用再同步改一遍。
func marshalTLSSection(c *model.TLSConfig, indent string) ([]byte, error) {
	return json.MarshalIndent(map[string]any{
		"default_certificate": map[string]string{
			"certificate": c.DefaultCertificate.Certificate,
			"key":         c.DefaultCertificate.Key,
		},
	}, indent, "  ")
}

// deleteTLSMember 删除顶层 tls 成员，保留其余字节。
//
// 非首个成员：连同它前面的逗号一起删（从上一个成员值的末尾到本成员值的末尾），
// 这样上一个成员与后面的成员之间仍有逗号。首个成员：从缩进处删到它自己的
// 逗号，留下后续成员的缩进。
func deleteTLSMember(raw []byte) ([]byte, error) {
	masked := jsonc.StripComments(raw)
	obj, err := jsonc.ScanObject(masked)
	if err != nil {
		return nil, err
	}
	for i, m := range obj.Members {
		if m.Key != "tls" {
			continue
		}
		start, end := m.KeyStart, m.ValueEnd
		if i > 0 {
			start = obj.Members[i-1].ValueEnd
		} else {
			for start > obj.Open+1 && (masked[start-1] == ' ' || masked[start-1] == '\t') {
				start--
			}
			if p := skipSpace(masked, end); p < len(masked) && masked[p] == ',' {
				end = p + 1
			}
		}
		out := make([]byte, 0, len(raw))
		out = append(out, raw[:start]...)
		out = append(out, raw[end:]...)
		return out, nil
	}
	return raw, nil
}

// verifyTLSSection 回读校验：结果必须还能解析，且 tls 段与期望一致。
func verifyTLSSection(patched []byte, want *model.TLSConfig) error {
	var probe struct {
		TLS model.TLSConfig `json:"tls"`
	}
	if err := schema.Unmarshal(patched, &probe); err != nil {
		return err
	}
	if want != nil && want.HasDefaultCertificate() {
		if probe.TLS.DefaultCertificate != want.DefaultCertificate {
			return fmt.Errorf("tls section does not round-trip")
		}
		return nil
	}
	if probe.TLS.HasDefaultCertificate() {
		return fmt.Errorf("tls section is still present after delete")
	}
	return nil
}

func skipSpace(b []byte, pos int) int {
	for pos < len(b) {
		switch b[pos] {
		case ' ', '\t', '\r', '\n':
			pos++
		default:
			return pos
		}
	}
	return pos
}
