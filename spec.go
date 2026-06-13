package main

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

type TunnelKind string

const (
	KindQuick  TunnelKind = "quick"
	KindStatic TunnelKind = "static"
)

type TunnelSpec struct {
	Name         string     `json:"name"`
	Target       string     `json:"target"`
	Kind         TunnelKind `json:"kind"`
	URL          string     `json:"url,omitempty"`
	Hostname     string     `json:"hostname,omitempty"`
	TunnelName   string     `json:"tunnel_name,omitempty"`
	PublicURL    string     `json:"public_url,omitempty"`
	OverwriteDNS bool       `json:"overwrite_dns,omitempty"`
	AutoCleanup  bool       `json:"auto_cleanup,omitempty"`
	Zone         string     `json:"zone,omitempty"`
	ZoneID       string     `json:"zone_id,omitempty"`
	Command      []string   `json:"command"`
	Verbose      bool       `json:"-"`
}

type ResolveOptions struct {
	Cloudflared   string
	Hostname      string
	Subdomain     string
	Domain        string
	TunnelName    string
	TryCloudflare bool
	OverwriteDNS  bool
	Keep          bool
	RandomStyle   string
	// RandomSuffix is primarily for tests. Empty means generate one.
	RandomSuffix string
}

type publicRoute struct {
	Hostname     string
	TunnelName   string
	PublicURL    string
	OverwriteDNS bool
	AutoCleanup  bool
	Zone         string
	ZoneID       string
}

func ResolveTarget(target string, cfg Config, cloudflaredOverride string) (TunnelSpec, error) {
	return ResolveTargetWithOptions(target, cfg, ResolveOptions{Cloudflared: cloudflaredOverride})
}

func ResolveQuickTarget(name, target, serviceURL string, cfg Config, opts ResolveOptions) (TunnelSpec, error) {
	cfg.applyDefaults()
	cloudflared := resolveCloudflared(cfg, opts)
	return quickSpec(name, target, serviceURL, ServiceConfig{}, cfg, cloudflared, opts)
}

func ResolveTargetWithOptions(target string, cfg Config, opts ResolveOptions) (TunnelSpec, error) {
	cfg.applyDefaults()
	target = strings.TrimSpace(target)
	if target == "" {
		return TunnelSpec{}, fmt.Errorf("missing target")
	}

	cloudflared := resolveCloudflared(cfg, opts)

	if svc, ok := cfg.Services[target]; ok {
		return specFromService(target, svc, cfg, cloudflared, opts)
	}

	if IsDigits(target) {
		port, _ := strconv.Atoi(target)
		if port <= 0 || port > 65535 {
			return TunnelSpec{}, fmt.Errorf("invalid port %q", target)
		}
		return quickSpec("port-"+target, target, fmt.Sprintf("http://localhost:%d", port), ServiceConfig{}, cfg, cloudflared, opts)
	}

	if LooksLikeURL(target) {
		return quickSpec(DefaultNameForTarget(target), target, target, ServiceConfig{}, cfg, cloudflared, opts)
	}

	if HostPortLike(target) {
		return quickSpec(DefaultNameForTarget(target), target, "http://"+target, ServiceConfig{}, cfg, cloudflared, opts)
	}

	// Convenience fallback for existing static tunnels: `flareduct up my-tunnel`
	// maps to `cloudflared tunnel run my-tunnel` even without a flareduct config.
	return staticSpec(SanitizeName(target), target, ServiceConfig{Tunnel: target}, cfg, cloudflared), nil
}

func resolveCloudflared(cfg Config, opts ResolveOptions) string {
	cloudflared := cfg.Cloudflared
	if opts.Cloudflared != "" {
		cloudflared = opts.Cloudflared
	}
	if cloudflared == "" {
		cloudflared = "cloudflared"
	}
	return ExpandPath(cloudflared)
}

func specFromService(name string, svc ServiceConfig, cfg Config, cloudflared string, opts ResolveOptions) (TunnelSpec, error) {
	kind := strings.ToLower(strings.TrimSpace(svc.Type))
	switch kind {
	case "", "quick", "url", "dynamic":
		if kind == "" && svc.URL == "" && svc.Port == 0 && (svc.Tunnel != "" || svc.Config != "") {
			return staticSpec(name, name, svc, cfg, cloudflared), nil
		}
		url, err := serviceURL(svc)
		if err != nil {
			return TunnelSpec{}, fmt.Errorf("service %q: %w", name, err)
		}
		return quickSpec(name, name, url, svc, cfg, cloudflared, opts)
	case "static", "named", "tunnel":
		return staticSpec(name, name, svc, cfg, cloudflared), nil
	default:
		return TunnelSpec{}, fmt.Errorf("service %q: unknown type %q", name, svc.Type)
	}
}

