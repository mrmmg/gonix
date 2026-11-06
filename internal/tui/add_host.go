package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrmmg/gonix/internal/certificates"
	"github.com/mrmmg/gonix/internal/hosts"
	"github.com/mrmmg/gonix/internal/nginx"
)

// newAddHostWizard builds the "Add New Host" interactive workflow described
// in the project specification: domain, mode, then mode-specific questions,
// then optional SSL, ending with a safe create+enable.
func newAddHostWizard(deps Deps) *wizardScreen {
	fields := []wizardField{
		{
			Key:         "domain",
			Label:       "Domain name (e.g. example.com):",
			Kind:        fieldText,
			Placeholder: "example.com",
			Validate:    func(v string) error { return hosts.ValidateDomain(v) },
		},
		{
			Key:         "aliases",
			Label:       "Additional domain names (optional, space-separated, e.g. www.example.com):",
			Kind:        fieldText,
			Placeholder: "www.example.com",
			Validate: func(v string) error {
				_, err := hosts.ParseAliases(v, "")
				return err
			},
		},
		{
			Key:     "mode",
			Label:   "Host mode:",
			Kind:    fieldChoice,
			Options: []string{modeReverseProxy, modeStatic, modeRedirect},
		},
	}
	// --- Reverse proxy questions ---
	fields = append(fields, upstreamTargetField(deps, "", isReverseProxy)...)
	fields = append(fields, []wizardField{
		{
			Key: "upstream_host", Label: "Upstream host/IP:", Kind: fieldText,
			Placeholder: "127.0.0.1",
			Validate:    requireNonEmpty("upstream host"),
			ShowIf:      isSingleReverseProxy,
		},
		{
			Key: "upstream_port", Label: "Upstream port:", Kind: fieldText,
			Placeholder: "8080",
			Validate:    validatePort,
			ShowIf:      isSingleReverseProxy,
		},
		{
			Key: "upstream_scheme", Label: "Upstream protocol:", Kind: fieldChoice,
			Options: []string{"http", "https"},
			ShowIf:  isReverseProxy,
		},
		{
			Key: "http_version", Label: "HTTP version to upstream:", Kind: fieldChoice,
			Options: []string{"1.1", "1.0"},
			ShowIf:  isReverseProxy,
		},
		{
			Key: "websocket", Label: "Enable WebSocket Support?", Kind: fieldBool,
			ShowIf: isReverseProxy,
		},
		{
			Key: "forward_headers", Label: "Add standard forwarded headers (Host, X-Real-IP, X-Forwarded-For, X-Forwarded-Proto)?",
			Kind: fieldBool, BoolDefault: true, ShowIf: isReverseProxy,
		},
		{
			Key: "connect_timeout", Label: "Connect timeout (e.g. 60s):", Kind: fieldText,
			Placeholder: "60s", Default: "60s", ShowIf: isReverseProxy,
		},
		{
			Key: "read_timeout", Label: "Read timeout (e.g. 60s):", Kind: fieldText,
			Placeholder: "60s", Default: "60s", ShowIf: isReverseProxy,
		},
		{
			Key: "send_timeout", Label: "Send timeout (e.g. 60s):", Kind: fieldText,
			Placeholder: "60s", Default: "60s", ShowIf: isReverseProxy,
		},
		// --- Static questions ---
		{
			Key: "root", Label: "Root directory to serve:", Kind: fieldText,
			Placeholder: "/var/www/example.com",
			Validate:    requireNonEmpty("root directory"),
			ShowIf:      func(v map[string]string) bool { return v["mode"] == modeStatic },
		},
	}...)
	fields = append(fields, redirectFields(nginx.RedirectConfig{Code: 301, PreservePath: true}, isRedirect)...)
	fields = append(fields, []wizardField{
		// --- SSL ---
		{
			Key: "ssl_enable", Label: "Enable SSL for this host?", Kind: fieldBool,
		},
	}...)

	// The certificate choice step is appended dynamically below because its
	// options depend on what's actually present on disk.
	certs, _ := certificates.Scan(deps.Config.Certificates.Directory)
	if len(certs) > 0 {
		opts := make([]string, len(certs))
		for i, c := range certs {
			opts[i] = c.Domain
		}
		fields = append(fields, wizardField{
			Key: "ssl_cert", Label: "Certificate (from /etc/nginx/certs):", Kind: fieldChoice,
			Options: opts,
			ShowIf:  func(v map[string]string) bool { return v["ssl_enable"] == "yes" },
		})
	}
	fields = append(fields,
		wizardField{
			Key: "ssl_http2", Label: "Enable HTTP/2?", Kind: fieldBool, BoolDefault: true,
			ShowIf: func(v map[string]string) bool { return v["ssl_enable"] == "yes" },
		},
		wizardField{
			Key: "ssl_http3", Label: http3Label, Kind: fieldBool,
			ShowIf: func(v map[string]string) bool {
				return v["ssl_enable"] == "yes" && deps.Manager.Renderer.Build.SupportsHTTP3()
			},
		},
		wizardField{
			Key: "ssl_force", Label: "Redirect HTTP to HTTPS?", Kind: fieldBool, BoolDefault: true,
			// A redirect host already sends HTTP straight to its target.
			ShowIf: func(v map[string]string) bool { return v["ssl_enable"] == "yes" && !isRedirect(v) },
		},
	)

	// Only offer error pages when at least one snippet exists.
	if opts := errorPagesOptions(deps); len(opts) > 1 {
		fields = append(fields, wizardField{
			Key: "error_pages", Label: "Error pages snippet:", Kind: fieldChoice, Options: opts,
			ShowIf: func(v map[string]string) bool { return !isRedirect(v) },
		})
	}

	return newWizard("Add New Host", fields, func(values map[string]string) (screen, tea.Cmd) {
		return finishAddHost(deps, values)
	}, func() (screen, tea.Cmd) { return nil, navPop() })
}

