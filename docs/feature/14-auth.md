# 14. Basic 认证

状态：✅ 已实现

## 功能

location 级 HTTP Basic 认证：凭据错误回 401 + `WWW-Authenticate`。定位是"给目录加把锁"的轻量保护，不是身份系统。

## 使用

```json
"auth": { "type": "basic", "user": "admin", "pswd": "secret", "realm": "Private area" }
```

- `type` 只接受 `basic`（大小写不敏感），写别的值 `check` 直接报错。
- 写了 `type` 就必须同时写 `user` 与 `pswd`，缺任何一项 `check` 报错。
- `realm` 可选，写进 401 响应的 `WWW-Authenticate`；留空时用站点名（`server_name:port`），再兜底成 `Fast-Https`。

## 实现方案（modules/auth/auth.go）

- 解析 `Authorization: Basic base64(user:pswd)`：字段数、scheme 大小写、base64 合法性、冒号分隔任何一步失败即 401——畸形头不再可能触发越界（2026-10 修复：此前 `result[1]`/`Split(":")[1]` 越界 panic 可被远程打死）。
- 密码取第一个冒号之后的全部内容（支持密码含冒号）。
- 凭据比较用 `crypto/subtle.ConstantTimeCompare` 常数时间，防时序侧信道。
- 配置里 `user` 或 `pswd` 为空时一律 401（2026-11 修复：此前只写 `type` 时 `Authorization: Basic Og==`，即空用户名口令，可以直接通过）。校验层同时在 `validateServer` 里拒绝这种配置。
- 未带凭据的请求回 401（`WWW-Authenticate: Basic realm="..."`，canonical 大小写 + Content-Length: 0）。
- 检查点在 location 匹配与 `allow_methods` 之后、应用防火墙与 handler 之前（`core.AuthRejected` 钩子），每请求一次；未配置 `auth` 时首行早退，零开销。

## 测试

- `modules/auth/auth_test.go`：畸形头、正确/错误凭据、空凭据拒绝、realm 取值（配置 → 站点名 → 内置串）。
- `config/auth_validate_test.go`：`type` 取值校验、`user`/`pswd` 缺失、`realm` 含引号或换行。

## 已知限制

- 凭据明文存于配置文件；无 htpasswd 多用户支持，也无法按用户审计。
- 只支持 Basic（RFC 7617），没有 Bearer/JWT、会话、登出。
- 401 响应无 body，未与 error_page 体系打通。
- `proxy_pass` 场景应由上游做认证，这里再拦一层容易变成两套凭据。
