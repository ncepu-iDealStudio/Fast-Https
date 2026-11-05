# 常见问题排查

按"症状"找条目。已修复的历史缺陷记在 [dev-issues.md](dev-issues.md)。

## 装包阶段

### apt 提示"无法获得锁 /var/lib/dpkg/lock-frontend"

后台自动更新（`unattended-upgrades`）正在跑。**不要 Ctrl-C**——dpkg 中途被打断会留下不一致状态。

```bash
sudo systemctl stop unattended-upgrades
sudo fuser -v /var/lib/dpkg/lock-frontend   # 确认没有进程持有
```

等锁时 apt 会自己重试，通常几十秒就好，让它等。

### 卸载时报"无法执行 prerm：没有那个文件或目录"

文件其实在，问题是脚本内容坏了（CRLF 行尾，shebang 变成 `#!/bin/sh\r`）。详见 [dev-issues.md#001](dev-issues.md#001-crlf-行尾打进包导致卸载失败)。

快速解锁：

```bash
printf '#!/bin/sh\nexit 0\n' | sudo tee /var/lib/dpkg/info/fast-https.prerm >/dev/null
printf '#!/bin/sh\nexit 0\n' | sudo tee /var/lib/dpkg/info/fast-https.postrm >/dev/null
sudo chmod 755 /var/lib/dpkg/info/fast-https.prerm /var/lib/dpkg/info/fast-https.postrm
sudo dpkg --purge fast-https
```

### 卸载时提示"目录非空，因而不会删除该目录"

分两种情况：

- 用 `apt remove`：配置、证书、日志都保留，本来就是设计如此，重装后能接着用。想清干净就 `sudo apt purge fast-https`。
- 用 `apt purge` 后还有残留：说明目录里存在**非本包创建**的文件。典型是 `/usr/share/fast-https/httpdoc` 下你自己的网站内容，或 `/etc/fast-https` 里手工放的文件（dpkg 不删不认识的路径，`/etc/fast-https` 目录也只在空时才 `rmdir`）。确认不要了手工删：

```bash
sudo ls -la /etc/fast-https /var/log/fast-https /usr/share/fast-https/httpdoc
sudo rm -rf /etc/fast-https /var/log/fast-https /usr/share/fast-https
```

### 升级后配置"不见了"

配置没丢，只是换地方了。1.5.0 之前所有东西在 `/usr/share/fast-https/config/`，现在在 `/etc/fast-https/`。postinstall 会复制过去，旧文件保留：

```bash
ls -l /etc/fast-https /usr/share/fast-https/config
```

## 启动阶段

### 普通用户 `fast-https start` 报 permission denied

包布局下 `/etc/fast-https`、`/var/log/fast-https` 归 root。用 `sudo`，或者交给 systemd：

```bash
sudo systemctl enable --now fast-https
```

错误信息里会明确写出哪个路径写不了。

### `check` 报 `ssl_certificate not found`

默认配置引用了 `./cert/localhost.pem`，首次启动才会生成。先跑一次 `start`，之后 `check` 就过了。

### 服务起不来

```bash
systemctl status fast-https --no-pager
sudo journalctl -u fast-https -n 50 --no-pager
sudo ls -l /var/log/fast-https
sudo tail -n 50 /var/log/fast-https/system.log
```

常见原因：配置语法错（先 `sudo fast-https check`）、证书路径不存在、端口被占。

### 端口被占用

```bash
sudo ss -ltnp | grep -E ':(80|443)'
```

80/443 需要 root 或 `CAP_NET_BIND_SERVICE`。容器或非 root 场景改 `listen` 到高位端口，前面挂反代。

### 日志目录不存在或写不进去

```bash
sudo mkdir -p /var/log/fast-https
sudo chown root:root /var/log/fast-https
```

systemd 托管的服务会通过 `LogsDirectory=` 自动建；手工启动时 `mkdirLogDir` 也会补建，两者都失败才是权限问题。

### 日志把磁盘占满

先看轮转有没有在跑：

```bash
ls -l /var/log/fast-https                 # 有没有 .1 / .2.gz
sudo logrotate -d /etc/logrotate.d/fast-https   # 干跑，看它认不认这个日志目录
systemctl list-timers logrotate.timer --no-pager 2>/dev/null || ls /etc/cron.daily/logrotate
```

- 归档 / Docker 布局没有这条配置，日志就在 `./logs` 里无限长，需要自己接 logrotate 或定期清理
- 包布局下没有 `/etc/logrotate.d/fast-https`：重装一次包即可恢复（它是包内 conffile，被手工删掉后 dpkg 不会自动补回，重装才会）
- 有配置但没轮转：`logrotate` 没装（包依赖被 `--no-install-recommends` 之类绕过），或文件一直为空被 `notifempty` 跳过

确有需要就改 `/etc/logrotate.d/fast-https` 里的 `rotate`（保留份数）与 `maxsize`（单文件上限）。改动属于 conffile 本地修改，升级不会被覆盖。

## 证书阶段

### 配好了 acme，但一个证书都没申请

先看这两处：

```bash
fast-https acme show          # "将申请的域名"是 (无) 还是你的域名
sudo tail -n 50 /var/log/fast-https/system.log | grep acme
```

如果"将申请的域名"是 `(无)`，说明 ACME 根本没发出请求，不是签发失败。原因只有一个：
**只有 `listen` 里带 `ssl` 的 server 块才会被申请证书**。只写 `"listen": 80` 的站点不会触发申请；如果唯一带 ssl 的站点是默认的 `server_name: localhost`，它会被当作非公网域名跳过（`acme show` 会打印 `跳过 localhost：not a public domain name`）。

一条命令就能加 443（只改监听与证书路径，`server_name`、`root` 和手写的其它 location 都不动）：

```bash
fast-https site update <站点名> --tls
```

等价于手改 `conf.d/<站点名>.json`（`fast-https site show <站点名>` 先看现状）：

```json
{
  "listen": [80, "443 ssl h2"],
  "server_name": "test.jfeng.asia",
  "ssl_certificate": "./cert/test.jfeng.asia.pem",
  "ssl_certificate_key": "./cert/test.jfeng.asia-key.pem",
  "location": [{ "url": "/", "type": "local", "root": "/var/www/jfeng.com", "index": ["index.html"] }]
}
```

`./cert/...` 相对于 `/etc/fast-https`；文件不存在时首次 `start` 先出自签，ACME 签发成功后覆盖同一路径并热替换。改完：

```bash
fast-https check            # 应输出 config ok
fast-https stop && fast-https start   # acme 段只在启动时读取，reload 不够
fast-https acme show        # 现在应列出 test.jfeng.asia
```

`fallback_self_signed` 建议保持默认 `true`：签发失败时还能用自签证书继续服务，改完再收紧。

### ACME 签发失败，回落自签

看日志里的 `cert_obtained` 与失败原因：

- 域名解析没指向本机 → `http-01` 拿不到校验请求
- 80 端口被前面一层代理挡住且没转发 `/.well-known/acme-challenge` → 需要透传
- 想先联调，用 `acme.staging=true`，避免触发生产环境速率限制

找不到原因时先在测试域名上跑 staging，确认链路通再切生产。

### 证书换了但浏览器还是旧的

进程只在 `start` 与 `reload` 时读证书。覆盖 PEM 后执行：

```bash
sudo fast-https reload
```

## 构建阶段

### `goreleaser check` 报 `field depends not found`

nfpm 的依赖字段在 goreleaser 里叫 `dependencies`，不是 `depends`。改 `build/goreleaser.yaml` 对应行。

### 打出来的包装得上、卸载不掉

打包脚本是 CRLF。跑 `go test ./tests/packaging` 确认，然后在 `.gitattributes` 里确认规则覆盖到该文件，重新检出后重打。

### Windows 上 `go test ./...` 偶发 `Access is denied`

并行测试抢同一个可执行文件。用 `go test -p 1 ./...`。

## 还解决不了

收集这三样再提 issue：

```bash
fast-https config path --json
sudo fast-https check
sudo journalctl -u fast-https -n 100 --no-pager
```
