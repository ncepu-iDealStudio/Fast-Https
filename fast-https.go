package main

import (
	"fast-https/cmd"
	_ "fast-https/config"
	"fast-https/modules/cache"
)

func main() {
	rootcmd := cmd.NewRootCmd()
	cache.LoadAllStatic()
	rootcmd.Execute()
}
