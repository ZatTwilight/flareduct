package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"flareduct/internal/cli"
	"flareduct/internal/cloudflare"
	"flareduct/internal/config"
	"flareduct/internal/paths"
)

func runDoctor(args []string, globals cli.GlobalOptions, stdout io.Writer) error {
	opts := globals
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			printDoctorHelp(stdout)
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
			return fmt.Errorf("unknown doctor flag %q", a)
		}
	}

	cfg, cfgPath, exists, err := config.LoadConfig(opts.ConfigPath)
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

	fmt.Fprintf(stdout, "config: %s", cfgPath)
	if !exists {
		fmt.Fprint(stdout, " (not created yet)")
	}
	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "state:  %s\n", paths.StateFilePath())
	fmt.Fprintf(stdout, "logs:   %s\n", paths.LogsDirPath())
	if cfg.Public.Domain != "" {
		fmt.Fprintf(stdout, "public: owned hostnames under %s\n", cfg.Public.Domain)
	} else {
		fmt.Fprintln(stdout, "public: trycloudflare.com quick tunnels")
	}
	originCert := os.Getenv("TUNNEL_ORIGIN_CERT")
	if originCert == "" {
		originCert = filepath.Join(paths.HomeDir(), ".cloudflared", "cert.pem")
	}
	if _, certErr := os.Stat(paths.ExpandPath(originCert)); certErr == nil {
		fmt.Fprintf(stdout, "login:  origin cert found at %s\n", paths.ExpandPath(originCert))
	} else {
		fmt.Fprintf(stdout, "login:  no origin cert found at %s (run cloudflared tunnel login for owned hostnames)\n", paths.ExpandPath(originCert))
	}
	fmt.Fprintf(stdout, "binary: %s\n", cloudflared)

	path, err := cloudflare.ResolveBinaryPath(cloudflared)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "found:  %s\n", path)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--version")
	out, err := cmd.CombinedOutput()
	if err == nil {
		fmt.Fprintf(stdout, "version: %s", string(out))
	}
	return nil
}
