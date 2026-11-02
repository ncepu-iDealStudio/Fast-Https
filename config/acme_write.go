package config

import (
	"encoding/json"
	"fast-https/infra/files"
	"fast-https/infra/jsonc"
	"fmt"
	"os"
	"strings"
)

// 配置文件允许注释与尾随逗号，直接 json.Unmarshal + MarshalIndent 写回会把
// 注释和字段顺序全丢掉。这里改为文本级局部替换：只动顶层 "acme" 这一个值，
// 其余字节原样保留。
//
// 定位靠 stripJSONComments 的等长掩码：注释位置在掩码里是空格，偏移与原文
// 一一对应，因此可以在掩码上做结构扫描，再回到原文切片。

// PreviewACMESection 返回写入 acme 段之后的完整文件内容，不落盘。
// --dry-run 与写入前的回读校验都用它。
func PreviewACMESection(path string, c *AcmeConfig) ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("acme: nothing to write")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	patched, err := patchACMESection(raw, c)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := verifyACMESection(patched, c); err != nil {
		return nil, fmt.Errorf("%s: refused to write, result does not parse back: %w", path, err)
	}
	return patched, nil
}

// WriteACMESection 把 acme 段写进配置文件：已有该键则只替换值，
// 没有则在根对象末尾插入。写入用临时文件 + rename，避免半个文件被读到。
func WriteACMESection(path string, c *AcmeConfig) error {
	patched, err := PreviewACMESection(path, c)
	if err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if stat, err := os.Stat(path); err == nil {
		mode = stat.Mode().Perm()
	}
	return files.WriteAtomic(path, patched, mode)
}

// marshalACMESection 生成 acme 段的值：string 字段统一过 json.Marshal，
// 邮箱与目录 URL 里的特殊字符照常转义。
//
// 缩进用 json.MarshalIndent：数组会被拆成一列一行，不如手写紧凑，但换来的是
// 字段由结构体定义决定——给 AcmeConfig 加一个字段，这里不用再同步改一遍。
//
// fallback_self_signed 在结构体里是指针：写配置时补成显式 true，
// 让人看文件就知道当前行为，不用回查默认值。
func marshalACMESection(c *AcmeConfig, indent string) ([]byte, error) {
	value := *c
	if value.FallbackSelfSigned == nil {
		enabled := true
		value.FallbackSelfSigned = &enabled
	}
	if len(value.Challenge) == 0 {
		value.Challenge = []string{ACMEChallengeAuto}
	}
	if value.RenewBeforeDays == 0 {
		value.RenewBeforeDays = DefaultACMERenewBeforeDays
	}
	if len(value.CAs) == 0 {
		value.CAs = DefaultACMECAs()
	}

	return json.MarshalIndent(value, indent, "  ")
}

// patchACMESection 在原文上替换或插入 acme 段。
// 定位与改写交给 infra/jsonc，这里只负责"acme 段长什么样"。
func patchACMESection(raw []byte, cfg *AcmeConfig) ([]byte, error) {
	return jsonc.PatchMember(raw, "acme", func(indent string) ([]byte, error) {
		return marshalACMESection(cfg, indent)
	}, jsonc.PatchOptions{
		// 默认配置在根对象末尾留了一段 acme 示例注释（含被注释掉的 ,"acme" 键）。
		// 现在要写真的了，那段留着会让人以为还要手工取消注释，整段丢掉。
		DropGapContaining: `"acme"`,
	})
}

// verifyACMESection 回读校验：写入结果必须还能解析，且 acme 段是我们写的那份。
func verifyACMESection(patched []byte, want *AcmeConfig) error {
	var probe struct {
		Acme *AcmeConfig `json:"acme"`
	}
	if err := unmarshalConfig(patched, &probe); err != nil {
		return err
	}
	if probe.Acme == nil {
		return fmt.Errorf("acme section is missing after patch")
	}
	if probe.Acme.Enabled != want.Enabled || probe.Acme.Email != strings.TrimSpace(want.Email) {
		return fmt.Errorf("acme section does not round-trip")
	}
	return nil
}


