package color

import (
	"fmt"
)

type ColorCode string

const (
	black  ColorCode = "0;30m"
	red    ColorCode = "0;31m"
	green  ColorCode = "0;32m"
	brown  ColorCode = "0;33m"
	navy   ColorCode = "0;34m"
	purple ColorCode = "0;35m"
	cyan   ColorCode = "0;36m"
	gray   ColorCode = "0;37m"
	dim    ColorCode = "1;30m"
	orange ColorCode = "1;31m"
	lime   ColorCode = "1;32m"
	yellow ColorCode = "1;33m"
	blue   ColorCode = "1;34m"
	pink   ColorCode = "1;35m"
	aqua   ColorCode = "1;36m"
	white  ColorCode = "1;37m"
)

func colorize(row interface{}, color ColorCode) string {
	return fmt.Sprintf("\033[%s%v\033[0m", color, row)
}

func Black(row interface{}) string {
	return colorize(row, black)
}

func Red(row interface{}) string {
	return colorize(row, red)
}

func Green(row interface{}) string {
	return colorize(row, green)
}

func Brown(row interface{}) string {
	return colorize(row, brown)
}

func Navy(row interface{}) string {
	return colorize(row, navy)
}

func Purple(row interface{}) string {
	return colorize(row, purple)
}

func Cyan(row interface{}) string {
	return colorize(row, cyan)
}

func Gray(row interface{}) string {
	return colorize(row, gray)
}

func Dim(row interface{}) string {
	return colorize(row, dim)
}

func Orange(row interface{}) string {
	return colorize(row, orange)
}

func Lime(row interface{}) string {
	return colorize(row, lime)
}

func Yellow(row interface{}) string {
	return colorize(row, yellow)
}

func Blue(row interface{}) string {
	return colorize(row, blue)
}

func Pink(row interface{}) string {
	return colorize(row, pink)
}

func Aqua(row interface{}) string {
	return colorize(row, aqua)
}

func White(row interface{}) string {
	return colorize(row, white)
}
