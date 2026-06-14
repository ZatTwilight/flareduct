package cli

import (
	"fmt"
	"strings"
	"time"

	"flareduct/internal/resolver"
)

type UpOptions struct {
	GlobalOptions
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

func (o UpOptions) ResolveOptions() resolver.ResolveOptions {
	return resolver.ResolveOptions{
		Cloudflared:   o.Cloudflared,
		Hostname:      o.Hostname,
		Subdomain:     o.Subdomain,
		Domain:        o.Domain,
		TunnelName:    o.TunnelName,
		TryCloudflare: o.TryCloudflare,
		OverwriteDNS:  o.OverwriteDNS,
		Keep:          o.Keep,
		RandomStyle:   o.RandomStyle,
	}
}

func ParseUpArgs(args []string, globals GlobalOptions) (UpOptions, string, error) {
	opts := UpOptions{GlobalOptions: globals, Wait: 20 * time.Second}
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
