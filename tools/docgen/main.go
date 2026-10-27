// Command docgen 把使用文档从 Markdown 生成为单文件 HTML。
//
// 只覆盖 docs/usage.md 实际用到的语法子集，不引入第三方依赖：
// 标题、代码块、表格、无序/有序列表、引用、分隔线、段落；
// 行内支持 `code`、**粗体**、[链接](url)。
//
// 页面版本号直接取 config.CURRENT_VERSION，避免发行文档与二进制版本各写一份。
//
// 用法：
//
//	go run ./tools/docgen
//	go run ./tools/docgen -in docs/usage.md -out docs/usage.html
package main

import (
	"flag"
	"fmt"
	"html"
	"os"
	"regexp"
	"strings"

	"fast-https/config"
)

var (
	reHeading  = regexp.MustCompile(`^(#{1,4})\s+(.*)$`)
	reUL       = regexp.MustCompile(`^\s*[-*]\s+(.*)$`)
	reOL       = regexp.MustCompile(`^\s*\d+\.\s+(.*)$`)
	reBold     = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	reLink     = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	reCodeSpan = regexp.MustCompile("`([^`]+)`")
)

func main() {
	in := flag.String("in", "docs/usage.md", "输入 Markdown 文件")
	out := flag.String("out", "docs/usage.html", "输出 HTML 文件")
	flag.Parse()

	src, err := os.ReadFile(*in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "docgen: 读取 %s 失败: %v\n", *in, err)
		os.Exit(1)
	}

	if err := os.WriteFile(*out, []byte(Render(src, config.CURRENT_VERSION)), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "docgen: 写入 %s 失败: %v\n", *out, err)
		os.Exit(1)
	}
}

type tocItem struct {
	level int
	text  string
	id    string
}

// Render 把 Markdown 渲染成一个自包含的 HTML 页面。
func Render(md []byte, version string) string {
	lines := strings.Split(strings.ReplaceAll(string(md), "\r\n", "\n"), "\n")

	var (
		body     strings.Builder
		toc      []tocItem
		para     []string
		list     []string
		listTag  string
		quote    []string
		headingN int
	)

	flushPara := func() {
		if len(para) == 0 {
			return
		}
		body.WriteString("<p>" + renderInline(strings.Join(para, " ")) + "</p>\n")
		para = nil
	}
	flushList := func() {
		if len(list) == 0 {
			return
		}
		body.WriteString("<" + listTag + ">\n")
		for _, item := range list {
			body.WriteString("<li>" + renderInline(item) + "</li>\n")
		}
		body.WriteString("</" + listTag + ">\n")
		list, listTag = nil, ""
	}
	flushQuote := func() {
		if len(quote) == 0 {
			return
		}
		body.WriteString("<blockquote><p>" + renderInline(strings.Join(quote, " ")) + "</p></blockquote>\n")
		quote = nil
	}
	flushAll := func() {
		flushPara()
		flushList()
		flushQuote()
	}

	for i := 0; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], " \t")
		trimmed := strings.TrimSpace(line)

		// 围栏代码块：内容原样转义，不做行内解析
		if strings.HasPrefix(trimmed, "```") {
			flushAll()
			lang := strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
			var buf []string
			for i++; i < len(lines); i++ {
				if strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
					break
				}
				buf = append(buf, strings.TrimRight(lines[i], "\r"))
			}
			body.WriteString("<pre><code")
			if lang != "" {
				body.WriteString(fmt.Sprintf(` class="language-%s"`, html.EscapeString(lang)))
			}
			body.WriteString(">" + html.EscapeString(strings.Join(buf, "\n")) + "</code></pre>\n")
			continue
		}

		if trimmed == "" {
			flushAll()
			continue
		}

		// 表格：当前行以 | 开头且下一行是分隔行
		if strings.HasPrefix(trimmed, "|") && i+1 < len(lines) && isTableSeparator(lines[i+1]) {
			flushAll()
			head := splitRow(trimmed)
			var rows [][]string
			for i += 2; i < len(lines); i++ {
				row := strings.TrimSpace(lines[i])
				if !strings.HasPrefix(row, "|") {
					break
				}
				rows = append(rows, splitRow(row))
			}
			i--
			writeTable(&body, head, rows)
			continue
		}

		if m := reHeading.FindStringSubmatch(trimmed); m != nil {
			flushAll()
			level := len(m[1])
			text := strings.TrimSpace(m[2])
			headingN++
			id := fmt.Sprintf("sec-%d", headingN)
			if level == 2 || level == 3 {
				toc = append(toc, tocItem{level: level, text: text, id: id})
			}
			body.WriteString(fmt.Sprintf("<h%d id=%q>%s</h%d>\n", level, id, renderInline(text), level))
			continue
		}

		if trimmed == "---" || trimmed == "***" {
			flushAll()
			body.WriteString("<hr>\n")
			continue
		}

		if strings.HasPrefix(trimmed, ">") {
			flushPara()
			flushList()
			quote = append(quote, strings.TrimSpace(strings.TrimPrefix(trimmed, ">")))
			continue
		}
		flushQuote()

		if m := reUL.FindStringSubmatch(trimmed); m != nil {
			flushPara()
			if listTag != "ul" {
				flushList()
				listTag = "ul"
			}
			list = append(list, strings.TrimSpace(m[1]))
			continue
		}

		if m := reOL.FindStringSubmatch(trimmed); m != nil {
			flushPara()
			if listTag != "ol" {
				flushList()
				listTag = "ol"
			}
			list = append(list, strings.TrimSpace(m[1]))
			continue
		}

		flushList()
		para = append(para, trimmed)
	}
	flushAll()

	return page(version, tocHTML(toc), body.String())
}

