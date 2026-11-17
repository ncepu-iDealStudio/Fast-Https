// Package siteops 在临时候选配置树上改站点，再交给事务引擎应用。
package siteops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"fast-https/config"
	"fast-https/infra/files"
	"fast-https/infra/jsonc"
	"fast-https/management/cfgdiff"
	"fast-https/management/operation"
	"fast-https/management/pathguard"
	"fast-https/management/txn"
	"fast-https/management/view"
	"fast-https/modules/site"
)

var (
	ErrConflict     = errors.New("site conflict")
	ErrNotFound     = errors.New("site not found")
	ErrNotSupported = errors.New("not supported")
	ErrUsage        = errors.New("invalid site input")
)

// AddInput 是 site add 的参数。
type AddInput struct {
	Domain    string
	Root      string
	Listens   []string
	ProxyPass string
	TLS       bool
	// SymlinkPolicy 只对 local 站点生效，取值见 config.SYMLINK_*；空表示默认。
	SymlinkPolicy string
	Banned        []string
	// SSLCertificate / SSLCertificateKey 留空表示走默认约定（config/cert/<域名>.pem）；
	// 显式给出时按给定路径写入，用于多站点共享同一张证书（如通配符证书）。
	SSLCertificate    string
	SSLCertificateKey string
}

// UpdateInput 是 site update 的白名单字段。未设置的指针表示不改。
//
// Zip / Limit / Auth / Cache 作用于站点的第一个 location（主 location）。
// 它们用指针表达三态：nil 不改；指向零值表示清除该配置；指向有值则写入。
type UpdateInput struct {
	Name       string
	Listens    *[]string
	ServerName *string
	Root       *string
	ProxyPass  *string
	TLS        *bool
	Zip        *[]string
	// ZipMinLength 三态：nil 不改；指向 0 表示清除阈值（不限制）；正数写入。
	ZipMinLength *int
	// ZipSkipTypes 三态：nil 不改；指向空列表表示回落内置已压缩类型清单。
	ZipSkipTypes *[]string
	Limit        *config.PathLimit
	Auth         *config.PathAuth
	Cache        *config.Cache
	// SymlinkPolicy 三态：nil 不改；指向空串表示回到默认策略；否则写入取值。
	SymlinkPolicy *string
	Banned        []string
	// 证书路径同样是三态：nil 不改；指向空串表示交回默认约定（首次 start 生成）。
	SSLCertificate    *string
	SSLCertificateKey *string
}

// RunInput 是一次站点写操作。
type RunInput struct {
	LiveMain  string
	RequestID string
	Command   string
	Summary   string
	DryRun    bool
	Args      map[string]string
	Mutate    func(candRoot string, dirs []string) error
}

// Run 复制线上配置树到临时目录，在副本上改站点，再走事务引擎。
func Run(ctx context.Context, eng *txn.Engine, in RunInput) (txn.Result, error) {
	if res, ok, err := replayIdempotent(eng, in); ok || err != nil {
		return res, err
	}
	cand, err := CloneLiveTree(in.LiveMain)
	if err != nil {
		return txn.Result{}, err
	}
	defer func() { _ = os.RemoveAll(cand) }()
	dirs := []string{filepath.Join(cand, "conf.d")}
	if err := os.MkdirAll(dirs[0], 0o755); err != nil {
		return txn.Result{}, err
	}
	if err := in.Mutate(cand, dirs); err != nil {
		return txn.Result{}, err
	}
	return eng.Apply(ctx, txn.Input{
		Candidate: cand,
		LiveMain:  in.LiveMain,
		RequestID: in.RequestID,
		Command:   in.Command,
		Summary:   in.Summary,
		DryRun:    in.DryRun,
		Args:      in.Args,
	})
}

