// Package color 提供终端输出使用的 ANSI 颜色辅助函数。
//
// 着色统一委托给 github.com/fatih/color：NO_COLOR 环境变量、TERM=dumb、
// 输出不是终端这三种情况都会自动降级为无色，避免把转义序列写进日志文件。
package color

import (
	"github.com/fatih/color"
)

// 常规色对应原 SGR 0;3Xm，加粗色对应 1;3Xm，保留原调色板语义。
var (
	black  = color.New(color.FgBlack)
	red    = color.New(color.FgRed)
	green  = color.New(color.FgGreen)
	brown  = color.New(color.FgYellow)
	navy   = color.New(color.FgBlue)
	purple = color.New(color.FgMagenta)
	cyan   = color.New(color.FgCyan)
	gray   = color.New(color.FgWhite)
	dim    = color.New(color.FgBlack, color.Bold)
	orange = color.New(color.FgRed, color.Bold)
	lime   = color.New(color.FgGreen, color.Bold)
	yellow = color.New(color.FgYellow, color.Bold)
	blue   = color.New(color.FgBlue, color.Bold)
	pink   = color.New(color.FgMagenta, color.Bold)
	aqua   = color.New(color.FgCyan, color.Bold)
	white  = color.New(color.FgWhite, color.Bold)
)

func colorize(c *color.Color, row interface{}) string {
	if s, ok := row.(string); ok {
		return c.Sprint(s)
	}
	return c.Sprint(row)
}

func Black(row interface{}) string {
	return colorize(black, row)
}

func Red(row interface{}) string {
	return colorize(red, row)
}

func Green(row interface{}) string {
	return colorize(green, row)
}

func Brown(row interface{}) string {
	return colorize(brown, row)
}

func Navy(row interface{}) string {
	return colorize(navy, row)
}

func Purple(row interface{}) string {
	return colorize(purple, row)
}

func Cyan(row interface{}) string {
	return colorize(cyan, row)
}

func Gray(row interface{}) string {
	return colorize(gray, row)
}

func Dim(row interface{}) string {
	return colorize(dim, row)
}

func Orange(row interface{}) string {
	return colorize(orange, row)
}

func Lime(row interface{}) string {
	return colorize(lime, row)
}

func Yellow(row interface{}) string {
	return colorize(yellow, row)
}

func Blue(row interface{}) string {
	return colorize(blue, row)
}

func Pink(row interface{}) string {
	return colorize(pink, row)
}

func Aqua(row interface{}) string {
	return colorize(aqua, row)
}

func White(row interface{}) string {
	return colorize(white, row)
}
