package cmd

import (
	"io"

	"flareduct/internal/registry"
)

func runList(args []string, stdout io.Writer) error {
	if hasHelp(args) {
		printListHelp(stdout)
		return nil
	}
	return registry.List(stdout)
}
