# 17. 配置系统

状态：✅ 已实现

## 功能

JSON 主配置 + `http.include`（目录/文件）展开站点配置；启动校验、reload 校验；运行时以不可变快照分发。

## 使用

主配置 `config/fast-https.json`，include 见各功能文档；`start` 与 `reload` 都会先用同一套 schema 校验，失败即停。

## 实现方案（config/config.go）

- **解析**：标准库 `encoding/json`（2026-10 从 viper 迁出，间接依赖 -20）；注释与尾随逗号的预处理由 `infra/jsonc` 提供（等长掩码，`SyntaxError.Offset` 仍指向原文件）。文件 schema（`jsonConfig/jsonServer/jsonPath`）与运行时结构（`Fast_Https`）解耦，转换层处理语义（`proxy_pass` 去协议头、`type` 字符串 → 类型码、`zip` 数组 → 位标志）。`listen` 用 `flexString` 兼容数字/字符串，用 `flexList` 再兼容数组（`"listen": ["80", "443 ssl h2"]`，加载时展开成多个运行期 server 块）。历史键名（`http.blaklist` 拼写）原样保留。
- **快照**：`currentConfig/currentMIME` 两枚 `atomic.Pointer`，`LoadFile` 校验全部通过后整体 Store；读取端经 `config.Current()/MIME()`。reload 失败旧快照原样保留。
- **校验**：与解析共用同一份 schema（结构错误在 Unmarshal 即报错），语义校验（listen 端口、server_name、ssl 证书存在性、location 必填项）消息与历史兼容；include 走同一 `validateServer`。
- **默认值**：`SetDefault` 补 max_header/max_body/log_root；`ResolveRuntimePaths` 把 root/证书/日志/include 路径在监听前转绝对路径（后续工作目录变化不影响）。
- **reload 路径不 Fatal**：mime/include 出错返回错误保留旧配置（2026-10 修复）。
- **新增段落（1.5.0）**：顶层 `acme`（ACME 签发，`config/acme.go`）与 `security`（`host_check` 三态 + 安全响应头，`config/security.go`）；`location.allow_methods`（方法白名单，转换时统一大写）。三者的默认值都在 `SetDefault` 里补，校验在 `ValidateConfigFile` 里做，缺省时行为与 1.4.0 一致。
- **文本级写回**（`config/acme_write.go` + `infra/jsonc`）：`acme` 段用 `jsonc.PatchMember` 定位根对象成员做局部替换/追加，缩进由文件自身的排版风格决定，注释、字段顺序与其余键原样保留；文件落盘走 `infra/files.WriteAtomic`。`config/site.go` 提供站点文件（`conf.d/*.json`）的校验、合并视图与 include 目录解析。

## 测试

`config/`（reload/paths/error_page/reload_window -race）、`tests/unit_test/config`（check_config 校验消息、handler_ready）。

## 已知限制

- 无环境变量/命令行覆盖配置的通道（JSON 单一来源，属有意简化）。
- 校验器为"最小校验"：字段拼写错误（非 schema 字段）会被静默忽略。
