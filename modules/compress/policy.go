package compress

import (
	"path"
	"strings"

	"fast-https/config"
	"fast-https/runtime/request"
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

// Wanted 按配置与客户端的 Accept-Encoding 协商响应压缩编码。
// 两种编码都可用时取 q 值更高的；q 相同按服务端偏好 br > gzip。
// q=0 视为明确拒绝；未显式列出的编码回落到 `*` 的 q 值。
// 返回空串表示不压缩。
func Wanted(zip uint16, accept string) string {
	var candidates []string
	switch zip {
	case config.ZIP_GZIP:
		candidates = []string{"gzip"}
	case config.ZIP_BR:
		candidates = []string{"br"}
	case config.ZIP_GZIP_BR:
		candidates = []string{"br", "gzip"}
	default:
		return ""
	}
	best, bestQ := "", -1.0
	for _, enc := range candidates {
		q, ok := qValue(accept, enc)
		if !ok || q <= 0 {
			continue
		}
		// 严格大于才替换，保证 q 相同时保留服务端偏好顺序（br 在前）。
		if q > bestQ {
			best, bestQ = enc, q
		}
	}
	return best
}

// qValue 返回某个编码的有效 q 值；显式条目优先，未提及才回落 `*`。
func qValue(accept, token string) (float64, bool) {
	if q, mentioned := request.QValue(accept, token); mentioned {
		return q, true
	}
	if q, mentioned := request.QValue(accept, "*"); mentioned {
		return q, true
	}
	return 0, false
}

// Skip 报告给定 Content-Type 是否应跳过压缩。
// skip 非空时按它判断（整体替换内置清单）；为空时用内置的已压缩类型清单。
// 每条规则可以是精确类型（image/png）、整类通配（image/*）、
// 部分通配（image/p*）或全局通配（* / *\/*）。
func Skip(contentType string, skip []string) bool {
	base := mediaType(contentType)
	if base == "" {
		return false
	}
	if len(skip) == 0 {
		return builtinCompressedTypes[base]
	}
	for _, item := range skip {
		if matchMediaType(item, base) {
			return true
		}
	}
	return false
}

// matchMediaType 报告 mime 是否命中一条跳过规则。
// 没有斜杠的规则按精确匹配；含斜杠的规则交给 path.Match，
// 因此 `*/*`、`image/*`、`image/p*` 都可用。
func matchMediaType(pattern, mime string) bool {
	p := strings.ToLower(strings.TrimSpace(pattern))
	if p == "" || mime == "" {
		return false
	}
	if p == "*" {
		return true
	}
	if !strings.Contains(p, "/") {
		return p == mime
	}
	ok, err := path.Match(p, mime)
	return err == nil && ok
}

// mediaType 取 Content-Type 分号前的媒体类型并转小写，
// 让 "image/png; charset=binary" 与 "IMAGE/PNG" 都能命中清单。
func mediaType(contentType string) string {
	base, _, _ := strings.Cut(contentType, ";")
	return strings.ToLower(strings.TrimSpace(base))
}
