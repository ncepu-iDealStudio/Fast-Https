// Package arch 用测试锁定分层方向，防止下层反向依赖上层。
package arch_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "fast-https"

// layer 描述一层允许依赖的对象。allow 中每一项是相对模块根的前缀，
// 例如 "runtime/listener" 只放行该包本身，"modules" 放行 modules 下所有包。
type layer struct {
	name  string
	match func(string) bool
	allow []string
	why   string
}

func prefix(p string) func(string) bool {
	return func(pkg string) bool {
		return pkg == p || strings.HasPrefix(pkg, p+"/")
	}
}

var layers = []layer{
	{
		name:  "infra",
		match: prefix("infra"),
		allow: []string{"infra"},
		why:   "基础设施层只依赖标准库与自身，反向依赖会形成配置层与基础层的双向耦合",
	},
	{
		name:  "config",
		match: prefix("config"),
		allow: []string{"infra"},
		why:   "配置层只能依赖基础设施层",
	},
	{
		name:  "core",
		match: prefix("core"),
		allow: []string{
			"infra", "config",
			// Event 结构体直接内嵌这三个数据模型，属于已确认的契约例外。
			"runtime/listener", "runtime/request", "runtime/response",
		},
		why: "契约层只放 Event 与处理函数注册，不允许依赖功能包与调度层",
	},
	{
		name:  "runtime",
		match: prefix("runtime"),
		allow: []string{"infra", "config", "core", "runtime"},
		why:   "数据面运行时不允许依赖可插拔功能包与组合根",
	},
	{
		name:  "modules",
		match: prefix("modules"),
		allow: []string{
			"infra", "config", "core", "modules",
			// 只允许读取与请求模型相关的运行时包，禁止触碰调度层。
			"runtime/listener", "runtime/request", "runtime/response", "runtime/dynlog",
		},
		why: "功能包只允许依赖下层与请求模型，不允许依赖 server/events/filters/metrics 这些调度层",
	},
	{
		name:  "management",
		match: prefix("management"),
		allow: []string{"infra", "config", "modules/site", "modules/certmanager", "management"},
		why:   "管理核心只依赖基础设施、配置与站点/证书模块，不允许依赖数据面运行时、core 或组合根",
	},
	{
		name:  "monitor",
		match: prefix("monitor"),
		allow: []string{"monitor"},
		why:   "独立 Monitor 进程只读本机 /runtime 与 PID 文件，不得依赖数据面、管理核心与组合根",
	},
	{
		name:  "agent",
		match: prefix("agent"),
		allow: []string{"agent"},
		why:   "独立 Agent 进程只读事件文件和本机管理命令，不得依赖数据面、管理核心与组合根",
	},
	{
		name: "composition",
		match: func(pkg string) bool {
			return pkg == "cmd" || pkg == "init" || pkg == "."
		},
		// 组合根允许装配所有层，不做限制。
		allow: nil,
		why:   "组合根是唯一允许装配全部层的地方",
	},
}

func TestLayering(t *testing.T) {
	for _, e := range importEdges(t) {
		if !strings.HasPrefix(e.to, modulePath) {
			// 第三方依赖由 TestNoViperLeak 单独约束。
			continue
		}
		from := relative(e.from)
		to := relative(e.to)
		if from == "" || to == "" || strings.HasPrefix(from, "tests") {
			continue
		}
		l, ok := layerOf(from)
		if !ok || l.allow == nil {
			continue
		}
		if allowed(l, to) {
			continue
		}
		t.Errorf("%s -> %s  (%s 层禁止: %s)", from, to, l.name, l.why)
	}
}

// TestNoViperLeak 保证配置库只出现在 config 包内，避免热重载被绕过。
func TestNoViperLeak(t *testing.T) {
	for _, e := range importEdges(t) {
		if e.to != "github.com/spf13/viper" {
			continue
		}
		from := relative(e.from)
		if strings.HasPrefix(from, "config") || strings.HasPrefix(from, "tests") {
			continue
		}
		t.Errorf("%s imports viper directly; configuration library must stay inside config", from)
	}
}

// TestNoProductionImportsTests 防止生产代码依赖测试辅助包。
func TestNoProductionImportsTests(t *testing.T) {
	for _, e := range importEdges(t) {
		to := relative(e.to)
		from := relative(e.from)
		if !strings.HasPrefix(to, "tests") || strings.HasPrefix(from, "tests") {
			continue
		}
		t.Errorf("%s imports %s; production code must not depend on tests", from, to)
	}
}

// TestInfraHasNoConfigDependency 单列一条最关键的边：基础设施层不得依赖配置层。
func TestInfraHasNoConfigDependency(t *testing.T) {
	for _, e := range importEdges(t) {
		from := relative(e.from)
		if !strings.HasPrefix(from, "infra") {
			continue
		}
		to := relative(e.to)
		if to == "config" || strings.HasPrefix(to, "config/") {
			t.Errorf("%s -> config: 基础设施层不得依赖配置层", from)
		}
	}
}

type edge struct {
	from string
	to   string
}

func relative(pkg string) string {
	return strings.TrimPrefix(strings.TrimPrefix(pkg, modulePath), "/")
}

func layerOf(pkg string) (layer, bool) {
	for _, l := range layers {
		if l.match(pkg) {
			return l, true
		}
	}
	return layer{}, false
}

func allowed(l layer, to string) bool {
	for _, a := range l.allow {
		if to == a || strings.HasPrefix(to, a+"/") {
			return true
		}
	}
	return false
}

// importEdges 扫描仓库内所有生产 .go 文件（跳过 tests 与 testdata），返回包级 import 边。
func importEdges(t *testing.T) []edge {
	t.Helper()
	seen := map[edge]struct{}{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(filepath.Join("..", ".."), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == "testdata" || name == "node_modules" || name == ".git" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		from := packagePath(path)
		for _, imp := range f.Imports {
			ip, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return err
			}
			if ip == modulePath || strings.HasPrefix(ip, modulePath+"/") || ip == "github.com/spf13/viper" {
				seen[edge{from: from, to: ip}] = struct{}{}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	edges := make([]edge, 0, len(seen))
	for e := range seen {
		edges = append(edges, e)
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].from != edges[j].from {
			return edges[i].from < edges[j].from
		}
		return edges[i].to < edges[j].to
	})
	return edges
}

// TestAgentDoesNotImportServer 锁住 Agent 与 Server 的方向：agent 不能引用数据面、组合根或管理核心。
// Server 侧的 cmd、runtime、monitor、core 也不能引用 agent。
func TestAgentDoesNotImportServer(t *testing.T) {
	bannedFromAgent := []string{"runtime", "cmd", "core", "management", "config", "modules"}
	for _, e := range importEdges(t) {
		from := relative(e.from)
		to := relative(e.to)
		if strings.HasPrefix(from, "agent") {
			for _, banned := range bannedFromAgent {
				if to == banned || strings.HasPrefix(to, banned+"/") {
					t.Errorf("agent imports %s", to)
				}
			}
		}
		if to == "agent" || strings.HasPrefix(to, "agent/") {
			if strings.HasPrefix(from, "cmd") || strings.HasPrefix(from, "runtime") || strings.HasPrefix(from, "monitor") || strings.HasPrefix(from, "core") {
				t.Errorf("%s imports %s", from, to)
			}
		}
	}
}

// packagePath 把文件路径换算成 import path。
func packagePath(path string) string {
	dir := filepath.ToSlash(filepath.Dir(path))
	dir = strings.TrimPrefix(dir, "../../")
	if dir == "." || dir == "" {
		return modulePath
	}
	return modulePath + "/" + dir
}
