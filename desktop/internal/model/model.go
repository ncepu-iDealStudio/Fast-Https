// Package model 放桌面端与前端之间传递的数据结构。
//
// 单独成包，让 view / siteops / configops / main 共用同一套 DTO 而不互相
// import。字段名上的 json tag 就是前端拿到的形状，改名要同步前端。
//
// 这些 DTO 是「界面视图模型」：数据全部来自 fast-https <cmd> --json 的
// data 投影（见 docs/cli/cli-json-shapes.md），桌面端不重新定义命令契约，
// 只做界面需要的归一化（例如 site list 的合并 listen 拆成逐端口行）。
package model

type RuntimePaths struct {
	Config  string `json:"config"`
	Cert    string `json:"cert"`
	Log     string `json:"log"`
	Pid     string `json:"pid"`
	RootCRT string `json:"root_crt"`
	State   string `json:"state"`
}

// Overview 是概览页一次拉取的全部内容：service status + site list +
// cert list + config show 的投影。
type Overview struct {
	// Status 是 service status 的四态：running / degraded / stopped / unknown。
	Status          string       `json:"status"`
	Running         bool         `json:"running"`
	PID             int          `json:"pid"`
	Version         string       `json:"version"`
	UptimeSeconds   int64        `json:"uptimeSeconds"`
	ConfigVersion   string       `json:"configVersion"`
	Ports           []string     `json:"ports"`
	SiteCount       int          `json:"siteCount"`
	TLSCount        int          `json:"tlsCount"`
	NearestCertDays int          `json:"nearestCertDays"`
	ConfigDirty     bool         `json:"configDirty"`
	Paths           RuntimePaths `json:"paths"`
	CheckedAt       string       `json:"checkedAt"`
	Error           string       `json:"error"`
}

// Site 是站点页/矩阵页的一行：**一个站点文件一行**。
//
// 一个站点常同时监听多个端口（如 "80, 443 ssl h2"），它们共享同一个
// 配置文件与同一个启停开关。Listen 是合并后的完整监听串（展示用），
// Listens 是拆开后的单条监听（矩阵页按端口列时用），Port 是其中第一个端口。
type Site struct {
	Id         string   `json:"id"`
	Name       string   `json:"name"`
	Port       string   `json:"port"`
	Listen     string   `json:"listen"`
	Listens    []string `json:"listens"`
	TLS        bool     `json:"tls"`
	Source     string   `json:"source"`
	Enabled    bool     `json:"enabled"`
	CertStatus string   `json:"certStatus"`
	Error      string   `json:"error"`
}

// Cert 是 cert list 的一行（按证书文件归并）。
type Cert struct {
	Domain     string `json:"domain"`
	Subject    string `json:"subject"`
	Names      string `json:"names"`
	Issuer     string `json:"issuer"`
	NotAfter   string `json:"notAfter"`
	DaysLeft   int    `json:"daysLeft"`
	SelfSigned bool   `json:"selfSigned"`
	// Local 表示「本机自动生成的证书」：自签，或由本地 CA 签发。
	Local bool `json:"local"`
	// Status 同 cert list：valid / expiring / expired / not_yet_valid /
	// missing / unreadable / unknown。
	Status      string `json:"status"`
	AcmeManaged bool   `json:"acmeManaged"`
	// File 是证书文件路径；cert list 永不输出私钥。
	File string `json:"file"`
	Note string `json:"note"`
}

// SiteInput 是「添加 / 编辑站点」弹窗提交的字段，与 site add / site update
// 的 flag 一一对应：域名、监听、类型（root / proxy_pass）。证书路径、首页与
// 文件名由 CLI 的事务自行处理（fillCerts / 站点名即文件名），不进表单。
type SiteInput struct {
	// Id 是站点名（site update <name> 用）；添加时为空。
	Id        string `json:"id"`
	Name      string `json:"name"`
	Listen    string `json:"listen"`
	Type      string `json:"type"`
	Root      string `json:"root"`
	ProxyPass string `json:"proxyPass"`

	// 高级配置（主 location）：响应压缩、限流、Basic 认证、代理缓存。
	// 编辑模式才提交；添加站点时 site add 不支持这些字段，前端不会带。
	Zip           []string `json:"zip"`
	LimitRate     int      `json:"limitRate"`
	LimitBurst    int      `json:"limitBurst"`
	LimitMem      int      `json:"limitMem"`
	LimitNodelay  bool     `json:"limitNodelay"`
	AuthOn        bool     `json:"authOn"`
	AuthUser      string   `json:"authUser"`
	AuthPass      string   `json:"authPass"`
	AuthRealm     string   `json:"authRealm"`
	CachePath     string   `json:"cachePath"`
	CacheValid    []string `json:"cacheValid"`
	CacheMaxSize  int      `json:"cacheMaxSize"`
}

