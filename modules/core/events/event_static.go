package events

var data = "HTTP/1.1 200 OK\r\nConnection: keep-alive\r\n\r\nHello World"

func StaticEvent() string {
	return data
}
