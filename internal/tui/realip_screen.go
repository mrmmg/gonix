package tui

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrmmg/gonix/internal/iplist"
	"github.com/mrmmg/gonix/internal/nginx"
	"github.com/mrmmg/gonix/internal/realip"
)

// realIPScreen manages the http-level real client IP settings used when
// Nginx sits behind Cloudflare or another proxy.
type realIPScreen struct {
	deps     Deps
	settings realip.Settings
	enabled  bool
	menu     *simpleMenu
}

const (
	riCloudflare = iota
	riCloudflareRefresh
	riCustom
	riView
	riDisable
)

func newRealIPScreen(deps Deps) *realIPScreen {
	s := &realIPScreen{deps: deps}
	s.settings, s.enabled, _ = deps.RealIP.Get()
	s.menu = newSimpleMenu([]menuItem{
		{title: "Use Cloudflare", desc: "trust Cloudflare's edge ranges and CF-Connecting-IP"},
		{title: "Use Cloudflare (download latest ranges)", desc: "fetch www.cloudflare.com/ips"},
		{title: "Custom Proxy / Load Balancer", desc: "your own trusted proxies and header"},
		{title: "View Configuration"},
		{title: "Disable"},
	})
	return s
}

func (s *realIPScreen) Init() tea.Cmd { return nil }

func (s *realIPScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		if s.menu.HandleKey(km) {
			return s, nil
		}
		switch km.String() {
		case "esc", "q":
			return s, navPop()
		case "enter":
			return s.dispatch(s.menu.Selected())
		}
	}
	return s, nil
}

func (s *realIPScreen) dispatch(choice int) (screen, tea.Cmd) {
	switch choice {
	case riCloudflare:
		return s.save(realip.Settings{Cloudflare: true, Header: "CF-Connecting-IP", Proxies: realip.CloudflareRanges})
	case riCloudflareRefresh:
		ctx, cancel := context.WithTimeout(backgroundCtx(), 15*time.Second)
		defer cancel()
		ranges, err := realip.FetchCloudflareRanges(ctx, &http.Client{Timeout: 15 * time.Second})
		if err != nil {
			return newResultScreen(s.deps, "Real Client IP", false,
				err.Error()+"\n\nThe built-in Cloudflare ranges can be used offline with \"Use Cloudflare\".", s), nil
		}
		return s.save(realip.Settings{Cloudflare: true, Header: "CF-Connecting-IP", Proxies: ranges})
	case riCustom:
		return newRealIPCustomWizard(s.deps, s.settings), nil
	case riView:
		if !s.enabled {
			return newResultScreen(s.deps, "Real Client IP", false, "Real client IP handling is not enabled.", s), nil
		}
		data, err := os.ReadFile(s.deps.RealIP.Path())
		if err != nil {
			return newResultScreen(s.deps, "Real Client IP", false, err.Error(), s), nil
		}
		return newViewerScreen(s.deps.RealIP.Path(), string(data), s), nil
	case riDisable:
		if !s.enabled {
			return s, nil
		}
		return newConfirmScreen("Disable Real Client IP",
			"Stop trusting proxy headers? IP rules and logs will see the proxy's address again.",
			func() (screen, tea.Cmd) {
				if err := s.deps.RealIP.Disable(backgroundCtx()); err != nil {
					return newResultScreen(s.deps, "Real Client IP", false, err.Error(), newRealIPScreen(s.deps)), nil
				}
				return newResultScreen(s.deps, "Real Client IP", true, "Real client IP handling disabled.", newRealIPScreen(s.deps)), nil
			},
			func() (screen, tea.Cmd) { return s, nil },
		), nil
	}
	return s, nil
}

func (s *realIPScreen) save(settings realip.Settings) (screen, tea.Cmd) {
	if err := s.deps.RealIP.Save(backgroundCtx(), settings); err != nil {
		return newResultScreen(s.deps, "Real Client IP", false, err.Error(), newRealIPScreen(s.deps)), nil
	}
	msg := fmt.Sprintf("Client addresses are now taken from %s for requests from %d trusted ranges.",
		settings.Header, len(settings.Proxies))
	return newResultScreen(s.deps, "Real Client IP", true, msg+includeWarning(s.deps), newRealIPScreen(s.deps)), nil
}

// includeWarning explains, when nginx.conf does not include the http-level
// snippets directory, that files GoNix writes there have no effect.
func includeWarning(deps Deps) string {
	dir := deps.Config.Nginx.HTTPSnippetsDir
	ok, err := nginx.IncludesDir(deps.Config.Nginx.ConfigDir, dir)
	if err != nil || ok {
		return ""
	}
	return fmt.Sprintf("\n\nWarning: %s/nginx.conf does not seem to include %s, so this setting has no effect yet. "+
		"Add `include %s/*.conf;` inside its http { } block, or change nginx.http_snippets_dir in /etc/gonix/gonix.yaml.",
		deps.Config.Nginx.ConfigDir, dir, dir)
}

func (s *realIPScreen) View(width, height int) string {
	body := headerStyle.Render("Real Client IP") + "\n"
	body += mutedStyle.Render("Behind Cloudflare or a load balancer, every request appears to come from the proxy.\n"+
		"Trusting the proxy's header restores the real client address for IP rules, rate limits and logs.") + "\n\n"
	if s.enabled {
		source := "custom proxies"
		if s.settings.Cloudflare {
			source = "Cloudflare"
		}
		body += fmt.Sprintf("%s ENABLED  %s, header %s, %d trusted ranges\n", enabledDot, source, s.settings.Header, len(s.settings.Proxies))
	} else {
		body += disabledDot + " DISABLED\n"
	}
	if w := includeWarning(s.deps); w != "" {
		body += errorBoxStyle.Render(strings.TrimSpace(w)) + "\n"
	}
	body += "\n" + s.menu.View()
	help := [][2]string{{"↑↓", "Navigate"}, {"Enter", "Select"}, {"Esc", "Back"}}
	return renderFrame(width, height, "GONIX", body, help)
}

func newRealIPCustomWizard(deps Deps, current realip.Settings) *wizardScreen {
	proxiesDefault := ""
	if !current.Cloudflare {
		proxiesDefault = strings.Join(current.Proxies, " ")
	}
	headers := realip.Headers[1:] // CF-Connecting-IP is covered by the Cloudflare option
	fields := []wizardField{
		{
			Key: "proxies", Label: "Trusted proxy addresses or ranges (space separated, e.g. your load balancer):",
			Kind: fieldText, Placeholder: "10.0.0.0/8", Default: proxiesDefault,
			Validate: func(v string) error { _, err := iplist.ParseEntries(v); return err },
		},
		{
			Key: "header", Label: "Header carrying the client address:", Kind: fieldChoice,
			Options: headers, ChoiceDefault: current.Header,
		},
	}
	return newWizard("Real Client IP — Custom Proxy", fields, func(v map[string]string) (screen, tea.Cmd) {
		proxies, _ := iplist.ParseEntries(v["proxies"])
		return newRealIPScreen(deps).save(realip.Settings{Header: v["header"], Proxies: proxies})
	}, func() (screen, tea.Cmd) { return newRealIPScreen(deps), nil })
}
