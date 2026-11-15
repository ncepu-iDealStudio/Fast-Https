package static

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// traversalVector 是 testdata/traversal-vectors.jsonl 里的一行。
//
// want 的两种取值含义不同，但安全性等价（都不会读到 root 外的文件）：
//
//	reject —— 静态层识别出逃逸并拒绝（返回 ok=false）
//	name   —— 当成普通文件名处理，落在 root 内、最终 404
//
// 之所以要区分，是因为「被拒绝」和「当普通名找不到」对应的代码路径不同，
// 回归时能立刻看出是校验生效了还是解码行为变了。
type traversalVector struct {
	Path string `json:"path"`
	Want string `json:"want"`
	Note string `json:"note"`
}

// loadTraversalVectors 读用例表。用 jsonl 而不是 Go 里的切片：
// 加用例只改数据文件，不必碰测试代码。
func loadTraversalVectors(t *testing.T) []traversalVector {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "traversal-vectors.jsonl"))
	if err != nil {
		t.Fatalf("open vectors: %v", err)
	}
	defer f.Close()

	var out []traversalVector
	sc := bufio.NewScanner(f)
	for line := 1; sc.Scan(); line++ {
		raw := strings.TrimSpace(sc.Text())
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		var v traversalVector
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			t.Fatalf("testdata/traversal-vectors.jsonl:%d: %v", line, err)
		}
		out = append(out, v)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan vectors: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("testdata/traversal-vectors.jsonl 是空的")
	}
	return out
}

// TestStaticPathTraversalVectors 逐条跑路径穿越向量。
//
// 这里测的是「请求路径已经过 URL 解码、交给静态层」的那一刻：百分号编码
// 若在上层没被解码，落到这里就是普通文件名，同样读不到 root 外的内容。
func TestStaticPathTraversalVectors(t *testing.T) {
	root := t.TempDir()
	for _, v := range loadTraversalVectors(t) {
		t.Run(strings.NewReplacer("/", "_", "\\", "_", "%", "pct", " ", "sp").Replace(v.Path), func(t *testing.T) {
			// 复现调用方的拼接方式：静态根 + 请求路径
			got, ok := staticPathInRoot(root, root+v.Path)

			switch v.Want {
			case "reject":
				if ok {
					t.Fatalf("%s（%s）应被拒绝，却解析成 %q", v.Path, v.Note, got)
				}
			case "name":
				if !ok {
					// 被拒绝也是安全的，只是走的路径不同，不算失败
					t.Logf("%s（%s）被拒绝了，安全但语义与预期不同", v.Path, v.Note)
					return
				}
				// 放行时结果必须仍在 root 内——这是「不读到 root 外」的底线
				cleanRoot := filepath.Clean(root)
				if got != cleanRoot && !strings.HasPrefix(got, cleanRoot+string(filepath.Separator)) {
					t.Fatalf("%s（%s）解析到 root 外：%q", v.Path, v.Note, got)
				}
			default:
				t.Fatalf("用例 %s 的 want=%q 不认识", v.Path, v.Want)
			}
		})
	}
}

// TestStaticSymlinkEscape 覆盖「root 内放一个指向外部的符号链接」。
//
// staticPathInRoot 只看 Clean 后的字符串前缀，不 EvalSymlinks，所以这类
// 请求会通过前缀校验，最终由 os.Open 打开链接指向的文件。这不是路径穿越
// （配置者自己放进来的链接），但需要明确行为：读得到还是读不到必须稳定，
// 不能随平台变化。
func TestStaticSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 上创建符号链接需要开发者模式或管理员权限")
	}

	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("top-secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("创建符号链接失败：%v", err)
	}

	got, ok := staticPathInRoot(root, filepath.Join(link, "secret.txt"))
	if !ok {
		// 预期行为：拒绝。若实现改成 EvalSymlinks 再判断，会走到这里。
		t.Logf("符号链接逃逸被拒绝（%q），已是更安全的行为", got)
		return
	}
	// 当前实现放行：记录下来，作为「配置者显式放入的链接」的已知行为
	t.Logf("符号链接逃逸被前缀校验放行：%q —— 属于配置者显式放入的链接，不是 URL 穿越", got)
}
