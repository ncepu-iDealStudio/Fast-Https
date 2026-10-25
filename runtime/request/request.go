// Package request 把 HTTP/1.1 请求（请求行、头部、正文）解析为 Request。
package request

// PathQuery 是 URL query 参数集（键值均为原始未解码形态）。
// 历史上的 Del/Set 辅助方法无调用方，已删除；直接用 map 操作。
type PathQuery map[string]string