// replayIdempotent 在改候选树之前认同一 request_id：线上已经有目标站点时，
// 再走 Add 会 CONFLICT，必须先把已有 operation 还回去。
func replayIdempotent(eng *txn.Engine, in RunInput) (txn.Result, bool, error) {
	if in.DryRun || eng == nil || strings.TrimSpace(in.RequestID) == "" {
		return txn.Result{}, false, nil
	}
	rec, ok, err := operation.Store{Dir: eng.State}.Lookup(in.RequestID)
	if err != nil || !ok {
		return txn.Result{}, false, err
	}
	if rec.ArgsHash != operation.HashArgs(in.Command, in.Args) {
		return txn.Result{}, false, operation.ErrRequestIDConflict
	}
	out := txn.Result{
		Operation:   rec,
		Idempotent:  true,
		ConfigState: txn.StateUnchanged,
		Diff:        cfgdiff.Result{Identical: true},
	}
	if rec.Result != nil {
		out.RestorePoint, _ = rec.Result["restore_point"].(string)
		out.Version, _ = rec.Result["version"].(string)
		out.RolledBack, _ = rec.Result["rolled_back"].(bool)
		if s, ok := rec.Result["config_state"].(string); ok && s != "" {
			out.ConfigState = s
		}
	}
	return out, true, nil
}

// CloneLiveTree 把主配置与站点 json（含停用文件）复制成一棵候选目录。
func CloneLiveTree(liveMain string) (string, error) {
	abs, err := filepath.Abs(liveMain)
	if err != nil {
		return "", err
	}
	dst, err := os.MkdirTemp("", "fast-https-site-")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Join(dst, "conf.d"), 0o755); err != nil {
		_ = os.RemoveAll(dst)
		return "", err
	}
	raw, err := os.ReadFile(abs)
	if err != nil {
		_ = os.RemoveAll(dst)
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dst, "fast-https.json"), raw, 0o644); err != nil {
		_ = os.RemoveAll(dst)
		return "", err
	}
	dirs, err := config.SiteDirsFromFile(abs)
	if err != nil {
		_ = os.RemoveAll(dst)
		return "", err
	}
	needRewrite := includeHasAbs(raw)
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !config.IsTreeConfigFile(e.Name()) {
				continue
			}
			from := filepath.Join(dir, e.Name())
			to := filepath.Join(dst, "conf.d", e.Name())
			if err := copyFile(from, to); err != nil {
				_ = os.RemoveAll(dst)
				return "", err
			}
		}
	}
	if needRewrite {
		patched, err := jsonc.PatchPath(raw, []jsonc.Edit{{
			Path:  []jsonc.Step{jsonc.Key("http"), jsonc.Key("include")},
			Value: []byte(`["conf.d"]`),
		}})
		if err != nil {
			_ = os.RemoveAll(dst)
			return "", err
		}
		if err := os.WriteFile(filepath.Join(dst, "fast-https.json"), patched, 0o644); err != nil {
			_ = os.RemoveAll(dst)
			return "", err
		}
	}
	return dst, nil
}

func includeHasAbs(raw []byte) bool {
	var root struct {
		HTTP struct {
			Include []string `json:"include"`
		} `json:"http"`
	}
	if err := jsonc.Unmarshal(raw, &root); err != nil {
		return true
	}
	for _, p := range root.HTTP.Include {
		if filepath.IsAbs(strings.TrimSpace(p)) {
			return true
		}
	}
	return false
}

func copyFile(from, to string) error {
	data, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return files.WriteAtomic(to, data, 0o644)
}

// NameFromDomain 把域名收成站点文件名。
func NameFromDomain(domain string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(domain))
	if name == "" || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return "", fmt.Errorf("%w: invalid domain %q", ErrUsage, domain)
	}
	if err := site.ValidateName(name); err != nil {
		return "", fmt.Errorf("%w: %v", ErrUsage, err)
	}
	return name, nil
}