// SiteDetail 是详情弹窗一次拉取的内容：表单字段 + 只读信息 + 是否可编辑。
// 表单字段对应 site update 的白名单；Cert / CertKey / Index / Raw 只读展示，
// 来自 site show 的 server 块。
type SiteDetail struct {
	Id        string `json:"id"`
	Source    string `json:"source"`
	Path      string `json:"path"`
	Editable  bool   `json:"editable"`
	Note      string `json:"note"`
	Name      string `json:"name"`
	Listen    string `json:"listen"`
	Type      string `json:"type"`
	Root      string `json:"root"`
	ProxyPass string `json:"proxyPass"`
	Cert      string `json:"cert"`
	CertKey   string `json:"certKey"`
	Index     string `json:"index"`
	Raw       string `json:"raw"`

	// 高级配置，回填弹窗用。AuthPass 恒为脱敏值或空串：site show 已经
	// 把 pswd 抹成 ***，这里再原样透出，前端留空即表示不修改。
	Zip          []string `json:"zip"`
	LimitRate    int      `json:"limitRate"`
	LimitBurst   int      `json:"limitBurst"`
	LimitMem     int      `json:"limitMem"`
	LimitNodelay bool     `json:"limitNodelay"`
	AuthOn       bool     `json:"authOn"`
	AuthUser     string   `json:"authUser"`
	AuthPass     string   `json:"authPass"`
	AuthRealm    string   `json:"authRealm"`
	CachePath    string   `json:"cachePath"`
	CacheValid   []string `json:"cacheValid"`
	CacheMaxSize int      `json:"cacheMaxSize"`
}

type ConfigField struct {
	Path    string   `json:"path"`
	Label   string   `json:"label"`
	Group   string   `json:"group"`
	Type    string   `json:"type"` // text | number | bool | select | list | textarea | email | multiselect | pathlist
	Value   string   `json:"value"`
	Options []string `json:"options,omitempty"`
	Help    string   `json:"help,omitempty"`
	Missing bool     `json:"missing"`
	// Browse 非空表示这是路径字段，界面要给「浏览…」按钮：
	// "dir" 选目录、"file" 选文件。前端据此调用 PickPath。
	Browse string `json:"browse,omitempty"`
}

// ConfigForm 是设置页的表单。字段值来自 config show（脱敏），
// 表单 schema 是纯 UI 元数据，留在桌面端。
type ConfigForm struct {
	Path      string        `json:"path"`
	Fields    []ConfigField `json:"fields"`
	Size      int64         `json:"size"`
	Modified  string        `json:"modified"`
	Writable  bool          `json:"writable"`
	Preserved []string      `json:"preserved"`
}

// LogEntry 是 logs query 的一条记录（白名单字段，无原始行）。
// Raw 是该记录的紧凑 JSON，仅作为悬浮提示用，不是日志原文。
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

// LogQuery 对应 logs query 的白名单过滤条件。
type LogQuery struct {
	Kind       string `json:"kind"` // access | error | system | safe
	Limit      int    `json:"limit"`
	Level      string `json:"level"`
	Status     string `json:"status"` // 500 / 4xx / 400-499
	Host       string `json:"host"`   // --domain
	Method     string `json:"method"`
	PathPrefix string `json:"pathPrefix"` // --path-prefix
	Reason     string `json:"reason"`
	Since      string `json:"since"`  // 时长（1h）或 RFC3339
	Until      string `json:"until"`  // 筛选条件：只看此刻之前的记录
	Cursor     string `json:"cursor"` // 分页游标（上一页最早一条的时间）；有值时收窄 until
}

// LogPage 是日志页一页结果。logs query 没有 offset 分页：
// 「加载更早」把 NextUntil（本页最早一条的时间）当下一页的 --until 游标。
type LogPage struct {
	Lines     []LogEntry `json:"lines"`
	HasMore   bool       `json:"hasMore"`
	NextUntil string     `json:"nextUntil"`
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