func renderInline(s string) string {
	out := html.EscapeString(s)
	out = reCodeSpan.ReplaceAllString(out, "<code>$1</code>")
	out = reBold.ReplaceAllString(out, "<strong>$1</strong>")
	out = reLink.ReplaceAllString(out, `<a href="$2">$1</a>`)
	return out
}

func isTableSeparator(line string) bool {
	s := strings.TrimSpace(line)
	if !strings.Contains(s, "-") || !strings.Contains(s, "|") {
		return false
	}
	for _, r := range s {
		switch r {
		case '|', '-', ':', ' ', '\t':
		default:
			return false
		}
	}
	return true
}

func splitRow(line string) []string {
	cells := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
	for i := range cells {
		cells[i] = strings.TrimSpace(cells[i])
	}
	return cells
}

func writeTable(b *strings.Builder, head []string, rows [][]string) {
	b.WriteString("<table>\n<thead><tr>")
	for _, h := range head {
		b.WriteString("<th>" + renderInline(h) + "</th>")
	}
	b.WriteString("</tr></thead>\n<tbody>\n")
	for _, row := range rows {
		b.WriteString("<tr>")
		for _, cell := range row {
			b.WriteString("<td>" + renderInline(cell) + "</td>")
		}
		b.WriteString("</tr>\n")
	}
	b.WriteString("</tbody>\n</table>\n")
}

