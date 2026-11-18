package config

import (
	"fast-https/config/internal/acme"
	"fast-https/config/internal/cacheconf"
	"fast-https/config/internal/loader"
	"fast-https/config/internal/paths"
	"fast-https/config/internal/proxyconf"
	"fast-https/config/internal/scan"
	"fast-https/config/internal/sitefile"
	"fast-https/config/internal/tlsconf"
	"fast-https/config/internal/validate"
)

// 配置校验。
func CheckConfig() error { return validate.CheckConfig() }

func ValidateConfigFile(path string) error { return validate.ValidateConfigFile(path) }

func ValidateConfigFileWithWarnings(path string) ([]string, error) {
	return validate.ValidateConfigFileWithWarnings(path)
}

func ValidateConfigTree(root string, certBases ...string) ([]string, error) {
	return validate.ValidateConfigTree(root, certBases...)
}

func ValidateSiteFile(path string) error { return validate.ValidateSiteFile(path) }

func ValidateNewSiteFile(path string) error { return validate.ValidateNewSiteFile(path) }

// 路径解析与候选树。
func ResolveRuntimePaths(cfg *Fast_Https) { paths.ResolveRuntimePaths(cfg) }

func ResolveCandidateRoot(path string) (string, error) { return paths.ResolveCandidateRoot(path) }

// SetDefault 补齐系统级默认值并把相对路径解析成绝对路径。
func SetDefault(cfg *Fast_Https) { loader.SetDefault(cfg) }

// acme 段读写。
func PreviewACMESection(path string, c *AcmeConfig) ([]byte, error) {
	return acme.PreviewACMESection(path, c)
}

func WriteACMESection(path string, c *AcmeConfig) error { return acme.WriteACMESection(path, c) }

// tls 段读写。
func PreviewTLSSection(path string, c *TLSConfig) ([]byte, error) {
	return tlsconf.PreviewTLSSection(path, c)
}

func WriteTLSSection(path string, c *TLSConfig) error { return tlsconf.WriteTLSSection(path, c) }

func ReadTLSDefaults(path string) (TLSConfig, error) { return tlsconf.ReadTLSDefaults(path) }

// proxy 段读写。
func PreviewProxySection(path string, c *ProxyConfig) ([]byte, error) {
	return proxyconf.PreviewProxySection(path, *c)
}

func WriteProxySection(path string, c *ProxyConfig) error {
	return proxyconf.WriteProxySection(path, *c)
}

func ReadProxySection(path string) (ProxyConfig, error) { return proxyconf.ReadProxySection(path) }

// IsDefaultProxy 报告 proxy 段是否等价于未配置；与写入时的删段判断共用同一逻辑。
func IsDefaultProxy(c ProxyConfig) bool { return proxyconf.IsDefault(c) }

// cache 段读写：缓存区在主配置定义，站点用 proxy_cache 引用区名。
func ReadCacheZones(path string) ([]CacheZone, error) { return cacheconf.ReadCacheZones(path) }

func PreviewCacheZones(path string, zones []CacheZone) ([]byte, error) {
	return cacheconf.PreviewCacheZones(path, zones)
}

func WriteCacheZones(path string, zones []CacheZone) error {
	return cacheconf.WriteCacheZones(path, zones)
}

// CacheZoneRefs 列出引用某个缓存区的站点位置，供删除前检查。
func CacheZoneRefs(mainPath, zone string) ([]string, error) {
	return cacheconf.CacheZoneRefs(mainPath, zone)
}

// 只读扫描。
func ListenPortsFromFile(path string) ([]int, error) { return scan.ListenPortsFromFile(path) }

func ListenPortsFromTree(root string) ([]int, error) { return scan.ListenPortsFromTree(root) }

func SSLServersFromFile(path string) ([]SSLDecl, *AcmeConfig, error) {
	return scan.SSLServersFromFile(path)
}

// 站点文件。
func IsTreeConfigFile(name string) bool { return sitefile.IsTreeConfigFile(name) }

func ParseSiteInput(raw []byte) (SiteInput, error) { return sitefile.ParseSiteInput(raw) }

func SiteDirsFromFile(path string) ([]string, error) { return sitefile.SiteDirsFromFile(path) }

func SnapshotServer(mainPath, name string) ([]byte, string, error) {
	return sitefile.SnapshotServer(mainPath, name)
}

func PatchSiteFile(raw []byte, old, want SiteInput) ([]byte, error) {
	return sitefile.PatchSiteFile(raw, old, want)
}
