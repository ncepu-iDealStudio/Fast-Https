package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fast-https/config"
)

func TestRenderBlocks(t *testing.T) {
	md := `# 标题

一段正文，含 ` + "`code`" + ` 与 **粗体**。

## 二级

- 项目一
- 项目二

1. 第一步

` + "```json" + `
{"listen": 8080}
` + "```" + `

| 键 | 说明 |
| --- | --- |
| listen | 端口 |

> 提示
`
	got := Render([]byte(md), "V1.2.3")

	wants := []string{
		`<h1 id="sec-1">标题</h1>`,
		`<h2 id="sec-2">二级</h2>`,
		`<code>code</code>`,
		`<strong>粗体</strong>`,
		`<ul>`,
		`<li>项目一</li>`,
		`<ol>`,
		`<li>第一步</li>`,
		`<pre><code class="language-json">`,
		`<th>键</th>`,
		`<td>listen</td>`,
		`<blockquote><p>提示</p></blockquote>`,
		`<span class="ver">V1.2.3</span>`,
	}
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("渲染结果缺少 %q\n---\n%s", want, got)
		}
	}
}

func TestRenderEscapesHTML(t *testing.T) {
	got := Render([]byte("# <script>alert(1)</script>\n"), "V0")
	if strings.Contains(got, "<script>alert(1)</script>") {
		t.Error("标题中的 HTML 未被转义")
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Errorf("期望转义后的标签，实际输出:\n%s", got)
	}
}

func TestRenderKeepsCodeBlockVerbatim(t *testing.T) {
	md := "```text\n**不是粗体** <b>不是标签</b>\n```\n"
	got := Render([]byte(md), "V0")
	if strings.Contains(got, "<strong>") {
		t.Error("代码块内不应做行内解析")
	}
	if !strings.Contains(got, "**不是粗体** &lt;b&gt;") {
		t.Errorf("代码块内容应原样保留，实际输出:\n%s", got)
	}
}

// TestUsageHTMLIsUpToDate 阻止改了 docs/usage.md 却忘记重新生成 HTML。
func TestUsageHTMLIsUpToDate(t *testing.T) {
	docPath := filepath.Join("..", "..", "docs", "usage.md")
	htmlPath := filepath.Join("..", "..", "docs", "usage.html")

	src, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("读取 %s 失败: %v", docPath, err)
	}
	want, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("读取 %s 失败: %v（先执行 go run ./tools/docgen）", htmlPath, err)
	}

	got := Render(src, config.CURRENT_VERSION)
	// 仓库在工作副本里使用 CRLF，比较前统一换行符。
	if got != strings.ReplaceAll(string(want), "\r\n", "\n") {
		t.Fatal("docs/usage.html 与 docs/usage.md 不一致，请执行 go run ./tools/docgen 重新生成")
	}
}
