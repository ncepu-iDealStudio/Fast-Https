package main

import (
	"fast-https/cmd"
	_ "fast-https/config"
)

func main() {

	rootcmd := cmd.RootCmd()
	rootcmd.Execute()
	// config.Init()
}
