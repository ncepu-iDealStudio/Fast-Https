package main

import (
	"fast-https/config"
	_ "fast-https/config"
)

func main() {
	//service.TestService("0.0.0.0:5000")
	//rootcmd := cmd.RootCmd()
	//rootcmd.Execute()
	config.Init()
}
