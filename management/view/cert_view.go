package view

import (
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"fast-https/config"
	"fast-https/modules/certmanager"
)

// 证书状态。missing、expired、valid、unknown 必须分得清，见规范 8.4。
const (
	CertValid       = "valid"
	CertExpiring    = "expiring"
	CertExpired     = "expired"
	CertNotYetValid = "not_yet_valid"
	CertMissing     = "missing"
	CertUnreadable  = "unreadable"
	CertUnknown     = "unknown"
)

// CertEntry 是一张证书文件的摘要。私钥内容与私钥路径都不输出。
type CertEntry struct {
	Domains     []string `json:"domains"`
	File        string   `json:"file"`
	Status      string   `json:"status"`
	Subject     string   `json:"subject,omitempty"`
	Issuer      string   `json:"issuer,omitempty"`
	Names       []string `json:"names,omitempty"`
	NotBefore   string   `json:"not_before,omitempty"`
	NotAfter    string   `json:"not_after,omitempty"`
	DaysLeft    *int     `json:"days_left,omitempty"`
	SelfSigned  *bool    `json:"self_signed,omitempty"`
	LocalCA     *bool    `json:"local_ca,omitempty"`
	ACMEManaged bool     `json:"acme_managed"`
	DeployedBy  []string `json:"deployed_by"`
	Error       string   `json:"error,omitempty"`
}

// CertListData 是 cert list 的数据。
type CertListData struct {
	RenewBeforeDays int         `json:"renew_before_days"`
	Certificates    []CertEntry `json:"certificates"`
}

// CheckItem 是 cert check 里的一项检查。Checked 为假表示这项没能执行（例如私钥读不了），
// 与「执行了但没通过」（Checked 为真、OK 为假）要分开。
type CheckItem struct {
	Checked bool   `json:"checked"`
	OK      bool   `json:"ok"`
	Detail  string `json:"detail,omitempty"`
}

// CertChecks 是 cert check 的各项检查结果。
type CertChecks struct {
	Exists   CheckItem `json:"exists"`
	Validity CheckItem `json:"validity"`
	Hostname CheckItem `json:"hostname"`
	KeyPair  CheckItem `json:"key_pair"`
	// Chain 目前不做链验证：没有可靠的信任根可用，只报告文件里证书的张数。
	Chain ChainCheck `json:"chain"`
}

// ChainCheck 说明链验证没有执行。
type ChainCheck struct {
	Checked      bool `json:"checked"`
	Certificates int  `json:"certificates_in_file"`
}

// CertCheckData 是 cert check 的数据。Passed 是各项已执行的检查都通过。
type CertCheckData struct {
	Domain      string     `json:"domain"`
	Passed      bool       `json:"passed"`
	Certificate CertEntry  `json:"certificate"`
	Checks      CertChecks `json:"checks"`
}

// CertList 列出配置里声明的证书（按证书文件分组）。数据来自配置声明的路径，
// 不遍历证书目录，以免把无关文件当成证书。
func CertList(mainPath string, now time.Time) (CertListData, []string, error) {
	decls, acme, err := config.SSLServersFromFile(mainPath)
	if err != nil {
		return CertListData{}, nil, err
	}
	renew := renewDays(acme)
	data := CertListData{RenewBeforeDays: renew, Certificates: buildEntries(decls, acme, now, renew)}
	var warnings []string
	if len(decls) == 0 {
		warnings = append(warnings, "no ssl server block is configured")
	}
	return data, warnings, nil
}

// CertCheck 检查某个域名使用的证书：文件是否存在、有效期、域名是否匹配、证书与私钥是否配对。
// 域名没有出现在任何 ssl server 块里时返回 ErrNotFound。
func CertCheck(mainPath, domain string, now time.Time) (CertCheckData, []string, error) {
	want := normalizeDomain(domain)
	if want == "" {
		return CertCheckData{}, nil, fmt.Errorf("%w: empty domain", ErrBadName)
	}
	decls, acme, err := config.SSLServersFromFile(mainPath)
	if err != nil {
		return CertCheckData{}, nil, err
	}

	var match *config.SSLDecl
	matches := 0
	for i := range decls {
		if normalizeDomain(decls[i].Domain) == want {
			if match == nil {
				match = &decls[i]
			}
			matches++
		}
	}
	if match == nil {
		return CertCheckData{}, nil, fmt.Errorf("%w: no ssl server block declares %q", ErrNotFound, domain)
	}

	renew := renewDays(acme)
	var warnings []string
	if matches > 1 {
		warnings = append(warnings, fmt.Sprintf("%q is declared by %d ssl server blocks; the first one is checked", domain, matches))
	}

	entry := entryFor(buildEntries(decls, acme, now, renew), match.Certificate, match.Domain)
	data := CertCheckData{Domain: domain, Certificate: entry}

	info, inspectErr := inspectOptional(match.Certificate)
	switch {
	case match.Certificate == "":
		data.Checks.Exists = CheckItem{Checked: true, Detail: "no ssl_certificate is declared"}
	case errors.Is(inspectErr, fs.ErrNotExist):
		data.Checks.Exists = CheckItem{Checked: true, Detail: "certificate file not found"}
	default:
		// 文件在，哪怕读不了或解析不了；这项只回答「文件存不存在」。
		data.Checks.Exists = CheckItem{Checked: true, OK: true}
	}

	if info == nil {
		skipped := "skipped: the certificate could not be read"
		data.Checks.Validity = CheckItem{Detail: skipped}
		data.Checks.Hostname = CheckItem{Detail: skipped}
		data.Checks.KeyPair = CheckItem{Detail: skipped}
		return data, warnings, nil
	}

	data.Checks.Chain = ChainCheck{Checked: false, Certificates: info.Chain}
	validityOK := entry.Status == CertValid || entry.Status == CertExpiring
	data.Checks.Validity = CheckItem{Checked: true, OK: validityOK, Detail: entry.Status}

	if err := info.VerifyHostname(domain); err != nil {
		data.Checks.Hostname = CheckItem{Checked: true, OK: false, Detail: "the certificate does not cover this domain"}
	} else {
		data.Checks.Hostname = CheckItem{Checked: true, OK: true}
	}

	data.Checks.KeyPair = keyPairItem(match)

	data.Passed = data.Checks.Exists.OK && validityOK && data.Checks.Hostname.OK &&
		(!data.Checks.KeyPair.Checked || data.Checks.KeyPair.OK)
	return data, warnings, nil
}

