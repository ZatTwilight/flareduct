package resolver

import (
	"fmt"
	"strings"

	"flareduct/internal/config"
	"flareduct/internal/names"
	"flareduct/internal/random"
	"flareduct/internal/strutil"
	"flareduct/internal/wordslug"
)

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
	RandomSuffix  string
}

type Route struct {
	Hostname     string
	TunnelName   string
	PublicURL    string
	OverwriteDNS bool
	AutoCleanup  bool
	Zone         string
	ZoneID       string
}

func Resolve(name string, svc config.ServiceConfig, cfg config.Config, opts ResolveOptions) (Route, error) {
	if opts.TryCloudflare {
		if opts.Hostname != "" || opts.Subdomain != "" || opts.Domain != "" || opts.TunnelName != "" {
			return Route{}, fmt.Errorf("--trycloudflare cannot be combined with --hostname, --subdomain, --domain, or --tunnel-name")
		}
		return Route{}, nil
	}

	mode := strings.ToLower(strings.TrimSpace(strutil.FirstNonEmpty(svc.Mode, cfg.Public.Mode)))
	ownedBitsConfigured := opts.Hostname != "" || opts.Subdomain != "" || opts.Domain != "" ||
		svc.Hostname != "" || svc.Subdomain != "" || svc.Domain != "" || cfg.Public.Domain != ""
	switch mode {
	case "", "hostname", "host", "owned", "domain", "custom":
		// ok
	case "trycloudflare", "quick", "random":
		if !ownedBitsConfigured {
			return Route{}, nil
		}
	default:
		return Route{}, fmt.Errorf("unknown public mode %q", mode)
	}
	if !ownedBitsConfigured {
		return Route{}, nil
	}

	if opts.Hostname != "" && opts.Subdomain != "" {
		return Route{}, fmt.Errorf("use either --hostname or --subdomain, not both")
	}
	if svc.Hostname != "" && svc.Subdomain != "" && opts.Hostname == "" && opts.Subdomain == "" {
		return Route{}, fmt.Errorf("service config uses both hostname and subdomain")
	}

	hostnameInput := strutil.FirstNonEmpty(opts.Hostname, svc.Hostname)
	domainInput := strutil.FirstNonEmpty(opts.Domain, svc.Domain, cfg.Public.Domain)
	subdomainInput := strutil.FirstNonEmpty(opts.Subdomain, svc.Subdomain)

	var hostname string
	if hostnameInput != "" {
		normalized, err := names.NormalizeHostname(hostnameInput)
		if err != nil {
			return Route{}, err
		}
		hostname = normalized
	} else {
		if domainInput == "" {
			return Route{}, fmt.Errorf("owned-hostname mode needs public.domain, service domain, or --domain")
		}
		domain, err := names.NormalizeHostname(domainInput)
		if err != nil {
			return Route{}, fmt.Errorf("invalid domain: %w", err)
		}
		subdomain := ""
		if subdomainInput != "" {
			subdomain, err = names.NormalizeSubdomain(subdomainInput)
			if err != nil {
				return Route{}, err
			}
		} else {
			subdomain, err = generatedSubdomain(cfg.Public, opts)
			if err != nil {
				return Route{}, err
			}
		}
		hostname, err = names.NormalizeHostname(subdomain + "." + domain)
		if err != nil {
			return Route{}, err
		}
	}

	tunnelName := strutil.FirstNonEmpty(opts.TunnelName, svc.TunnelName)
	if tunnelName == "" {
		prefix := cfg.Public.TunnelPrefix
		if prefix == "" {
			prefix = "flareduct"
		}
		tunnelName = names.SanitizeName(prefix + "-" + strings.ReplaceAll(hostname, ".", "-"))
	} else {
		tunnelName = names.SanitizeName(tunnelName)
	}

	return Route{
		Hostname:     hostname,
		TunnelName:   tunnelName,
		PublicURL:    "https://" + hostname,
		OverwriteDNS: cfg.Public.OverwriteDNS || svc.OverwriteDNS || opts.OverwriteDNS,
		AutoCleanup:  !cfg.Public.Keep && !opts.Keep,
		Zone:         cfg.Public.Zone,
		ZoneID:       cfg.Public.ZoneID,
	}, nil
}

func generatedSubdomain(public config.PublicConfig, opts ResolveOptions) (string, error) {
	prefix := strings.TrimSpace(public.SubdomainPrefix)
	if prefix != "" {
		prefix = names.SanitizeDNSLabel(prefix)
	}
	randomSuffix := opts.RandomSuffix
	if randomSuffix == "" {
		var err error
		randomSuffix, err = generatedRandomSuffix(public, opts)
		if err != nil {
			return "", err
		}
	} else {
		randomSuffix = names.SanitizeDNSLabel(randomSuffix)
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

func generatedRandomSuffix(public config.PublicConfig, opts ResolveOptions) (string, error) {
	style := strings.ToLower(strings.TrimSpace(strutil.FirstNonEmpty(opts.RandomStyle, public.RandomStyle)))
	switch style {
	case "", "words", "word", "natural", "fun":
		return wordslug.WordSlug(public.RandomWords), nil
	case "hex", "hash":
		return random.HexSuffix(3), nil
	default:
		return "", fmt.Errorf("unknown random_style %q (use words or hex)", style)
	}
}
