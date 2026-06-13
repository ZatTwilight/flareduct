package main

import (
	"fmt"
	"io"
)

func hasHelp(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	return false
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
