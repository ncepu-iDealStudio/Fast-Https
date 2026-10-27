package logging

// TimeFormat 是日志时间字段的编码格式。
//
// 采用 RFC3339 带毫秒与时区：字典序等于时间序，可以直接做字符串范围比较，
// 不必在查询侧再做时间解析。
const TimeFormat = "2006-01-02T15:04:05.000Z07:00"

const (
	// DefaultLogRoot 是未指定 log_root 时相对于工作目录的日志目录。
	DefaultLogRoot = "logs"

	// 四个日志文件的固定名字。
	SystemLogName = "system.log"
	AccessLogName = "access.log"
	ErrorLogName  = "error.log"
	SafeLogName   = "safe.log"
)

// kind 字段的取值。合并多个日志文件读取时，用它区分来源，无需依赖文件名。
const (
	KindConsole = "console"
	KindSystem  = "system"
	KindAccess  = "access"
	KindError   = "error"
	KindSafe    = "safe"
)

// 以下是访问日志的字段名契约。
//
// 日志查询与智能体过滤依赖这些名字，改名属于破坏性变更；
// 值为数字的字段写成 JSON number，比较状态码区间时不需要类型转换。
const (
	FieldKind       = "kind"
	FieldRemoteAddr = "remote_addr"
	FieldType       = "type"
	FieldMethod     = "method"
	FieldPath       = "path"
	FieldHost       = "host"
	FieldStatus     = "status"
	FieldSize       = "size"
	FieldUserAgent  = "user_agent"
	// FieldReason 说明安全日志里拒绝的具体原因。
	FieldReason = "reason"
)
