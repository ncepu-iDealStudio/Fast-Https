package main

import (
	"fast-https/config"
)

func main() {
	config.Process()
	// server.Daemon(0, 1)
	// fmt.Println(config.G_config.HttpServer[0].Path)
	// server.Run()
}
