package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type Client struct {
	token string
	http  *http.Client
}

func NewClient(token string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{token: token, http: httpClient}
}

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type Zone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type DNSRecord struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type Response[T any] struct {
	Success bool    `json:"success"`
	Errors  []Error `json:"errors"`
	Result  T       `json:"result"`
}

func (c *Client) FindZoneID(ctx context.Context, hostname, preferredZone string) (string, error) {
	candidates := zoneCandidates(hostname, preferredZone)
	for _, candidate := range candidates {
		zones, err := c.ListZones(ctx, candidate)
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

func (c *Client) ListZones(ctx context.Context, name string) ([]Zone, error) {
	var response Response[[]Zone]
	path := "/client/v4/zones?name=" + url.QueryEscape(name)
	if err := c.do(ctx, http.MethodGet, path, nil, &response); err != nil {
		return nil, err
	}
	return response.Result, nil
}

func (c *Client) FindDNSRecords(ctx context.Context, zoneID, hostname string) ([]DNSRecord, error) {
	var response Response[[]DNSRecord]
	path := "/client/v4/zones/" + url.PathEscape(zoneID) + "/dns_records?type=CNAME&name=" + url.QueryEscape(hostname)
	if err := c.do(ctx, http.MethodGet, path, nil, &response); err != nil {
		return nil, err
	}
	return response.Result, nil
}

func (c *Client) DeleteDNSRecord(ctx context.Context, zoneID, recordID string) error {
	var response Response[map[string]any]
	path := "/client/v4/zones/" + url.PathEscape(zoneID) + "/dns_records/" + url.PathEscape(recordID)
	return c.do(ctx, http.MethodDelete, path, nil, &response)
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
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
		return fmt.Errorf("Cloudflare API %s %s returned %s: %s", method, path, resp.Status, formatError(out))
	}
	if !success(out) {
		return fmt.Errorf("Cloudflare API %s %s failed: %s", method, path, formatError(out))
	}
	return nil
}

func success(out any) bool {
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

func formatError(out any) string {
	data, err := json.Marshal(out)
	if err != nil {
		return "unknown error"
	}
	var meta struct {
		Errors []Error `json:"errors"`
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
