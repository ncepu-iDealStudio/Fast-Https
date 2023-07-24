package main

import (
	_ "fast-https/config"
	"fast-https/internal/run"
)

// start taskBox in windows platform
func main() {

	run.StartWindows()

}
