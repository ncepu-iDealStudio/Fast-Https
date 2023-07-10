// coding: utf-8
// @Author : lryself
// @Date : 2022/3/22 22:04
// @Software: GoLand

package errHelper

import (
	"IPFS-Blog-Hugo/utils/message"
)

func Error(err error) {
	if err != nil {
		message.PrintErr(err)
	}
}

func ErrExit(err error) {
	if err != nil {
		message.PrintErr(err)
		message.Exit()
	}
}

func ErrorFunc(err error, handler func(error)) {
	if err != nil {
		handler(err)
	}
}
