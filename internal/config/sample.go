package config

func SampleConfig() string {
	return `# flareduct config
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

# Optional Cloudflare Pages settings for flareduct ship.
# pages:
#   account_id: 99ed1e4f3a7c251328fddfe5661a6195 # optional; auto-detected from wrangler
#   domain: pages.example.com # optional; falls back to public.domain

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
}
