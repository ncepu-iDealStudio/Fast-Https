// Package config 是配置的公开入口：运行期类型、快照与加载/校验函数。
//
// 实现按域放在 internal 下，本包只做类型别名与函数转发，外部调用点
// （runtime、modules、management、cmd…）看到的 API 与重构前一致：
//
//	internal/model    运行期类型、常量、编译期路径基址、配置快照
//	internal/schema   配置文件的 JSON 结构与 JSONC 解码
//	internal/source   http.include 展开与 MIME 表读取
//	internal/paths    相对路径解析、候选树约束
//	internal/acme     acme 段：默认值、校验、http-01 告警、写回
//	internal/loader   schema → 运行期转换、默认值、加载/重载入口
//	internal/validate check 与 reload 的完整校验（含站点文件）
//	internal/sitefile conf.d 站点文件的生成、解析、补丁与快照
//	internal/scan     监听端口与证书声明扫描
package config

import (
	"fast-https/config/internal/loader"
	"fast-https/config/internal/model"
)

// Current 返回当前生效的配置快照。尚未加载配置时是零值快照。
func Current() *Fast_Https { return model.Current() }

// MIME 返回当前生效的 MIME 表。表在发布后不再修改，读取方无需加锁。
func MIME() map[string]string { return model.MIME() }

// Store 替换配置快照。加载入口在校验全部通过后调用；
// 导出是为了测试能够保存与恢复快照。
func Store(cfg *Fast_Https, mime map[string]string) { model.Store(cfg, mime) }

// Init 加载编译期基址上的主配置文件。
func Init() error { return loader.Init() }

// LoadFile 读取一份配置与 MIME 表，然后整体替换配置快照。
// 任何一步失败都不改动现有快照，读取方不会看到清空或半成品。
func LoadFile(path string) error { return loader.LoadFile(path) }

// Reload 重新加载编译期基址上的主配置。
func Reload() error { return loader.Reload() }

// ReloadFile 先校验再加载，校验或加载失败都保留原快照。
func ReloadFile(path string) error { return loader.ReloadFile(path) }
