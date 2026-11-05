# 配置证书与 ACME 挑战

这篇讲"怎么让 HTTPS 真的可用"：选哪种证书、ACME 怎么配、挑战请求到底怎么走、失败了看哪里。

先记住一句话：**证书本身从来不是瓶颈，卡住的通常是"站点没有 443 ssl"或"域名/端口的可达性"。**

## 1. 三条路，先选一条

| 场景 | 做法 | 需要什么 |
| --- | --- | --- |
| 内网、本机、测试 | 什么都不用配，首次 `start` 自动出自签 | 无 |
| 已有商业/云厂商证书 | 把文件放进 `/etc/fast-https/cert/`，站点指向它 | 证书 + 私钥 |
| 公网域名、要浏览器信任 | ACME（Let's Encrypt / ZeroSSL） | 域名解析正确 + 80 或 443 公网可达 |

## 2. 自签证书（默认）

首次 `start` 会用本地 CA 生成一套：

```text
/etc/fast-https/cert/root.key               CA 私钥（不对外）
/etc/fast-https/cert/localhost.pem          默认站点的证书
/etc/fast-https/cert/localhost-key.pem      默认站点的私钥
/usr/share/fast-https/httpdoc/root/root.crt CA 证书，浏览器可下载安装
```

浏览器会报"不受信任"，内网用没问题。装一次 `root.crt` 到系统/浏览器信任列表就能消掉警告。

## 3. 用自己的证书

```bash
sudo install -m 0644 fullchain.pem /etc/fast-https/cert/jfeng.asia.pem
sudo install -m 0600 privkey.pem   /etc/fast-https/cert/jfeng.asia-key.pem
```

站点里指向它（`./cert/...` 相对 `/etc/fast-https`）：

```json
"ssl_certificate": "./cert/jfeng.asia.pem",
"ssl_certificate_key": "./cert/jfeng.asia-key.pem"
```

`fast-https reload` 即可重新读取证书，不用重启。**续期后记得让新文件覆盖同一路径再 reload**，进程不会自己盯着文件变化。

## 4. ACME 签发

### 4.1 先做前置检查，不然一定白折腾

| 检查 | 命令 | 判据 |
| --- | --- | --- |
| 域名解析 | `dig +short test.jfeng.asia` | 返回这台机的公网 IP |
| 端口可达 | 外部访问 `http://域名/.well-known/acme-challenge/ping` | 有响应（404 也算通），不是超时/重置 |
| 80 是否被占 | `sudo ss -ltnp \| grep ':80 '` | 被 nginx 占了就按 5.1 转发 |
| 大陆服务器备案 | — | 未备案的 80/443 会被云厂商拦截，表现为超时 |
| 系统时间 | `timedatectl` | 偏差大会被 CA 拒绝 |
| 本地能通 | `curl -vk https://127.0.0.1/ -H 'Host: test.jfeng.asia'` | 先确认服务本身没问题 |

### 4.2 站点必须带 443 ssl

**ACME 只给 `listen` 里带 `ssl` 的 server 块申请证书。** 只监听 80 的站点不会触发任何申请——这是最常踩的坑。

```bash
fast-https site update jfeng.com --tls
```

只改监听与证书路径，`server_name`、`root` 和手写的其它 location 都不动。等价于手改：

```json
{
  "listen": [80, "443 ssl h2"],
  "server_name": "test.jfeng.asia",
  "ssl_certificate": "./cert/test.jfeng.asia.pem",
  "ssl_certificate_key": "./cert/test.jfeng.asia-key.pem"
}
```

> 包布局下 `site update` 会把证书路径写成绝对路径 `/etc/fast-https/cert/test.jfeng.asia.pem`；写成 `./cert/...` 也等价，相对路径按 `/etc/fast-https` 解析。

确认它进了申请列表：

```bash
fast-https acme show      # "将申请的域名" 里应有 test.jfeng.asia
```

### 4.3 写 `acme` 段

交互向导（推荐第一次用）：

```bash
fast-https acme
```

脚本/无人值守：

```bash
fast-https acme setup \
  --email you@example.com \
  --ca letsencrypt \
  --challenge http-01 \
  --non-interactive
```

手写等价片段：

```json
"acme": {
  "enabled": true,
  "email": "you@example.com",
  "challenge": ["http-01"],
  "renew_before_days": 30,
  "fallback_self_signed": true,
  "storage": "./cert/acme",
  "cas": [{ "name": "letsencrypt", "directory": "https://acme-v02.api.letsencrypt.org/directory" }]
}
```

`storage` 的相对路径按配置基址解析：包布局落在 `/etc/fast-https/cert/acme`，归档布局是 `./config/cert/acme`。**与二进制所在目录无关**，`acme show` 会打印实际路径。

### 4.4 重启，不是 reload

`acme` 段只在启动时读取，`reload` 不会重建 ACME 管理器：

```bash
fast-https stop && fast-https start
# 或 systemctl restart fast-https
```

### 4.5 确认真的在申请

```bash
sudo tail -f /var/log/fast-https/system.log | grep -i acme
```

| 日志 | 含义 |
| --- | --- |
| `acme: requesting certificate for test.jfeng.asia` | 开始申请，说明配置生效了 |
| `acme: certificate for test.jfeng.asia written to /etc/fast-https/cert/…pem` | 签发成功，已落盘并热替换 |
| `acme: certificate for … could not be obtained, keeping the current certificate` | 签发失败，回落自签，按 4.1 逐项查 |
| `acme: http-01 is enabled but no http.server listens on port 80` | 80 不在本进程手上，需要 5.1 的前置反代转发 |

