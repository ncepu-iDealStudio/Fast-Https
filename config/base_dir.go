//go:build !rpm

package config

// 归档（tar.gz）、Docker 镜像、Windows / Darwin 包等"解压即用"布局。
// 四个基址各管一类内容，配置里的相对路径按内容类型选对应基址解析：
//
//	./config —— 主配置、MIME 表、证书（含 ACME 缓存）、conf.d
//	./       —— 站点根 httpdoc/、pid
//	./logs   —— 日志
//	./state  —— 管理核心的配置版本与 operation（CLI S6）
//
// 这些值与 旧版 的单前缀布局逐字等价：升级归档不改变任何现有路径。
const (
	FAST_HTTPS_ETC_DIR   string = "./config"
	FAST_HTTPS_DATA_DIR  string = "."
	FAST_HTTPS_LOG_DIR   string = "./logs"
	FAST_HTTPS_RUN_DIR   string = "."
	FAST_HTTPS_STATE_DIR string = "./state"
)
