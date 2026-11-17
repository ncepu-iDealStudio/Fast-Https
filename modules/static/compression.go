package static

import (
	"fast-https/config"
	"fast-https/modules/compress"
	"fast-https/runtime/listener"
)

// compressible 报告本次静态响应是否允许压缩。
// 未启用压缩、文件小于 zip_min_length、Content-Type 命中已压缩类型
// 清单时都返回 false，调用方改为流式原样返回。
func compressible(cfg *listener.ListenCfg, contentType string, size int64) bool {
	if cfg.Zip == config.ZIP_NONE {
		return false
	}
	if cfg.ZipMinLength > 0 && size < int64(cfg.ZipMinLength) {
		return false
	}
	return !compress.Skip(contentType, cfg.ZipSkipTypes)
}
