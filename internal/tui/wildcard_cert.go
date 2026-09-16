package tui

import (
	"fmt"
	"os"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrmmg/gonix/internal/acme"
	"github.com/mrmmg/gonix/internal/audit"
)

// newWildcardCertWizard drives the "Get Wildcard Certificate (Certbot)"
// submenu under Certificates. It asks for the domain, a Cloudflare API
// token and an optional contact email, then automates the whole DNS-01
// challenge end to end: it verifies the token and the domain's Cloudflare
// zone up front, runs certbot with --manual-auth-hook/--manual-cleanup-hook
// pointed back at this same gonix binary (see internal/acme) so the required
// _acme-challenge TXT records are created, propagation-checked and removed
// via the Cloudflare API automatically, and finally installs the resulting
// fullchain.pem/privkey.pem into the centralized certificates directory. No
// manual DNS editing or keypresses are required.
func newWildcardCertWizard(deps Deps) *wizardScreen {
	fields := []wizardField{
		{Key: "domain", Label: "Base domain (without \"*.\", e.g. example.com):", Kind: fieldText,
			Placeholder: "example.com", Validate: validateWildcardDomain},
		{Key: "token", Label: "Cloudflare API Token (needs Zone:DNS:Edit for this domain's zone):",
			Kind: fieldText, Sensitive: true, Validate: validateCloudflareToken},
		{Key: "email", Label: "Contact email for Let's Encrypt (optional, Enter to skip):", Kind: fieldText},
	}
	return newWizard("Get Wildcard Certificate (Certbot)", fields, func(v map[string]string) (screen, tea.Cmd) {
		domain := strings.TrimSpace(v["domain"])
		token := sanitizeCloudflareToken(v["token"])
		email := strings.TrimSpace(v["email"])
		return newCertRunScreen(deps, domain, token, email, false), nil
	}, func() (screen, tea.Cmd) { return newCertificatesScreen(deps), nil })
}

// newRenewCertWizard drives certificate renewal for an already-known domain
// (the "Renew Certificate" action from the certificates screen). Only the
// Cloudflare token (never persisted between runs) and an optional email are
// asked for; the domain is fixed to the certificate being renewed.
func newRenewCertWizard(deps Deps, domain string) *wizardScreen {
	fields := []wizardField{
		{Key: "token", Label: fmt.Sprintf("Cloudflare API Token (needs Zone:DNS:Edit for %s's zone):", domain),
			Kind: fieldText, Sensitive: true, Validate: validateCloudflareToken},
		{Key: "email", Label: "Contact email for Let's Encrypt (optional, Enter to skip):", Kind: fieldText},
	}
	return newWizard("Renew Wildcard Certificate — "+domain, fields, func(v map[string]string) (screen, tea.Cmd) {
		token := sanitizeCloudflareToken(v["token"])
		email := strings.TrimSpace(v["email"])
		return newCertRunScreen(deps, domain, token, email, true), nil
	}, func() (screen, tea.Cmd) { return newCertificatesScreen(deps), nil })
}

func validateWildcardDomain(v string) error {
	if v == "" {
		return fmt.Errorf("domain is required")
	}
	if strings.Contains(v, "*") {
		return fmt.Errorf("enter the base domain only, without the leading \"*.\"")
	}
	return nil
}

// invisibleRunes are zero-width/bidi-control code points that some
// terminals or IMEs can silently insert while typing or pasting (this is
// especially easy to hit on a system with a Persian/Arabic or other RTL
// keyboard layout active). They are invisible in the masked token field but
// make the resulting Authorization header byte sequence invalid, which
// Cloudflare's API rejects with "6003: Invalid request headers" even though
// the token "looks" right on screen.
var invisibleRunes = map[rune]bool{
	0x200b: true, 0x200c: true, 0x200d: true, // zero-width space/non-joiner/joiner
	0x200e: true, 0x200f: true, // left-to-right / right-to-left marks
	0x202a: true, 0x202b: true, 0x202c: true, 0x202d: true, 0x202e: true, // bidi embedding/override
	0x2066: true, 0x2067: true, 0x2068: true, 0x2069: true, // bidi isolates
	0xfeff: true, // BOM / zero-width no-break space
}

