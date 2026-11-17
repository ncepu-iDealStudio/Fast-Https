package static

import (
	"fast-https/config"
	"fast-https/modules/compress"
	"fast-https/runtime/listener"
)

// compressibleReason 返回本次静态响应不能压缩时命中的规则名；可以压缩时
// 返回空串。未启用压缩、文件小于 zip_min_length、Content-Type 命中已压缩
// 类型清单都返回具体规则，供拒绝日志记录 rule 字段。
func compressibleReason(cfg *listener.ListenCfg, contentType string, size int64) string {
	switch {
	case cfg.Zip == config.ZIP_NONE:
		return "zip"
	case cfg.ZipMinLength > 0 && size < int64(cfg.ZipMinLength):
		return "zip_min_length"
	case compress.Skip(contentType, cfg.ZipSkipTypes):
		return "zip_skip_types"
	default:
		return ""
	}
}
