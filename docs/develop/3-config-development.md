# 3. 配置项开发指引

以新增一个 location 级配置项 `example` 为例，走完从 schema 到验收的六步。配置文件是纯 JSON，解析用标准库 `encoding/json`（viper 已移除），**文件 schema 与运行时结构分离**。

## 1. 定义 schema 与运行时字段

`config/config.go`：

```go
// 文件 schema：jsonPath 里加字段（json tag 即配置键名）
type jsonPath struct {
    Example string `json:"example"`
    ...
}

// 运行时结构：Path 里加字段（与文件结构解耦，可转换语义）
type Path struct {
    Example string
    ...
}
```

命名约定：历史键名原样保留（`http.blaklist` 的拼写、`HeaderKey` 大小写）；`listen` 类"数字或字符串"用 `flexString`。

## 2. 转换

`convertPath` 里把 schema 字段映射进运行时结构；需要语义转换的（如 `type` 字符串 → 类型码、`zip` 数组 → 位标志）写成独立小函数并校验非法值。

## 3. 默认值

`SetDefault` 补默认值——零值不合法的字段必须在这里兜底（参考 `MaxBodySize`）。

## 4. 校验

`ValidateConfigFile` / `validateServer`：与解析共用同一份 schema，语义校验（取值范围、必填、互斥）在这里，错误消息带 `where` 前缀（`http.server[0]: ...`）。枚举值用 switch 白名单（参考 `waf.mode`），**拼写错误必须报错而不是静默忽略**。

## 5. 消费

运行时读取一律经 `config.Current()`（不可变快照），**不要缓存配置指针跨请求使用**——reload 后它就是旧快照（对 in-flight 请求这是正确语义，对新请求不是）。

## 6. 测试与文档

- 解析测试（schema → 运行时字段断言，参考 `error_page_test.go`）
- 校验测试（非法值错误消息，参考 `tests/unit_test/config/check_config_test.go`）
- 行为测试（消费端生效）
- `docs/feature/` 对应篇章的配置示例更新；若是新依赖，根 README 依赖表同步

## 检查清单

- [ ] schema json tag（含历史拼写核对）
- [ ] 转换与非法值处理
- [ ] SetDefault 默认值
- [ ] 校验器 + 错误消息
- [ ] reload 后生效（BuildWAF/Init 类钩子若需要）
- [ ] 解析/校验/行为三层测试
- [ ] feature 文档同步
