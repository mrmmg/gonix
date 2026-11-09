package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrmmg/gonix/internal/audit"
	"github.com/mrmmg/gonix/internal/certificates"
)

// certActionScreen is the per-certificate submenu opened from the
// Certificates screen: inspect it, or renew it via certbot + Cloudflare.
type certActionScreen struct {
	deps Deps
	cert certificates.Certificate
	menu *simpleMenu
}

const (
	caViewDetails = iota
	caRenew
)

func newCertActionScreen(deps Deps, c certificates.Certificate) *certActionScreen {
	renew := menuItem{title: "Renew Certificate (Certbot + Cloudflare)", desc: c.RemainingHuman()}
	if c.Source == certificates.SourceCertbot {
		// Let's Encrypt's certificates are renewed by certbot itself, with
		// the settings they were issued with.
		renew = menuItem{title: "Renew Certificate (certbot)", desc: c.RemainingHuman()}
		if _, ok := certificates.CertbotPath(); !ok {
			renew.desc = mutedStyle.Render("certbot was not found on this system")
		}
	}
	items := []menuItem{{title: "View Details"}, renew}
	return &certActionScreen{deps: deps, cert: c, menu: newSimpleMenu(items)}
}

func (s *certActionScreen) Init() tea.Cmd { return nil }

func (s *certActionScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		if s.menu.HandleKey(km) {
			return s, nil
		}
		switch km.String() {
		case "esc", "q":
			return newCertificatesScreen(s.deps), nil
		case "enter":
			switch s.menu.Selected() {
			case caViewDetails:
				return newViewerScreen("Certificate: "+s.cert.Domain, renderCertDetail(s.cert), s), nil
			case caRenew:
				return s.startRenew()
			}
		}
	}
	return s, nil
}

// startRenew warns before renewing a certificate that isn't due yet
// (certbot/Let's Encrypt only recommend renewing within the last 30 days of
// validity — see certificates.RenewalWindow), since renewing needlessly
// early just burns into Let's Encrypt's rate limits for no benefit. It's
// still allowed on confirmation, e.g. for a domain whose DNS setup changed.
func (s *certActionScreen) startRenew() (screen, tea.Cmd) {
	if s.cert.Source == certificates.SourceCertbot {
		return s.startCertbotRenew()
	}
	if !s.cert.DueForRenewal() {
		remainingDays := int(time.Until(s.cert.NotAfter).Hours() / 24)
		windowDays := int(certificates.RenewalWindow.Hours() / 24)
		return newConfirmScreen(
			"Renew Certificate",
			fmt.Sprintf("%s is not due for renewal for about %d more days (renewing is recommended only within the last %d days, to avoid needlessly burning into Let's Encrypt's rate limits). Renew anyway?",
				s.cert.Domain, remainingDays, windowDays),
			func() (screen, tea.Cmd) { return newRenewCertWizard(s.deps, s.cert.Domain), nil },
			func() (screen, tea.Cmd) { return s, nil },
		), nil
	}
	return newRenewCertWizard(s.deps, s.cert.Domain), nil
}

// startCertbotRenew renews a Let's Encrypt certificate with certbot. When it
// is not due yet, certbot would skip it, so renewing anyway is a separate,
// confirmed choice.
func (s *certActionScreen) startCertbotRenew() (screen, tea.Cmd) {
	certbot, ok := certificates.CertbotPath()
	if !ok {
		return newResultScreen(s.deps, "Renew Certificate", false,
			"certbot was not found on this system, so GoNix cannot renew "+s.cert.Domain+". Install certbot, or renew it with whatever issued it.", s), nil
	}
	if s.cert.DueForRenewal() {
		return newCertbotRenewScreen(s.deps, certbot, s.cert, false), nil
	}
	return newConfirmScreen(
		"Renew Certificate",
		fmt.Sprintf("%s has %s and is not due for renewal yet; certbot would skip it. Renew it now anyway? (Let's Encrypt limits how often certificates can be reissued.)",
			s.cert.Domain, s.cert.Short()),
		func() (screen, tea.Cmd) { return newCertbotRenewScreen(s.deps, certbot, s.cert, true), nil },
		func() (screen, tea.Cmd) { return s, nil },
	), nil
}

// newCertbotRenewScreen runs `certbot renew --cert-name` with live output,
// then makes Nginx pick the renewed files up.
func newCertbotRenewScreen(deps Deps, certbot string, c certificates.Certificate, force bool) *liveRunScreen {
	run := func(emit func(string)) error {
		l, _ := certificates.CertbotLineage(c.CertPath)
		name := l.Name
		if name == "" {
			name = c.Domain
		}
		err := certificates.RenewCertbot(backgroundCtx(), certbot, deps.Config.Certificates.LetsEncryptDir, name, force, emit)
		entry := audit.Entry{User: audit.CurrentUser(), Action: "certbot_certificate_renewed", Target: name, Result: audit.ResultSuccess}
		if err != nil {
			entry.Result, entry.Error = audit.ResultFailure, err.Error()
		} else {
			reloadAfterRenewal(deps, emit)
		}
		if deps.Audit != nil {
			_ = deps.Audit.Log(entry)
		}
		return err
	}
	return newLiveRunScreen("Renew Certificate (certbot) — "+c.Domain, run, func(err error) (screen, tea.Cmd) {
		return newCertificatesScreen(deps), nil
	})
}

func (s *certActionScreen) View(width, height int) string {
	body := headerStyle.Render("Certificate: "+s.cert.Domain) + "\n"
	body += sourceTag(s.cert) + "\n"
	body += statusBadge(s.cert.Status) + " " + s.cert.RemainingHuman() + "\n\n"
	body += s.menu.ViewFit(body, height)
	help := [][2]string{{"↑↓", "Navigate"}, {"Enter", "Select"}, {"Esc", "Back"}}
	return renderFrame(width, height, "GONIX", body, help)
}
