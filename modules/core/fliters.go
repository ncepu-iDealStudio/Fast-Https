package core

import (
	"fast-https/modules/core/listener"
	"fmt"
)

type FliterInterface interface {
	fliter1(*Event) bool
	fliter2(*Event) bool
	fliter3(*RRcircle) bool
	fliter4(listener.ListenCfg, *Event) bool
}

type ConnFliter struct {
	Name string
}

// server Bucket, BlackList
func (this *ConnFliter) fliter1(*Event) bool {
	fmt.Println(this.Name, "这是针对 建立连接的 fliter")
	return true
}

type ListenFliter struct {
	ConnFliter //继承
}

// tcp proxy, websocket
func (this *ListenFliter) fliter2(listener.Listener, *Event) bool {
	fmt.Println(this.Name, "这是针对 能够有效建立连接的事件的 fliter")
	return true
}

type HttpParseFliter struct {
	ConnFliter //继承
}

func (this *HttpParseFliter) fliter3(*RRcircle) bool {
	fmt.Println(this.Name, "这是针对 HTTP请求解析的 fliter")
	return true
}

type RequestFliter struct {
	ConnFliter //继承
}

func (this *RequestFliter) fliter4(listener.ListenCfg, *Event) bool {
	fmt.Println(this.Name, "这是针对 HTTP请求目的的 fliter")
	return true
}
