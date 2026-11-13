// Package siteops 在临时候选配置树上改站点，再交给事务引擎应用。
package siteops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	Banned    []string
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
	Limit      *config.PathLimit
	Auth       *config.PathAuth
	Cache      *config.Cache
	Banned     []string
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
	if err := rejectDuplicateServerName(dirs, "", strings.TrimSpace(in.Domain)); err != nil {
		return "", err
	}
	si := config.SiteInput{
		Listens:    resolveListens(in.TLS, in.Listens),
		ServerName: strings.TrimSpace(in.Domain),
	}
	if strings.TrimSpace(in.ProxyPass) != "" {
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
			Index: []string{"index.html", "index.htm"},
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
		if in.ServerName != nil {
			name := strings.TrimSpace(*in.ServerName)
			if name == "" {
				return fmt.Errorf("%w: --server-name must not be empty", ErrUsage)
			}
			if err := rejectDuplicateServerName(dirs, in.Name, name); err != nil {
				return err
			}
			cur.ServerName = name
		}
		tls := in.TLS != nil && *in.TLS
		if in.Listens != nil {
			cur.Listens = resolveListens(tls, *in.Listens)
		} else if in.TLS != nil {
			cur.Listens = resolveListens(*in.TLS, cur.Listens)
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

// applyAdvanced 把 --zip / --limit-* / --auth-* / --cache-* 落到主 location 上。
//
// 密码特殊处理：GUI 读到的是脱敏值 "***"，用户没改密码时前端原样回传，
// 这里遇到空串或 "***" 就保留文件里的原密码，避免把真实口令写成星号。
func applyAdvanced(cur *config.SiteInput, in UpdateInput) error {
	if in.Zip == nil && in.Limit == nil && in.Auth == nil && in.Cache == nil {
		return nil
	}
	loc := primaryLoc(cur)

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

// Delete 只删站点配置文件。
func Delete(dirs []string, liveMain, name string) (string, error) {
	if err := site.ValidateName(name); err != nil {
		return "", fmt.Errorf("%w: %v", ErrUsage, err)
	}
	if inlineSite(liveMain, name) {
		return "", fmt.Errorf("%w: site %s is inline in the main config", ErrNotSupported, name)
	}
	p, err := site.Remove(dirs, name)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	return p, nil
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

func rejectDuplicateServerName(dirs []string, skipName, serverName string) error {
	want := strings.ToLower(strings.TrimSpace(serverName))
	if want == "" {
		return nil
	}
	entries, err := site.ListAll(dirs)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if skipName != "" && e.Name == skipName {
			continue
		}
		if strings.ToLower(e.ServerName) == want {
			return fmt.Errorf("%w: server_name %s already used by %s", ErrConflict, serverName, e.Name)
		}
	}
	return nil
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
