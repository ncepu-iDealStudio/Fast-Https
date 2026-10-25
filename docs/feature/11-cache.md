# 11. 代理响应缓存

状态：✅ 已实现

## 功能

把上游响应按 `proxy_cache.key` 落盘持久化并内存加速，命中直接回放，不回源。

## 使用

```json
"proxy_cache": {
  "path": "./cache",
  "key": "$request_uri",
  "valid": ["200:1h", "404:1m", "any:5m"],
  "methods": ["GET", "HEAD"],
  "max_size": 1073741824
}
```

## 实现方案（modules/cache/、modules/proxy/cache.go）

- **键**：`proxy_cache.key` 变量插值后取 MD5（`GetMd5`）；按 MD5 尾部两位分目录存储，避免单目录文件爆炸。
- **写入**：请求协程把条目投进 1000 深的异步队列（满则丢弃，不阻塞请求），落盘协程写文件成功后才建索引（防止半成品被读路径当成有效条目）。
- **索引**：`CacheIndex`——`map[string]*CacheNode`（键 → 过期时间/落盘路径），RWMutex 全程持锁；"读即续期"（命中顺延一个 valid 周期）。
- **热层**（2026-10）：落盘成功同步进内存副本，命中零 IO；双上限（4096 条/64MB）LRU 淘汰，过期清理/索引失效同步摘除热副本。
- **过期**：清扫协程每秒全量扫描，过期条目摘索引 + 删盘 + 释放记账。
- **回放**：剔除逐跳头与 `Set-Cookie`/`Date`/`Content-Length`，回放时重写 Date 与实际长度；默认仅 GET/HEAD，`no-cache/no-store` 尊重。
- **容量**：`max_size` 为软上限，超限丢弃新条目并告警（空间由过期回收）。

## 测试

`modules/cache/`：cache_test（写入回放/续期/过期/max_size/磁盘一致性）、hot_lru_test（热层/LRU/字节上限/过期同步）。

## 已知限制

- 软上限无 LRU 淘汰磁盘条目（只有热层是 LRU），磁盘满依赖过期回收。
- 每秒全量扫描在数十万条目级会明显占 CPU（热层只加速读，不减少扫描）。
