# 10. URL 重写与开发模式

状态：✅ 已实现

## 功能

`rewrite`：把 location 的请求 301 重定向到指定地址。`devmod`：开发模式专用的静态服务变体。

## 使用

```json
{ "url": "/old", "type": "rewrite", "rewrite": "https://new.example.com/new" }
```

## 实现方案（modules/rewrite/rewrite.go）

- `ReWriteEvent` 直接构造 301 响应：`Location` 头 + `Connection: close`，写出即关闭连接。
- `devmod`（`modules/dev_mod`）：DEVMOD 类型注册的处理变体，行为近似本地静态服务，配合 `dev` 命令使用。

## 测试

`tests/`（rewrite 包随整体回归覆盖）。

## 已知限制

- 仅支持固定目标地址的 301，不支持正则捕获组替换（nginx rewrite 语义的子集）。