// Add 在候选树上新建站点。
func Add(dirs []string, in AddInput) (string, error) {
	name, err := NameFromDomain(in.Domain)
	if err != nil {
		return "", err
	}
	if _, ok := site.Find(dirs, name); ok {
		return "", fmt.Errorf("%w: site %s already exists", ErrConflict, name)
	}
	if _, ok := site.FindDisabled(dirs, name); ok {
		return "", fmt.Errorf("%w: site %s already exists (disabled)", ErrConflict, name)
	}
	listens := resolveListens(in.TLS, in.Listens)
	if err := rejectDuplicateServerName(dirs, "", strings.TrimSpace(in.Domain), listens); err != nil {
		return "", err
	}
	if (in.SSLCertificate != "" || in.SSLCertificateKey != "") && !hasSSL(listens) {
		return "", fmt.Errorf("%w: --ssl-certificate requires a TLS listen (use --tls or \"443 ssl\" in --listen)", ErrUsage)
	}
	si := config.SiteInput{
		Listens:           listens,
		ServerName:        strings.TrimSpace(in.Domain),
		SSLCertificate:    strings.TrimSpace(in.SSLCertificate),
		SSLCertificateKey: strings.TrimSpace(in.SSLCertificateKey),
	}
	policy, err := normalizeSymlinkPolicy(in.SymlinkPolicy)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(in.ProxyPass) != "" {
		if policy != "" {
			return "", fmt.Errorf("%w: --symlink-policy only applies to local locations", ErrUsage)
		}
		si.Locations = []config.SiteLocation{{URL: "/", Type: "proxy", ProxyPass: strings.TrimSpace(in.ProxyPass)}}
	} else {
		root := in.Root
		if strings.TrimSpace(root) == "" {
			root = "./httpdoc/root"
		}
		abs, err := pathguard.CheckRoot(root, in.Banned)
		if err != nil {
			return "", err
		}
		si.Locations = []config.SiteLocation{{
			URL: "/", Type: "local", Root: abs,
			Index:         []string{"index.html", "index.htm"},
			SymlinkPolicy: policy,
		}}
	}
	fillCerts(&si)
	path, err := site.Create(dirs, name, si, false)
	if err != nil {
		return "", asInvalid(err)
	}
	return path, nil
}

// Update 只改白名单字段。
func Update(dirs []string, liveMain string, in UpdateInput) (string, error) {
	if err := site.ValidateName(in.Name); err != nil {
		return "", fmt.Errorf("%w: %v", ErrUsage, err)
	}
	if inlineSite(liveMain, in.Name) {
		return "", fmt.Errorf("%w: site %s is inline in the main config (http.server); move it to conf.d first", ErrNotSupported, in.Name)
	}
	if _, ok := site.Find(dirs, in.Name); !ok {
		return "", fmt.Errorf("%w: %s", ErrNotFound, in.Name)
	}
	path, err := site.Update(dirs, in.Name, func(cur *config.SiteInput) error {
		// 先算出生效的 listen：server_name 冲突按「域名 + 端口」判，
		// 需要知道改完之后这个站点实际监听哪些端口。
		tls := in.TLS != nil && *in.TLS
		effListens := cur.Listens
		if in.Listens != nil {
			effListens = resolveListens(tls, *in.Listens)
		} else if in.TLS != nil {
			effListens = resolveListens(*in.TLS, cur.Listens)
		}
		if in.ServerName != nil {
			name := strings.TrimSpace(*in.ServerName)
			if name == "" {
				return fmt.Errorf("%w: --server-name must not be empty", ErrUsage)
			}
			if err := rejectDuplicateServerName(dirs, in.Name, name, effListens); err != nil {
				return err
			}
			followCertRename(cur, name)
			cur.ServerName = name
		}
		if in.Listens != nil || in.TLS != nil {
			cur.Listens = effListens
		}
		// 显式指定证书放在改名跟随之后：同时给两者时以用户给的路径为准。
		if in.SSLCertificate != nil {
			cur.SSLCertificate = strings.TrimSpace(*in.SSLCertificate)
		}
		if in.SSLCertificateKey != nil {
			cur.SSLCertificateKey = strings.TrimSpace(*in.SSLCertificateKey)
		}
		if (in.SSLCertificate != nil || in.SSLCertificateKey != nil) && !hasSSL(cur.Listens) {
			return fmt.Errorf("%w: --ssl-certificate requires a TLS listen", ErrUsage)
		}
		if in.Root != nil || in.ProxyPass != nil {
			loc := primaryLoc(cur)
			if in.ProxyPass != nil {
				pass := strings.TrimSpace(*in.ProxyPass)
				if pass == "" {
					return fmt.Errorf("%w: --proxy-pass must not be empty", ErrUsage)
				}
				loc.Type = "proxy"
				loc.ProxyPass = pass
				loc.Root = ""
				loc.Index = nil
			} else if in.Root != nil {
				abs, err := pathguard.CheckRoot(*in.Root, in.Banned)
				if err != nil {
					return err
				}
				loc.Type = "local"
				loc.Root = abs
				loc.ProxyPass = ""
				if len(loc.Index) == 0 {
					loc.Index = []string{"index.html", "index.htm"}
				}
			}
		}
		if err := applyAdvanced(cur, in); err != nil {
			return err
		}
		fillCerts(cur)
		return nil
	})
	if err != nil {
		return "", asInvalid(err)
	}
	return path, nil
}

