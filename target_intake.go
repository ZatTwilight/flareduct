package main

import (
	"fmt"
	"io"
)

type TargetIntakeOptions struct {
	Resolve ResolveOptions
	Detach  bool
}

// PrepareUpTarget turns the user's up target into a TunnelSpec. Config aliases
// intentionally win over same-named local paths; otherwise local files and
// directories get an embedded foreground-only HTTP server.
func PrepareUpTarget(target string, cfg Config, opts TargetIntakeOptions, stdout io.Writer) (TunnelSpec, func(), error) {
	if _, isAlias := cfg.Services[target]; isAlias {
		spec, err := ResolveTargetWithOptions(target, cfg, opts.Resolve)
		return spec, nil, err
	}

	staticServer, err := StartStaticServerIfPath(target)
	if err != nil {
		return TunnelSpec{}, nil, err
	}
	if staticServer == nil {
		spec, err := ResolveTargetWithOptions(target, cfg, opts.Resolve)
		return spec, nil, err
	}

	cleanup := func() { _ = staticServer.Close() }
	if opts.Detach {
		cleanup()
		return TunnelSpec{}, nil, fmt.Errorf("--detach is not supported for static file targets yet")
	}

	fmt.Fprintf(stdout, "flareduct: serving %s %s at %s\n", staticServer.Kind, staticServer.Path, staticServer.URL)
	spec, err := ResolveQuickTarget(staticServer.Name, target, staticServer.URL, cfg, opts.Resolve)
	if err != nil {
		cleanup()
		return TunnelSpec{}, nil, err
	}
	return spec, cleanup, nil
}
