// Package acme automates obtaining a wildcard TLS certificate with certbot's
// DNS-01 challenge, using the Cloudflare API to create, verify and remove the
// required _acme-challenge TXT records. Certbot's interactive
// "press Enter once the TXT record is set" step is replaced by
// --manual-auth-hook/--manual-cleanup-hook scripts (this same gonix binary,
// see hook.go) that certbot runs itself, so no manual DNS editing or
// keypresses are required.
package acme

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const cloudflareAPIBase = "https://api.cloudflare.com/client/v4"

// CloudflareClient is a minimal Cloudflare API v4 client covering only the
// token verification and DNS record operations this package needs.
type CloudflareClient struct {
	Token      string
	HTTPClient *http.Client
	BaseURL    string // defaults to cloudflareAPIBase; overridable in tests
}

// NewCloudflareClient returns a client authenticating with the given API
// token (needs Zone:DNS:Edit permission on the target zone).
func NewCloudflareClient(token string) *CloudflareClient {
	return &CloudflareClient{Token: token, HTTPClient: &http.Client{Timeout: 30 * time.Second}, BaseURL: cloudflareAPIBase}
}

type cfResponse struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	Result json.RawMessage `json:"result"`
}

func (c *CloudflareClient) do(ctx context.Context, method, path string, body any) (cfResponse, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return cfResponse{}, fmt.Errorf("encoding cloudflare request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reqBody)
	if err != nil {
		return cfResponse{}, fmt.Errorf("building cloudflare request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return cfResponse{}, fmt.Errorf("calling cloudflare api: %w", err)
	}
	defer resp.Body.Close()

	var out cfResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return cfResponse{}, fmt.Errorf("decoding cloudflare response: %w", err)
	}
	if !out.Success {
		var msgs []string
		for _, e := range out.Errors {
			msgs = append(msgs, fmt.Sprintf("%d: %s", e.Code, e.Message))
		}
		if len(msgs) == 0 {
			msgs = []string{fmt.Sprintf("http %d", resp.StatusCode)}
		}
		return cfResponse{}, fmt.Errorf("cloudflare api error: %s", strings.Join(msgs, "; "))
	}
	return out, nil
}

// VerifyToken checks that the token is well-formed and active.
func (c *CloudflareClient) VerifyToken(ctx context.Context) error {
	_, err := c.do(ctx, http.MethodGet, "/user/tokens/verify", nil)
	return err
}

type cfZone struct {
	ID string `json:"id"`
}

// FindZoneID resolves the Cloudflare zone ID that owns domain, trying
// progressively shorter suffixes so a host under a zone (not just the zone
// apex) also resolves correctly.
func (c *CloudflareClient) FindZoneID(ctx context.Context, domain string) (string, error) {
	labels := strings.Split(strings.TrimPrefix(domain, "*."), ".")
	for i := 0; i < len(labels)-1; i++ {
		candidate := strings.Join(labels[i:], ".")
		out, err := c.do(ctx, http.MethodGet, "/zones?name="+candidate, nil)
		if err != nil {
			return "", err
		}
		var zones []cfZone
		if err := json.Unmarshal(out.Result, &zones); err != nil {
			return "", fmt.Errorf("decoding cloudflare zones: %w", err)
		}
		if len(zones) > 0 {
			return zones[0].ID, nil
		}
	}
	return "", fmt.Errorf("no cloudflare zone found for domain %q (is it added to this account?)", domain)
}

type cfDNSRecord struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

// CreateTXT creates a new TXT record and returns its Cloudflare record ID.
func (c *CloudflareClient) CreateTXT(ctx context.Context, zoneID, name, content string) (string, error) {
	body := map[string]any{"type": "TXT", "name": name, "content": content, "ttl": 120}
	out, err := c.do(ctx, http.MethodPost, "/zones/"+zoneID+"/dns_records", body)
	if err != nil {
		return "", err
	}
	var rec cfDNSRecord
	if err := json.Unmarshal(out.Result, &rec); err != nil {
		return "", fmt.Errorf("decoding cloudflare dns record: %w", err)
	}
	return rec.ID, nil
}

// ListTXT returns every TXT record at name in zoneID.
func (c *CloudflareClient) ListTXT(ctx context.Context, zoneID, name string) ([]cfDNSRecord, error) {
	out, err := c.do(ctx, http.MethodGet, "/zones/"+zoneID+"/dns_records?type=TXT&name="+name, nil)
	if err != nil {
		return nil, err
	}
	var recs []cfDNSRecord
	if err := json.Unmarshal(out.Result, &recs); err != nil {
		return nil, fmt.Errorf("decoding cloudflare dns records: %w", err)
	}
	return recs, nil
}

// DeleteTXT removes a previously created TXT record.
func (c *CloudflareClient) DeleteTXT(ctx context.Context, zoneID, recordID string) error {
	_, err := c.do(ctx, http.MethodDelete, "/zones/"+zoneID+"/dns_records/"+recordID, nil)
	return err
}
