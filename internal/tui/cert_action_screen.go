package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

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
	items := []menuItem{
		{title: "View Details"},
		{title: "Renew Certificate (Certbot + Cloudflare)", desc: c.RemainingHuman()},
	}
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

func (s *certActionScreen) View(width, height int) string {
	body := headerStyle.Render("Certificate: "+s.cert.Domain) + "\n"
	body += statusBadge(s.cert.Status) + " " + s.cert.RemainingHuman() + "\n\n"
	body += s.menu.View()
	help := [][2]string{{"↑↓", "Navigate"}, {"Enter", "Select"}, {"Esc", "Back"}}
	return renderFrame(width, height, "GONIX", body, help)
}