// normalizeSymlinkPolicy 校验 --symlink-policy 取值；空串表示清除（回到默认）。
func normalizeSymlinkPolicy(raw string) (string, error) {
	switch v := strings.ToLower(strings.TrimSpace(raw)); v {
	case "", config.SYMLINK_ALLOW, config.SYMLINK_REJECT, config.SYMLINK_CHECK_AT_LOAD:
		return v, nil
	default:
		return "", fmt.Errorf("%w: --symlink-policy accepts allow, reject or check-at-load, got %q", ErrUsage, raw)
	}
}

// applyAdvanced 把 --zip / --limit-* / --auth-* / --cache-* / --symlink-policy
// 落到主 location 上。
//
// 密码特殊处理：GUI 读到的是脱敏值 "***"，用户没改密码时前端原样回传，
// 这里遇到空串或 "***" 就保留文件里的原密码，避免把真实口令写成星号。
func applyAdvanced(cur *config.SiteInput, in UpdateInput) error {
	if in.Zip == nil && in.ZipMinLength == nil && in.ZipSkipTypes == nil &&
		in.Limit == nil && in.Auth == nil && in.Cache == nil && in.SymlinkPolicy == nil {
		return nil
	}
	loc := primaryLoc(cur)

	if in.SymlinkPolicy != nil {
		policy, err := normalizeSymlinkPolicy(*in.SymlinkPolicy)
		if err != nil {
			return err
		}
		if policy != "" && loc.Type != "" && loc.Type != "local" {
			return fmt.Errorf("%w: --symlink-policy only applies to local locations", ErrUsage)
		}
		loc.SymlinkPolicy = policy
	}

	if in.Zip != nil {
		zip := make([]string, 0, len(*in.Zip))
		for _, raw := range *in.Zip {
			v := strings.ToLower(strings.TrimSpace(raw))
			if v != "gzip" && v != "br" {
				return fmt.Errorf("%w: --zip accepts only gzip or br, got %q", ErrUsage, raw)
			}
			zip = append(zip, v)
		}
		loc.Zip = &zip
	}

	if in.ZipMinLength != nil {
		v := *in.ZipMinLength
		if v < 0 {
			return fmt.Errorf("%w: --zip-min-length must be >= 0, got %d", ErrUsage, v)
		}
		loc.ZipMinLength = &v
	}

	if in.ZipSkipTypes != nil {
		types := make([]string, 0, len(*in.ZipSkipTypes))
		seen := make(map[string]bool, len(*in.ZipSkipTypes))
		for _, raw := range *in.ZipSkipTypes {
			v := strings.ToLower(strings.TrimSpace(raw))
			if v == "" || seen[v] {
				continue
			}
			if !strings.Contains(v, "/") {
				return fmt.Errorf("%w: --zip-skip-types wants MIME types like image/png, got %q", ErrUsage, raw)
			}
			seen[v] = true
			types = append(types, v)
		}
		loc.ZipSkipTypes = &types
	}

	if in.Limit != nil {
		v := *in.Limit
		if v.Size < 0 || v.Rate < 0 || v.Burst < 0 {
			return fmt.Errorf("%w: --limit-* must not be negative", ErrUsage)
		}
		loc.Limit = &v
	}

	if in.Auth != nil {
		v := *in.Auth
		v.AuthType = strings.TrimSpace(v.AuthType)
		v.User = strings.TrimSpace(v.User)
		v.Realm = strings.TrimSpace(v.Realm)
		if v.AuthType == "" && v.User == "" && v.Pswd == "" && v.Realm == "" {
			loc.Auth = &config.PathAuth{} // 全空 = 关闭认证，补丁会删掉 auth 键
		} else {
			if v.AuthType == "" {
				v.AuthType = "basic"
			}
			if v.AuthType != "basic" {
				return fmt.Errorf("%w: --auth-type only supports basic, got %q", ErrUsage, v.AuthType)
			}
			if v.User == "" {
				return fmt.Errorf("%w: --auth-user is required when enabling auth", ErrUsage)
			}
			// 空串或脱敏值表示"沿用原密码"
			if keepPassword(v.Pswd) {
				if loc.Auth == nil || loc.Auth.Pswd == "" {
					return fmt.Errorf("%w: --auth-pass is required when enabling auth for the first time", ErrUsage)
				}
				v.Pswd = loc.Auth.Pswd
			}
			loc.Auth = &v
		}
	}

	if in.Cache != nil {
		v := *in.Cache
		v.Path = strings.TrimSpace(v.Path)
		if v.Path == "" && len(v.Valid) == 0 && v.MaxSize == 0 {
			loc.Cache = &config.Cache{} // 全空 = 关闭缓存
		} else {
			if v.Path == "" {
				return fmt.Errorf("%w: --cache-path is required when enabling proxy cache", ErrUsage)
			}
			loc.Cache = &v
		}
	}

	return nil
}

