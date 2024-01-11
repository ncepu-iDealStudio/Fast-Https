package main

import (
	"fast-https/cmd"
)

func main() {
	rootcmd := cmd.RootCmd()
	rootcmd.Execute()
}
