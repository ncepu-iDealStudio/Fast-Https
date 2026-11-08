//go:build rpm

package config

// deb / rpm / apk 安装后的 FHS 分离布局。该 tag 由三种包共用
// （goreleaser 的 linux-pkg 构建与 build/spec 都带 -tags=rpm），不只服务 rpm。
//
//	/etc/fast-https       —— 主配置、MIME 表、证书与 ACME 缓存、conf.d
//	/usr/share/fast-https —— 站点根等架构无关只读数据
//	/var/log/fast-https   —— 日志
//	/run/fast-https       —— pid
//	/var/lib/fast-https   —— 管理核心的配置版本与 operation
const (
	FAST_HTTPS_ETC_DIR   string = "/etc/fast-https"
	FAST_HTTPS_DATA_DIR  string = "/usr/share/fast-https"
	FAST_HTTPS_LOG_DIR   string = "/var/log/fast-https"
	FAST_HTTPS_RUN_DIR   string = "/run/fast-https"
	FAST_HTTPS_STATE_DIR string = "/var/lib/fast-https"
)
