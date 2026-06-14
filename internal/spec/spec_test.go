package spec

import (
	"reflect"
	"regexp"
	"testing"

	"flareduct/internal/config"
	"flareduct/internal/resolver"
)

func TestResolvePortTarget(t *testing.T) {
	spec, err := ResolveTarget("3000", config.DefaultConfig(), "")
	if err != nil {
		t.Fatalf("ResolveTarget returned error: %v", err)
	}
	if spec.Kind != KindQuick {
		t.Fatalf("kind = %s, want %s", spec.Kind, KindQuick)
	}
	if spec.Name != "port-3000" {
		t.Fatalf("name = %q", spec.Name)
	}
	if spec.URL != "http://localhost:3000" {
		t.Fatalf("url = %q", spec.URL)
	}
	want := []string{"cloudflared", "tunnel", "--url", "http://localhost:3000"}
	if !reflect.DeepEqual(spec.Command, want) {
		t.Fatalf("command = %#v, want %#v", spec.Command, want)
	}
}

func TestResolveHostPortTarget(t *testing.T) {
	spec, err := ResolveTarget("127.0.0.1:5173", config.DefaultConfig(), "")
	if err != nil {
		t.Fatalf("ResolveTarget returned error: %v", err)
	}
	if spec.Kind != KindQuick || spec.URL != "http://127.0.0.1:5173" {
		t.Fatalf("spec = %#v", spec)
	}
}

func TestResolveAliasQuick(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Defaults.QuickArgs = []string{"--edge-ip-version", "auto"}
	cfg.Services["api"] = config.ServiceConfig{Port: 8080, Args: []string{"--loglevel", "debug"}}

	spec, err := ResolveTarget("api", cfg, "/opt/bin/cloudflared")
	if err != nil {
		t.Fatalf("ResolveTarget returned error: %v", err)
	}
	want := []string{"/opt/bin/cloudflared", "tunnel", "--edge-ip-version", "auto", "--loglevel", "debug", "--url", "http://localhost:8080"}
	if !reflect.DeepEqual(spec.Command, want) {
		t.Fatalf("command = %#v, want %#v", spec.Command, want)
	}
}

func TestResolveOwnedHostnameFromConfiguredDomain(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Public.Domain = "dev.example.com"
	cfg.Public.TunnelPrefix = "fd"

	spec, err := ResolveTargetWithOptions("3000", cfg, resolver.ResolveOptions{RandomSuffix: "abc123"})
	if err != nil {
		t.Fatalf("ResolveTargetWithOptions returned error: %v", err)
	}
	if spec.Hostname != "abc123.dev.example.com" {
		t.Fatalf("hostname = %q", spec.Hostname)
	}
	if spec.PublicURL != "https://abc123.dev.example.com" {
		t.Fatalf("public url = %q", spec.PublicURL)
	}
	want := []string{"cloudflared", "tunnel", "run", "--url", "http://localhost:3000", "fd-abc123-dev-example-com"}
	if !reflect.DeepEqual(spec.Command, want) {
		t.Fatalf("command = %#v, want %#v", spec.Command, want)
	}
}

func TestResolveOwnedHostnameUsesWordSuffixByDefault(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Public.Domain = "example.com"

	spec, err := ResolveTargetWithOptions("3000", cfg, resolver.ResolveOptions{})
	if err != nil {
		t.Fatalf("ResolveTargetWithOptions returned error: %v", err)
	}
	if !regexp.MustCompile(`^[a-z]+-[a-z]+\.example\.com$`).MatchString(spec.Hostname) {
		t.Fatalf("hostname = %q", spec.Hostname)
	}
}

func TestResolveOwnedHostnameCanUseHexSuffix(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Public.Domain = "example.com"
	cfg.Public.RandomStyle = "hex"

	spec, err := ResolveTargetWithOptions("3000", cfg, resolver.ResolveOptions{})
	if err != nil {
		t.Fatalf("ResolveTargetWithOptions returned error: %v", err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{6}\.example\.com$`).MatchString(spec.Hostname) {
		t.Fatalf("hostname = %q", spec.Hostname)
	}
}

func TestResolveOwnedHostnameFromSubdomainFlag(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Public.Domain = "dev.example.com"

	spec, err := ResolveTargetWithOptions("3000", cfg, resolver.ResolveOptions{Subdomain: "demo", OverwriteDNS: true})
	if err != nil {
		t.Fatalf("ResolveTargetWithOptions returned error: %v", err)
	}
	if spec.Hostname != "demo.dev.example.com" {
		t.Fatalf("hostname = %q", spec.Hostname)
	}
	if !spec.OverwriteDNS {
		t.Fatalf("OverwriteDNS = false, want true")
	}
	want := []string{"cloudflared", "tunnel", "run", "--url", "http://localhost:3000", "flareduct-demo-dev-example-com"}
	if !reflect.DeepEqual(spec.Command, want) {
		t.Fatalf("command = %#v, want %#v", spec.Command, want)
	}
}

func TestResolveOwnedHostnameFromHostnameFlag(t *testing.T) {
	spec, err := ResolveTargetWithOptions("http://localhost:5173", config.DefaultConfig(), resolver.ResolveOptions{Hostname: "demo.example.com", TunnelName: "demo-tunnel"})
	if err != nil {
		t.Fatalf("ResolveTargetWithOptions returned error: %v", err)
	}
	want := []string{"cloudflared", "tunnel", "run", "--url", "http://localhost:5173", "demo-tunnel"}
	if !reflect.DeepEqual(spec.Command, want) {
		t.Fatalf("command = %#v, want %#v", spec.Command, want)
	}
}

func TestTryCloudflareOverridesConfiguredDomain(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Public.Domain = "dev.example.com"

	spec, err := ResolveTargetWithOptions("3000", cfg, resolver.ResolveOptions{TryCloudflare: true})
	if err != nil {
		t.Fatalf("ResolveTargetWithOptions returned error: %v", err)
	}
	want := []string{"cloudflared", "tunnel", "--url", "http://localhost:3000"}
	if spec.Hostname != "" || spec.PublicURL != "" || !reflect.DeepEqual(spec.Command, want) {
		t.Fatalf("spec = %#v", spec)
	}
}

func TestResolveAliasStatic(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Defaults.StaticArgs = []string{"--loglevel", "info"}
	cfg.Services["blog"] = config.ServiceConfig{Tunnel: "blog", Config: "~/.cloudflared/blog.yml", PublicURL: "https://blog.example.com"}

	spec, err := ResolveTarget("blog", cfg, "")
	if err != nil {
		t.Fatalf("ResolveTarget returned error: %v", err)
	}
	if spec.Kind != KindStatic {
		t.Fatalf("kind = %s", spec.Kind)
	}
	if spec.PublicURL != "https://blog.example.com" {
		t.Fatalf("public url = %q", spec.PublicURL)
	}
	if got := spec.Command[len(spec.Command)-2:]; !reflect.DeepEqual(got, []string{"run", "blog"}) {
		t.Fatalf("command suffix = %#v", got)
	}
}

func TestResolveUnknownAsNamedTunnel(t *testing.T) {
	spec, err := ResolveTarget("existing-tunnel", config.DefaultConfig(), "")
	if err != nil {
		t.Fatalf("ResolveTarget returned error: %v", err)
	}
	want := []string{"cloudflared", "tunnel", "run", "existing-tunnel"}
	if spec.Kind != KindStatic || !reflect.DeepEqual(spec.Command, want) {
		t.Fatalf("spec = %#v", spec)
	}
}
