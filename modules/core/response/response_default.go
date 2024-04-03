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
	HTTP_TEST         = "<h1 style='text-align:center;'>This is a test Page</h1>"
	HTTP_NOTFOUND     = "<h1 style='text-align:center;'>404 Not Found!</h1>"
	HTTP_SERVER_ERROR = "<h1 style='text-align:center;'>500 Server Error!</h1>"
	HTTP_TOO_MANY     = "<h1 style='text-align:center;'>403 too many!</h1>"
	HTTP_BLACK_BAN    = "<h1 style='text-align:center;'>403 Forbidden!</h1>"
)

func DefaultTest() *Response {
	res := ResponseInit()
	res.SetFirstLine(200, "OK")
	res.SetHeader("Server", "Fast-Https")
	res.SetHeader("Date", time.Now().String())

	res.SetHeader("Content-Type", "text/html")
	res.SetHeader("Content-Length", strconv.Itoa(len([]byte(HTTP_NOTFOUND))))
	res.SetBody([]byte(HTTP_NOTFOUND))
	return res
}

func DefaultNotFound() *Response {
	res := ResponseInit()
	res.SetFirstLine(404, "NOTFOUND")
	res.SetHeader("Server", "Fast-Https")
	res.SetHeader("Date", time.Now().String())

	res.SetHeader("Content-Type", "text/html")
	res.SetHeader("Content-Length", strconv.Itoa(len([]byte(HTTP_NOTFOUND))))
	res.SetBody([]byte(HTTP_NOTFOUND))
	return res
}

func DefaultTooMany() *Response {
	res := ResponseInit()
	res.SetFirstLine(403, "NOTTOOMANY")
	res.SetHeader("Server", "Fast-Https")
	res.SetHeader("Date", time.Now().String())

	res.SetHeader("Content-Type", "text/html")
	res.SetHeader("Content-Length", strconv.Itoa(len([]byte(HTTP_TOO_MANY))))
	res.SetBody([]byte(HTTP_TOO_MANY))
	return res
}

func DefaultServerError() *Response {
	res := ResponseInit()
	res.SetFirstLine(500, "SERVERERROR")
	res.SetHeader("Server", "Fast-Https")
	res.SetHeader("Date", time.Now().String())

	res.SetHeader("Content-Type", "text/html")
	res.SetHeader("Content-Length", strconv.Itoa(len([]byte(HTTP_SERVER_ERROR))))
	res.SetBody([]byte(HTTP_SERVER_ERROR))
	return res
}

func DefaultBlackBan() *Response {
	res := ResponseInit()
	res.SetFirstLine(403, "BAN")
	res.SetHeader("Server", "Fast-Https")
	res.SetHeader("Date", time.Now().String())

	res.SetHeader("Content-Type", "text/html")
	res.SetHeader("Content-Length", strconv.Itoa(len([]byte(HTTP_BLACK_BAN))))
	res.SetBody([]byte(HTTP_BLACK_BAN))
	return res
}
