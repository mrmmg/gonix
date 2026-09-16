package acme

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// RunConfig describes one wildcard-certificate request.
type RunConfig struct {
	Domain          string // base domain, e.g. "example.com" (no leading "*.")
	CloudflareToken string
	Email           string // optional; when empty certbot registers without one
	CertbotPath     string // defaults to "certbot"
	SelfPath        string // absolute path to the running gonix binary, used as the hook command
	// ForceRenewal passes --force-renewal, unconditionally reissuing the
	// certificate even if it isn't yet due for renewal. Set this from the
	// certificates screen's explicit "Renew" action; leave it false for a
	// fresh "Get Wildcard Certificate" run, where certbot's own default
	// behavior (skip reissuing a certificate that isn't close to expiring,
	// simply leaving the existing one — archive symlinks and all — in
	// place) is exactly what's wanted.
	ForceRenewal bool
}

// letsEncryptLiveDir is where certbot stores issued certificates, keyed by
// --cert-name. It is a package variable so tests can point it elsewhere.
var letsEncryptLiveDir = "/etc/letsencrypt/live"

// Run drives `certbot certonly` for "domain" and "*.domain" using the DNS-01
// challenge. --manual-auth-hook/--manual-cleanup-hook point back at this same
// gonix binary (see hook.go's RunHook, wired up as the hidden "acme-hook"
// subcommand in cmd/gonix), so the Cloudflare TXT record dance happens
// automatically and certbot never needs an interactive terminal.
//
// certbot's combined stdout+stderr is streamed line by line to onLine as it
// runs (rather than buffered until completion), so a caller can show live
// progress instead of an unresponsive-looking UI during the minutes certbot
// spends waiting for DNS propagation.
func Run(ctx context.Context, cfg RunConfig, onLine func(line string)) error {
	certbotPath := cfg.CertbotPath
	if certbotPath == "" {
		certbotPath = "certbot"
	}
	hook := fmt.Sprintf("%q acme-hook", cfg.SelfPath)

	args := []string{
		"certonly", "--manual",
		"--preferred-challenges", "dns",
		"--manual-auth-hook", hook + " auth",
		"--manual-cleanup-hook", hook + " cleanup",
		// The hook command above is always this same trusted gonix binary,
		// never user-supplied input, so skipping certbot's PATH-based hook
		// validation is safe here. It also matters in practice: SelfPath can
		// be a transient path (e.g. a "go run" temp build) that legitimately
		// exists and is executable but doesn't satisfy certbot's stricter
		// shutil.which()-based check.
		"--disable-hook-validation",
		"--non-interactive",
		"--agree-tos",
		"--cert-name", cfg.Domain,
		"-d", "*." + cfg.Domain,
		"-d", cfg.Domain,
	}
	if cfg.ForceRenewal {
		args = append(args, "--force-renewal")
	}
	if cfg.Email != "" {
		args = append(args, "--email", cfg.Email)
	} else {
		args = append(args, "--register-unsafely-without-email")
	}

	cmd := exec.CommandContext(ctx, certbotPath, args...)
	cmd.Env = append(os.Environ(), "CLOUDFLARE_API_TOKEN="+cfg.CloudflareToken)

	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	scanDone := make(chan struct{})
	go func() {
		defer close(scanDone)
		scanner := bufio.NewScanner(pr)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			if onLine != nil {
				onLine(scanner.Text())
			}
		}
	}()

	if err := cmd.Start(); err != nil {
		pw.Close()
		return fmt.Errorf("starting certbot: %w", err)
	}

	// certbot's manual-auth-hook only reports its own progress once it
	// exits (see hook.go), so most of the several minutes this can take —
	// creating the TXT record and waiting for it to propagate — would
	// otherwise produce no output at all. This heartbeat keeps the caller's
	// live progress view visibly alive in the meantime.
	heartbeatDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		elapsed := 0
		for {
			select {
			case <-heartbeatDone:
				return
			case <-ticker.C:
				elapsed += 15
				if onLine != nil {
					onLine(fmt.Sprintf("... still working (%ds elapsed; mostly spent waiting for DNS propagation)", elapsed))
				}
			}
		}
	}()

	waitErr := cmd.Wait()
	close(heartbeatDone)
	pw.Close()
	<-scanDone
	return waitErr
}

// InstallCertificate copies the fullchain.pem/privkey.pem certbot issued for
// domain from its Let's Encrypt live directory into GoNix's centralized
// certificates directory (certDir/domain/...), making both files
// world-readable (0644) the way internal/certificates.Scan expects.
//
// The live directory's fullchain.pem/privkey.pem are themselves symlinks
// into /etc/letsencrypt/archive/<domain>/ (certbot's actual storage,
// numbered per issuance); os.ReadFile follows them transparently, so this
// works the same whether certbot just issued a brand new certificate or, as
// it does when asked to renew one that isn't due yet, left the existing
// archive entry (and its live/ symlinks) untouched.
func InstallCertificate(domain, certDir string) error {
	src := filepath.Join(letsEncryptLiveDir, domain)
	dst := filepath.Join(certDir, domain)
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", dst, err)
	}
	for _, name := range []string{"fullchain.pem", "privkey.pem"} {
		if err := copyFile(filepath.Join(src, name), filepath.Join(dst, name)); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("reading %s: %w", src, err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", dst, err)
	}
	// WriteFile's mode is subject to umask; force the intended permission.
	if err := os.Chmod(dst, 0o644); err != nil {
		return fmt.Errorf("chmod %s: %w", dst, err)
	}
	return nil
}

// Wildcard runs the full flow: it validates the Cloudflare token and zone up
// front (so a bad token/domain fails fast instead of after certbot has
// already started), runs certbot, and installs the resulting certificate
// into certDir. onProgress receives both discrete stage announcements and
// certbot's own output, one line at a time, so a caller can render live
// progress.
func Wildcard(ctx context.Context, cfg RunConfig, certDir string, onProgress func(string)) error {
	progress := func(s string) {
		if onProgress != nil {
			onProgress(s)
		}
	}

	progress("Verifying Cloudflare API token...")
	client := NewCloudflareClient(cfg.CloudflareToken)
	if err := client.VerifyToken(ctx); err != nil {
		return fmt.Errorf("cloudflare token check failed: %w", err)
	}

	progress("Checking that the domain's zone exists on Cloudflare...")
	if _, err := client.FindZoneID(ctx, cfg.Domain); err != nil {
		return fmt.Errorf("cloudflare zone check failed: %w", err)
	}

	progress("Running certbot (this can take a few minutes while the TXT record propagates)...")
	if err := Run(ctx, cfg, progress); err != nil {
		return fmt.Errorf("certbot failed: %w", err)
	}

	progress("Installing certificate into " + certDir + "...")
	if err := InstallCertificate(cfg.Domain, certDir); err != nil {
		return fmt.Errorf("certbot succeeded but installing the certificate failed: %w", err)
	}
	progress("Done.")
	return nil
}
