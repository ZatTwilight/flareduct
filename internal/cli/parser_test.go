package cli

import (
	"reflect"
	"testing"
	"time"
)

func TestParseLeadingGlobalsStopsAtCommand(t *testing.T) {
	globals, rest, err := ParseLeadingGlobals([]string{"--config", "cfg.yml", "--cloudflared=/bin/cf", "up", "3000"})
	if err != nil {
		t.Fatalf("ParseLeadingGlobals returned error: %v", err)
	}
	if globals.ConfigPath != "cfg.yml" || globals.Cloudflared != "/bin/cf" || globals.Help {
		t.Fatalf("globals = %#v", globals)
	}
	wantRest := []string{"up", "3000"}
	if !reflect.DeepEqual(rest, wantRest) {
		t.Fatalf("rest = %#v, want %#v", rest, wantRest)
	}
}

func TestParseUpArgsAcceptsCommandAndGlobalFlags(t *testing.T) {
	opts, target, err := ParseUpArgs([]string{
		"3000",
		"--detach",
		"--replace",
		"--name", "Demo App",
		"--wait=1500ms",
		"--config", "cmd.yml",
		"--cloudflared=/bin/cf",
		"--subdomain=demo",
		"--random-style", "hex",
		"--overwrite-dns",
		"--keep",
		"--verbose",
	}, GlobalOptions{ConfigPath: "global.yml", Cloudflared: "global-cf"})
	if err != nil {
		t.Fatalf("ParseUpArgs returned error: %v", err)
	}
	if target != "3000" {
		t.Fatalf("target = %q", target)
	}
	if !opts.Detach || !opts.Replace || !opts.OverwriteDNS || !opts.Keep || !opts.Verbose {
		t.Fatalf("boolean opts not set: %#v", opts)
	}
	if opts.Name != "Demo App" || opts.Wait != 1500*time.Millisecond || opts.ConfigPath != "cmd.yml" || opts.Cloudflared != "/bin/cf" || opts.Subdomain != "demo" || opts.RandomStyle != "hex" {
		t.Fatalf("opts = %#v", opts)
	}
}

func TestParseUpArgsRejectsUnknownFlagAndExtraTarget(t *testing.T) {
	if _, _, err := ParseUpArgs([]string{"--bogus"}, GlobalOptions{}); err == nil || err.Error() != "unknown up flag \"--bogus\"" {
		t.Fatalf("unknown flag err = %v", err)
	}
	if _, _, err := ParseUpArgs([]string{"3000", "4000"}, GlobalOptions{}); err == nil || err.Error() != "too many targets: \"3000\" and \"4000\"" {
		t.Fatalf("extra target err = %v", err)
	}
}
