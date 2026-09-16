package acme

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

// dnsPropagationCheckServer is queried directly (bypassing any local
// resolver cache) to confirm a freshly created TXT record is visible before
// letting certbot ask Let's Encrypt to validate it. Using Cloudflare's own
// resolver keeps this consistent with the DNS provider being automated.
const dnsPropagationCheckServer = "1.1.1.1:53"

// RunHook implements the certbot manual-auth-hook / manual-cleanup-hook
// contract for the DNS-01 challenge. Certbot invokes
// "<gonix binary> acme-hook auth" (or "cleanup") as a subprocess for each
// domain being validated, setting CERTBOT_DOMAIN and CERTBOT_VALIDATION in
// its environment; CLOUDFLARE_API_TOKEN is set by Run (see certbot.go) and
// inherited down to this subprocess.
//
// "auth" creates the _acme-challenge TXT record and blocks until it is
// visible on the public DNS — replacing the interactive "press Enter once
// the TXT record is set" step from a manual certbot run. "cleanup" removes
// the record(s) it created.
func RunHook(ctx context.Context, action string) error {
	domain := os.Getenv("CERTBOT_DOMAIN")
	validation := os.Getenv("CERTBOT_VALIDATION")
	token := os.Getenv("CLOUDFLARE_API_TOKEN")
	if domain == "" || validation == "" || token == "" {
		return fmt.Errorf("acme-hook: missing CERTBOT_DOMAIN, CERTBOT_VALIDATION or CLOUDFLARE_API_TOKEN in environment")
	}
	domain = strings.TrimPrefix(domain, "*.")
	recordName := "_acme-challenge." + domain

	client := NewCloudflareClient(token)
	zoneID, err := client.FindZoneID(ctx, domain)
	if err != nil {
		return err
	}

	switch action {
	case "auth":
		fmt.Printf("acme-hook: creating TXT record %s via Cloudflare API\n", recordName)
		if _, err := client.CreateTXT(ctx, zoneID, recordName, validation); err != nil {
			return fmt.Errorf("creating TXT record %s: %w", recordName, err)
		}
		fmt.Printf("acme-hook: waiting for %s to propagate on public DNS...\n", recordName)
		if err := waitForTXT(ctx, recordName, validation); err != nil {
			return err
		}
		fmt.Printf("acme-hook: %s propagated, continuing\n", recordName)
		return nil
	case "cleanup":
		recs, err := client.ListTXT(ctx, zoneID, recordName)
		if err != nil {
			return fmt.Errorf("listing TXT records %s: %w", recordName, err)
		}
		for _, r := range recs {
			if r.Content != validation {
				continue
			}
			if err := client.DeleteTXT(ctx, zoneID, r.ID); err != nil {
				return fmt.Errorf("deleting TXT record %s: %w", recordName, err)
			}
		}
		fmt.Printf("acme-hook: removed TXT record %s\n", recordName)
		return nil
	default:
		return fmt.Errorf("acme-hook: unknown action %q (expected \"auth\" or \"cleanup\")", action)
	}
}

// waitForTXT polls the public DNS for name until value shows up among its
// TXT records, or propagationTimeout elapses.
func waitForTXT(ctx context.Context, name, value string) error {
	const (
		propagationTimeout = 5 * time.Minute
		pollInterval       = 5 * time.Second
	)
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: 5 * time.Second}
			return d.DialContext(ctx, network, dnsPropagationCheckServer)
		},
	}

	deadline := time.Now().Add(propagationTimeout)
	for {
		values, err := resolver.LookupTXT(ctx, name)
		if err == nil {
			for _, v := range values {
				if v == value {
					// A short extra margin so other resolvers Let's Encrypt
					// may use have a chance to catch up too.
					time.Sleep(10 * time.Second)
					return nil
				}
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for TXT record %s to propagate", name)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}
