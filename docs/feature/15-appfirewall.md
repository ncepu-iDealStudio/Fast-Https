# 15. 应用防火墙

状态：🚧 部分实现（内部 sql/xss 规则已实现；GuardianWAF 引擎接入进行中）

## 功能

对经过 HTTP 管线的请求做应用层攻击检测：`sql`（SQL 注入）、`xss`（跨站脚本清洗检测），按 location 配置启用。

## 使用

```json
"appfirewall": ["sql", "xss"]
```

## 当前实现方案（modules/appfirewall/）

- **sql**（sql.go，2026-10-25 落地）：对 query 参数与请求 body 做 11 条高置信度注入特征的正则检测（union select、drop table、or 1=1、sleep()、information_schema 等），命中即拒绝；此前为恒放行的空壳。
- **xss**（xss.go）：按 Content-Type 分派处理（JSON/form-urlencoded/multipart/query），bluemonday 策略清洗后检测 `<script` 是否消失，被清洗即判定为攻击并拒绝；策略包级共享（每请求构建已被移除）。
- **拦截语义**：`Blocks()` 决定规则是否有拦截资格，`HandleAppFireWall` 依配置串行执行，任一规则拒绝即 403（`DefaultForbidden`）。
- **调用点**：`proxy.ProxyFilterHandler` 与 `static.HandelSlash` 各自调用（收敛到统一检查点的改造随 GuardianWAF 一并落地）。

## 测试

`modules/appfirewall/`：sql_test（注入样本拒绝/正常内容放行）、firewall_test（xss 拒绝与放行、sql 拦截语义）。

## 已知限制与演进

- 检测面窄（2 类），正则/清洗模型易绕过。
- **GuardianWAF 集成已立项**：11 类检测器 + 分词评分模型，方案见 [20-planned-guardianwaf.md](20-planned-guardianwaf.md) 与 `docs/guardianwaf-adoption.md`；本篇描述的内部实现将作为该方案的回退路径保留到阶段 3。
