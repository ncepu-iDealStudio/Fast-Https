package main

import (
	"fast-https/cmd"
	_ "fast-https/config"
)

func main() {
	//cache.LoadAllStatic()
	cmd.Execute()
}
