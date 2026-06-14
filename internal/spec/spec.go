package spec

import (
	"fmt"
	"strconv"
	"strings"

	"flareduct/internal/config"
	"flareduct/internal/names"
	"flareduct/internal/paths"
	"flareduct/internal/resolver"
	"flareduct/internal/strutil"
)

type Kind string

const (
	KindQuick  Kind = "quick"
	KindStatic Kind = "static"
)

type Spec struct {
	Name         string   `json:"name"`
	Target       string   `json:"target"`
	Kind         Kind     `json:"kind"`
	URL          string   `json:"url,omitempty"`
	Hostname     string   `json:"hostname,omitempty"`
	TunnelName   string   `json:"tunnel_name,omitempty"`
	PublicURL    string   `json:"public_url,omitempty"`
	OverwriteDNS bool     `json:"overwrite_dns,omitempty"`
	AutoCleanup  bool     `json:"auto_cleanup,omitempty"`
	Zone         string   `json:"zone,omitempty"`
	ZoneID       string   `json:"zone_id,omitempty"`
	Command      []string `json:"command"`
	Verbose      bool     `json:"-"`
}

func ResolveTarget(target string, cfg config.Config, cloudflaredOverride string) (Spec, error) {
	return ResolveTargetWithOptions(target, cfg, resolver.ResolveOptions{Cloudflared: cloudflaredOverride})
}

func ResolveQuickTarget(name, target, serviceURL string, cfg config.Config, opts resolver.ResolveOptions) (Spec, error) {
	cfg.ApplyDefaults()
	cloudflared := resolveCloudflared(cfg, opts)
	return quickSpec(name, target, serviceURL, config.ServiceConfig{}, cfg, cloudflared, opts)
}

func ResolveTargetWithOptions(target string, cfg config.Config, opts resolver.ResolveOptions) (Spec, error) {
	cfg.ApplyDefaults()
	target = strings.TrimSpace(target)
	if target == "" {
		return Spec{}, fmt.Errorf("missing target")
	}

	cloudflared := resolveCloudflared(cfg, opts)

	if svc, ok := cfg.Services[target]; ok {
		return specFromService(target, svc, cfg, cloudflared, opts)
	}

	if strutil.IsDigits(target) {
		port, _ := strconv.Atoi(target)
		if port <= 0 || port > 65535 {
			return Spec{}, fmt.Errorf("invalid port %q", target)
		}
		return quickSpec("port-"+target, target, fmt.Sprintf("http://localhost:%d", port), config.ServiceConfig{}, cfg, cloudflared, opts)
	}

	if strutil.LooksLikeURL(target) {
		return quickSpec(names.DefaultNameForTarget(target), target, target, config.ServiceConfig{}, cfg, cloudflared, opts)
	}

	if strutil.HostPortLike(target) {
		return quickSpec(names.DefaultNameForTarget(target), target, "http://"+target, config.ServiceConfig{}, cfg, cloudflared, opts)
	}

	return staticSpec(names.SanitizeName(target), target, config.ServiceConfig{Tunnel: target}, cfg, cloudflared), nil
}

func resolveCloudflared(cfg config.Config, opts resolver.ResolveOptions) string {
	cloudflared := cfg.Cloudflared
	if opts.Cloudflared != "" {
		cloudflared = opts.Cloudflared
	}
	if cloudflared == "" {
		cloudflared = "cloudflared"
	}
	return paths.ExpandPath(cloudflared)
}

func specFromService(name string, svc config.ServiceConfig, cfg config.Config, cloudflared string, opts resolver.ResolveOptions) (Spec, error) {
	kind := strings.ToLower(strings.TrimSpace(svc.Type))
	switch kind {
	case "", "quick", "url", "dynamic":
		if kind == "" && svc.URL == "" && svc.Port == 0 && (svc.Tunnel != "" || svc.Config != "") {
			return staticSpec(name, name, svc, cfg, cloudflared), nil
		}
		url, err := serviceURL(svc)
		if err != nil {
			return Spec{}, fmt.Errorf("service %q: %w", name, err)
		}
		return quickSpec(name, name, url, svc, cfg, cloudflared, opts)
	case "static", "named", "tunnel":
		return staticSpec(name, name, svc, cfg, cloudflared), nil
	default:
		return Spec{}, fmt.Errorf("service %q: unknown type %q", name, svc.Type)
	}
}

func quickSpec(name, target, serviceURL string, svc config.ServiceConfig, cfg config.Config, cloudflared string, opts resolver.ResolveOptions) (Spec, error) {
	route, err := resolver.Resolve(name, svc, cfg, opts)
	if err != nil {
		return Spec{}, err
	}

	cmd := []string{cloudflared, "tunnel"}
	cmd = append(cmd, cfg.Defaults.QuickArgs...)
	cmd = append(cmd, svc.Args...)
	if route.Hostname != "" {
		cmd = append(cmd, "run", "--url", serviceURL, route.TunnelName)
	} else {
		cmd = append(cmd, "--url", serviceURL)
	}

	publicURL := svc.PublicURL
	if route.PublicURL != "" {
		publicURL = route.PublicURL
	}

	return Spec{
		Name:         names.SanitizeName(name),
		Target:       target,
		Kind:         KindQuick,
		URL:          serviceURL,
		Hostname:     route.Hostname,
		TunnelName:   route.TunnelName,
		PublicURL:    publicURL,
		OverwriteDNS: route.OverwriteDNS,
		AutoCleanup:  route.AutoCleanup,
		Zone:         route.Zone,
		ZoneID:       route.ZoneID,
		Command:      cmd,
	}, nil
}

func staticSpec(name, target string, svc config.ServiceConfig, cfg config.Config, cloudflared string) Spec {
	cmd := []string{cloudflared, "tunnel"}
	cmd = append(cmd, cfg.Defaults.StaticArgs...)
	cmd = append(cmd, svc.Args...)
	if svc.Config != "" {
		cmd = append(cmd, "--config", paths.ExpandPath(svc.Config))
	}
	cmd = append(cmd, "run")
	if svc.Tunnel != "" {
		cmd = append(cmd, svc.Tunnel)
	}
	return Spec{
		Name:      names.SanitizeName(name),
		Target:    target,
		Kind:      KindStatic,
		PublicURL: svc.PublicURL,
		Command:   cmd,
	}
}
