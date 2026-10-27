# 9. WebSocket 代理

状态：✅ 已实现

## 功能

把 WebSocket 升级连接反向代理到上游：握手经 HTTP 管线转发，升级成功后整条连接变成双向字节管道。

## 使用

客户端带 `Upgrade: websocket` 的请求会自动处理；location 为 proxy 类型即可，无需额外配置。

## 实现方案（modules/websocket/websocket.go）

- 升级请求先走普通代理路径（`ParseHost` 后 `GHttpParseFilter` 标记 `ev.Upgrade="websocket"`），握手响应由上游返回并透传给客户端。
- 升级成功后 `WebSocketHandler` 接管：`CircleData` 中持有的上游连接**整条会话独占**（不进上游连接池），两个搬运循环双向转发，10s 滚动 deadline。
- 断言失败/上游连接未初始化时关连接返回（2026-10 修复：此前断言失败继续用 nil 指针）。
- 类型断言链依赖 `getProxy` 的 CircleData 契约（`modules/proxy/event_proxy.go` getProxy 的 websocket 分支）。

## 测试

`modules/websocket/websocket_test.go`。

## 已知限制

- 不解析 WebSocket 帧（纯字节搬运），帧级校验/压缩扩展交由两端。
- 1KB 缓冲、10s deadline 轮询对高吞吐场景偏保守。
