package message

import (
	"context"
	"fast-https/utils"
	"fast-https/utils/loggers"
	"fmt"
	"github.com/fatih/color"
	"github.com/fufuok/chanx"
	"os"
	"runtime"
	"sync"
)

var outputChan *chanx.UnboundedChanOf[message]
var msgMap map[string]func(...any) error
var rwMutex sync.RWMutex
var msgMapOnce sync.Once

func InitMsg() {
	rwMutex.Lock()
	defer utils.GetWaitGroup().Done()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var err error
	outputChan = chanx.NewUnboundedChanOf[message](ctx, 10, 0)
	initMsgHandler()
	rwMutex.Unlock()
	for msg := range outputChan.Out {
		go func(m message) {
			defer func() {
				if err := recover(); err != nil { //产生了panic异常
					PrintRecover(err)
				}
			}()
			err = msgMap[m.Type](m.Context)
			if err != nil {
				PrintErr(color.RedString(err.Error()))
			}
		}(msg)
	}
	_, _ = fmt.Fprintf(os.Stdout, "\\033[1;37;40m%s\\033[0m\\n", "系统服务已结束")
	os.Exit(1)
}
func AddMsgHandler(msg string, f func(args ...any) error) {
	msgMapOnce.Do(func() {
		msgMap = map[string]func(...any) error{}
	})
	msgMap[msg] = f
}

func initMsgHandler() {
	AddMsgHandler("exit", func(args ...any) error {
		var log = loggers.GetLogger()
		rwMutex.Lock()
		log.Infoln("程序终止！")
		close(outputChan.In)
		return nil
	})
	AddMsgHandler("info", func(args ...any) error {
		var log = loggers.GetLogger()
		log.Infoln(args)
		return nil
	})
	AddMsgHandler("err", func(args ...any) error {
		var log = loggers.GetLogger()
		log.Errorln(color.RedString("", args))
		return nil
	})
	AddMsgHandler("warn", func(args ...any) error {
		var log = loggers.GetLogger()
		log.Warnln(args)
		return nil
	})
	AddMsgHandler("recover", func(errs ...any) error {
		for _, err := range errs {
			switch err.(type) {
			case runtime.Error: // 运行时错误
				PrintErr("runtime error:", err)
			default: // 非运行时错误
				PrintErr("error:", err)
			}
		}
		return nil
	})
}
