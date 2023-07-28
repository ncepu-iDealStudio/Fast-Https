package response

import (
	"strconv"
	"time"
)

const (
	HTTP_SPLIT = "\r\n"
	FAST_HTTPS = "Server: Fast-Https"
)

const (
	HTTP_NOTFOUND     = "<h1 style='text-align:center;'>404 Not Found!</h1>"
	HTTP_SERVER_ERROR = "<h1 style='text-align:center;'>500 Server Error!</h1>"
)

func Default_not_found() []byte {
	res := Response_init()
	res.Set_first_line(404, "NOT FOUND")
	res.Set_header("Server", "Fast-Https")
	res.Set_header("Date", time.Now().String())

	res.Set_header("Content-Type", "text/html")
	res.Set_header("Content-Length", strconv.Itoa(len([]byte(HTTP_NOTFOUND))))
	res.Set_body([]byte(HTTP_NOTFOUND))
	return res.Generate_response()
}

func Default_server_error() []byte {
	res := Response_init()
	res.Set_first_line(500, "SERVER ERROR")
	res.Set_header("Server", "Fast-Https")
	res.Set_header("Date", time.Now().String())

	res.Set_header("Content-Type", "text/html")
	res.Set_header("Content-Length", strconv.Itoa(len([]byte(HTTP_SERVER_ERROR))))
	res.Set_body([]byte(HTTP_SERVER_ERROR))
	return res.Generate_response()
}
