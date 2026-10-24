// Package request 把 HTTP/1.1 请求（请求行、头部、正文）解析为 Request。
package request

type PathQuery map[string]string

func (pq *PathQuery) Del(key string) {
	delete(*pq, key)
}

func (pq *PathQuery) Set(key string, val string) {
	(*pq)[key] = val
}
