# 14. Basic 认证

状态：✅ 已实现

## 功能

location 级 HTTP Basic 认证：凭据错误回 401 + `WWW-Authenticate`。

## 使用

```json
"auth": { "type": "basic", "user": "admin", "pswd": "secret" }
```

## 实现方案（modules/auth/auth.go）

- 解析 `Authorization: Basic base64(user:pswd)`：字段数、scheme 大小写、base64 合法性、冒号分隔任何一步失败即 401——畸形头不再可能触发越界（2026-10 修复：此前 `result[1]`/`Split(":")[1]` 越界 panic 可被远程打死）。
- 密码取第一个冒号之后的全部内容（支持密码含冒号）。
- 凭据比较用 `crypto/subtle.ConstantTimeCompare` 常数时间，防时序侧信道。
- 未带凭据的请求回 401（`WWW-Authenticate: Basic realm="..."`，canonical 大小写 + Content-Length: 0）。
- 检查点在 location 匹配之后、handler 之前（`core.AuthRejected` 钩子）。

## 测试

随整体回归覆盖（畸形凭据路径在 P0 加固轮验证）。

## 已知限制

- 凭据明文存于配置文件；无 htpasswd 多用户支持。
- 401 响应无 body，未与 error_page 体系打通。
