package main

import (
	_ "fast-https/config"
<<<<<<< HEAD
)

func main() {
	//cache.LoadAllStatic()
	//cmd.Execute()
=======
	"fast-https/modules/cache"
	"fast-https/modules/core/server"
	"fast-https/service"
)

func main() {
	cache.LoadAllStatic()
	// cmd.Execute()
>>>>>>> c8b8e285b6fdfb1c6cef94d87272af80da2448fd

	service.TestService("0.0.0.0:5000")
	server.Run()
	// select {}
}