func tocHTML(items []tocItem) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<nav class="toc"><p class="toc-title">目录</p><ul>`)
	for _, it := range items {
		if it.level == 3 {
			b.WriteString(`<li class="lvl3"><a href="#` + it.id + `">` + renderInline(it.text) + `</a></li>`)
			continue
		}
		b.WriteString(`<li><a href="#` + it.id + `">` + renderInline(it.text) + `</a></li>`)
	}
	b.WriteString("</ul></nav>")
	return b.String()
}

func page(version, toc, body string) string {
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Fast-Https 使用文档</title>
<style>
:root{--fg:#1f2328;--muted:#59636e;--line:#d8dee4;--bg:#fff;--soft:#f6f8fa;--accent:#0969da}
*{box-sizing:border-box}
body{margin:0;color:var(--fg);background:var(--bg);font:16px/1.7 -apple-system,BlinkMacSystemFont,"Segoe UI","PingFang SC","Hiragino Sans GB","Microsoft YaHei",sans-serif}
header.top{position:sticky;top:0;z-index:10;display:flex;align-items:center;gap:12px;padding:10px 24px;background:rgba(255,255,255,.92);backdrop-filter:blur(6px);border-bottom:1px solid var(--line)}
header.top .brand{font-weight:700;letter-spacing:.3px}
header.top .ver{font-size:13px;color:var(--muted);border:1px solid var(--line);border-radius:999px;padding:1px 10px}
.layout{display:flex;align-items:flex-start;max-width:1180px;margin:0 auto;padding:0 24px}
nav.toc{position:sticky;top:52px;flex:0 0 232px;max-height:calc(100vh - 72px);overflow:auto;padding:20px 16px 40px 0;font-size:14px}
nav.toc .toc-title{margin:0 0 8px;font-size:12px;letter-spacing:.12em;text-transform:uppercase;color:var(--muted)}
nav.toc ul{list-style:none;margin:0;padding:0}
nav.toc li{margin:2px 0}
nav.toc li.lvl3{padding-left:14px;font-size:13px}
nav.toc a{color:var(--muted);text-decoration:none;display:block;padding:2px 0}
nav.toc a:hover{color:var(--accent)}
main{flex:1 1 auto;min-width:0;padding:20px 0 80px}
h1{font-size:28px;margin:8px 0 16px;padding-bottom:8px;border-bottom:1px solid var(--line)}
h2{font-size:21px;margin:34px 0 12px;padding-bottom:6px;border-bottom:1px solid var(--line)}
h3{font-size:17px;margin:24px 0 8px}
h4{font-size:15px;margin:18px 0 6px;color:var(--muted)}
p{margin:10px 0}
a{color:var(--accent);text-decoration:none}
a:hover{text-decoration:underline}
code{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,"Liberation Mono",monospace;font-size:.9em;background:var(--soft);border:1px solid var(--line);border-radius:5px;padding:1px 5px}
pre{background:var(--soft);border:1px solid var(--line);border-radius:8px;padding:12px 14px;overflow:auto;margin:12px 0}
pre code{background:none;border:0;padding:0;font-size:13px;line-height:1.6}
table{border-collapse:collapse;width:100%;margin:14px 0;font-size:14px;display:block;overflow:auto}
th,td{border:1px solid var(--line);padding:7px 10px;text-align:left;vertical-align:top}
th{background:var(--soft);font-weight:600;white-space:nowrap}
tbody tr:nth-child(even){background:#fafbfc}
blockquote{margin:14px 0;padding:8px 14px;border-left:3px solid var(--accent);background:var(--soft);color:var(--muted)}
blockquote p{margin:4px 0}
hr{border:0;border-top:1px solid var(--line);margin:26px 0}
ul,ol{margin:10px 0;padding-left:24px}
li{margin:4px 0}
footer{max-width:1180px;margin:0 auto;padding:16px 24px 40px;color:var(--muted);font-size:13px;border-top:1px solid var(--line)}
@media (max-width:860px){nav.toc{display:none}.layout{padding:0 18px}main{padding-top:8px}}
@media print{header.top,nav.toc{display:none}main{padding:0}pre,table{break-inside:avoid}}
</style>
</head>
<body>
<header class="top"><span class="brand">Fast-Https</span><span class="ver">` + html.EscapeString(version) + `</span></header>
<div class="layout">
`)
	b.WriteString(toc)
	b.WriteString("\n<main>\n")
	b.WriteString(body)
	b.WriteString(`</main>
</div>
<footer>本文档由 docs/usage.md 生成，请勿直接编辑 HTML。版本 ` + html.EscapeString(version) + `。</footer>
</body>
</html>
`)
	return b.String()
}
