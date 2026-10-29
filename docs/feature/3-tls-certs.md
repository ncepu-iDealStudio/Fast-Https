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

## 已知限制

- 证书路径在主配置中相对工作目录解析，include 文件中相对该文件或主配置（`pathExists` 多候选）。
- 无 OCSP stapling、无 session ticket 调优。
