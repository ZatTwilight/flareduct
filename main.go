package main

import (
	"os"

	"flareduct/cmd"
)

var version = "dev"

func main() {
	os.Exit(cmd.Run(os.Args[1:], os.Stdout, os.Stderr))
}
