// Package view 把磁盘上的配置、站点与进程状态整理成 CLI 输出用的数据结构。
// 这里只读、无副作用，返回的数据已经脱敏；输出格式（文本或 JSON 封套）由 cmd 决定。
package view

import (
	"errors"
	"io/fs"
)

// ErrNotFound 表示请求的对象不存在（例如没有这个站点）。
// 读文件时的不存在直接用 fs.ErrNotExist，调用方两种都要认。
var ErrNotFound = errors.New("not found")

// ErrBadSection 表示 --section 取值不在白名单里。
var ErrBadSection = errors.New("unsupported section")

// ErrBadName 表示站点名不能安全地当文件名用。
var ErrBadName = errors.New("invalid site name")

// InvalidError 表示配置内容本身有问题（解析失败或校验不通过），Message 是人能读的原因。
type InvalidError struct{ Message string }

func (e *InvalidError) Error() string { return e.Message }

// IsNotFound 报告错误是否表示「不存在」。
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound) || errors.Is(err, fs.ErrNotExist)
}
