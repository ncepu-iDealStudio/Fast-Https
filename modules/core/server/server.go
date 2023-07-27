package server

import (
	"fast-https/modules/core/events"
	"fast-https/modules/core/listener"
	"fast-https/output"
	"fast-https/utils/message"
)

// listen and serve one port
func serve_one_port(listener listener.ListenInfo) {
	for {
		conn, err := listener.Lfd.Accept()

		each_event := events.Event{}
		each_event.Conn = conn
		each_event.Lis_info = listener
		each_event.Timer = nil

		if err != nil {
			message.PrintErr("Error accepting connection:", err)
			continue
		}
		go events.Handle_event(each_event)
	}
}

func Run() {
	output.PrintPortsListenerStart()
	// service.TestService("0.0.0.0:5000")
	listens := listener.Listen()
	for _, value := range listens {
		go serve_one_port(value)
	}
	select {}
}