再看证书是谁签的：

```bash
echo | openssl s_client -connect 127.0.0.1:443 -servername test.jfeng.asia 2>/dev/null \
  | openssl x509 -noout -issuer -subject -dates
```

`issuer` 出现 `Let's Encrypt` 才算成功；还是 `fast-https` 自签就是没换过来。

## 5. 挑战到底发生了什么

### http-01

1. 本进程向 CA 注册账户，提交订单
2. CA 返回 token，本进程把它放进**内存表**（不写文件）
3. CA 访问 `http://test.jfeng.asia/.well-known/acme-challenge/<token>`
4. 请求到达数据面后，在 location 匹配**之前**按路径查表，命中就返回 keyAuthorization
5. CA 校验通过 → 签发证书 → 写到 `ssl_certificate` 指定路径 → 热替换

三个推论：

- `/etc/fast-https/cert/acme` 下**不会有挑战文件**。那里只有账户私钥与订单缓存，**只有真正发起过申请才有内容**；刚装完是空的，这是正常的。
- 挑战路径不依赖站点根目录，不需要往 `/var/www` 写任何东西。
- 只有未知 token 才落到正常路由，所以你站点自己托管的 `/.well-known/acme-challenge/` 不会被劫持。

### tls-alpn-01

在 443 上用临时证书应答，ALPN 通告 `acme-tls/1`。要求外部能**直连 443**，中间有 TLS 终结设备（CDN、LB、WAF）时会失败。

### auto

`"challenge": ["auto"]` 同时启用两种，由客户端按成功率选。80/443 都可直连时用它最省心；有前置反代时建议显式写 `["http-01"]`。

### 5.1 前面有 nginx 占着 80

本进程必须让出 80，改用别的端口（例如 8080），站点 `listen` 同步改成 `[8080, "443 ssl h2"]`，再让 nginx 把挑战路径转过来：

```nginx
server {
    listen 80;
    server_name test.jfeng.asia;

    location ^~ /.well-known/acme-challenge/ {
        proxy_pass http://127.0.0.1:8080;    # fast-https 的 http 监听端口
        proxy_set_header Host $host;
    }

    location / {
        return 301 https://$host$request_uri;
    }
}
```

本进程的 http-01 应答挂在数据面前面，所以**任意 http 监听端口都能应答**，不必是 80——这正是启动时那条 "assuming a front proxy forwards ..." 告警描述的用法。

### 5.2 不等 ACME，先自测可达

在外部机器上：

```bash
curl -v http://test.jfeng.asia/.well-known/acme-challenge/ping
```

- 返回 404：请求到了本进程，链路 OK（token 不存在，本来就 404）
- 返回 502/503：反代没配对
- 超时/拒绝：DNS、安全组、备案、防火墙，别急着怀疑 ACME

## 6. 续期、回退与限流

- **续期**：后台任务在到期前 `renew_before_days` 天完成，成功后热替换，不用 reload 也不用重启。
- **回退**：`fallback_self_signed: true`（默认）时签发失败继续用自签，站点不会因为 CA 抽风而中断。签发稳定前不要关掉它。
- **联调**：先用 `--ca staging`（Let's Encrypt 测试环境）跑通，再切生产。测试环境签的证书浏览器不信任，属正常。
- **限流**：同一组域名在生产环境反复失败重试会触发 CA 限流。改配置后先 `fast-https acme show` 确认域名列表，再重启，避免带着错误配置猛撞。

## 7. 目录里会出现什么

```text
/etc/fast-https/cert/
  localhost.pem / localhost-key.pem      自签（默认站点）
  root.crt / root.key                    本地 CA
  test.jfeng.asia.pem                     ACME 签发后写入这里
  test.jfeng.asia-key.pem
  acme/                                   storage：账户私钥、订单与证书缓存
```

备份时把 `/etc/fast-https/cert` 整个打包即可：换了机器不用重新走一遍订单，续期也能接上。

## 8. 排错

| 症状 | 先看 | 去 |
| --- | --- | --- |
| `acme show` 的"将申请的域名"是 `(无)` | 站点没有带 ssl 的 listen | 4.2 |
| 日志里没有 `requesting certificate` | `acme.enabled` 或没重启（reload 不够） | 4.3、4.4 |
| 有申请但挑战失败 | 80/443 可达性、前置反代、备案 | 4.1、5.1 |
| 证书换了但浏览器还是旧的 | 看 `issuer`，确认请求打到了哪台机器 | 4.5 |
| `ssl_certificate not found` | 首次 start 前证书还不存在，属正常 | 先跑一次 `start` |

更细的启动、端口、日志排查见 [troubleshooting.md](troubleshooting.md)；证书模块的设计细节见 [../feature/3-tls-certs.md](../feature/3-tls-certs.md)。

## 9. 最短路径（TL;DR）

```bash
fast-https site update jfeng.com --tls                  # 1. 站点加 443 ssl
fast-https acme setup --email you@example.com --non-interactive   # 2. 写 acme 段
fast-https acme show                                    # 3. 域名进了列表？
fast-https stop && fast-https start                     # 4. 重启（不能 reload）
sudo tail -f /var/log/fast-https/system.log | grep -i acme        # 5. 等 requesting certificate
```
