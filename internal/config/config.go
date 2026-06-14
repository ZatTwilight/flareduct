package config

import (
	"fmt"
	"os"
	"strings"

	"flareduct/internal/paths"
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
	Domain string `yaml:"domain,omitempty" json:"domain,omitempty"`
	Mode   string `yaml:"mode,omitempty" json:"mode,omitempty"`

	SubdomainPrefix string `yaml:"subdomain_prefix,omitempty" json:"subdomain_prefix,omitempty"`
	TunnelPrefix    string `yaml:"tunnel_prefix,omitempty" json:"tunnel_prefix,omitempty"`

	RandomStyle string `yaml:"random_style,omitempty" json:"random_style,omitempty"`
	RandomWords int    `yaml:"random_words,omitempty" json:"random_words,omitempty"`

	Zone   string `yaml:"zone,omitempty" json:"zone,omitempty"`
	ZoneID string `yaml:"zone_id,omitempty" json:"zone_id,omitempty"`

	Keep         bool `yaml:"keep,omitempty" json:"keep,omitempty"`
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

func (c *Config) ApplyDefaults() {
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
		path = paths.DefaultConfigPath()
	}
	path = paths.ExpandPath(path)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) && explicitPath == "" {
			return cfg, path, false, nil
		}
		return cfg, path, false, err
	}

	if strings.TrimSpace(string(data)) == "" {
		cfg.ApplyDefaults()
		return cfg, path, true, nil
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, path, true, fmt.Errorf("parse config %s: %w", path, err)
	}
	cfg.ApplyDefaults()
	return cfg, path, true, nil
}

func ConfigToYAML(cfg Config) ([]byte, error) {
	cfg.ApplyDefaults()
	return yaml.Marshal(cfg)
}
