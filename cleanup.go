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
