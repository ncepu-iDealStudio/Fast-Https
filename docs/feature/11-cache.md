# 11. 代理响应缓存

状态：✅ 已实现

## 功能

把上游响应落盘持久化并内存加速，命中直接回放，不回源。缓存区（目录、容量、
有效期）在主配置 `cache` 段定义一次，站点 location 用 `proxy_cache` 引用区名。

## 使用

```json
// fast-https.json：缓存区可以定义多个；随包默认预置一个可共用的 default_cache
"cache": [
  { "name": "default_cache", "path": "./cache", "valid": ["200:1h", "404:1m", "any:5m"], "max_size": 1073741824 }
]
```

```json
// conf.d/blog.json：站点只写缓存区名
"location": [
  { "url": "/", "type": "proxy", "proxy_pass": "http://127.0.0.1:8080", "proxy_cache": "default_cache" }
]
```

`valid` 是 `状态码:时长`，`any` 匹配其余状态码。缓存键由程序生成
（`$host$request_uri`，带区名前缀），方法固定 GET/HEAD；站点不再配置目录、
键或方法。旧的对象写法（`"proxy_cache": {"path": ...}`）在解析期直接报错。

CLI：`fast-https cache list / add / update / delete`；`site add|update --cache-zone <区名>`。
桌面端：左侧「缓存区」页。

缓存区的增删改走配置事务（候选树校验、临时备份、原子切换），并与站点、配置
变更一样登记 operation 记录（`cache.add` / `cache.update` / `cache.delete`），
可在 `operation list` 与桌面端「操作记录」里查到；`list` 与 `--dry-run` 不产生记录。

## 实现方案（modules/cache/、modules/proxy/cache.go）

- **键**：`区名|$host$request_uri` 取 MD5（`GetMd5`）；按 MD5 尾部两位分目录存储，避免单目录文件爆炸。区名前缀保证两个区即使 URI 相同也不会互相命中。
- **写入**：请求协程把条目投进 1000 深的异步队列（满则丢弃，不阻塞请求），落盘协程写文件成功后才建索引（防止半成品被读路径当成有效条目）。
- **索引**：`CacheIndex`——`map[string]*CacheNode`（键 → 过期时间/落盘路径），RWMutex 全程持锁；"读即续期"（命中顺延一个 valid 周期）。
- **热层**（2026-10）：落盘成功同步进内存副本，命中零 IO；双上限（4096 条/64MB）LRU 淘汰，过期清理/索引失效同步摘除热副本。
- **过期**：清扫协程每秒全量扫描，过期条目摘索引 + 删盘 + 释放记账。
- **回放**：剔除逐跳头与 `Set-Cookie`/`Date`/`Content-Length`，回放时重写 Date 与实际长度；仅 GET/HEAD 可缓存，`no-cache/no-store` 尊重。
- **容量**：缓存区的 `max_size` 为软上限，超限丢弃新条目并告警（空间由过期回收）。
- **启动恢复**：遍历主配置 `cache` 段里的每个区目录，重建索引，顺手删掉已过期与旧格式条目。

## 测试

`modules/cache/`：cache_test（写入回放/续期/过期/max_size/磁盘一致性）、hot_lru_test（热层/LRU/字节上限/过期同步）、concurrent_test（并发写读清理无丢索引、同键覆盖写记账一致）、load_cache_test（按缓存区目录恢复）。
`modules/proxy/`：cache_key_test（区名 + `$host$request_uri` 键生成、区隔离、空区/空目录、Host 大小写与端口差异、稳定性）。

## 已知限制

- 软上限无 LRU 淘汰磁盘条目（只有热层是 LRU），磁盘满依赖过期回收。
- 每秒全量扫描在数十万条目级会明显占 CPU（热层只加速读，不减少扫描）。
