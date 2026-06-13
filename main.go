package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

const version = "dev"

type globalOptions struct {
	ConfigPath  string
	Cloudflared string
	Help        bool
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	globals, rest, err := parseLeadingGlobals(args)
	if err != nil {
		fmt.Fprintf(stderr, "flareduct: %v\n", err)
		return 2
	}
	if globals.Help || len(rest) == 0 {
		printHelp(stdout)
		return 0
	}

	cmd := rest[0]
	cmdArgs := rest[1:]
	var runErr error
	switch cmd {
	case "up", "run":
		runErr = doUp(cmdArgs, globals, stdout, stderr)
	case "list", "ls", "ps":
		runErr = doList(cmdArgs, stdout)
	case "down", "stop":
		runErr = doDown(cmdArgs, stdout, stderr)
	case "logs", "log":
		runErr = doLogs(cmdArgs, stdout)
	case "login":
		runErr = doLogin(cmdArgs, globals, stdout, stderr)
	case "token":
		runErr = doToken(cmdArgs, stdout, stderr)
	case "doctor":
		runErr = doDoctor(cmdArgs, globals, stdout)
	case "config":
		runErr = doConfig(cmdArgs, globals, stdout)
	case "version", "--version", "-v":
		fmt.Fprintln(stdout, version)
		return 0
	case "help", "--help", "-h":
		printHelp(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "flareduct: unknown command %q\n\n", cmd)
		printHelp(stderr)
		return 2
	}
	if runErr != nil {
		fmt.Fprintf(stderr, "flareduct: %v\n", runErr)
		return 1
	}
	return 0
}

func parseLeadingGlobals(args []string) (globalOptions, []string, error) {
	var opts globalOptions
	for len(args) > 0 {
		a := args[0]
		switch {
		case a == "--":
			return opts, args[1:], nil
		case a == "-h" || a == "--help":
			opts.Help = true
			args = args[1:]
		case a == "--config":
			if len(args) < 2 {
				return opts, nil, fmt.Errorf("--config needs a path")
			}
			opts.ConfigPath = args[1]
			args = args[2:]
		case strings.HasPrefix(a, "--config="):
			opts.ConfigPath = strings.TrimPrefix(a, "--config=")
			args = args[1:]
		case a == "--cloudflared":
			if len(args) < 2 {
				return opts, nil, fmt.Errorf("--cloudflared needs a path")
			}
			opts.Cloudflared = args[1]
			args = args[2:]
		case strings.HasPrefix(a, "--cloudflared="):
			opts.Cloudflared = strings.TrimPrefix(a, "--cloudflared=")
			args = args[1:]
		default:
			return opts, args, nil
		}
	}
	return opts, args, nil
}

type upOptions struct {
	globalOptions
	Detach        bool
	Replace       bool
	Name          string
	Hostname      string
	Subdomain     string
	Domain        string
	TunnelName    string
	TryCloudflare bool
	OverwriteDNS  bool
	Keep          bool
	Verbose       bool
	RandomStyle   string
	Wait          time.Duration
	Help          bool
}

func doUp(args []string, globals globalOptions, stdout, stderr io.Writer) error {
	opts, target, err := parseUpArgs(args, globals)
	if err != nil {
		return err
	}
	if opts.Help {
		printUpHelp(stdout)
		return nil
	}
	if target == "" {
		printUpHelp(stderr)
		return fmt.Errorf("missing target")
	}

	cfg, _, _, err := LoadConfig(opts.ConfigPath)
	if err != nil {
		return err
	}
	resolveOpts := ResolveOptions{
		Cloudflared:   opts.Cloudflared,
		Hostname:      opts.Hostname,
		Subdomain:     opts.Subdomain,
		Domain:        opts.Domain,
		TunnelName:    opts.TunnelName,
		TryCloudflare: opts.TryCloudflare,
		OverwriteDNS:  opts.OverwriteDNS,
		Keep:          opts.Keep,
		RandomStyle:   opts.RandomStyle,
	}

	var staticServer *StaticServer
	var spec TunnelSpec
	if _, isAlias := cfg.Services[target]; !isAlias {
		staticServer, err = StartStaticServerIfPath(target)
		if err != nil {
			return err
		}
	}
	if staticServer != nil {
		if opts.Detach {
			_ = staticServer.Close()
			return fmt.Errorf("--detach is not supported for static file targets yet")
		}
		defer staticServer.Close()
		fmt.Fprintf(stdout, "flareduct: serving %s %s at %s\n", staticServer.Kind, staticServer.Path, staticServer.URL)
		spec, err = ResolveQuickTarget(staticServer.Name, target, staticServer.URL, cfg, resolveOpts)
	} else {
		spec, err = ResolveTargetWithOptions(target, cfg, resolveOpts)
	}
	if err != nil {
		return err
	}
	if opts.Name != "" {
		spec.Name = SanitizeName(opts.Name)
	}
	spec.Verbose = opts.Verbose

	if opts.Detach {
		return StartDetached(spec, DetachOptions{Name: spec.Name, Replace: opts.Replace, Wait: opts.Wait}, stdout, stderr)
	}
	return RunForeground(spec, stdout, stderr)
}

