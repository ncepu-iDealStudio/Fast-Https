// Package paths 把配置里的相对路径按"内容类型对应的基址"解析成绝对路径，
// 并提供配置树/候选树的路径约束。归档布局四个基址都在工作目录下，
// 包布局则分别落在 /etc、/usr/share、/var/log、/run。
package paths

import (
	"os"
	"path/filepath"
	"strings"

	"fast-https/config/internal/model"
)

// ResolveRuntimePaths turns site, certificate, log and include paths into
// absolute paths before the process listens. Later directory changes then
// still open the files that were configured.
func ResolveRuntimePaths(cfg *model.Fast_Https) {
	cfg.LogRoot = LogPath(cfg.LogRoot)
	// 缓存目录属于主配置的缓存区，站点只按名字引用。
	for i := range cfg.Cache {
		cfg.Cache[i].Path = DataPath(cfg.Cache[i].Path)
	}
	cfg.TLS.DefaultCertificate.Certificate = ETCPath(cfg.TLS.DefaultCertificate.Certificate)
	cfg.TLS.DefaultCertificate.Key = ETCPath(cfg.TLS.DefaultCertificate.Key)
	for i := range cfg.Include {
		cfg.Include[i] = ETCPath(cfg.Include[i])
	}
	for i := range cfg.Servers {
		server := &cfg.Servers[i]
		server.SSLCertificate = ETCPath(server.SSLCertificate)
		server.SSLCertificateKey = ETCPath(server.SSLCertificateKey)
		for j := range server.Path {
			server.Path[j].Root = DataPath(server.Path[j].Root)
			server.Path[j].ErrorNotFound = DataPath(server.Path[j].ErrorNotFound)
			server.Path[j].ErrorProxy = DataPath(server.Path[j].ErrorProxy)
		}
	}
}

// ETCPath 解析配置、证书一类路径：包布局是 /etc/fast-https，归档是 ./config。
// "." 作为兜底基准保留 旧版 的行为——用 --config 指到别处的配置文件时，
// include 与证书仍能按启动目录解析，而不是突然全部找不到。
func ETCPath(path string) string { return PathIn(model.FAST_HTTPS_ETC_DIR, path, ".") }

// DataPath 解析站点根、错误页、缓存一类路径：包布局是 /usr/share/fast-https。
func DataPath(path string) string { return PathIn(model.FAST_HTTPS_DATA_DIR, path, ".") }

// LogPath 解析日志路径。日志是唯一按工作目录解析的相对路径：默认值来自
// DEFAULT_LOG_ROOT 常量（本来就是对应布局的绝对路径），手工写 "./logs" 时
// 落回启动目录下的 logs，与 旧版 归档布局的行为逐字一致。
func LogPath(path string) string {
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

// PathIn 把相对路径按给定基址解析成绝对路径。
// 拆掉安装根下的 config 层之后，老配置里的 "./config/xxx" 会多出一级前缀，
// 因此拼出来的路径不存在时剥掉该前缀再试一次；新配置一次命中，不触发回退。
// fallbacks 是最后才尝试的兜底基准，用于兼容 旧版 按启动目录解析的行为。
// 严格模式不传 fallbacks：候选树缺文件时不会落到 "." 下的线上文件。
func PathIn(base, path string, fallbacks ...string) string {
	path = strings.TrimSpace(path)
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return path
	}
	abs := filepath.Join(baseAbs, path)
	if FileExists(abs) {
		return abs
	}
	if trimmed, ok := trimConfigPrefix(path); ok {
		if alt := filepath.Join(baseAbs, trimmed); FileExists(alt) {
			return alt
		}
	}
	for _, fallback := range fallbacks {
		fallbackAbs, err := filepath.Abs(fallback)
		if err != nil {
			continue
		}
		if alt := filepath.Join(fallbackAbs, path); FileExists(alt) {
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

// FileExists 报告路径是否存在。
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
