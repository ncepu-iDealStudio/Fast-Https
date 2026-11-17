package validate

import (
	"fmt"
	"os"
	"path/filepath"

	"fast-https/config/internal/schema"
)

// ValidateSiteFile 用主配置同一套 schema 校验单个站点文件，
// 报错里带文件路径与字段路径（例如 conf.d/a.json: location[0].proxy_pass ...）。
func ValidateSiteFile(path string) error {
	return validateSiteFile(path, true)
}

// ValidateNewSiteFile 与 ValidateSiteFile 相同，但允许 TLS 证书文件还不存在：
// site init 生成的默认证书路径要等首次 start 才由本地 CA 落盘。
func ValidateNewSiteFile(path string) error {
	return validateSiteFile(path, false)
}

func validateSiteFile(path string, requireCerts bool) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read site file failed: %w", err)
	}
	var server schema.Server
	if err := schema.Unmarshal(content, &server); err != nil {
		return fmt.Errorf("parse site json failed (%s): %w", path, err)
	}
	baseDir := filepath.Dir(path)
	// include 文件里的相对路径以文件所在目录为基准，rootBaseDir 用主配置目录，
	// 这里没有主配置上下文，两者都取文件目录。
	return validateServer(&server, baseDir, baseDir, path, requireCerts)
}