// Host mode choices offered by the Add New Host wizard.
const (
	modeReverseProxy = "Reverse Proxy"
	modeStatic       = "Direct / Static"
	modeRedirect     = "Redirect"
)

// http3Label is only asked when the installed Nginx supports QUIC.
const http3Label = "Enable HTTP/3 (QUIC)? Requires UDP port 443 to be open in the firewall."

func isReverseProxy(v map[string]string) bool { return v["mode"] == modeReverseProxy }
func isRedirect(v map[string]string) bool     { return v["mode"] == modeRedirect }
func isSingleReverseProxy(v map[string]string) bool {
	return isReverseProxy(v) && isSingleUpstream(v)
}

// redirectFields asks for a redirect host's target, status code and path
// handling, prefilled from current. showIf may be nil.
func redirectFields(current nginx.RedirectConfig, showIf func(map[string]string) bool) []wizardField {
	codes := make([]string, len(nginx.RedirectCodes))
	for i, c := range nginx.RedirectCodes {
		codes[i] = strconv.Itoa(c) + " " + redirectCodeLabel(c)
	}
	return []wizardField{
		{
			Key: "redirect_target", Label: "Redirect to (domain or URL, e.g. example.com or https://example.com/new):",
			Kind: fieldText, Placeholder: "https://example.com", Default: current.Target,
			Validate: func(v string) error { return nginx.ValidateRedirectTarget(nginx.NormalizeRedirectTarget(v)) },
			ShowIf:   showIf,
		},
		{
			Key: "redirect_code", Label: "Redirect status code:", Kind: fieldChoice, Options: codes,
			ChoiceDefault: strconv.Itoa(current.Code) + " " + redirectCodeLabel(current.Code),
			ShowIf:        showIf,
		},
		{
			Key: "redirect_preserve", Label: "Keep the requested path and query string (e.g. /blog?id=5)?",
			Kind: fieldBool, BoolDefault: current.PreservePath, ShowIf: showIf,
		},
	}
}

func redirectCodeLabel(code int) string {
	switch code {
	case 301:
		return "(permanent)"
	case 302:
		return "(temporary)"
	case 307:
		return "(temporary, keeps method)"
	case 308:
		return "(permanent, keeps method)"
	}
	return ""
}