func parseUpArgs(args []string, globals globalOptions) (upOptions, string, error) {
	opts := upOptions{globalOptions: globals, Wait: 20 * time.Second}
	var target string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			opts.Help = true
		case a == "-d" || a == "--detach":
			opts.Detach = true
		case a == "--replace":
			opts.Replace = true
		case a == "--trycloudflare":
			opts.TryCloudflare = true
		case a == "--verbose":
			opts.Verbose = true
		case a == "--overwrite-dns":
			opts.OverwriteDNS = true
		case a == "--keep":
			opts.Keep = true
		case a == "--hostname":
			if i+1 >= len(args) {
				return opts, target, fmt.Errorf("--hostname needs a value")
			}
			i++
			opts.Hostname = args[i]
		case strings.HasPrefix(a, "--hostname="):
			opts.Hostname = strings.TrimPrefix(a, "--hostname=")
		case a == "--subdomain":
			if i+1 >= len(args) {
				return opts, target, fmt.Errorf("--subdomain needs a value")
			}
			i++
			opts.Subdomain = args[i]
		case strings.HasPrefix(a, "--subdomain="):
			opts.Subdomain = strings.TrimPrefix(a, "--subdomain=")
		case a == "--domain":
			if i+1 >= len(args) {
				return opts, target, fmt.Errorf("--domain needs a value")
			}
			i++
			opts.Domain = args[i]
		case strings.HasPrefix(a, "--domain="):
			opts.Domain = strings.TrimPrefix(a, "--domain=")
		case a == "--tunnel-name":
			if i+1 >= len(args) {
				return opts, target, fmt.Errorf("--tunnel-name needs a value")
			}
			i++
			opts.TunnelName = args[i]
		case strings.HasPrefix(a, "--tunnel-name="):
			opts.TunnelName = strings.TrimPrefix(a, "--tunnel-name=")
		case a == "--random-style":
			if i+1 >= len(args) {
				return opts, target, fmt.Errorf("--random-style needs a value")
			}
			i++
			opts.RandomStyle = args[i]
		case strings.HasPrefix(a, "--random-style="):
			opts.RandomStyle = strings.TrimPrefix(a, "--random-style=")
		case a == "--name":
			if i+1 >= len(args) {
				return opts, target, fmt.Errorf("--name needs a value")
			}
			i++
			opts.Name = args[i]
		case strings.HasPrefix(a, "--name="):
			opts.Name = strings.TrimPrefix(a, "--name=")
		case a == "--wait":
			if i+1 >= len(args) {
				return opts, target, fmt.Errorf("--wait needs a duration")
			}
			i++
			d, err := time.ParseDuration(args[i])
			if err != nil {
				return opts, target, err
			}
			opts.Wait = d
		case strings.HasPrefix(a, "--wait="):
			d, err := time.ParseDuration(strings.TrimPrefix(a, "--wait="))
			if err != nil {
				return opts, target, err
			}
			opts.Wait = d
		case a == "--config":
			if i+1 >= len(args) {
				return opts, target, fmt.Errorf("--config needs a path")
			}
			i++
			opts.ConfigPath = args[i]
		case strings.HasPrefix(a, "--config="):
			opts.ConfigPath = strings.TrimPrefix(a, "--config=")
		case a == "--cloudflared":
			if i+1 >= len(args) {
				return opts, target, fmt.Errorf("--cloudflared needs a path")
			}
			i++
			opts.Cloudflared = args[i]
		case strings.HasPrefix(a, "--cloudflared="):
			opts.Cloudflared = strings.TrimPrefix(a, "--cloudflared=")
		case strings.HasPrefix(a, "-"):
			return opts, target, fmt.Errorf("unknown up flag %q", a)
		default:
			if target != "" {
				return opts, target, fmt.Errorf("too many targets: %q and %q", target, a)
			}
			target = a
		}
	}
	return opts, target, nil
}

