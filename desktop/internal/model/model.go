// Package model 放桌面端与前端之间传递的数据结构。
//
// 单独成包，让 view / siteops / configops / main 共用同一套 DTO 而不互相
// import。字段名上的 json tag 就是前端拿到的形状，改名要同步前端。
package model

type RuntimePaths struct {
	Config  string `json:"config"`
	Cert    string `json:"cert"`
	Log     string `json:"log"`
	Pid     string `json:"pid"`
	RootCRT string `json:"root_crt"`
}

type Overview struct {
	Running         bool         `json:"running"`
	PID             int          `json:"pid"`
	Version         string       `json:"version"`
	Ports           []string     `json:"ports"`
	SiteCount       int          `json:"siteCount"`
	TLSCount        int          `json:"tlsCount"`
	NearestCertDays int          `json:"nearestCertDays"`
	ConfigDirty     bool         `json:"configDirty"`
	Paths           RuntimePaths `json:"paths"`
	CheckedAt       string       `json:"checkedAt"`
	Error           string       `json:"error"`
}

type Site struct {
	Name     string `json:"name"`
	Port     string `json:"port"`
	Listen   string `json:"listen"`
	TLS      bool   `json:"tls"`
	Root     string `json:"root"`
	CertFile string `json:"certFile"`
	Source   string `json:"source"`
	Enabled  bool   `json:"enabled"`
}

type Cert struct {
	Domain     string `json:"domain"`
	Subject    string `json:"subject"`
	Names      string `json:"names"`
	Issuer     string `json:"issuer"`
	NotAfter   string `json:"notAfter"`
	DaysLeft   int    `json:"daysLeft"`
	SelfSigned bool   `json:"selfSigned"`
	// Local 表示「本机自动生成的证书」：自签，或由本地 CA（O=fast-https）签发。
	// 后者 Issuer != Subject，只用 selfSigned 会误标成外部 CA 已签发。
	Local bool   `json:"local"`
	Note  string `json:"note"`
}

type SiteInput struct {
	Name      string `json:"name"`
	Listen    string `json:"listen"`
	Type      string `json:"type"`
	Root      string `json:"root"`
	ProxyPass string `json:"proxyPass"`
	Index     string `json:"index"`
	Cert      string `json:"cert"`
	CertKey   string `json:"certKey"`
	FileName  string `json:"fileName"`
	Source    string `json:"source"`
}

type SiteDetail struct {
	Source    string `json:"source"`
	Path      string `json:"path"`
	Editable  bool   `json:"editable"`
	Note      string `json:"note"`
	Name      string `json:"name"`
	Listen    string `json:"listen"`
	Type      string `json:"type"`
	Root      string `json:"root"`
	ProxyPass string `json:"proxyPass"`
	Index     string `json:"index"`
	Cert      string `json:"cert"`
	CertKey   string `json:"certKey"`
	Raw       string `json:"raw"`
}

type ConfigField struct {
	Path    string   `json:"path"`
	Label   string   `json:"label"`
	Group   string   `json:"group"`
	Type    string   `json:"type"` // text | number | bool | select | list
	Value   string   `json:"value"`
	Options []string `json:"options,omitempty"`
	Help    string   `json:"help,omitempty"`
	Missing bool     `json:"missing"`
}

type ConfigForm struct {
	Path      string        `json:"path"`
	Fields    []ConfigField `json:"fields"`
	Size      int64         `json:"size"`
	Modified  string        `json:"modified"`
	Writable  bool          `json:"writable"`
	Preserved []string      `json:"preserved"`
}

type LogEntry struct {
	Raw        string `json:"raw"`
	Time       string `json:"time"`
	Level      string `json:"level"`
	Kind       string `json:"kind"`
	Message    string `json:"message"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	Status     string `json:"status"`
	Size       string `json:"size"`
	Host       string `json:"host"`
	RemoteAddr string `json:"remoteAddr"`
}

type LogPage struct {
	Lines   []LogEntry `json:"lines"`
	HasMore bool       `json:"hasMore"`
	Matched int        `json:"matched"`
}

type CertTrustGuide struct {
	Platform      string   `json:"platform"`
	PlatformLabel string   `json:"platformLabel"`
	CertPath      string   `json:"certPath"`
	DirPath       string   `json:"dirPath"`
	KeyPath       string   `json:"keyPath"`
	Exists        bool     `json:"exists"`
	Command       string   `json:"command"`
	Steps         []string `json:"steps"`
	Risk          string   `json:"risk"`
}
