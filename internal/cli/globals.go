package cli

import (
	"fmt"
	"strings"
)

type GlobalOptions struct {
	ConfigPath  string
	Cloudflared string
	Help        bool
}

func ParseLeadingGlobals(args []string) (GlobalOptions, []string, error) {
	var opts GlobalOptions
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
