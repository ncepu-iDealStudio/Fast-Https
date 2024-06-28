package request

type PathQuery map[string]string

func (pq *PathQuery) Del(key string) {
	delete(*pq, key)
}

func (pq *PathQuery) Set(key string, val string) {
	(*pq)[key] = val
}
