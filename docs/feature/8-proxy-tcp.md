# 8. TCP 代理

状态：✅ 已实现

## 功能

四层透明转发：把一个端口的原始 TCP 连接双向搬运到上游地址。

## 使用

```json
{ "listen": "3306 tcp" }
```

（上游地址取该 location 的 `proxy_pass`。）

## 实现方案（modules/proxy_tcp/event_proxy_tcp.go）

- 连接接入后 `net.Dial` 上游，两个方向各一个搬运循环：客户端→上游、上游→客户端，10s 滚动 deadline，EOF/错误即双向收尾。
- 建连失败关闭客户端连接（2026-10 修复：此前失败后继续用 nil conn）。
- 不做任何协议解析，内容不经过 HTTP 管线（也不经过应用防火墙）。

## 测试

`modules/proxy_tcp/event_proxy_tcp_test.go`。

## 已知限制

- 无连接数/速率上限、无上游健康检查；缓冲 1KB/轮偏小（大吞吐场景 syscall 偏多）。
