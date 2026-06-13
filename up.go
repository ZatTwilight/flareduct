package main

import (
	"fmt"
	"io"
)

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

	spec, cleanup, err := PrepareUpTarget(target, cfg, TargetIntakeOptions{
		Resolve: opts.resolveOptions(),
		Detach:  opts.Detach,
	}, stdout)
	if cleanup != nil {
		defer cleanup()
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
