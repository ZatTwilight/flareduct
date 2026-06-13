package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Cloudflared string                   `yaml:"cloudflared" json:"cloudflared"`
	Defaults    Defaults                 `yaml:"defaults,omitempty" json:"defaults,omitempty"`
	Public      PublicConfig             `yaml:"public,omitempty" json:"public,omitempty"`
	Services    map[string]ServiceConfig `yaml:"services,omitempty" json:"services,omitempty"`
}

type Defaults struct {
	QuickArgs  []string `yaml:"quick_args,omitempty" json:"quick_args,omitempty"`
	StaticArgs []string `yaml:"static_args,omitempty" json:"static_args,omitempty"`
}

type PublicConfig struct {
	// Domain enables owned-hostname mode for quick tunnels. For example,
	// domain: dev.example.com makes `flareduct up 3000` route a generated
	// hostname like cozy-otter.dev.example.com through cloudflared.
	Domain string `yaml:"domain,omitempty" json:"domain,omitempty"`
	// Mode can be "hostname" or "trycloudflare". Empty means hostname when a
	// domain is configured, otherwise trycloudflare.
	Mode string `yaml:"mode,omitempty" json:"mode,omitempty"`
	// Optional prefixes for generated DNS labels and Cloudflare tunnel names.
	SubdomainPrefix string `yaml:"subdomain_prefix,omitempty" json:"subdomain_prefix,omitempty"`
	TunnelPrefix    string `yaml:"tunnel_prefix,omitempty" json:"tunnel_prefix,omitempty"`
	// RandomStyle controls generated suffixes: "words" (default) or "hex".
	RandomStyle string `yaml:"random_style,omitempty" json:"random_style,omitempty"`
	// RandomWords controls word suffix length in words mode. Default 2, max 4.
	RandomWords int `yaml:"random_words,omitempty" json:"random_words,omitempty"`
	// Zone/ZoneID help DNS cleanup when public.domain is a subdomain. If unset,
	// cleanup tries to discover the Cloudflare zone by hostname suffix.
	Zone   string `yaml:"zone,omitempty" json:"zone,omitempty"`
	ZoneID string `yaml:"zone_id,omitempty" json:"zone_id,omitempty"`
	// Keep leaves owned quick-tunnel Cloudflare resources behind after stop/down.
	Keep bool `yaml:"keep,omitempty" json:"keep,omitempty"`
	// Adds cloudflared's --overwrite-dns flag for owned-hostname quick tunnels.
	OverwriteDNS bool `yaml:"overwrite_dns,omitempty" json:"overwrite_dns,omitempty"`
}

type ServiceConfig struct {
	Type         string   `yaml:"type,omitempty" json:"type,omitempty"`
	URL          string   `yaml:"url,omitempty" json:"url,omitempty"`
	Port         int      `yaml:"port,omitempty" json:"port,omitempty"`
	Host         string   `yaml:"host,omitempty" json:"host,omitempty"`
	Scheme       string   `yaml:"scheme,omitempty" json:"scheme,omitempty"`
	Tunnel       string   `yaml:"tunnel,omitempty" json:"tunnel,omitempty"`
	Config       string   `yaml:"config,omitempty" json:"config,omitempty"`
	PublicURL    string   `yaml:"public_url,omitempty" json:"public_url,omitempty"`
	Mode         string   `yaml:"mode,omitempty" json:"mode,omitempty"`
	Domain       string   `yaml:"domain,omitempty" json:"domain,omitempty"`
	Subdomain    string   `yaml:"subdomain,omitempty" json:"subdomain,omitempty"`
	Hostname     string   `yaml:"hostname,omitempty" json:"hostname,omitempty"`
	TunnelName   string   `yaml:"tunnel_name,omitempty" json:"tunnel_name,omitempty"`
	OverwriteDNS bool     `yaml:"overwrite_dns,omitempty" json:"overwrite_dns,omitempty"`
	Args         []string `yaml:"args,omitempty" json:"args,omitempty"`
}

func DefaultConfig() Config {
	cfg := Config{
		Cloudflared: "cloudflared",
		Services:    map[string]ServiceConfig{},
	}
	return cfg
}

func (c *Config) applyDefaults() {
	if c.Cloudflared == "" {
		c.Cloudflared = "cloudflared"
	}
	if c.Services == nil {
		c.Services = map[string]ServiceConfig{}
	}
}

func LoadConfig(explicitPath string) (Config, string, bool, error) {
	cfg := DefaultConfig()
	path := explicitPath
	if path == "" {
		path = DefaultConfigPath()
	}
	path = ExpandPath(path)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) && explicitPath == "" {
			return cfg, path, false, nil
		}
		return cfg, path, false, err
	}

	if strings.TrimSpace(string(data)) == "" {
		cfg.applyDefaults()
		return cfg, path, true, nil
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, path, true, fmt.Errorf("parse config %s: %w", path, err)
	}
	cfg.applyDefaults()
	return cfg, path, true, nil
}

func ConfigToYAML(cfg Config) ([]byte, error) {
	cfg.applyDefaults()
	return yaml.Marshal(cfg)
}

func DefaultConfigPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		if userConfigDir, err := os.UserConfigDir(); err == nil && userConfigDir != "" {
			base = userConfigDir
		} else {
			base = filepath.Join(HomeDir(), ".config")
		}
	}
	return filepath.Join(base, "flareduct", "config.yaml")
}

func DefaultStateDir() string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		base = filepath.Join(HomeDir(), ".local", "state")
	}
	return filepath.Join(base, "flareduct")
}

func StateFilePath() string {
	return filepath.Join(DefaultStateDir(), "tunnels.json")
}

func LogsDirPath() string {
	return filepath.Join(DefaultStateDir(), "logs")
}

const sampleConfig = `# flareduct config
# Path to cloudflared, or just "cloudflared" if it is on PATH.
cloudflared: cloudflared

# Extra args are inserted after "cloudflared tunnel" and before the
# action-specific bits ("--url ..." for quick tunnels, "run ..." for static).
defaults:
  quick_args: []
  static_args: []

# Uncomment this after flareduct login to make quick tunnels use
# hostnames you own instead of random trycloudflare.com URLs.
# public:
#   domain: dev.example.com
#   mode: hostname
#   tunnel_prefix: flareduct
#   subdomain_prefix: fd
#   random_style: words # words (default) or hex
#   random_words: 2
#   # For DNS auto-cleanup, set CLOUDFLARE_API_TOKEN with Zone:Read + DNS:Edit.
#   # zone: zat.wtf # optional; useful if domain is a subdomain
#   keep: false
#   overwrite_dns: false

services:
  # Quick tunnel alias. flareduct up web runs:
  # cloudflared tunnel --url http://localhost:3000
  # If public.domain is configured, it will instead create/route/run an owned
  # hostname. Set subdomain/hostname here for a stable public name.
  web:
    url: http://localhost:3000
    # subdomain: web

  # Port shorthand alias. Equivalent to http://localhost:8080.
  api:
    port: 8080

  # Existing named/static Cloudflare Tunnel. This points at your normal
  # cloudflared config and runs: cloudflared tunnel --config ... run blog
  blog:
    tunnel: blog
    config: ~/.cloudflared/blog.yml
    public_url: https://blog.example.com
`
