package main

<<<<<<< HEAD
func main() {
	//config.Process()
	// server.Daemon(0, 1)
	// fmt.Println(config.G_config.HttpServer[0].Path)
	//server.Run()
	//httpparse.Process_HttpParse()
=======
import (
	"fast-https/cmd"
	_ "fast-https/config"
	"fast-https/modules/cache"
)

func main() {
	cache.LoadAllStatic()
	cmd.Execute()

>>>>>>> 7d181f62054dbea9b80706977d341143f684f31b
}
