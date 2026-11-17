package static

import (
	"strings"

	"fast-https/config"
	"fast-https/runtime/listener"
)

// builtinCompressedTypes 是默认跳过的已压缩 MIME 类型清单。
// 这些格式本身已经压缩过，再压一层收益接近零，却要付 CPU 与内存，
// 有时还会让体积变大。配置了 zip_skip_types 时整体替换本清单，不追加。
var builtinCompressedTypes = map[string]bool{
	"image/jpeg":                    true,
	"image/png":                     true,
	"image/gif":                     true,
	"image/webp":                    true,
	"image/avif":                    true,
	"image/x-icon":                  true,
	"image/vnd.microsoft.icon":      true,
	"video/mp4":                     true,
	"video/webm":                    true,
	"video/quicktime":               true,
	"audio/mpeg":                    true,
	"audio/ogg":                     true,
	"audio/aac":                     true,
	"audio/webm":                    true,
	"audio/mp4":                     true,
	"application/zip":               true,
	"application/gzip":              true,
	"application/x-gzip":            true,
	"application/x-7z-compressed":   true,
	"application/x-rar-compressed":  true,
	"application/x-bzip2":           true,
	"font/woff":                     true,
	"font/woff2":                    true,
	"application/font-woff":         true,
	"application/font-woff2":        true,
	"application/vnd.ms-fontobject": true,
}

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
	return !shouldSkipCompression(contentType, cfg.ZipSkipTypes)
}

// shouldSkipCompression 报告给定 Content-Type 是否应跳过压缩。
// skip 非空时按它判断（整体替换内置清单）；为空时用内置的已压缩类型清单。
func shouldSkipCompression(contentType string, skip []string) bool {
	base := mediaType(contentType)
	if base == "" {
		return false
	}
	if len(skip) == 0 {
		return builtinCompressedTypes[base]
	}
	for _, item := range skip {
		if strings.EqualFold(strings.TrimSpace(item), base) {
			return true
		}
	}
	return false
}

// mediaType 取 Content-Type 分号前的媒体类型并转小写，
// 让 "image/png; charset=binary" 与 "IMAGE/PNG" 都能命中清单。
func mediaType(contentType string) string {
	base, _, _ := strings.Cut(contentType, ";")
	return strings.ToLower(strings.TrimSpace(base))
}
