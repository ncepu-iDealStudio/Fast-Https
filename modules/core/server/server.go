package server

import (
	"fast-https/modules/core/events"
	"fast-https/modules/core/listener"
	"fast-https/output"
	"fast-https/utils/message"
	"net"
	"time"
)

// listen and serve one port
func serve_one_port(listener listener.ListenInfo) {
	for {
		conn, err := listener.Lfd.Accept()
		now := time.Now()
		conn.SetDeadline(now.Add(time.Second * 20))

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

// ScanPorts scan ports to check whether they've been used
func ScanPorts() error {
	ports := listener.Process_ports()
	for _, port := range ports {
		conn, err := net.Listen("tcp", "0.0.0.0:"+port)
		if err != nil {
			listener.Lisinfos = []listener.ListenInfo{}
			return err
		}
		conn.Close()
	}
	listener.Lisinfos = []listener.ListenInfo{}
	return nil
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