// keepPassword 报告前端回传的口令是否代表"不修改"：空串或脱敏值都算。
func keepPassword(pswd string) bool {
	trimmed := strings.TrimSpace(pswd)
	return trimmed == "" || trimmed == "***"
}

func asInvalid(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrUsage) || errors.Is(err, ErrConflict) || errors.Is(err, ErrNotFound) ||
		errors.Is(err, ErrNotSupported) || errors.Is(err, pathguard.ErrPath) {
		return err
	}
	var inv *view.InvalidError
	if errors.As(err, &inv) {
		return err
	}
	return &view.InvalidError{Message: err.Error()}
}

// Delete 只删站点配置文件。返回被保留（未删除）的证书文件，供调用方提示。
//
// 证书与私钥按域名放在 config/cert 下、会被同名站点复用，所以删除站点时
// 刻意不动它们：重建同名站点能直接拿回原来的证书，不需要重新签发。
func Delete(dirs []string, liveMain, name string) (string, []string, error) {
	if err := site.ValidateName(name); err != nil {
		return "", nil, fmt.Errorf("%w: %v", ErrUsage, err)
	}
	if inlineSite(liveMain, name) {
		return "", nil, fmt.Errorf("%w: site %s is inline in the main config", ErrNotSupported, name)
	}
	kept := certFilesOf(dirs, name)
	p, err := site.Remove(dirs, name)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	return p, kept, nil
}

