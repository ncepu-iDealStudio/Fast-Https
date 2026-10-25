// Package appfirewall 是应用层防火墙：基于 GuardianWAF 的分词评分检测引擎。
//
// 检测入口是 Allowed（每请求恰好一次，在 location 匹配与认证之后调用）。
// 引擎按配置快照构建，reload 时原子替换；未配置防火墙的容器不构建引擎，
// Allowed 直接放行（零开销默认）。
//
// guardianwaf 依赖被隔离在本包与 cmd 内（tests/arch 守卫），
// 替换检测引擎只需重写本包。
package appfirewall
