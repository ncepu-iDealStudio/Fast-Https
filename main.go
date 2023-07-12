package main

import "fast-https/modules/core/server"

func main() {
	server.Daemon(0, 1)

	server.Run()
}
