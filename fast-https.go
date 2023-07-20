package main

import (
	_ "fast-https/config"
	"fast-https/modules/cache"
	"fast-https/modules/core/server"
)

func main() {
	//rootcmd := cmd.RootCmd()
	cache.LoadAllStatic()

	server.Run()
	//rootcmd.Execute()
}
