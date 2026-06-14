package main

import (
	"os"

	"flareduct/cmd"
)

var version = "dev"

func main() {
	cmd.SetVersion(version)
	os.Exit(cmd.Run(os.Args[1:], os.Stdout, os.Stderr))
}
