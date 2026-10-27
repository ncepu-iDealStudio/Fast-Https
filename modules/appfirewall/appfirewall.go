// Package appfirewall 是应用层防火墙：基于 GuardianWAF 的分词评分检测引擎。
//
// 文件职责：
//   - waf.go     引擎生命周期（构建/原子切换/释放）与唯一检测入口 Allowed
//   - mapping.go 配置 → 检测器选项的映射（appfirewall 规则名、waf 块）
//   - adapter.go core.Event → *http.Request 的合成适配
//
// 边界约束（tests/arch 守卫）：guardianwaf 只允许出现在本包与 cmd，
// core/runtime 不感知检测引擎的存在。
package appfirewall
