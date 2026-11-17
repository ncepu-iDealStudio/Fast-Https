// Package proxyconf 只改写配置文件顶层的 proxy 段。
//
// 与 tlsconf 同样的做法：文本级局部替换，注释、缩进、字段顺序原样保留；
// 写入前回读校验。整段都是默认值时删除整个 proxy 成员，不留空对象。
package proxyconf

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"

	"fast-https/config/internal/model"
	"fast-https/config/internal/schema"
	"fast-https/infra/files"
	"fast-https/infra/jsonc"
)

// Section 是磁盘上 proxy 段的写法，命令行直接读写它。
type Section = model.ProxyConfig

// IsDefault 报告这份配置是否等价于「未配置」：无意可信代理、链长不限、
// Host 透传、上游超时用默认值。此时 proxy 段可以整段删掉。
func IsDefault(c Section) bool {
	return len(c.TrustedProxies) == 0 && c.ForwardedFor.MaxHops == 0 &&
		c.PreserveHostEnabled() && c.UpstreamTimeout == 0
}

// PreviewProxySection 返回写入 proxy 段之后的完整文件内容，不落盘。
// 等价于未配置时删除整个 proxy 成员。
func PreviewProxySection(path string, c Section) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var patched []byte
	if IsDefault(c) {
		patched, err = deleteProxyMember(raw)
	} else {
		patched, err = jsonc.PatchMember(raw, "proxy", func(indent string) ([]byte, error) {
			return marshalProxySection(c, indent)
		}, jsonc.PatchOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := verifyProxySection(patched, c); err != nil {
		return nil, fmt.Errorf("%s: refused to write, result does not parse back: %w", path, err)
	}
	return patched, nil
}

// ReadProxySection 读取配置里的 proxy 段；没有该段时返回零值。
func ReadProxySection(path string) (Section, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Section{}, fmt.Errorf("read %s: %w", path, err)
	}
	var probe struct {
		Proxy Section `json:"proxy"`
	}
	if err := schema.Unmarshal(raw, &probe); err != nil {
		return Section{}, fmt.Errorf("%s: %w", path, err)
	}
	return probe.Proxy, nil
}

// WriteProxySection 把 proxy 段写进配置文件：已有该键只替换值，没有则追加；
// 整段为默认值时删除该键。写入用临时文件 + rename，避免半个文件被读到。
func WriteProxySection(path string, c Section) error {
	patched, err := PreviewProxySection(path, c)
	if err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if stat, err := os.Stat(path); err == nil {
		mode = stat.Mode().Perm()
	}
	return files.WriteAtomic(path, patched, mode)
}

// marshalProxySection 生成 proxy 段的值。trusted_proxies 为空时写空数组，
// 避免读回时出现 null 与 [] 两种「没配」的表示。
func marshalProxySection(c Section, indent string) ([]byte, error) {
	trusted := c.TrustedProxies
	if trusted == nil {
		trusted = []string{}
	}
	return json.MarshalIndent(map[string]any{
		"trusted_proxies":  trusted,
		"forwarded_for":    map[string]int{"max_hops": c.ForwardedFor.MaxHops},
		"preserve_host":    c.PreserveHostEnabled(),
		"upstream_timeout": c.UpstreamTimeout,
	}, indent, "  ")
}

// deleteProxyMember 删除顶层 proxy 成员，保留其余字节。
// 规则与 tlsconf 一致：非首个成员连同前面的逗号一起删；首个成员删到
// 自己的逗号为止。
func deleteProxyMember(raw []byte) ([]byte, error) {
	masked := jsonc.StripComments(raw)
	obj, err := jsonc.ScanObject(masked)
	if err != nil {
		return nil, err
	}
	for i, m := range obj.Members {
		if !strings.EqualFold(m.Key, "proxy") {
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

// verifyProxySection 回读校验：结果必须还能解析，且 proxy 段与期望一致。
func verifyProxySection(patched []byte, want Section) error {
	var probe struct {
		Proxy Section `json:"proxy"`
	}
	if err := schema.Unmarshal(patched, &probe); err != nil {
		return err
	}
	if IsDefault(want) {
		if !IsDefault(probe.Proxy) {
			return fmt.Errorf("proxy section is still present after delete")
		}
		return nil
	}
	if !slices.Equal(probe.Proxy.TrustedProxies, want.TrustedProxies) ||
		probe.Proxy.ForwardedFor.MaxHops != want.ForwardedFor.MaxHops ||
		probe.Proxy.PreserveHostEnabled() != want.PreserveHostEnabled() {
		return fmt.Errorf("proxy section does not round-trip")
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
