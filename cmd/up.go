package cmd

import (
	"fmt"
	"io"

	"flareduct/internal/cli"
	"flareduct/internal/cloudflare"
	"flareduct/internal/config"
	"flareduct/internal/intake"
	"flareduct/internal/names"
	"flareduct/internal/process"
	"flareduct/internal/registry"
)

func runUp(args []string, globals cli.GlobalOptions, stdout, stderr io.Writer) error {
	opts, target, err := cli.ParseUpArgs(args, globals)
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

	cfg, _, _, err := config.LoadConfig(opts.ConfigPath)
	if err != nil {
		return err
	}

	s, cleanup, err := intake.Prepare(target, cfg, intake.Options{
		Resolve: opts.ResolveOptions(),
		Detach:  opts.Detach,
	}, stdout)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return err
	}
	if opts.Name != "" {
		s.Name = names.SanitizeName(opts.Name)
	}
	s.Verbose = opts.Verbose

	if len(s.Command) == 0 {
		return fmt.Errorf("no command to run")
	}
	if _, err := cloudflare.ResolveBinaryPath(s.Command[0]); err != nil {
		return err
	}

	if opts.Detach {
		return registry.Start(s, registry.Options{Name: s.Name, Replace: opts.Replace, Wait: opts.Wait}, stdout, stderr)
	}
	return process.RunForeground(s, stdout, stderr)
}
