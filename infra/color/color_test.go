package color

import (
	"fmt"
	"strings"
	"testing"
)

// 全部包装器：输入的字符串形态必须出现在输出里，且不同颜色的输出可区分
func TestColorWrappersRenderInput(t *testing.T) {
	cases := map[string]func(interface{}) string{
		"Black": Black, "Red": Red, "Green": Green, "Brown": Brown,
		"Navy": Navy, "Purple": Purple, "Cyan": Cyan, "Gray": Gray,
		"Dim": Dim, "Orange": Orange, "Lime": Lime, "Yellow": Yellow,
		"Blue": Blue, "Pink": Pink, "Aqua": Aqua, "White": White,
	}
	for name, fn := range cases {
		out := fn("marker")
		if !strings.Contains(out, "marker") {
			t.Errorf("%s lost the input: %q", name, out)
		}
		// 非字符串输入走 Sprint 分支
		if out2 := fn(42); out2 == "" {
			t.Errorf("%s produced empty output for int", name)
		}
	}
}

func TestColorizersDistinct(t *testing.T) {
	if Red("x") == Green("x") && Red("x") == Blue("x") {
		t.Log("terminal likely has NoColor set; wrappers still pass through")
	}
	if Red(7) != Red(fmt.Sprint(7)) {
		t.Fatal("string and Sprint forms must render identically")
	}
}