// redirectFromValues builds a RedirectConfig from redirectFields answers.
func redirectFromValues(v map[string]string) nginx.RedirectConfig {
	code, _ := strconv.Atoi(strings.Fields(v["redirect_code"] + " 301")[0])
	return nginx.RedirectConfig{
		Target:       nginx.NormalizeRedirectTarget(v["redirect_target"]),
		Code:         code,
		PreservePath: v["redirect_preserve"] == "yes",
	}
}

func requireNonEmpty(label string) func(string) error {
	return func(v string) error {
		if v == "" {
			return fmt.Errorf("%s must not be empty", label)
		}
		return nil
	}
}

func validatePort(v string) error {
	p, err := strconv.Atoi(v)
	if err != nil || p < 1 || p > 65535 {
		return fmt.Errorf("port must be a number between 1 and 65535")
	}
	return nil
}

func finishAddHost(deps Deps, v map[string]string) (screen, tea.Cmd) {
	h := nginx.Host{
		ServerName: v["domain"],
		Listen:     deps.Config.Nginx.HTTPPort,
		ListenSSL:  deps.Config.Nginx.HTTPSPort,
		AccessLog:  true,
		ErrorLog:   true,
	}

	aliases, err := hosts.ParseAliases(v["aliases"], v["domain"])
	if err != nil {
		return newResultScreen(deps, "Add New Host", false, err.Error(), newMainMenu(deps)), nil
	}
	h.ServerAlias = aliases

	switch v["mode"] {
	case modeStatic:
		h.Mode = nginx.ModeStatic
		h.Root = v["root"]
	case modeRedirect:
		h.Mode = nginx.ModeRedirect
		h.Redirect = redirectFromValues(v)
	default:
		h.Mode = nginx.ModeReverseProxy
		port, _ := strconv.Atoi(v["upstream_port"])
		loc := nginx.Location{
			Path: "/",
			Proxy: &nginx.ProxyConfig{
				UpstreamScheme:   v["upstream_scheme"],
				UpstreamHost:     v["upstream_host"],
				UpstreamPort:     port,
				UpstreamGroup:    upstreamGroupChoice(v),
				HTTPVersion:      v["http_version"],
				WebSocket:        v["websocket"] == "yes",
				ForwardHost:      v["forward_headers"] == "yes",
				ForwardFor:       v["forward_headers"] == "yes",
				ForwardProto:     v["forward_headers"] == "yes",
				ConnectTimeout:   v["connect_timeout"],
				ReadTimeout:      v["read_timeout"],
				SendTimeout:      v["send_timeout"],
				BufferingEnabled: true,
			},
		}
		h.Locations = []nginx.Location{loc}
	}

	if v["ssl_enable"] == "yes" {
		h.SSL.Enabled = true
		h.SSL.HTTP2 = v["ssl_http2"] == "yes"
		h.SSL.HTTP3 = v["ssl_http3"] == "yes"
		h.SSL.ForceHTTPS = v["ssl_force"] == "yes"
		certName := v["ssl_cert"]
		if certName == "" {
			certName = v["domain"]
		}
		h.SSL.CertFile = deps.Config.Certificates.Directory + "/" + certName + "/fullchain.pem"
		h.SSL.KeyFile = deps.Config.Certificates.Directory + "/" + certName + "/privkey.pem"
	}

	if sn := v["error_pages"]; sn != "" && sn != "(none)" {
		h.ErrorPagesSnippet = sn
	}

	if _, err := deps.HostService.CreateHost(backgroundCtx(), h); err != nil {
		return newResultScreen(deps, "Add New Host", false, err.Error(), newMainMenu(deps)), nil
	}
	_ = deps.HostService.EnableHost(backgroundCtx(), h.ServerName)

	return newResultScreen(deps, "Add New Host", true,
		fmt.Sprintf("Host %s was created and enabled successfully.", h.ServerName),
		newHostsListScreen(deps)), nil
}
