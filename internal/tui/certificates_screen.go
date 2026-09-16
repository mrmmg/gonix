package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

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

func (s *certificatesScreen) reload() {
	certs, err := certificates.Scan(s.deps.Config.Certificates.Directory)
	if err != nil {
		s.err = err.Error()
		return
	}
	s.certs = certs
	items := []menuItem{
		{title: "+ Get Wildcard Certificate (Certbot)", desc: "Automates certbot + Cloudflare DNS-01"},
	}
	for _, c := range certs {
		items = append(items, menuItem{title: statusBadge(c.Status) + " " + c.Domain, desc: string(c.KeyType) + "  " + c.RemainingHuman()})
	}
	if len(certs) == 0 {
		items = append(items, menuItem{title: "(no certificates found in " + s.deps.Config.Certificates.Directory + ")"})
	}
	s.menu = newSimpleMenu(items)
}

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
	if s.err != "" {
		body += errorBoxStyle.Render(s.err)
	} else {
		body += s.menu.View()
	}
	help := [][2]string{{"↑↓", "Navigate"}, {"Enter", "Details"}, {"r", "Refresh"}, {"Esc", "Back"}}
	return renderFrame(width, height, "GONIX", body, help)
}
