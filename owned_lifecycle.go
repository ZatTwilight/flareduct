package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

type ProvisionResult struct {
	CreatedTunnel bool
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

func ProvisionOwnedTunnel(spec TunnelSpec, stdout, stderr io.Writer) (ProvisionResult, error) {
	var result ProvisionResult
	if spec.Hostname == "" || spec.TunnelName == "" {
		return result, nil
	}
	if len(spec.Command) == 0 {
		return result, fmt.Errorf("empty command")
	}
	cloudflared := spec.Command[0]

	exists, err := tunnelExists(cloudflared, spec.TunnelName)
	if err != nil {
		return result, err
	}
	if exists {
		fmt.Fprintf(stdout, "flareduct: tunnel %s already exists; auto-cleanup will not delete existing resources\n", spec.TunnelName)
	} else {
		createCmd := []string{cloudflared, "tunnel", "create", spec.TunnelName}
		if err := runProvisionCommand("creating tunnel "+spec.TunnelName, createCmd, stdout, stderr, spec.Verbose); err != nil {
			return result, err
		}
		result.CreatedTunnel = true
	}

	routeCmd := []string{cloudflared, "tunnel", "route", "dns"}
	if spec.OverwriteDNS {
		routeCmd = append(routeCmd, "--overwrite-dns")
	}
	routeCmd = append(routeCmd, spec.TunnelName, spec.Hostname)
	if err := runProvisionCommand("routing DNS "+spec.Hostname, routeCmd, stdout, stderr, spec.Verbose); err != nil {
		if result.CreatedTunnel {
			cleanupOwnedTunnelBestEffort(spec, result, stdout, stderr)
		}
		return result, err
	}
	return result, nil
}

func tunnelExists(cloudflared, tunnelName string) (bool, error) {
	cmd := exec.Command(cloudflared, "tunnel", "info", tunnelName)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	if _, ok := err.(*exec.ExitError); ok {
		return false, nil
	}
	return false, err
}

func runProvisionCommand(label string, args []string, stdout, stderr io.Writer, verbose bool) error {
	fmt.Fprintf(stdout, "flareduct: %s\n", label)
	if verbose {
		fmt.Fprintf(stdout, "flareduct: running %s\n", ShellQuote(args))
	}
	cmd := exec.Command(args[0], args[1:]...)
	if verbose {
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("provisioning failed for %s: %w", ShellQuote(args), err)
		}
		return nil
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		if len(out) > 0 {
			fmt.Fprint(stderr, string(out))
		}
		return fmt.Errorf("provisioning failed for %s: %w", ShellQuote(args), err)
	}
	return nil
}

func cleanupOwnedTunnelBestEffort(spec TunnelSpec, provision ProvisionResult, stdout, stderr io.Writer) {
	if err := CleanupOwnedTunnel(spec, provision, stdout, stderr); err != nil {
		fmt.Fprintf(stderr, "flareduct: cleanup warning: %v\n", err)
	}
}

func specFromStateEntry(entry StateEntry) TunnelSpec {
	return TunnelSpec{
		Name:        entry.Name,
		Target:      entry.Target,
		Kind:        entry.Kind,
		URL:         entry.URL,
		Hostname:    entry.Hostname,
		TunnelName:  entry.TunnelName,
		PublicURL:   entry.PublicURL,
		AutoCleanup: entry.AutoCleanup,
		Zone:        entry.Zone,
		ZoneID:      entry.ZoneID,
		Command:     entry.Command,
	}
}

func CleanupOwnedTunnel(spec TunnelSpec, provision ProvisionResult, stdout, stderr io.Writer) error {
	if spec.Hostname == "" || spec.TunnelName == "" || !spec.AutoCleanup || !provision.CreatedTunnel {
		return nil
	}
	if len(spec.Command) == 0 {
		return fmt.Errorf("empty command")
	}
	cloudflared := spec.Command[0]

	fmt.Fprintf(stdout, "flareduct: cleaning up %s\n", spec.PublicURL)
	if err := cleanupDNSRecord(spec, stdout, stderr); err != nil {
		fmt.Fprintf(stderr, "flareduct: DNS cleanup warning: %v\n", err)
	}

	deleteCmd := []string{cloudflared, "tunnel", "delete", "-f", spec.TunnelName}
	fmt.Fprintf(stdout, "flareduct: cleanup %s\n", ShellQuote(deleteCmd))
	cmd := execCommand(deleteCmd, stdout, stderr)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("delete tunnel %s: %w", spec.TunnelName, err)
	}
	return nil
}

