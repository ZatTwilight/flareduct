package cmd

import (
	"fmt"
	"io"
	"strings"

	"flareduct/internal/registry"
)

func runDown(args []string, stdout, stderr io.Writer) error {
	all := false
	var key string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "-h", "--help":
			printDownHelp(stdout)
			return nil
		case "--all", "-a":
			all = true
		default:
			if strings.HasPrefix(a, "-") {
				return fmt.Errorf("unknown down flag %q", a)
			}
			if key != "" {
				return fmt.Errorf("too many arguments")
			}
			key = a
		}
	}
	if !all && key == "" {
		printDownHelp(stdout)
		return fmt.Errorf("missing name or pid")
	}
	return registry.Stop(key, all, stdout, stderr)
}
