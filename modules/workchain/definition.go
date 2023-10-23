/**
* @Author:刘浩宇
* @Description：定义一条工作链上每个结点所需实现功能
* @File：definition.go
* @Version:1.0.0
* @Date:2023/10/22 16:51:08
 */

package workchain

import (
	"context"
	"math"
)

// Handler 定义了一条工作链上的每个工作结点的执行方法的定义
// 我们通过执行ctx.Next()方法，执行工作链上下一个工作节点的Handle()方法
type Handler interface {
	Handle(ctx *WorkChain)
}

const abortIndex = math.MaxInt8 >> 1

type WorkChain struct {
	context.Context
	index    int8
	handlers []Handler
}

func NewContext(c context.Context) *WorkChain {
	return &WorkChain{
		Context:  c,
		index:    -1,
		handlers: make([]Handler, 0),
	}
}

func (c *WorkChain) Use(h ...Handler) {
	c.handlers = append(c.handlers, h...)
}

func (c *WorkChain) Next() {
	c.index++
	for c.index < int8(len(c.handlers)) {
		c.handlers[c.index].Handle(c)
		c.index++
	}
}

func (c *WorkChain) Abort() {
	c.index = abortIndex
}

func (c *WorkChain) Restart() {
	c.index = -1
}
