package intake

import (
	"fmt"
	"io"

	"flareduct/internal/config"
	"flareduct/internal/resolver"
	"flareduct/internal/spec"
	"flareduct/internal/staticserver"
)

type Options struct {
	Resolve resolver.ResolveOptions
	Detach  bool
}

func Prepare(target string, cfg config.Config, opts Options, stdout io.Writer) (spec.Spec, func(), error) {
	if _, isAlias := cfg.Services[target]; isAlias {
		s, err := spec.ResolveTargetWithOptions(target, cfg, opts.Resolve)
		return s, nil, err
	}

	server, err := staticserver.StartIfPath(target)
	if err != nil {
		return spec.Spec{}, nil, err
	}
	if server == nil {
		s, err := spec.ResolveTargetWithOptions(target, cfg, opts.Resolve)
		return s, nil, err
	}

	cleanup := func() { _ = server.Close() }
	if opts.Detach {
		cleanup()
		return spec.Spec{}, nil, fmt.Errorf("--detach is not supported for static file targets yet")
	}

	fmt.Fprintf(stdout, "flareduct: serving %s %s at %s\n", server.Kind, server.Path, server.URL)
	s, err := spec.ResolveQuickTarget(server.Name, target, server.URL, cfg, opts.Resolve)
	if err != nil {
		cleanup()
		return spec.Spec{}, nil, err
	}
	return s, cleanup, nil
}
