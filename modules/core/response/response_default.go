package response

import (
	"fast-https/config"
	"fmt"
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

func DefaultResponseBody(title, message string) string {
	body := `<html>
<head><title>%s</title></head>
<body>
<center><h1>%s</h1></center>
<hr><center>fast-https/%s</center>
</body>
</html>`
	return fmt.Sprintf(body, title, message, config.CURRENT_VERSION)
}

func DefaultTest() *Response {
	res := ResponseInit()
	res.SetFirstLine(200, "OK")
	res.SetHeader("Server", "Fast-Https")
	res.SetHeader("Date", time.Now().String())

	res.SetHeader("Content-Type", "text/html")
	body := []byte(DefaultResponseBody("Welcome to Fast-Https!", "Welcome to Fast-Https!"))
	res.SetHeader("Content-Length", strconv.Itoa(len(body)))
	res.SetBody(body)
	return res
}

func DefaultNotFound() *Response {
	res := ResponseInit()
	res.SetFirstLine(404, "NOTFOUND")
	res.SetHeader("Server", "Fast-Https")
	res.SetHeader("Date", time.Now().String())

	res.SetHeader("Content-Type", "text/html")
	body := []byte(DefaultResponseBody("404 Not Found", "The requested URL was not found on this server."))
	res.SetHeader("Content-Length", strconv.Itoa(len(body)))
	res.SetBody(body)
	return res
}

func DefaultTooMany() *Response {
	res := ResponseInit()
	res.SetFirstLine(429, "TOOMANY")
	res.SetHeader("Server", "Fast-Https")
	res.SetHeader("Date", time.Now().String())

	res.SetHeader("Content-Type", "text/html")
	body := []byte(DefaultResponseBody("429 Too Many Requests", "The request was rejected because the client has sent too many requests in a short period of time."))
	res.SetHeader("Content-Length", strconv.Itoa(len(body)))
	res.SetBody(body)
	return res
}

func DefaultServerError() *Response {
	res := ResponseInit()
	res.SetFirstLine(500, "SERVERERROR")
	res.SetHeader("Server", "Fast-Https")
	res.SetHeader("Date", time.Now().String())

	res.SetHeader("Content-Type", "text/html")
	body := []byte(DefaultResponseBody("500 Server Error", "The server encountered an internal error and was unable to complete your request."))
	res.SetHeader("Content-Length", strconv.Itoa(len(body)))
	res.SetBody(body)
	return res
}

func DefaultServerHeaderError() *Response {
	res := ResponseInit()
	res.SetFirstLine(500, "SERVERHeaderERROR")
	res.SetHeader("Server", "Fast-Https")
	res.SetHeader("Date", time.Now().String())

	res.SetHeader("Content-Type", "text/html")
	body := []byte(DefaultResponseBody("500 Server Error", "The server encountered an proxy header error"))
	res.SetHeader("Content-Length", strconv.Itoa(len(body)))
	res.SetBody(body)
	return res
}

func DefaultBlackBan() *Response {
	res := ResponseInit()
	res.SetFirstLine(403, "FORBIDDEN")
	res.SetHeader("Server", "Fast-Https")
	res.SetHeader("Date", time.Now().String())

	res.SetHeader("Content-Type", "text/html")
	body := []byte(DefaultResponseBody("403 Forbidden", "The request was rejected because the client has been blacklisted."))
	res.SetHeader("Content-Length", strconv.Itoa(len(body)))
	res.SetBody(body)
	return res
}
