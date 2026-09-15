package tui

import (
	"fmt"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/shiva/gonix/internal/certificates"
	"github.com/shiva/gonix/internal/hosts"
	"github.com/shiva/gonix/internal/nginx"
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
			Key:     "mode",
			Label:   "Host mode:",
			Kind:    fieldChoice,
			Options: []string{"Reverse Proxy", "Direct / Static"},
		},
		// --- Reverse proxy questions ---
		{
			Key: "upstream_host", Label: "Upstream host/IP:", Kind: fieldText,
			Placeholder: "127.0.0.1",
			Validate:    requireNonEmpty("upstream host"),
			ShowIf:      isReverseProxy,
		},
		{
			Key: "upstream_port", Label: "Upstream port:", Kind: fieldText,
			Placeholder: "8080",
			Validate:    validatePort,
			ShowIf:      isReverseProxy,
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
			ShowIf:      func(v map[string]string) bool { return v["mode"] == "Direct / Static" },
		},
		// --- SSL ---
		{
			Key: "ssl_enable", Label: "Enable SSL for this host?", Kind: fieldBool,
		},
	}

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
			Key: "ssl_force", Label: "Redirect HTTP to HTTPS?", Kind: fieldBool, BoolDefault: true,
			ShowIf: func(v map[string]string) bool { return v["ssl_enable"] == "yes" },
		},
	)

	return newWizard("Add New Host", fields, func(values map[string]string) (screen, tea.Cmd) {
		return finishAddHost(deps, values)
	}, func() (screen, tea.Cmd) { return nil, navPop() })
}

func isReverseProxy(v map[string]string) bool { return v["mode"] == "Reverse Proxy" }

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
		Listen:     80,
		ListenSSL:  443,
		AccessLog:  true,
		ErrorLog:   true,
	}

	if v["mode"] == "Direct / Static" {
		h.Mode = nginx.ModeStatic
		h.Root = v["root"]
	} else {
		h.Mode = nginx.ModeReverseProxy
		port, _ := strconv.Atoi(v["upstream_port"])
		loc := nginx.Location{
			Path: "/",
			Proxy: &nginx.ProxyConfig{
				UpstreamScheme:   v["upstream_scheme"],
				UpstreamHost:     v["upstream_host"],
				UpstreamPort:     port,
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
		h.SSL.ForceHTTPS = v["ssl_force"] == "yes"
		certName := v["ssl_cert"]
		if certName == "" {
			certName = v["domain"]
		}
		h.SSL.CertFile = deps.Config.Certificates.Directory + "/" + certName + "/fullchain.pem"
		h.SSL.KeyFile = deps.Config.Certificates.Directory + "/" + certName + "/privkey.pem"
	}

	_, err := deps.HostService.CreateHost(backgroundCtx(), h)
	if err != nil {
		return newResultScreen(deps, "Add New Host", false, err.Error(), newMainMenu(deps)), nil
	}
	_ = deps.HostService.EnableHost(backgroundCtx(), h.ServerName)

	return newResultScreen(deps, "Add New Host", true,
		fmt.Sprintf("Host %s was created and enabled successfully.", h.ServerName),
		newHostsListScreen(deps)), nil
}
