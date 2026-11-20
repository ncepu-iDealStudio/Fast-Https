# 常见配置示例

本页是可以直接照抄的配置片段，覆盖最常见的站点形态。完整字段说明见
[usage.md](usage.md)，路径级代理的细节见 [feature/7-proxy.md](feature/7-proxy.md)。

## 文件放在哪

- 主配置：`config/fast-https.json`，放全局段（`cache`、`proxy`、`tls`、`security`、`waf`）与内联 `http.server[]`。
- 站点文件：`config/conf.d/<名称>.json`，一个文件就是一个 server 块。推荐用站点文件，改一个站点不会碰其它站点。
- 文件支持 JSONC（`//` 注释、末尾逗号）。
- 改完不自动生效：`fast-https reload`，或桌面端保存时自动走配置事务。

## 1. 最小静态站点

`config/conf.d/site.json`：

```json
{
  "listen": 80,
  "server_name": "example.com",
  "location": [
    { "url": "/", "type": "local", "root": "D:\\www\\site", "index": ["index.html", "index.htm"] }
  ]
}
```

## 2. HTTPS 静态站点

```json
{
  "listen": [80, "443 ssl h2"],
  "server_name": "example.com",
  "ssl_certificate": "./cert/example.com.pem",
  "ssl_certificate_key": "./cert/example.com-key.pem",
  "location": [
    { "url": "/", "type": "local", "root": "D:\\www\\site" }
  ]
}
```

证书留空时首次启动会按域名生成自签证书；生产环境用 ACME 或自己的证书。

## 3. 整站反向代理

```json
{
  "listen": 80,
  "server_name": "app.example.com",
  "location": [
    { "url": "/", "type": "proxy", "proxy_pass": "http://127.0.0.1:8080" }
  ]
}
```

## 4. 静态站点 + `/api` 反向代理

这是最常见的前后端分离形态：`/` 读磁盘，`/api` 转发给后端。

```json
{
  "listen": 80,
  "server_name": "example.com",
  "location": [
    { "url": "/", "type": "local", "root": "D:\\www\\site" },
    { "url": "/api", "type": "proxy", "proxy_pass": "http://127.0.0.1:8080" }
  ]
}
```

请求 `/api/users` 转发到上游仍是 `/api/users`，后端路由需要带 `/api` 前缀。

## 5. 静态站点 + `/api` 反向代理（去掉前缀）

`proxy_pass` 带路径时，按 nginx 规则用该路径替换命中的 location 前缀：

```json
{ "url": "/api", "type": "proxy", "proxy_pass": "http://127.0.0.1:8080/" }
```

`/api/users` 转发成 `/users`。写 `http://127.0.0.1:8080/v1` 则转发成 `/v1/users`。

## 6. 多个代理前缀

一个站点可以有多条 location，按最长前缀匹配：

```json
{
  "listen": 80,
  "server_name": "example.com",
  "location": [
    { "url": "/", "type": "local", "root": "D:\\www\\site" },
    { "url": "/api", "type": "proxy", "proxy_pass": "http://127.0.0.1:8080" },
    { "url": "/ws", "type": "proxy", "proxy_pass": "http://127.0.0.1:8081" },
    { "url": "/static", "type": "local", "root": "D:\\www\\static" }
  ]
}
```

## 7. 多域名

`server_name` 用空格分隔，域名共享同一组 location 和证书：

```json
{
  "listen": "443 ssl h2",
  "server_name": "example.com www.example.com",
  "location": [
    { "url": "/", "type": "local", "root": "D:\\www\\site" }
  ]
}
```

## 8. 反向代理 + 缓存

主配置定义缓存区（目录、容量、有效期只定义一次）：

```json
{
  "cache": [
    { "name": "edge", "path": "./cache/edge", "valid": ["200:1h", "404:1m", "any:5m"], "max_size": 1073741824 }
  ]
}
```

站点里按名字引用：

```json
{
  "url": "/api", "type": "proxy", "proxy_pass": "http://127.0.0.1:8080", "proxy_cache": "edge"
}
```

缓存只对代理响应生效；静态站点直接读磁盘，不经过代理缓存。

## 9. 未知 SNI 的兜底证书

主配置：

```json
{
  "tls": {
    "default_certificate": {
      "certificate": "./cert/default.pem",
      "key": "./cert/default-key.pem"
    }
  }
}
```

客户端用未知域名握手时返回这张证书，而不是回落到第一张站点证书。

## 10. 压缩、限流与 Basic 认证

```json
{
  "url": "/",
  "type": "local",
  "root": "D:\\www\\site",
  "zip": ["br", "gzip"],
  "zip_min_length": 1024,
  "limit": { "rate": 100, "burst": 200, "nodelay": true },
  "auth": { "type": "basic", "user": "admin", "pswd": "change-me", "realm": "site" }
}
```

## 常用命令

```powershell
# 新建静态站点
fast-https site add --domain example.com --root D:\www\site --listen 80 --yes

# 给站点加 /api 反代（主 location 之外）
fast-https site location add example.com --url /api --type proxy --proxy-pass http://127.0.0.1:8080

# 查看站点全部 location
fast-https site location list example.com

# 改主 location 的根目录 / 上游
fast-https site update example.com --root D:\www\site-v2 --yes

# 重新加载配置
fast-https reload
```

桌面端对应位置：站点编辑弹窗 →「路径代理」用于额外路径；主 location 仍在基础信息里改。
