# flareduct

`flareduct` is a small CLI wrapper around [`cloudflared`](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/) for common Cloudflare Tunnel workflows:

- quick, throwaway public URLs: `flareduct up 3000`
- owned-domain quick tunnels: `flareduct up 3000 --subdomain demo`
- quick static file servers: `flareduct up .` or `flareduct up coolfile.html`
- existing named/static Cloudflare Tunnels: `flareduct up blog`

## Install / build

```sh
go build -o bin/flareduct .
```

`cloudflared` must be installed and available on `PATH`, or configured with `--cloudflared` / `cloudflared:` in the config file.

## Quick tunnels

```sh
flareduct up 3000
```

runs roughly:

```sh
cloudflared tunnel --url http://localhost:3000
```

`flareduct` prints a compact panel with the public URL and writes noisy `cloudflared` logs to `${XDG_STATE_HOME:-~/.local/state}/flareduct/logs`. Use `--verbose` if you want raw `cloudflared` logs in the terminal. Ctrl-C stops the tunnel.

You can also pass a URL or host:port:

```sh
flareduct up http://localhost:5173
flareduct up 127.0.0.1:8080
```

## Static file servers

Serve a directory or a single file without starting your own web server first:

```sh
flareduct up .
flareduct up coolfile.html
```

For a file target, `/` serves that file and sibling assets remain available by path, so a small HTML page can reference nearby CSS/images.

Static file targets run in foreground mode for now; `--detach` is rejected because the embedded local file server would otherwise exit immediately.

## Owned-domain quick tunnels

Run `flareduct login` once (a small wrapper around `cloudflared tunnel login`), then configure a domain you control:

```yaml
public:
  domain: dev.example.com
  mode: hostname
  tunnel_prefix: flareduct
  random_style: words # words (default) or hex
  random_words: 2
```

Now a plain port target generates a fun word-based subdomain. `flareduct` explicitly creates the named tunnel, routes DNS, then runs it:

```sh
flareduct up 3000
# roughly:
# cloudflared tunnel create flareduct-cozy-otter-dev-example-com
# cloudflared tunnel route dns flareduct-cozy-otter-dev-example-com cozy-otter.dev.example.com
# cloudflared tunnel run --url http://localhost:3000 flareduct-cozy-otter-dev-example-com
```

If you prefer the compact hex style, set `random_style: hex` or pass `--random-style hex`; that produces names like `a1b2c3.dev.example.com`.

Pick the subdomain yourself:

```sh
flareduct up 3000 --subdomain demo
# https://demo.dev.example.com
```

Or specify the full hostname:

```sh
flareduct up 3000 --hostname demo.example.com
```

Use `--trycloudflare` to force the old random `trycloudflare.com` behavior even when `public.domain` is configured.

### Cleanup

For owned quick tunnels, `flareduct` cleans up the Cloudflare Tunnel resource when the foreground process exits or when detached tunnels are stopped with `flareduct down`. Add `--keep` if you want to leave resources behind.

DNS record cleanup needs a Cloudflare API token because `cloudflared` can create DNS routes but does not expose a DNS-route delete command. Store it once:

```sh
flareduct token set
```

This saves the token to `~/.config/flareduct/cloudflare-api-token` with `0600` permissions. `CLOUDFLARE_API_TOKEN` / `CF_API_TOKEN` still work and take priority if set.

The token needs `Zone:Read` and `DNS:Edit` for the zone. If `public.domain` is a subdomain, set the zone explicitly:

```yaml
public:
  domain: dev.example.com
  zone: example.com
```

Without a configured token, `flareduct` warns and deletes the tunnel resource only; the DNS hostname may remain in Cloudflare until removed in the dashboard/API.

Cloudflare Access/auth policies are still configured in Cloudflare Zero Trust for the hostname. `flareduct` handles creating/running the tunnel route; Access decides who can open the URL.

## Detached mode

```sh
flareduct up 3000 --detach
flareduct list
flareduct logs port-3000 -f
flareduct down port-3000
```

Detached state is stored under `${XDG_STATE_HOME:-~/.local/state}/flareduct`.

Useful flags:

```sh
flareduct up 3000 --detach --name demo
flareduct up api --detach --replace
flareduct up 3000 --detach --wait 30s
```

## Config aliases

Create a starter config:

```sh
flareduct config init
```

Default path:

```sh
~/.config/flareduct/config.yaml
```

Example:

```yaml
cloudflared: cloudflared

defaults:
  quick_args: []
  static_args: []

public:
  # Uncomment after `flareduct login` to make quick tunnels use
  # owned hostnames instead of random trycloudflare.com URLs.
  # domain: dev.example.com
  # mode: hostname
  # tunnel_prefix: flareduct
  # random_style: words
  # random_words: 2
  # zone: example.com # optional DNS cleanup hint

services:
  web:
    url: http://localhost:3000
    # subdomain: web

  api:
    port: 8080

  blog:
    tunnel: blog
    config: ~/.cloudflared/blog.yml
    public_url: https://blog.example.com
```

Then:

```sh
flareduct up web
flareduct up api --detach
flareduct up blog
```

If no alias matches a non-port target, `flareduct up <name>` falls back to:

```sh
cloudflared tunnel run <name>
```

so existing named tunnels can be run without adding a `flareduct` alias first.

## Commands

```text
flareduct up <port|url|host:port|path|alias|tunnel> [--detach] [--subdomain NAME|--hostname HOST]
flareduct list
flareduct down <name|pid>
flareduct logs <name|pid> [-f]
flareduct login
flareduct token <set|status|path|remove>
flareduct config <path|init|show>
flareduct doctor
```
