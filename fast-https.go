package main

import (
	"fast-https/cmd"
	"fast-https/utils/logger"
)

func init() {
	logger.Level(4)
}

func main() {
	rootcmd := cmd.RootCmd()
	rootcmd.Execute()
}
