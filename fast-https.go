package main

import (
	"fast-https/cmd"
	_ "fast-https/config"
	"fast-https/internal/run"
	"runtime"
)

func main() {

	//rootcmd := cmd.RootCmd()
	//rootcmd.Execute()
	// config.Init()
	if runtime.GOOS == "windows" {
		run.StartWindows()
	} else {
		rootcmd := cmd.RootCmd()
		rootcmd.Execute()
	}

}
