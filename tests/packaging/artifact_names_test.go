package packaging

import (
	"strings"
	"testing"
)

// TestDistArtifactNamesHaveNoVersion 锁住 dist 产物的命名：版本号只写在
// 包元数据与 checksums 里，不写进文件名，保证下载链接与脚本引用的名字
// 跨版本稳定。
func TestDistArtifactNamesHaveNoVersion(t *testing.T) {
	gr := readPkgFile(t, "build/goreleaser.yaml")
	if !strings.Contains(gr, `file_name_template: "{{ .PackageName }}_{{ .Os }}_{{ .Arch }}"`) {
		t.Error("goreleaser nfpm 应设置不带版本的 file_name_template")
	}
	if strings.Contains(gr, "{{ .Version }}") {
		t.Error("goreleaser 产物名模板不应包含 {{ .Version }}")
	}

	ps := readPkgFile(t, "build/package.ps1")
	if !strings.Contains(ps, `dist\fast-https-windows-$arch-setup.exe`) {
		t.Error(`package.ps1 的安装包名应为 "fast-https-windows-<arch>-setup.exe"`)
	}
	if strings.Contains(ps, `dist\fast-https-$version-windows`) {
		t.Error("package.ps1 的安装包名不应包含版本号")
	}
}
