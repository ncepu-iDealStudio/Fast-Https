package response

const (
	HTTP_SPLIT = "\r\n"
	FAST_HTTPS = "Server: Fast-Https"
)

const (
	HTTP_NOTFOUND     = "HTTP/1.1 404 NOTFOUND"
	HTTP_SERVER_ERROR = "HTTP/1.1 500 SERVERERROR"
)

var Default_not_found []byte = []byte(HTTP_NOTFOUND + HTTP_SPLIT + HTTP_SPLIT + "<h1>404 Not Found!</h1>")

var Default_server_error []byte = []byte(HTTP_NOTFOUND + HTTP_SPLIT + HTTP_SPLIT + "<h1>500 Server Error!</h1>")
