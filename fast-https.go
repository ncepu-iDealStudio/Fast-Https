package main

import (
	"fast-https/cmd"
	_ "fast-https/config"
	"fast-https/service"
)

func main() {
	service.TestService("0.0.0.0:5000")
	rootcmd := cmd.RootCmd()
	rootcmd.Execute()
}
