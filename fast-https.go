package main

import (
	"fast-https/config"
	_ "fast-https/config"
)

func main() {

	//rootcmd := cmd.RootCmd()
	//rootcmd.Execute()
	config.Init()
}
