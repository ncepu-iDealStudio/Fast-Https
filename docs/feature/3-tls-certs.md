# 3. TLS 与证书

状态：✅ 已实现

## 功能

多 server_name 的 SNI 证书分发、自签根 CA 与站点证书自动生成、reload 时证书热替换。

## 使用

```json
{
  "listen": "443 ssl h2",
  "server_name": "example.com",
  "ssl_certificate": "config/cert/example.com.pem",
  "ssl_certificate_key": "config/cert/example.com-key.pem"
}
```

证书文件不存在时自动生成（`config/cert/` 下按 server_name 命名）；根 CA 位于 `httpdoc/root/`。

## 实现方案（runtime/listener/certs.go、init/autocert.go）

- `listenSsl`：`net.Listen`（SO_REUSEPORT）+ `tls.NewListener`，`tls.Config.GetCertificate = certs.get`——按 ClientHello 的 SNI 动态选证书，一个端口可承载多域名。
- `certSet`：持有多份证书，`replace()` 在 reload 时整体换入——同名端口 reload 后证书更新无需重启（`tests/unit_test/.../reload_cert_test.go` 验证：证书缺失时监听保持打开）。
- 自签链：`init/autocert.go` 首次启动生成 3072 位 RSA 根 CA（证书 `httpdoc/root/root.crt` 供首页下载，私钥 `config/cert/root.key` 不放在站点根下），再为每个 ssl server_name 签发站点证书。
- ALPN：只有 `listen ... h2` 才通告 `h2`，否则纯 HTTP/1.1（防止客户端协商到 h2 却被 HTTP/1.1 解析器处理）。

## 测试

`runtime/listener/cert_reload_test.go`、`runtime/listener/alpn_test.go`、h2 E2E。

## ACME 自动签发（modules/certmanager）

配置 `acme.enabled`（手写 JSON，或跑 `fast-https acme` 向导写回）后，`cmd.afterListen` 在监听建立、自签证书就位之后启动管理器，按以下链路工作：

1. **编排在控制面**：`modules/certmanager` 用 `certmagic` 管证书生命周期（私钥生成、CSR、存储、失败退避重试、到期前续期），签发动作实现为 `certmagic.Issuer`（`caIssuer`），内部用 `acmez` 走 ACME 订单。
2. **挑战应答在数据面**：
   - `http-01`：`http01Solver.Present` 把 `token → keyAuthorization` 写进内存表；`core.Services.HandleEarlyRequest` 在 `runtime/events.EventHandler` 里、location 匹配之前命中 `/.well-known/acme-challenge/<token>` 并写 `200 text/plain`。
   - `tls-alpn-01`：`tlsalpn01Solver.Present` 用 `acmez.TLSALPN01ChallengeCert` 生成临时证书，通过 `listener.RegisterTLSALPNProvider` 注入；`certSet.get` 在握手时若客户端点名 `acme-tls/1` 先查挑战表，`listenSsl` 的 `NextProtos` 追加 `acme-tls/1`。
3. **落盘与热替换**：`cert_obtained` 事件里从 certmagic 存储读回 PEM，校验私钥配对，写临时文件后 `rename` 到该 server 块配置的 `ssl_certificate` / `ssl_certificate_key`，再调用 `listener.ReloadCerts()` 就地换证，监听 socket 不变。
4. **回落**：申请失败只记日志，自签证书继续服务；`CertInit` 只在文件缺失时生成，不会覆盖 ACME 写入的 PEM。

### 申请范围

只有 `listen` 里带 `ssl` 的 server 块才会被申请证书（`certmanager.Plan`）。只写 `"listen": 80` 的站点不会触发任何证书请求，`acme show` 的"将申请的域名"会是空的；IP、`localhost`、不含点的名字与通配符会被跳过并给出原因。配置写完后若"将申请的域名"为空，`fast-https acme` 会直接提示这一点并给出加 443 的写法。

运维侧的完整操作与排错见 [../help/tls-certificates.md](../help/tls-certificates.md)。

## 已知限制

- 证书路径在主配置中相对工作目录解析，include 文件中相对该文件或主配置（`pathExists` 多候选）。
- 无 OCSP stapling、无 session ticket 调优。
- ACME 只支持 `http-01` 与 `tls-alpn-01`；通配符需要 `dns-01`，未实现。`reload` 不重建 ACME 管理器，改 `acme` 段需重启。
