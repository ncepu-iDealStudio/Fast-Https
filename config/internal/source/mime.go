package source

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"fast-https/config/internal/model"
	"fast-https/config/internal/paths"
)

// LoadContentTypes 读 MIME 表。mime 文件缺失或非法返回错误：启动时由组合根
// 决定是否退出，reload 时保留旧配置继续服务。
func LoadContentTypes(configPath string) (map[string]string, error) {
	path := resolveMIMEPath(configPath)
	confBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("can't open mime file %s: %w", path, err)
	}

	mime := make(map[string]string)
	err = json.Unmarshal(confBytes, &mime)
	if err != nil {
		return nil, fmt.Errorf("can't unmarshal mime file %s: %w", path, err)
	}

	return mime, nil
}

// resolveMIMEPath 找 mime.json：先用主配置同目录的那份，这样 --config 指到
// 别的目录时 MIME 表跟着配置文件走，不受进程工作目录影响；没有再用安装
// 基准目录下的那份（包安装后是 /etc/fast-https/mime.json）。
func resolveMIMEPath(configPath string) string {
	candidates := []string{
		filepath.Join(filepath.Dir(configPath), "mime.json"),
		model.MIME_FILE_PATH,
	}
	if abs, err := filepath.Abs(model.MIME_FILE_PATH); err == nil {
		candidates = append(candidates, abs)
	}
	for _, candidate := range candidates {
		if paths.FileExists(candidate) {
			return candidate
		}
	}
	// 都不存在时按第一项报错，错误信息里带上实际尝试的路径。
	return candidates[0]
}