func doList(args []string, stdout io.Writer) error {
	if hasHelp(args) {
		printListHelp(stdout)
		return nil
	}
	state, err := LoadState()
	if err != nil {
		return err
	}
	if len(state.Entries) == 0 {
		fmt.Fprintln(stdout, "No detached tunnels.")
		return nil
	}

	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tPID\tSTATUS\tKIND\tTARGET\tPUBLIC URL\tLOG")
	for _, entry := range state.Entries {
		status := "stopped"
		if ProcessAlive(entry.PID) {
			status = "running"
		}
		publicURL := entry.PublicURL
		if publicURL == "" {
			publicURL = "-"
		}
		fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\t%s\t%s\n", entry.Name, entry.PID, status, entry.Kind, entry.Target, publicURL, entry.LogPath)
	}
	return tw.Flush()
}

func doDown(args []string, stdout, stderr io.Writer) error {
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

	state, err := LoadState()
	if err != nil {
		return err
	}
	if len(state.Entries) == 0 {
		fmt.Fprintln(stdout, "No detached tunnels.")
		return nil
	}

	if all {
		for _, entry := range state.Entries {
			if ProcessAlive(entry.PID) {
				fmt.Fprintf(stdout, "Stopping %s (pid %d)\n", entry.Name, entry.PID)
				if err := TerminatePID(entry.PID, 5*time.Second); err != nil {
					return err
				}
			}
			cleanupOwnedTunnelBestEffort(specFromStateEntry(entry), ProvisionResult{CreatedTunnel: entry.CreatedTunnel}, stdout, stderr)
		}
		state.Entries = nil
		return SaveState(state)
	}

	entry, idx, ok := state.Find(key)
	if !ok {
		return fmt.Errorf("no detached tunnel named/pid %q", key)
	}
	if ProcessAlive(entry.PID) {
		fmt.Fprintf(stdout, "Stopping %s (pid %d)\n", entry.Name, entry.PID)
		if err := TerminatePID(entry.PID, 5*time.Second); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(stdout, "%s was already stopped\n", entry.Name)
	}
	cleanupOwnedTunnelBestEffort(specFromStateEntry(entry), ProvisionResult{CreatedTunnel: entry.CreatedTunnel}, stdout, stderr)
	state.RemoveIndex(idx)
	return SaveState(state)
}

func doLogs(args []string, stdout io.Writer) error {
	follow := false
	lines := 80
	var key string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			printLogsHelp(stdout)
			return nil
		case a == "-f" || a == "--follow":
			follow = true
		case a == "-n" || a == "--lines":
			if i+1 >= len(args) {
				return fmt.Errorf("%s needs a number", a)
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n < 0 {
				return fmt.Errorf("invalid line count %q", args[i])
			}
			lines = n
		case strings.HasPrefix(a, "--lines="):
			n, err := strconv.Atoi(strings.TrimPrefix(a, "--lines="))
			if err != nil || n < 0 {
				return fmt.Errorf("invalid line count %q", a)
			}
			lines = n
		case strings.HasPrefix(a, "-"):
			return fmt.Errorf("unknown logs flag %q", a)
		default:
			if key != "" {
				return fmt.Errorf("too many arguments")
			}
			key = a
		}
	}
	if key == "" {
		printLogsHelp(stdout)
		return fmt.Errorf("missing name or pid")
	}

	state, err := LoadState()
	if err != nil {
		return err
	}
	entry, _, ok := state.Find(key)
	if !ok {
		return fmt.Errorf("no detached tunnel named/pid %q", key)
	}
	if entry.LogPath == "" {
		return fmt.Errorf("no log path stored for %q", entry.Name)
	}
	if err := PrintLinesTail(entry.LogPath, lines, stdout); err != nil {
		return err
	}
	if follow {
		return FollowFile(entry.LogPath, stdout)
	}
	return nil
}

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

