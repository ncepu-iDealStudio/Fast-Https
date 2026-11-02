// 相对路径解析与 MIME 表：所有"配置里写的相对路径"都按
// FAST_HTTPS_BASE_DIR 解析成绝对路径，而不是进程工作目录。
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveRuntimePaths turns site, certificate, log and include paths into
// absolute paths before the process listens. Later directory changes then
// still open the files that were configured.
func ResolveRuntimePaths(cfg *Fast_Https) {
	cfg.LogRoot = absConfigPath(cfg.LogRoot)
	for i := range cfg.Include {
		cfg.Include[i] = absConfigPath(cfg.Include[i])
	}
	for i := range cfg.Servers {
		server := &cfg.Servers[i]
		server.SSLCertificate = absConfigPath(server.SSLCertificate)
		server.SSLCertificateKey = absConfigPath(server.SSLCertificateKey)
		for j := range server.Path {
			server.Path[j].Root = absConfigPath(server.Path[j].Root)
			server.Path[j].ProxyCache.Path = absConfigPath(server.Path[j].ProxyCache.Path)
			server.Path[j].ErrorNotFound = absConfigPath(server.Path[j].ErrorNotFound)
			server.Path[j].ErrorProxy = absConfigPath(server.Path[j].ErrorProxy)
		}
	}
}

// absConfigPath 把配置里的相对路径按 FAST_HTTPS_BASE_DIR 解析成绝对路径，
// 而不是按进程的工作目录。装了包之后从任意目录执行 fast-https 都指向同一份资源。
// 非包构建的 FAST_HTTPS_BASE_DIR 是 "./"，解析结果仍等于启动目录下的路径。
func absConfigPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	base, err := filepath.Abs(FAST_HTTPS_BASE_DIR)
	if err != nil {
		return path
	}
	return filepath.Join(base, path)
}

// loadContentTypes 读 MIME 表。mime 文件缺失或非法返回错误：启动时由组合根
// 决定是否退出，reload 时保留旧配置继续服务。
func loadContentTypes(configPath string) (map[string]string, error) {
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
// 基准目录下的 config/mime.json（包安装后是 /usr/share/fast-https/config/）。
func resolveMIMEPath(configPath string) string {
	candidates := []string{
		filepath.Join(filepath.Dir(configPath), "mime.json"),
		MIME_FILE_PATH,
		absConfigPath(MIME_FILE_PATH),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	// 都不存在时按第一项报错，错误信息里带上实际尝试的路径。
	return candidates[0]
}
