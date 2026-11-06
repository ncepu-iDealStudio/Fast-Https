# 5. 路由与虚拟主机

状态：✅ 已实现

## 功能

按 `server_name`（域名）+ `location url`（路径正则）两级匹配，把请求分发到对应处理模块；未命中时回落默认端口配置。

## 使用

```json
{ "server_name": "example.com", "location": [ { "url": "/api", "type": "proxy", ... } ] }
```

## 实现方案（runtime/filters/filters.go、runtime/listener/listener.go）

- 启动/reload 时 `processListenData` 把每个 location 编译为 `ListenCfg`（含 `PathRe` 正则、`ID`——限流计数、error_page、auth、limit 等全部字段），`processHostMap` 按 `server_name:port` 建 HostMap。
- 请求匹配 `GFilterHostPath`：按 Host 取候选 location 列表，再选最具体的一条；命中后记录 `OriginPath`/`PathLocation`（location 前缀之后的部分，静态/代理据此拼真实路径）。Host 未命中 → 按 nginx 的 `default_server` 语义回落该端口**第一个 server 块**（`Listener.DefaultHosts()`），因此用 IP 或别名访问不会再整站 404。
- **location 优先级**（`matchLocation`）：不再是"按配置顺序取第一个命中"，否则 `location /`（静态）会把 `/api`（反代）永远挡在后面。现在按 nginx 前缀语义排序：起点命中的优于中途命中；同类里匹配更长的优先；完全相同才按配置顺序取先写的。
- **Host 归一化**（`hostCfgs`）：HostMap 的键是 `server_name:port`，而客户端访问非标准端口时不会把端口写进 Host（`ParseHost` 只给 80/443 补端口）。只按 Host 查会让 `example.com:8080` 上的站点全部落到 default_server，表现为"配了反代却进了别的站点的静态目录"；未命中时补一次 `host:port` 再查。
- 路由表在 `Listener` 内以 `atomic.Pointer` 快照发布（`Hosts()/Cfgs()`），reload 热更新走 `updateCfg` 整体替换。
- 301：location url 不以 `/` 结尾且 OriginPath 为空时补斜杠重定向（`HandelSlash`）；请求指向真实目录但缺结尾斜杠时，静态模块也会 301 到带斜杠的地址（见 [6-static.md](6-static.md)）。

## 测试

`runtime/filters`（match_test：最长前缀、起点优先、同长保序、Host 带端口归一化）、`tests/unit_test/runtime/listener`、`runtime/events`、`modules/static`（路由后缀处理）。

## 已知限制

- 正则为配置原样 `regexp.MustCompile`（未锚定）——语义是"包含匹配"，配置需自约；中途命中的规则优先级最低，只作兜底。
- 不支持 nginx 的精确匹配 `=`/前缀 `^~` 优先级语法：`url` 既当前缀又当正则，按"起点命中 + 最长匹配"排序，不做 `~`/`^~` 区分。
