package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mrmmg/gonix/internal/certificates"
)

type certificatesScreen struct {
	deps  Deps
	certs []certificates.Certificate
	menu  *simpleMenu
	err   string
}

func newCertificatesScreen(deps Deps) *certificatesScreen {
	s := &certificatesScreen{deps: deps}
	s.reload()
	return s
}

// sourceTag is the fixed-width, coloured tag that says whose a certificate
// is, so GoNix's own and Let's Encrypt's can never be mistaken for each other.
func sourceTag(c certificates.Certificate) string {
	tag := fmt.Sprintf("%-15s", "["+c.SourceLabel()+"]")
	if c.Source == certificates.SourceCertbot {
		return certbotTagStyle.Render(tag)
	}
	return gonixTagStyle.Render(tag)
}

// remainingText is the time left, in red when it is 14 days or less.
func remainingText(c certificates.Certificate) string {
	if c.NeedsAttention() {
		return dangerStyle.Render("⚠ " + c.Short())
	}
	return c.Short()
}

func (s *certificatesScreen) reload() {
	s.certs = allCerts(s.deps)
	items := []menuItem{
		{title: "+ Get Wildcard Certificate (Certbot)", desc: "Automates certbot + Cloudflare DNS-01", pinned: true},
	}
	w := certNameWidth(s.certs)
	for _, c := range s.certs {
		items = append(items, menuItem{
			title:  statusBadge(c.Status) + " " + fmt.Sprintf("%-*s", w, c.Domain) + "  " + sourceTag(c),
			desc:   string(c.KeyType) + "  " + remainingText(c),
			search: strings.Join(c.SANs, " ") + " " + c.SourceLabel() + " " + c.Source + " " + c.Issuer,
		})
	}
	if len(s.certs) == 0 {
		items = append(items, menuItem{title: "(no certificates found in " + s.deps.Config.Certificates.Directory + " or " + s.deps.Config.Certificates.LetsEncryptDir + "/live)"})
	}
	if s.menu == nil {
		s.menu = newSearchMenu(items, "domain, name, source or issuer")
	} else {
		s.menu.SetItems(items)
	}
}

var (
	gonixTagStyle   = lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
	certbotTagStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("141")).Bold(true)
)

func statusBadge(st certificates.Status) string {
	switch st {
	case certificates.StatusValid:
		return successStyle.Render("●")
	case certificates.StatusExpiringSoon:
		return warningStyle.Render("●")
	case certificates.StatusExpired:
		return dangerStyle.Render("●")
	default:
		return mutedStyle.Render("●")
	}
}

func (s *certificatesScreen) Init() tea.Cmd { return nil }

func (s *certificatesScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		if s.menu.HandleKey(km) {
			return s, nil
		}
		switch km.String() {
		case "esc", "q":
			return s, navPop()
		case "r":
			s.reload()
		case "enter":
			idx := s.menu.Selected()
			if idx < 0 {
				return s, nil
			}
			if idx == 0 {
				return newWildcardCertWizard(s.deps), nil
			}
			certIdx := idx - 1
			if certIdx < 0 || certIdx >= len(s.certs) {
				return s, nil
			}
			c := s.certs[certIdx]
			return newCertActionScreen(s.deps, c), nil
		}
	}
	return s, nil
}

func renderCertDetail(c certificates.Certificate) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Domain:              %s\n", c.Domain)
	fmt.Fprintf(&b, "Source:              %s\n", c.SourceLabel())
	if c.Source == certificates.SourceCertbot {
		if l, ok := certificates.CertbotLineage(c.CertPath); ok {
			auth := l.Authenticator
			if auth == "" {
				auth = "unknown"
			}
			fmt.Fprintf(&b, "Renewed by:          certbot (authenticator: %s), settings in %s\n", auth, l.RenewalFile)
		}
	} else {
		fmt.Fprintf(&b, "Renewed by:          GoNix (certbot + Cloudflare DNS-01)\n")
	}
	fmt.Fprintf(&b, "Status:              %s\n", c.Status)
	fmt.Fprintf(&b, "Key Type:            %s\n", c.KeyType)
	fmt.Fprintf(&b, "Expires (exact):     %s\n", c.ExpiresAtExact())
	fmt.Fprintf(&b, "Expires (relative):  %s\n", c.RemainingHuman())
	fmt.Fprintf(&b, "Issuer:              %s\n", c.Issuer)
	fmt.Fprintf(&b, "Subject:             %s\n", c.Subject)
	fmt.Fprintf(&b, "SANs:                %s\n", strings.Join(c.SANs, ", "))
	fmt.Fprintf(&b, "Fingerprint (SHA-256): %s\n", c.Fingerprint)
	fmt.Fprintf(&b, "Certificate path:    %s\n", c.CertPath)
	fmt.Fprintf(&b, "Private key path:    %s\n", c.KeyPath)
	fmt.Fprintf(&b, "Key matches cert:    %v\n", c.KeyMatches)
	if err := certificates.VerifyChain(c.CertPath); err != nil {
		fmt.Fprintf(&b, "Chain verification:  FAILED (%v)\n", err)
	} else {
		fmt.Fprintf(&b, "Chain verification:  OK\n")
	}
	if c.ParseError != "" {
		fmt.Fprintf(&b, "\nParse error: %s\n", c.ParseError)
	}
	return b.String()
}

func (s *certificatesScreen) View(width, height int) string {
	body := headerStyle.Render("Certificates") + "\n"
	if n := len(certificates.Attention(s.certs)); n > 0 {
		body += callout(width-4, calloutDanger, fmt.Sprintf("%d certificate(s) expired or expiring within %d days — shown in red below",
			n, int(certificates.AttentionWindow.Hours()/24)), nil) + "\n"
	}
	if _, ok := certificates.CertbotPath(); ok {
		body += mutedStyle.Render("certbot found: Let's Encrypt certificates can be renewed from here") + "\n"
	} else {
		body += mutedStyle.Render("certbot not found: Let's Encrypt certificates cannot be renewed from here") + "\n"
	}
	if s.err != "" {
		body += errorBoxStyle.Render(s.err)
	} else {
		body += s.menu.ViewFit(body, height)
	}
	help := searchHelp(s.menu, [][2]string{{"↑↓", "Navigate"}, {"Enter", "Details"}, {"r", "Refresh"}, {"Esc", "Back"}})
	return renderFrame(width, height, "GONIX", body, help)
}