func hasHelp(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	return false
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

func printHelp(w io.Writer) {
	fmt.Fprint(w, `flareduct wraps cloudflared for quick and named tunnels.

Usage:
  flareduct up <port|url|host:port|path|alias|tunnel> [--detach] [--name NAME]
  flareduct list
  flareduct down <name|pid>
  flareduct logs <name|pid> [-f]
  flareduct login
  flareduct token <set|status|path|remove>
  flareduct config <path|init|show>
  flareduct doctor

Examples:
  flareduct up 3000                         # quick tunnel to http://localhost:3000
  flareduct up 3000 --subdomain demo        # demo.<public.domain>
  flareduct up 3000 --hostname demo.dev.tld # exact owned hostname
  flareduct up .                            # serve current directory
  flareduct up coolfile.html                # serve one HTML file
  flareduct login                           # cloudflared tunnel login helper
  flareduct token set                       # save API token for DNS cleanup
  flareduct up api --detach                 # alias from config, runs in background
  flareduct up blog                         # static alias or named cloudflared tunnel
  flareduct down port-3000

Global flags:
  --config PATH       flareduct config path (default: ~/.config/flareduct/config.yaml)
  --cloudflared PATH  cloudflared binary override

`)
}

func printUpHelp(w io.Writer) {
	fmt.Fprint(w, `Usage:
  flareduct up <port|url|host:port|path|alias|tunnel> [flags]

Flags:
  -d, --detach            run in the background and store state/logs
      --name NAME         detached process name (default derived from target)
      --hostname HOST     owned full hostname, e.g. demo.dev.example.com
      --subdomain NAME    subdomain under public.domain, e.g. demo
      --domain DOMAIN     domain override for --subdomain/generated names
      --tunnel-name NAME  Cloudflare Tunnel name for owned-hostname mode
      --trycloudflare     force a random trycloudflare.com quick tunnel
      --verbose           stream raw cloudflared logs to the terminal
      --random-style S    generated suffix style: words or hex
      --overwrite-dns     pass cloudflared --overwrite-dns for owned hostname
      --keep              do not clean up owned tunnel/DNS resources on stop/down
      --replace           stop an existing detached tunnel with the same name first
      --wait DURATION     wait for a quick tunnel public URL in detached mode (default 20s)
      --config PATH       flareduct config path
      --cloudflared PATH

`)
}

func printListHelp(w io.Writer) {
	fmt.Fprintln(w, "Usage: flareduct list")
}

func printDownHelp(w io.Writer) {
	fmt.Fprint(w, `Usage:
  flareduct down <name|pid>
  flareduct down --all

`)
}

func printLogsHelp(w io.Writer) {
	fmt.Fprint(w, `Usage:
  flareduct logs <name|pid> [-n LINES] [-f]

`)
}

func printLoginHelp(w io.Writer) {
	fmt.Fprintln(w, "Usage: flareduct login [--cloudflared PATH]")
}

func printTokenHelp(w io.Writer) {
	fmt.Fprint(w, `Usage:
  flareduct token set [TOKEN]
  flareduct token status
  flareduct token path
  flareduct token remove

Stores a Cloudflare API token in a 0600 file for DNS cleanup. Prefer
interactive `+"`flareduct token set`"+` over passing TOKEN as an argument.

`)
}

func printDoctorHelp(w io.Writer) {
	fmt.Fprintln(w, "Usage: flareduct doctor [--config PATH] [--cloudflared PATH]")
}

func printConfigHelp(w io.Writer) {
	fmt.Fprint(w, `Usage:
  flareduct config path
  flareduct config init [--force]
  flareduct config show

`)
}