// sanitizeCloudflareToken strips invisible/bidi control characters and
// surrounding whitespace from a pasted or typed API token.
func sanitizeCloudflareToken(v string) string {
	var b strings.Builder
	for _, r := range v {
		if invisibleRunes[r] {
			continue
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

func validateCloudflareToken(v string) error {
	cleaned := sanitizeCloudflareToken(v)
	if cleaned == "" {
		return fmt.Errorf("cloudflare API token is required")
	}
	for _, r := range cleaned {
		if r > unicode.MaxASCII {
			return fmt.Errorf("token contains a non-ASCII character — check that no non-English keyboard layout or IME altered it while typing/pasting")
		}
		if unicode.IsSpace(r) {
			return fmt.Errorf("token must not contain spaces")
		}
	}
	return nil
}

// newCertRunScreen builds the live-progress screen that actually drives
// certbot + Cloudflare (see internal/acme.Wildcard), for both a fresh
// "Get Wildcard Certificate" and a "Renew Certificate" action. When
// forceRenewal is set, a successful run also tests and reloads Nginx
// afterwards, since the certificate's file paths don't change on renewal —
// only Nginx picking the new bytes back up does.
func newCertRunScreen(deps Deps, domain, token, email string, forceRenewal bool) *liveRunScreen {
	title := "Get Wildcard Certificate — " + domain
	action := "wildcard_certificate_issued"
	if forceRenewal {
		title = "Renew Wildcard Certificate — " + domain
		action = "wildcard_certificate_renewed"
	}

	run := func(emit func(string)) error {
		self, err := os.Executable()
		if err != nil {
			return fmt.Errorf("resolving gonix binary path: %w", err)
		}

		cfg := acme.RunConfig{
			Domain: domain, CloudflareToken: token, Email: email,
			SelfPath: self, ForceRenewal: forceRenewal,
		}
		runErr := acme.Wildcard(backgroundCtx(), cfg, deps.Config.Certificates.Directory, emit)

		entry := audit.Entry{User: audit.CurrentUser(), Action: action, Target: domain, Result: audit.ResultSuccess}
		if runErr != nil {
			entry.Result = audit.ResultFailure
			entry.Error = runErr.Error()
		} else if forceRenewal {
			reloadAfterRenewal(deps, emit)
		}
		if deps.Audit != nil {
			_ = deps.Audit.Log(entry)
		}
		return runErr
	}

	return newLiveRunScreen(title, run, func(err error) (screen, tea.Cmd) {
		return newCertificatesScreen(deps), nil
	})
}

// reloadAfterRenewal re-tests and reloads Nginx so a renewed certificate's
// new bytes take effect. The certificate's file path is unchanged by
// renewal, so no host configuration needs to be touched — Nginx just needs
// to reread the file.
func reloadAfterRenewal(deps Deps, emit func(string)) {
	if deps.HostService == nil || deps.HostService.Tester == nil {
		return
	}
	result, err := deps.HostService.Tester.Test(backgroundCtx())
	if err != nil {
		emit("Warning: could not test Nginx configuration after renewal: " + err.Error())
		return
	}
	if !result.OK {
		emit("Warning: Nginx configuration test failed after renewal; reload skipped:\n" + result.Output)
		return
	}
	emit("Nginx configuration test passed.")
	if deps.HostService.Reloader == nil {
		return
	}
	if err := deps.HostService.Reloader.Reload(backgroundCtx()); err != nil {
		emit("Warning: reloading Nginx failed: " + err.Error())
		return
	}
	emit("Nginx reloaded.")
}
