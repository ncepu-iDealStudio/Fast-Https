package response

const (
	HTTP_SPLIT = "\r\n"
	FAST_HTTPS = "Server: Fast-Https"
)

const (
	HTTP_OK           = "HTTP/1.1 200 OK"
	HTTP_NOTFOUND     = "HTTP/1.1 404 NOTFOUND"
	HTTP_SERVER_ERROR = "HTTP/1.1 500 SERVERERROR"
)

var Default_not_found []byte = []byte(HTTP_NOTFOUND + HTTP_SPLIT + HTTP_SPLIT + "[event_static:65]: Can't find this file")