func cleanupDNSRecord(spec TunnelSpec, stdout, stderr io.Writer) error {
	token, source, ok, err := LoadCloudflareAPIToken()
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no Cloudflare API token configured; run `flareduct token set` or set CLOUDFLARE_API_TOKEN; DNS record %s may remain", spec.Hostname)
	}
	fmt.Fprintf(stdout, "flareduct: using Cloudflare API token from %s for DNS cleanup\n", source)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client := &cloudflareClient{token: token, http: http.DefaultClient}

	zoneID := spec.ZoneID
	if zoneID == "" {
		var err error
		zoneID, err = client.findZoneID(ctx, spec.Hostname, spec.Zone)
		if err != nil {
			return err
		}
	}

	records, err := client.findDNSRecords(ctx, zoneID, spec.Hostname)
	if err != nil {
		return err
	}
	if len(records) == 0 {
		fmt.Fprintf(stdout, "flareduct: DNS record %s was already gone\n", spec.Hostname)
		return nil
	}
	for _, record := range records {
		fmt.Fprintf(stdout, "flareduct: deleting DNS record %s (%s)\n", record.Name, record.Type)
		if err := client.deleteDNSRecord(ctx, zoneID, record.ID); err != nil {
			return err
		}
	}
	return nil
}

type commandRunner interface {
	Run() error
}

var execCommand = func(args []string, stdout, stderr io.Writer) commandRunner {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd
}

type cloudflareClient struct {
	token string
	http  *http.Client
}

type cfError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cfZone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type cfDNSRecord struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type cfResponse[T any] struct {
	Success bool      `json:"success"`
	Errors  []cfError `json:"errors"`
	Result  T         `json:"result"`
}

func (c *cloudflareClient) findZoneID(ctx context.Context, hostname, preferredZone string) (string, error) {
	candidates := zoneCandidates(hostname, preferredZone)
	for _, candidate := range candidates {
		zones, err := c.listZones(ctx, candidate)
		if err != nil {
			return "", err
		}
		if len(zones) > 0 {
			return zones[0].ID, nil
		}
	}
	return "", fmt.Errorf("could not find Cloudflare zone for %s; set public.zone_id or public.zone", hostname)
}

func zoneCandidates(hostname, preferredZone string) []string {
	seen := map[string]bool{}
	var candidates []string
	add := func(s string) {
		s = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(s), "."))
		if s != "" && !seen[s] {
			seen[s] = true
			candidates = append(candidates, s)
		}
	}
	add(preferredZone)
	labels := strings.Split(strings.ToLower(strings.TrimSuffix(hostname, ".")), ".")
	for i := 1; i < len(labels)-1; i++ {
		add(strings.Join(labels[i:], "."))
	}
	return candidates
}

func (c *cloudflareClient) listZones(ctx context.Context, name string) ([]cfZone, error) {
	var response cfResponse[[]cfZone]
	path := "/client/v4/zones?name=" + url.QueryEscape(name)
	if err := c.do(ctx, http.MethodGet, path, nil, &response); err != nil {
		return nil, err
	}
	return response.Result, nil
}

func (c *cloudflareClient) findDNSRecords(ctx context.Context, zoneID, hostname string) ([]cfDNSRecord, error) {
	var response cfResponse[[]cfDNSRecord]
	path := "/client/v4/zones/" + url.PathEscape(zoneID) + "/dns_records?type=CNAME&name=" + url.QueryEscape(hostname)
	if err := c.do(ctx, http.MethodGet, path, nil, &response); err != nil {
		return nil, err
	}
	return response.Result, nil
}

func (c *cloudflareClient) deleteDNSRecord(ctx context.Context, zoneID, recordID string) error {
	var response cfResponse[map[string]any]
	path := "/client/v4/zones/" + url.PathEscape(zoneID) + "/dns_records/" + url.PathEscape(recordID)
	return c.do(ctx, http.MethodDelete, path, nil, &response)
}

func (c *cloudflareClient) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, "https://api.cloudflare.com"+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("Cloudflare API returned invalid JSON: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Cloudflare API %s %s returned %s: %s", method, path, resp.Status, cloudflareErrorMessage(out))
	}
	if !cloudflareSuccess(out) {
		return fmt.Errorf("Cloudflare API %s %s failed: %s", method, path, cloudflareErrorMessage(out))
	}
	return nil
}

func cloudflareSuccess(out any) bool {
	data, err := json.Marshal(out)
	if err != nil {
		return false
	}
	var meta struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return false
	}
	return meta.Success
}

func cloudflareErrorMessage(out any) string {
	data, err := json.Marshal(out)
	if err != nil {
		return "unknown error"
	}
	var meta struct {
		Errors []cfError `json:"errors"`
	}
	if err := json.Unmarshal(data, &meta); err != nil || len(meta.Errors) == 0 {
		return "unknown error"
	}
	parts := make([]string, 0, len(meta.Errors))
	for _, e := range meta.Errors {
		parts = append(parts, e.Message)
	}
	return strings.Join(parts, "; ")
}