// certFilesOf 读出站点文件里声明的、磁盘上确实存在的证书/私钥路径。
func certFilesOf(dirs []string, name string) []string {
	path, ok := site.Find(dirs, name)
	if !ok {
		path, ok = site.FindDisabled(dirs, name)
	}
	if !ok {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	si, err := config.ParseSiteInput(raw)
	if err != nil {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, declared := range []string{si.SSLCertificate, si.SSLCertificateKey} {
		declared = strings.TrimSpace(declared)
		if declared == "" {
			continue
		}
		for _, candidate := range []string{
			declared,
			filepath.Join(filepath.Dir(path), declared),
			// 相对主配置根目录（conf.d 的上一级）解析：站点文件里常见 "./cert/x.pem"
			filepath.Join(filepath.Dir(filepath.Dir(path)), declared),
			filepath.Join(config.CERT_DIR, filepath.Base(declared)),
		} {
			if _, err := os.Stat(candidate); err != nil || seen[candidate] {
				continue
			}
			seen[candidate] = true
			out = append(out, candidate)
			break
		}
	}
	return out
}

// followCertRename 在站点改名时让「按域名自动命名」的证书路径跟着改名。
//
// 只跟随约定路径（文件名等于 <旧主域名>.pem / <旧主域名>-key.pem，且位于证书
// 目录）：自定义路径可能是通配符证书或多站点共享的同一张证书，改名时不能动。
func followCertRename(cur *config.SiteInput, newServerName string) {
	oldPrimary, newPrimary := primaryName(cur.ServerName), primaryName(newServerName)
	if oldPrimary == "" || newPrimary == "" || strings.EqualFold(oldPrimary, newPrimary) {
		return
	}
	if conventionalCertPath(cur.SSLCertificate, oldPrimary) {
		cur.SSLCertificate = filepath.Join(filepath.Dir(cur.SSLCertificate), newPrimary+".pem")
	}
	if conventionalCertPath(cur.SSLCertificateKey, oldPrimary) {
		cur.SSLCertificateKey = filepath.Join(filepath.Dir(cur.SSLCertificateKey), newPrimary+"-key.pem")
	}
}

// primaryName 取 server_name 的第一个域名（多域名写法空格分隔）。
func primaryName(serverName string) string {
	fields := strings.Fields(serverName)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// conventionalCertPath 判断 path 是否是「文件名就是主域名」的约定路径。
func conventionalCertPath(path, primary string) bool {
	path = strings.TrimSpace(path)
	if path == "" || primary == "" {
		return false
	}
	base := strings.ToLower(filepath.Base(path))
	want := strings.ToLower(primary)
	if base != want+".pem" && base != want+"-key.pem" {
		return false
	}
	dir := filepath.Dir(path)
	return dir == "." || strings.EqualFold(filepath.Base(dir), "cert")
}

// SetEnabled 启用或停用。
func SetEnabled(dirs []string, liveMain, name string, enabled bool) (string, error) {
	if err := site.ValidateName(name); err != nil {
		return "", fmt.Errorf("%w: %v", ErrUsage, err)
	}
	if inlineSite(liveMain, name) {
		return "", fmt.Errorf("%w: site %s is inline in the main config", ErrNotSupported, name)
	}
	if enabled {
		if _, ok := site.Find(dirs, name); ok {
			return "", fmt.Errorf("%w: site %s is already enabled", ErrConflict, name)
		}
		p, err := site.Enable(dirs, name)
		if err != nil {
			return "", fmt.Errorf("%w: %v", ErrNotFound, err)
		}
		return p, nil
	}
	if _, ok := site.FindDisabled(dirs, name); ok {
		return "", fmt.Errorf("%w: site %s is already disabled", ErrConflict, name)
	}
	p, err := site.Disable(dirs, name)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	return p, nil
}

func inlineSite(liveMain, name string) bool {
	_, source, err := config.SnapshotServer(liveMain, name)
	if err != nil {
		return false
	}
	absMain, _ := filepath.Abs(liveMain)
	absSrc, _ := filepath.Abs(source)
	return filepath.Clean(absMain) == filepath.Clean(absSrc)
}

// rejectDuplicateServerName 检查「同一端口上的 server_name 重复」。
//
// 冲突判据是 **域名 + 端口**，不是只看域名：运行时的 HostMap 键是
// server_name:port（见 runtime/listener），同一个域名挂在 80 和 8080 上是
// 两个互不干扰的 server 块，nginx 语义里也完全合法。只有同一端口上出现
// 重复域名才会让请求归属变得不确定，那才需要拒绝。
func rejectDuplicateServerName(dirs []string, skipName, serverName string, listens []string) error {
	want := domainSet(serverName)
	if len(want) == 0 {
		return nil
	}
	wantPorts := listenPortSet(listens...)
	entries, err := site.ListAll(dirs)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if skipName != "" && e.Name == skipName {
			continue
		}
		dup := sharedDomain(want, e.ServerName)
		if dup == "" {
			continue
		}
		// 端口集合解不出来时退回旧口径（只看域名），宁可多拦一次也不
		// 放行一个连端口都读不懂的配置。
		if len(wantPorts) == 0 {
			return fmt.Errorf("%w: server_name %s already used by %s", ErrConflict, dup, e.Name)
		}
		shared := sharedPorts(wantPorts, listenPortSet(e.Listen))
		if len(shared) == 0 {
			continue
		}
		return fmt.Errorf("%w: server_name %s already used by %s on port %s",
			ErrConflict, dup, e.Name, strings.Join(shared, ","))
	}
	return nil
}

// sharedDomain 返回两个 server_name 之间重复的域名（排序后取第一个，
// 保证报错文案稳定）。没有交集时返回空串。
func sharedDomain(want map[string]struct{}, serverName string) string {
	var dup []string
	for name := range domainSet(serverName) {
		if _, ok := want[name]; ok {
			dup = append(dup, name)
		}
	}
	if len(dup) == 0 {
		return ""
	}
	sort.Strings(dup)
	return dup[0]
}

// domainSet 把「空格分隔的多域名 server_name」拆成小写集合。
func domainSet(serverName string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, name := range strings.Fields(serverName) {
		if name = strings.ToLower(name); name != "" {
			out[name] = struct{}{}
		}
	}
	return out
}

// listenPortSet 取出 listen 串里的端口集合，兼容 "80"、"443 ssl h2"、
// "80, 443 ssl h2" 以及 ":8080" / "127.0.0.1:8080" 这些写法。
func listenPortSet(listens ...string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, raw := range listens {
		segs := strings.FieldsFunc(raw, func(r rune) bool {
			return r == ',' || r == '\n' || r == '\r'
		})
		for _, seg := range segs {
			seg = strings.TrimSpace(seg)
			if seg == "" {
				continue
			}
			port := strings.Fields(seg)[0]
			if i := strings.LastIndex(port, ":"); i >= 0 {
				port = port[i+1:]
			}
			if port == "" {
				continue
			}
			// 归一化 "080" 这类写法，避免字符串比较误判。
			if n, err := strconv.Atoi(port); err == nil {
				port = strconv.Itoa(n)
			}
			out[port] = struct{}{}
		}
	}
	return out
}

// sharedPorts 返回两个端口集合的交集，排序后给出稳定输出。
func sharedPorts(a, b map[string]struct{}) []string {
	var out []string
	for port := range a {
		if _, ok := b[port]; ok {
			out = append(out, port)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		x, errX := strconv.Atoi(out[i])
		y, errY := strconv.Atoi(out[j])
		if errX == nil && errY == nil {
			return x < y
		}
		return out[i] < out[j]
	})
	return out
}

func primaryLoc(in *config.SiteInput) *config.SiteLocation {
	for i := range in.Locations {
		if url := strings.TrimSpace(in.Locations[i].URL); url == "/" || url == "" {
			return &in.Locations[i]
		}
	}
	in.Locations = append(in.Locations, config.SiteLocation{URL: "/"})
	return &in.Locations[len(in.Locations)-1]
}

func resolveListens(tls bool, given []string) []string {
	var listens []string
	for _, item := range given {
		if v := strings.TrimSpace(item); v != "" {
			listens = append(listens, v)
		}
	}
	if len(listens) == 0 {
		if tls {
			return []string{"80", "443 ssl h2"}
		}
		return []string{"80"}
	}
	if !tls || hasSSL(listens) {
		return listens
	}
	out := append([]string(nil), listens...)
	upgraded := false
	for i, l := range out {
		port := strings.Fields(l)
		if len(port) == 0 {
			continue
		}
		p := strings.TrimPrefix(port[0], ":")
		if p == "80" {
			continue
		}
		if p == "443" || !upgraded {
			out[i] = withTLS(l)
			upgraded = true
		}
	}
	if !upgraded {
		out = append(out, "443 ssl h2")
	}
	return out
}

func hasSSL(listens []string) bool {
	for _, l := range listens {
		if strings.Contains(l, "ssl") {
			return true
		}
	}
	return false
}

func withTLS(listen string) string {
	if !strings.Contains(listen, "ssl") {
		listen += " ssl"
	}
	if !strings.Contains(listen, "h2") && !strings.Contains(listen, "tcp") {
		listen += " h2"
	}
	return listen
}

func fillCerts(in *config.SiteInput) {
	if !hasSSL(in.Listens) {
		return
	}
	name := strings.TrimSpace(in.ServerName)
	cert := filepath.Join(config.CERT_DIR, name+".pem")
	// 首次 start 之前本地 CA 还没签发这张证书，此时留空：CertInit 会按
	// 站点自动生成。写死一个不存在的路径会被 config apply 级别的校验
	// 拒绝（ssl_certificate not found），而这条路径本来就是启动时才产生。
	if _, err := os.Stat(cert); err != nil {
		return
	}
	if in.SSLCertificate == "" {
		in.SSLCertificate = cert
	}
	if in.SSLCertificateKey == "" {
		in.SSLCertificateKey = filepath.Join(config.CERT_DIR, name+"-key.pem")
	}
}
