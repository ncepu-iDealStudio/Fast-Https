// 相对路径解析与 MIME 表：配置里写的相对路径按"内容类型对应的基址"
// 解析成绝对路径，而不是按进程工作目录。归档布局四个基址都在工作目录下，
// 包布局则分别落在 /etc、/usr/share、/var/log、/run。
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
	cfg.LogRoot = absLogPath(cfg.LogRoot)
	for i := range cfg.Include {
		cfg.Include[i] = absETCPath(cfg.Include[i])
	}
	for i := range cfg.Servers {
		server := &cfg.Servers[i]
		server.SSLCertificate = absETCPath(server.SSLCertificate)
		server.SSLCertificateKey = absETCPath(server.SSLCertificateKey)
		for j := range server.Path {
			server.Path[j].Root = absDataPath(server.Path[j].Root)
			server.Path[j].ProxyCache.Path = absDataPath(server.Path[j].ProxyCache.Path)
			server.Path[j].ErrorNotFound = absDataPath(server.Path[j].ErrorNotFound)
			server.Path[j].ErrorProxy = absDataPath(server.Path[j].ErrorProxy)
		}
	}
}

// absETCPath 解析配置、证书一类路径：包布局是 /etc/fast-https，归档是 ./config。
// "." 作为兜底基准保留 旧版 的行为——用 --config 指到别处的配置文件时，
// include 与证书仍能按启动目录解析，而不是突然全部找不到。
func absETCPath(path string) string { return absPathIn(FAST_HTTPS_ETC_DIR, path, ".") }

// absDataPath 解析站点根、错误页、缓存一类路径：包布局是 /usr/share/fast-https。
func absDataPath(path string) string { return absPathIn(FAST_HTTPS_DATA_DIR, path, ".") }

// absLogPath 解析日志路径。日志是唯一按工作目录解析的相对路径：默认值来自
// DEFAULT_LOG_ROOT 常量（本来就是对应布局的绝对路径），手工写 "./logs" 时
// 落回启动目录下的 logs，与 旧版 归档布局的行为逐字一致。
func absLogPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

// absPathIn 把相对路径按给定基址解析成绝对路径。
// 拆掉安装根下的 config 层之后，老配置里的 "./config/xxx" 会多出一级前缀，
// 因此拼出来的路径不存在时剥掉该前缀再试一次；新配置一次命中，不触发回退。
// fallbacks 是最后才尝试的兜底基准，用于兼容 旧版 按启动目录解析的行为。
// 严格模式不传 fallbacks：候选树缺文件时不会落到 "." 下的线上文件。
func absPathIn(base, path string, fallbacks ...string) string {
	path = strings.TrimSpace(path)
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return path
	}
	abs := filepath.Join(baseAbs, path)
	if pathExistsFile(abs) {
		return abs
	}
	if trimmed, ok := trimConfigPrefix(path); ok {
		if alt := filepath.Join(baseAbs, trimmed); pathExistsFile(alt) {
			return alt
		}
	}
	for _, fallback := range fallbacks {
		fallbackAbs, err := filepath.Abs(fallback)
		if err != nil {
			continue
		}
		if alt := filepath.Join(fallbackAbs, path); pathExistsFile(alt) {
			return alt
		}
	}
	return abs
}

// trimConfigPrefix 剥掉 旧版 配置里的 config/ 前缀："./config/cert/x.pem"
// 与 "config/cert/x.pem" 都归一成 "cert/x.pem"。
func trimConfigPrefix(path string) (string, bool) {
	p := strings.TrimPrefix(filepath.ToSlash(path), "./")
	rest, ok := strings.CutPrefix(p, "config/")
	if !ok || rest == "" {
		return "", false
	}
	return rest, true
}

func pathExistsFile(path string) bool {
	_, err := os.Stat(path)
	return err == nil
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
// 基准目录下的那份（包安装后是 /etc/fast-https/mime.json）。
func resolveMIMEPath(configPath string) string {
	candidates := []string{
		filepath.Join(filepath.Dir(configPath), "mime.json"),
		MIME_FILE_PATH,
	}
	if abs, err := filepath.Abs(MIME_FILE_PATH); err == nil {
		candidates = append(candidates, abs)
	}
	for _, candidate := range candidates {
		if pathExistsFile(candidate) {
			return candidate
		}
	}
	// 都不存在时按第一项报错，错误信息里带上实际尝试的路径。
	return candidates[0]
}
