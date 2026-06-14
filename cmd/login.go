package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"flareduct/internal/cli"
	"flareduct/internal/cloudflare"
	"flareduct/internal/config"
	"flareduct/internal/paths"
)

func runLogin(args []string, globals cli.GlobalOptions, stdout, stderr io.Writer) error {
	opts := globals
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			printLoginHelp(stdout)
			return nil
		case a == "--config":
			if i+1 >= len(args) {
				return fmt.Errorf("--config needs a path")
			}
			i++
			opts.ConfigPath = args[i]
		case strings.HasPrefix(a, "--config="):
			opts.ConfigPath = strings.TrimPrefix(a, "--config=")
		case a == "--cloudflared":
			if i+1 >= len(args) {
				return fmt.Errorf("--cloudflared needs a path")
			}
			i++
			opts.Cloudflared = args[i]
		case strings.HasPrefix(a, "--cloudflared="):
			opts.Cloudflared = strings.TrimPrefix(a, "--cloudflared=")
		default:
			return fmt.Errorf("unknown login flag %q", a)
		}
	}

	cfg, _, _, err := config.LoadConfig(opts.ConfigPath)
	if err != nil {
		return err
	}
	cloudflared := cfg.Cloudflared
	if opts.Cloudflared != "" {
		cloudflared = opts.Cloudflared
	}
	if cloudflared == "" {
		cloudflared = "cloudflared"
	}
	cloudflared = paths.ExpandPath(cloudflared)

	if _, err := cloudflare.ResolveBinaryPath(cloudflared); err != nil {
		return err
	}

	fmt.Fprintln(stdout, "flareduct: running cloudflared tunnel login")
	cmd := exec.Command(cloudflared, "tunnel", "login")
	cmd.Stdin = os.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}
