package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"flareduct/internal/cli"
	"flareduct/internal/config"
	"flareduct/internal/paths"
)

func runConfig(args []string, globals cli.GlobalOptions, stdout io.Writer) error {
	if len(args) == 0 || hasHelp(args) {
		printConfigHelp(stdout)
		return nil
	}
	subcmd := args[0]
	rest := args[1:]
	switch subcmd {
	case "path":
		path := globals.ConfigPath
		if path == "" {
			path = paths.DefaultConfigPath()
		}
		fmt.Fprintln(stdout, paths.ExpandPath(path))
		return nil
	case "init":
		force := false
		path := globals.ConfigPath
		for i := 0; i < len(rest); i++ {
			a := rest[i]
			switch {
			case a == "--force" || a == "-f":
				force = true
			case a == "--config":
				if i+1 >= len(rest) {
					return fmt.Errorf("--config needs a path")
				}
				i++
				path = rest[i]
			case strings.HasPrefix(a, "--config="):
				path = strings.TrimPrefix(a, "--config=")
			default:
				return fmt.Errorf("unknown config init flag %q", a)
			}
		}
		if path == "" {
			path = paths.DefaultConfigPath()
		}
		path = paths.ExpandPath(path)
		if !force {
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf("%s already exists (use --force to overwrite)", path)
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		if err := os.MkdirAll(filepathDir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(config.SampleConfig()), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "wrote %s\n", path)
		return nil
	case "show":
		cfg, _, _, err := config.LoadConfig(globals.ConfigPath)
		if err != nil {
			return err
		}
		data, err := config.ConfigToYAML(cfg)
		if err != nil {
			return err
		}
		_, err = stdout.Write(data)
		return err
	default:
		return fmt.Errorf("unknown config subcommand %q", subcmd)
	}
}

func filepathDir(path string) string {
	idx := strings.LastIndexAny(path, `/\\`)
	if idx < 0 {
		return "."
	}
	if idx == 0 {
		return path[:1]
	}
	return path[:idx]
}
