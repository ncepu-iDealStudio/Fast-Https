package sitecmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/charmbracelet/x/term"
)

// 站点根目录这类路径没法靠参数背下来，所以给一个带 Tab 补全的单行输入：
// 按 Tab 补到唯一匹配，多个匹配时补公共前缀并列出候选，和 shell 里一样。
// 只处理 ASCII 可见字符的逐字节编辑，路径里出现中文时退格按字节删，
// 显示可能短暂错位，但最终写进配置的字符串是正确的 UTF-8。

// pathCompletions 返回给定输入能补全的候选路径。
// 目录候选带结尾分隔符，方便连续按 Tab 往里走。
func pathCompletions(input string) []string {
	if strings.TrimSpace(input) == "" {
		input = "." + string(os.PathSeparator)
	}

	dir, prefix := filepath.Split(input)
	if dir == "" {
		dir = "." + string(os.PathSeparator)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	// 前缀以点开头时才列隐藏项，避免一按 Tab 满屏 .cache。
	showHidden := strings.HasPrefix(prefix, ".")

	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		candidate := filepath.Join(dir, name)
		if entry.IsDir() {
			candidate += string(os.PathSeparator)
		}
		out = append(out, candidate)
	}
	sort.Strings(out)
	return out
}

// commonPrefix 返回所有候选的公共前缀，用于"多个候选时先补到分岔点"。
func commonPrefix(items []string) string {
	if len(items) == 0 {
		return ""
	}
	prefix := items[0]
	for _, item := range items[1:] {
		for !strings.HasPrefix(item, prefix) {
			prefix = prefix[:len(prefix)-1]
			if prefix == "" {
				return ""
			}
		}
	}
	return prefix
}

// canReadRaw 判断能不能进入原始模式：必须是终端，且输入确实是那个终端。
func canReadRaw(opts Options) (*os.File, bool) {
	// Windows 的控制台不支持 raw 模式下的逐字节读取（Tab 补全依赖它），
	// term.MakeRaw 之后的读循环会抛 "not supported by windows"。
	// 统一退回普通逐行输入，与重定向输入走同一条路径。
	if runtime.GOOS == "windows" {
		return nil, false
	}
	file, ok := opts.In.(*os.File)
	if !ok || !term.IsTerminal(file.Fd()) {
		return nil, false
	}
	return file, true
}

// readPathLine 读一行路径，Tab 补全。非终端时退回普通逐行读取，
// 保证重定向输入时不会卡住。
func readPathLine(opts Options, title, initial string) (string, error) {
	file, raw := canReadRaw(opts)
	if !raw {
		return readPlainPathLine(opts, title, initial)
	}

	state, err := term.MakeRaw(file.Fd())
	if err != nil {
		return readPlainPathLine(opts, title, initial)
	}
	defer func() { _ = term.Restore(file.Fd(), state) }()

	buf := []byte(initial)
	render := func() {
		fmt.Fprintf(opts.Out, "\r\x1b[K%s：%s", title, string(buf))
	}
	fmt.Fprintf(opts.Out, "%s（Tab 补全，Enter 确认）\r\n", title)
	render()

	reader := bufio.NewReader(file)
	for {
		b, err := reader.ReadByte()
		if err != nil {
			return "", err
		}
		switch {
		case b == '\r' || b == '\n':
			fmt.Fprint(opts.Out, "\r\n")
			return strings.TrimSpace(string(buf)), nil
		case b == 3: // Ctrl+C
			fmt.Fprint(opts.Out, "^C\r\n")
			return "", errors.New("已取消，未生成站点文件")
		case b == 127 || b == 8: // 退格
			if len(buf) > 0 {
				buf = buf[:len(buf)-1]
				render()
			}
		case b == '\t':
			buf = applyCompletion(opts, buf)
			render()
		case b >= 32:
			buf = append(buf, b)
			render()
		}
	}
}

// applyCompletion 处理一次 Tab：唯一匹配直接补全，多个匹配补公共前缀并列出候选。
// 行内重绘交给调用方，这里只负责算出新内容（列候选时直接往 out 写）。
func applyCompletion(opts Options, buf []byte) []byte {
	matches := pathCompletions(string(buf))
	switch len(matches) {
	case 0:
		fmt.Fprint(opts.Out, "\a")
		return buf
	case 1:
		return []byte(matches[0])
	default:
		prefix := commonPrefix(matches)
		if len(prefix) > len(buf) {
			return []byte(prefix)
		}
		fmt.Fprint(opts.Out, "\r\n")
		for _, match := range matches {
			fmt.Fprintf(opts.Out, "  %s\r\n", match)
		}
		return buf
	}
}

// readPlainPathLine 用于非终端输入：读一行，空行保留默认值。
func readPlainPathLine(opts Options, title, initial string) (string, error) {
	fmt.Fprintf(opts.Out, "%s [%s]：", title, initial)
	reader := bufio.NewReader(opts.In)
	line, err := reader.ReadString('\n')
	if err != nil && strings.TrimSpace(line) == "" {
		return "", err
	}
	if value := strings.TrimSpace(line); value != "" {
		return value, nil
	}
	return initial, nil
}
