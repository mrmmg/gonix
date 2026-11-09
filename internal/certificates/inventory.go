package certificates

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// AttentionWindow is how little validity a certificate may have left before
// GoNix flags it in red everywhere (the main menu included).
const AttentionWindow = 14 * 24 * time.Hour

// SourceCustom is a certificate that lives neither in GoNix's directory nor
// in certbot's: a path some host file points at, renewed by whoever put it
// there.
const SourceCustom = "custom"

// All lists the certificates of both stores: GoNix's own directory first,
// then certbot's live directory (leDir may be empty to skip it).
func All(certDir, leDir string) []Certificate {
	certs, _ := Scan(certDir)
	if leDir != "" {
		certs = append(certs, ScanCertbot(leDir)...)
	}
	return certs
}

// SourceOf says where the certificate file certPath lives.
func SourceOf(certPath, certDir, leDir string) string {
	dir := filepath.Dir(filepath.Dir(filepath.Clean(certPath)))
	switch {
	case certDir != "" && dir == filepath.Clean(certDir):
		return SourceGoNix
	case leDir != "" && dir == filepath.Join(filepath.Clean(leDir), "live"):
		return SourceCertbot
	}
	return SourceCustom
}

// SourceName is the label of a source, as shown to people.
func SourceName(source string) string {
	switch source {
	case SourceGoNix:
		return "GoNix"
	case SourceCertbot:
		return "Let's Encrypt"
	}
	return "Custom"
}

// SourceLabel is SourceName for this certificate.
func (c Certificate) SourceLabel() string { return SourceName(c.Source) }

// DaysLeft is the whole days until expiry, negative once expired.
func (c Certificate) DaysLeft() int {
	d := time.Until(c.NotAfter)
	if d <= 0 {
		return -int((-d).Hours() / 24)
	}
	return int(d.Hours() / 24)
}

// NeedsAttention reports whether c is expired, unreadable, or has
// AttentionWindow or less validity left.
func (c Certificate) NeedsAttention() bool {
	if c.Status == StatusInvalid {
		return true
	}
	return time.Until(c.NotAfter) <= AttentionWindow
}

// Short is the time left in a few characters: "42d left", "5d left",
// "9h left", "expired 3d ago".
func (c Certificate) Short() string {
	if c.Status == StatusInvalid {
		return "unreadable"
	}
	d := time.Until(c.NotAfter)
	switch {
	case d <= 0:
		if days := int((-d).Hours() / 24); days > 0 {
			return fmt.Sprintf("expired %dd ago", days)
		}
		return "expired today"
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh left", int(d.Hours()))
	}
	return fmt.Sprintf("%dd left", int(d.Hours()/24))
}

// Attention describes, for the certificates that need it, what is wrong, one
// entry per certificate: "example.com (Let's Encrypt): 5d left".
func Attention(certs []Certificate) []string {
	var out []string
	for _, c := range certs {
		if c.NeedsAttention() {
			out = append(out, fmt.Sprintf("%s (%s): %s", c.Domain, c.SourceLabel(), c.Short()))
		}
	}
	return out
}

// CertbotPath finds the certbot executable.
func CertbotPath() (string, bool) {
	p, err := exec.LookPath("certbot")
	return p, err == nil
}

// RenewCertbot renews the certbot certificate name with `certbot renew
// --cert-name`. Without force, certbot itself skips a certificate that is
// not yet due. Its output is passed to onLine as it arrives.
func RenewCertbot(ctx context.Context, certbot, leDir, name string, force bool, onLine func(string)) error {
	args := []string{"renew", "--cert-name", name, "--non-interactive"}
	if leDir != "" && leDir != DefaultLetsEncryptDir {
		args = append(args, "--config-dir", leDir)
	}
	if force {
		args = append(args, "--force-renewal")
	}
	cmd := exec.CommandContext(ctx, certbot, args...)
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	done := make(chan struct{})
	go func() {
		defer close(done)
		sc := bufio.NewScanner(pr)
		for sc.Scan() {
			if onLine != nil {
				onLine(strings.TrimRight(sc.Text(), "\r"))
			}
		}
	}()
	if err := cmd.Start(); err != nil {
		pw.Close()
		return fmt.Errorf("starting certbot: %w", err)
	}
	err := cmd.Wait()
	pw.Close()
	<-done
	if err != nil {
		return fmt.Errorf("certbot renew failed: %w", err)
	}
	return nil
}
