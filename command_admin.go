package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func doToken(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || hasHelp(args) {
		printTokenHelp(stdout)
		return nil
	}
	switch args[0] {
	case "set":
		if len(args) > 2 {
			return fmt.Errorf("usage: flareduct token set [TOKEN]")
		}
		token := ""
		var err error
		if len(args) == 2 {
			token = strings.TrimSpace(args[1])
			fmt.Fprintln(stderr, "flareduct: warning: passing tokens as arguments can leave them in shell history; prefer `flareduct token set` and paste when prompted")
		} else {
			token, err = ReadTokenInteractively(os.Stdin, stdout)
			if err != nil {
				return err
			}
		}
		path, err := SaveCloudflareAPIToken(token)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "saved Cloudflare API token to %s\n", path)
		fmt.Fprintln(stdout, "token file permissions set to 0600")
		return nil
	case "status":
		_, source, ok, err := LoadCloudflareAPIToken()
		if err != nil {
			return err
		}
		if ok {
			fmt.Fprintf(stdout, "Cloudflare API token configured from %s\n", source)
		} else {
			fmt.Fprintf(stdout, "No Cloudflare API token configured. Token file path: %s\n", source)
		}
		return nil
	case "path":
		fmt.Fprintln(stdout, DefaultTokenPath())
		return nil
	case "remove", "rm":
		path, err := RemoveCloudflareAPIToken()
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "removed %s\n", path)
		return nil
	default:
		return fmt.Errorf("unknown token subcommand %q", args[0])
	}
}

func doLogin(args []string, globals globalOptions, stdout, stderr io.Writer) error {
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

	cfg, _, _, err := LoadConfig(opts.ConfigPath)
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
	cloudflared = ExpandPath(cloudflared)

	fmt.Fprintln(stdout, "flareduct: running cloudflared tunnel login")
	cmd := exec.Command(cloudflared, "tunnel", "login")
	cmd.Stdin = os.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func doDoctor(args []string, globals globalOptions, stdout io.Writer) error {
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

	cfg, cfgPath, exists, err := LoadConfig(opts.ConfigPath)
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
	cloudflared = ExpandPath(cloudflared)

	fmt.Fprintf(stdout, "config: %s", cfgPath)
	if !exists {
		fmt.Fprint(stdout, " (not created yet)")
	}
	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "state:  %s\n", StateFilePath())
	fmt.Fprintf(stdout, "logs:   %s\n", LogsDirPath())
	if cfg.Public.Domain != "" {
		fmt.Fprintf(stdout, "public: owned hostnames under %s\n", cfg.Public.Domain)
	} else {
		fmt.Fprintln(stdout, "public: trycloudflare.com quick tunnels")
	}
	originCert := os.Getenv("TUNNEL_ORIGIN_CERT")
	if originCert == "" {
		originCert = filepath.Join(HomeDir(), ".cloudflared", "cert.pem")
	}
	if _, certErr := os.Stat(ExpandPath(originCert)); certErr == nil {
		fmt.Fprintf(stdout, "login:  origin cert found at %s\n", ExpandPath(originCert))
	} else {
		fmt.Fprintf(stdout, "login:  no origin cert found at %s (run cloudflared tunnel login for owned hostnames)\n", ExpandPath(originCert))
	}
	fmt.Fprintf(stdout, "binary: %s\n", cloudflared)

	path, err := exec.LookPath(cloudflared)
	if err != nil && strings.Contains(cloudflared, string(os.PathSeparator)) {
		if info, statErr := os.Stat(cloudflared); statErr == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			path = cloudflared
			err = nil
		}
	}
	if err != nil {
		return fmt.Errorf("cloudflared not found: %w", err)
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

func doConfig(args []string, globals globalOptions, stdout io.Writer) error {
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
			path = DefaultConfigPath()
		}
		fmt.Fprintln(stdout, ExpandPath(path))
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
			path = DefaultConfigPath()
		}
		path = ExpandPath(path)
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
		if err := os.WriteFile(path, []byte(sampleConfig), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "wrote %s\n", path)
		return nil
	case "show":
		cfg, _, _, err := LoadConfig(globals.ConfigPath)
		if err != nil {
			return err
		}
		data, err := ConfigToYAML(cfg)
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