func serviceURL(svc ServiceConfig) (string, error) {
	if svc.URL != "" {
		if !LooksLikeURL(svc.URL) {
			return "", fmt.Errorf("url must include a scheme and host, got %q", svc.URL)
		}
		return svc.URL, nil
	}
	if svc.Port == 0 {
		return "", fmt.Errorf("set url, port, tunnel, or config")
	}
	if svc.Port < 0 || svc.Port > 65535 {
		return "", fmt.Errorf("invalid port %d", svc.Port)
	}
	host := svc.Host
	if host == "" {
		host = "localhost"
	}
	scheme := svc.Scheme
	if scheme == "" {
		scheme = "http"
	}
	return (&url.URL{Scheme: scheme, Host: fmt.Sprintf("%s:%d", host, svc.Port)}).String(), nil
}

func quickSpec(name, target, serviceURL string, svc ServiceConfig, cfg Config, cloudflared string, opts ResolveOptions) (TunnelSpec, error) {
	route, err := resolvePublicRoute(name, svc, cfg, opts)
	if err != nil {
		return TunnelSpec{}, err
	}

	cmd := []string{cloudflared, "tunnel"}
	cmd = append(cmd, cfg.Defaults.QuickArgs...)
	cmd = append(cmd, svc.Args...)
	if route.Hostname != "" {
		// Avoid cloudflared's one-shot --hostname/--name path. In some versions it
		// forwards the tunnel-level output default into `tunnel create` and can even
		// panic after creating the tunnel. flareduct provisions explicitly, then runs.
		cmd = append(cmd, "run", "--url", serviceURL, route.TunnelName)
	} else {
		cmd = append(cmd, "--url", serviceURL)
	}

	publicURL := svc.PublicURL
	if route.PublicURL != "" {
		publicURL = route.PublicURL
	}

	return TunnelSpec{
		Name:         SanitizeName(name),
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

func resolvePublicRoute(name string, svc ServiceConfig, cfg Config, opts ResolveOptions) (publicRoute, error) {
	if opts.TryCloudflare {
		if opts.Hostname != "" || opts.Subdomain != "" || opts.Domain != "" || opts.TunnelName != "" {
			return publicRoute{}, fmt.Errorf("--trycloudflare cannot be combined with --hostname, --subdomain, --domain, or --tunnel-name")
		}
		return publicRoute{}, nil
	}

	mode := strings.ToLower(strings.TrimSpace(firstNonEmpty(svc.Mode, cfg.Public.Mode)))
	ownedBitsConfigured := opts.Hostname != "" || opts.Subdomain != "" || opts.Domain != "" || svc.Hostname != "" || svc.Subdomain != "" || svc.Domain != "" || cfg.Public.Domain != ""
	switch mode {
	case "", "hostname", "host", "owned", "domain", "custom":
		// ok
	case "trycloudflare", "quick", "random":
		if !ownedBitsConfigured {
			return publicRoute{}, nil
		}
	default:
		return publicRoute{}, fmt.Errorf("unknown public mode %q", mode)
	}
	if !ownedBitsConfigured {
		return publicRoute{}, nil
	}

	if opts.Hostname != "" && opts.Subdomain != "" {
		return publicRoute{}, fmt.Errorf("use either --hostname or --subdomain, not both")
	}
	if svc.Hostname != "" && svc.Subdomain != "" && opts.Hostname == "" && opts.Subdomain == "" {
		return publicRoute{}, fmt.Errorf("service config uses both hostname and subdomain")
	}

	hostnameInput := firstNonEmpty(opts.Hostname, svc.Hostname)
	domainInput := firstNonEmpty(opts.Domain, svc.Domain, cfg.Public.Domain)
	subdomainInput := firstNonEmpty(opts.Subdomain, svc.Subdomain)

	var hostname string
	if hostnameInput != "" {
		normalized, err := NormalizeHostname(hostnameInput)
		if err != nil {
			return publicRoute{}, err
		}
		hostname = normalized
	} else {
		if domainInput == "" {
			return publicRoute{}, fmt.Errorf("owned-hostname mode needs public.domain, service domain, or --domain")
		}
		domain, err := NormalizeHostname(domainInput)
		if err != nil {
			return publicRoute{}, fmt.Errorf("invalid domain: %w", err)
		}
		subdomain := ""
		if subdomainInput != "" {
			subdomain, err = NormalizeSubdomain(subdomainInput)
			if err != nil {
				return publicRoute{}, err
			}
		} else {
			subdomain, err = generatedSubdomain(cfg.Public, opts)
			if err != nil {
				return publicRoute{}, err
			}
		}
		hostname, err = NormalizeHostname(subdomain + "." + domain)
		if err != nil {
			return publicRoute{}, err
		}
	}

	tunnelName := firstNonEmpty(opts.TunnelName, svc.TunnelName)
	if tunnelName == "" {
		prefix := cfg.Public.TunnelPrefix
		if prefix == "" {
			prefix = "flareduct"
		}
		tunnelName = SanitizeName(prefix + "-" + strings.ReplaceAll(hostname, ".", "-"))
	} else {
		tunnelName = SanitizeName(tunnelName)
	}

	return publicRoute{
		Hostname:     hostname,
		TunnelName:   tunnelName,
		PublicURL:    "https://" + hostname,
		OverwriteDNS: cfg.Public.OverwriteDNS || svc.OverwriteDNS || opts.OverwriteDNS,
		AutoCleanup:  !cfg.Public.Keep && !opts.Keep,
		Zone:         cfg.Public.Zone,
		ZoneID:       cfg.Public.ZoneID,
	}, nil
}

func generatedSubdomain(public PublicConfig, opts ResolveOptions) (string, error) {
	prefix := strings.TrimSpace(public.SubdomainPrefix)
	if prefix != "" {
		prefix = SanitizeDNSLabel(prefix)
	}
	randomSuffix := opts.RandomSuffix
	if randomSuffix == "" {
		var err error
		randomSuffix, err = generatedRandomSuffix(public, opts)
		if err != nil {
			return "", err
		}
	} else {
		randomSuffix = SanitizeDNSLabel(randomSuffix)
	}

	pieces := make([]string, 0, 2)
	if prefix != "" {
		pieces = append(pieces, prefix)
	}
	pieces = append(pieces, randomSuffix)

	label := strings.Trim(strings.Join(pieces, "-"), "-")
	if len(label) <= 63 {
		return label, nil
	}

	maxSuffix := 63
	if prefix != "" {
		maxSuffix = 63 - len(prefix) - 1
		if maxSuffix < 1 {
			maxSuffix = 1
		}
	}
	if len(randomSuffix) > maxSuffix {
		randomSuffix = strings.Trim(randomSuffix[:maxSuffix], "-")
		if randomSuffix == "" {
			randomSuffix = "tunnel"
		}
	}
	pieces = pieces[:0]
	if prefix != "" {
		pieces = append(pieces, prefix)
	}
	pieces = append(pieces, randomSuffix)
	label = strings.Trim(strings.Join(pieces, "-"), "-")
	if len(label) > 63 {
		label = strings.Trim(label[:63], "-")
	}
	if label == "" {
		label = "tunnel"
	}
	return label, nil
}

func generatedRandomSuffix(public PublicConfig, opts ResolveOptions) (string, error) {
	style := strings.ToLower(strings.TrimSpace(firstNonEmpty(opts.RandomStyle, public.RandomStyle)))
	switch style {
	case "", "words", "word", "natural", "fun":
		return RandomWordSlug(public.RandomWords), nil
	case "hex", "hash":
		return RandomHexSuffix(3), nil
	default:
		return "", fmt.Errorf("unknown random_style %q (use words or hex)", style)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func staticSpec(name, target string, svc ServiceConfig, cfg Config, cloudflared string) TunnelSpec {
	cmd := []string{cloudflared, "tunnel"}
	cmd = append(cmd, cfg.Defaults.StaticArgs...)
	cmd = append(cmd, svc.Args...)
	if svc.Config != "" {
		cmd = append(cmd, "--config", ExpandPath(svc.Config))
	}
	cmd = append(cmd, "run")
	if svc.Tunnel != "" {
		cmd = append(cmd, svc.Tunnel)
	}
	return TunnelSpec{
		Name:      SanitizeName(name),
		Target:    target,
		Kind:      KindStatic,
		PublicURL: svc.PublicURL,
		Command:   cmd,
	}
}