func keyPairItem(d *config.SSLDecl) CheckItem {
	if d.CertificateKey == "" {
		return CheckItem{Detail: "no ssl_certificate_key is declared"}
	}
	switch certmanager.CheckKeyPair(d.Certificate, d.CertificateKey) {
	case certmanager.KeyPairOK:
		return CheckItem{Checked: true, OK: true}
	case certmanager.KeyPairInvalid:
		return CheckItem{Checked: true, OK: false, Detail: "the certificate and the private key do not match"}
	case certmanager.KeyPairKeyMissing:
		return CheckItem{Checked: true, OK: false, Detail: "private key file not found"}
	case certmanager.KeyPairKeyUnreadable:
		return CheckItem{Detail: "private key file is not readable by the current user"}
	default:
		return CheckItem{Detail: "the certificate file could not be read"}
	}
}

// inspectOptional 读取证书；读不到时返回 nil 与原因。
func inspectOptional(path string) (*certmanager.Info, error) {
	if path == "" {
		return nil, errors.New("no certificate path")
	}
	return certmanager.Inspect(path)
}

func renewDays(acme *config.AcmeConfig) int {
	if acme != nil && acme.RenewBeforeDays > 0 {
		return acme.RenewBeforeDays
	}
	return config.DefaultACMERenewBeforeDays
}

func normalizeDomain(d string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(d), "."))
}

func entryFor(entries []CertEntry, file, domain string) CertEntry {
	want := normalizeDomain(domain)
	for _, e := range entries {
		if e.File != file {
			continue
		}
		for _, d := range e.Domains {
			if d == want {
				return e
			}
		}
	}
	return CertEntry{}
}

// buildEntries 把按域名展开的声明按证书文件归并：同一个文件被几个域名、几个站点使用，只出一条。
func buildEntries(decls []config.SSLDecl, acme *config.AcmeConfig, now time.Time, renew int) []CertEntry {
	managed := acmeManaged(decls, acme)

	type group struct {
		file    string
		domains map[string]struct{}
		sites   map[string]struct{}
	}
	groups := map[string]*group{}
	var order []string
	for _, d := range decls {
		key := d.Certificate
		if key == "" {
			key = "<none>:" + d.Site + ":" + normalizeDomain(d.Domain)
		}
		g, ok := groups[key]
		if !ok {
			g = &group{file: d.Certificate, domains: map[string]struct{}{}, sites: map[string]struct{}{}}
			groups[key] = g
			order = append(order, key)
		}
		g.domains[normalizeDomain(d.Domain)] = struct{}{}
		g.sites[d.Site] = struct{}{}
	}

	entries := make([]CertEntry, 0, len(order))
	for _, key := range order {
		g := groups[key]
		e := CertEntry{
			Domains:    sortedKeys(g.domains),
			File:       g.file,
			DeployedBy: sortedKeys(g.sites),
		}
		for _, dom := range e.Domains {
			if managed[dom] {
				e.ACMEManaged = true
			}
		}
		fillStatus(&e, now, renew)
		entries = append(entries, e)
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].File < entries[j].File })
	return entries
}

func fillStatus(e *CertEntry, now time.Time, renew int) {
	if e.File == "" {
		e.Status, e.Error = CertUnknown, "no ssl_certificate is declared"
		return
	}
	info, err := certmanager.Inspect(e.File)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		e.Status, e.Error = CertMissing, "certificate file not found"
		return
	case errors.Is(err, fs.ErrPermission):
		e.Status, e.Error = CertUnreadable, "certificate file is not readable by the current user"
		return
	case err != nil:
		e.Status, e.Error = CertUnreadable, err.Error()
		return
	}

	days := info.DaysLeft(now)
	selfSigned, localCA := info.SelfSigned, info.LocalCA
	e.Subject, e.Issuer, e.Names = info.Subject, info.Issuer, info.Names
	e.NotBefore, e.NotAfter = info.NotBefore.Format(time.RFC3339), info.NotAfter.Format(time.RFC3339)
	e.DaysLeft, e.SelfSigned, e.LocalCA = &days, &selfSigned, &localCA

	switch {
	case now.Before(info.NotBefore):
		e.Status = CertNotYetValid
	case now.After(info.NotAfter):
		e.Status = CertExpired
	case days <= renew:
		e.Status = CertExpiring
	default:
		e.Status = CertValid
	}
}

// acmeManaged 返回会由 ACME 申请证书的域名集合。判断规则与运行期一致，直接复用 certmanager.Plan。
func acmeManaged(decls []config.SSLDecl, acme *config.AcmeConfig) map[string]bool {
	managed := map[string]bool{}
	if !acme.Active() {
		return managed
	}
	cfg := &config.Fast_Https{Acme: acme}
	for _, d := range decls {
		cfg.Servers = append(cfg.Servers, config.Server{Listen: d.Listens[0], ServerName: d.Domain})
	}
	for _, name := range certmanager.Plan(cfg).Managed {
		managed[name] = true
	}
	return managed
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
