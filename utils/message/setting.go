package message

import (
	"context"
	"fast-https/utils"
	"fmt"
	"os"
	"runtime"
	"sync"

	"github.com/fatih/color"
	"github.com/fufuok/chanx"
)

var outputChan *chanx.UnboundedChanOf[message]
var msgMap map[string]func(...any) error
var rwMutex sync.RWMutex
var msgMapOnce sync.Once

func InitMsg(logRootPath string) {
	MessageFormat(logRootPath)
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
		var log = Glog.SystemLog
		rwMutex.RLock()
		log.Infoln("Fast-https server exit!")
		close(outputChan.In)
		return nil
	})
	AddMsgHandler("info", func(args ...any) error {
		var log = Glog.SystemLog
		log.Infoln(args)
		return nil
	})
	AddMsgHandler("err", func(args ...any) error {
		var log = Glog.ErrorLog
		log.Errorln(color.RedString("", args))
		return nil
	})
	AddMsgHandler("warn", func(args ...any) error {
		var log = Glog.SystemLog
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
	AddMsgHandler("access", func(args ...any) error {
		var log = Glog.AccessLog
		message := args[0].(map[string]interface{})
		log.WithField("host", message["host"]).Infoln(message["message"])
		return nil
	})

	AddMsgHandler("safe", func(args ...any) error {
		var log = Glog.SafeLog
		log.Warnln(args)
		return nil
	})
}
