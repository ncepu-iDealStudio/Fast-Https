package core

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

const (
	corePkg    = "fast-https/modules/core"
	modulesPkg = "fast-https/modules"
	stepInject = "阶段 3 第 3 步：改为由组合层注入"
)

// Existing violations of the core -> feature rule. Entries may only be removed.
var coreImportAllowlist = map[string]string{
	"fast-https/modules/core/events -> fast-https/modules/auth":       stepInject,
	"fast-https/modules/core/events -> fast-https/modules/safe":       stepInject,
	"fast-https/modules/core/filters -> fast-https/modules/proxy_tcp": stepInject,
	"fast-https/modules/core/filters -> fast-https/modules/safe":      stepInject,
	"fast-https/modules/core/filters -> fast-https/modules/websocket": stepInject,
	"fast-https/modules/core/server -> fast-https/modules/safe": stepInject,
}

func forbiddenForCore(path string) bool {
	if path == corePkg || strings.HasPrefix(path, corePkg+"/") {
		return false
	}
	return path == modulesPkg || strings.HasPrefix(path, modulesPkg+"/")
}

func coreFeatureImports(t *testing.T) map[string]bool {
	t.Helper()
	found := map[string]bool{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" {
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
		pkg := corePkg
		if dir := filepath.ToSlash(filepath.Dir(path)); dir != "." {
			pkg += "/" + dir
		}
		for _, imp := range f.Imports {
			ip, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return err
			}
			if forbiddenForCore(ip) {
				found[pkg+" -> "+ip] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func TestCoreDoesNotImportFeaturePackages(t *testing.T) {
	found := coreFeatureImports(t)

	var added, stale []string
	for edge := range found {
		if _, ok := coreImportAllowlist[edge]; !ok {
			added = append(added, edge)
		}
	}
	for edge := range coreImportAllowlist {
		if !found[edge] {
			stale = append(stale, edge)
		}
	}
	sort.Strings(added)
	sort.Strings(stale)

	for _, edge := range added {
		t.Errorf("modules/core must not import feature packages: %s", edge)
	}
	for _, edge := range stale {
		t.Errorf("import no longer exists, remove it from coreImportAllowlist: %s", edge)
	}
}

func TestForbiddenForCore(t *testing.T) {
	cases := map[string]bool{
		"fast-https/modules/core":           false,
		"fast-https/modules/core/listener":  false,
		"fast-https/modules/static":         true,
		"fast-https/modules/compress":       true,
		"fast-https/modules":                true,
		"fast-https/modules/corex":          true,
		"fast-https/config":                 false,
		"fast-https/utils/message":          false,
		"github.com/stretchr/testify/assert": false,
	}
	for path, want := range cases {
		if got := forbiddenForCore(path); got != want {
			t.Errorf("forbiddenForCore(%q) = %v, want %v", path, got, want)
		}
	}
}
