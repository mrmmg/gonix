package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrmmg/gonix/internal/certificates"
	"github.com/mrmmg/gonix/internal/hosts"
	"github.com/mrmmg/gonix/internal/nginx"
	"github.com/mrmmg/gonix/internal/presets"
)

// hostDetailScreen implements the per-host management submenu described in
// the project specification (section 6).
type hostDetailScreen struct {
	deps    Deps
	host    nginx.ListedHost
	menu    *simpleMenu
	actions []int // hd* action of each menu row, in display order
}

// Host detail actions. Which ones are offered depends on the host's mode,
// so menu rows are mapped to actions through hostDetailScreen.actions.
const (
	hdToggleEnable = iota
	hdMaintenance
	hdRedirect
	hdAliases
	hdAddPath
	hdAddCustomPath
	hdSSL
	hdPresets
	hdWebSocket
	hdProxy
	hdAccessList
	hdIPRules
	hdCountry
	hdRateLimit
	hdErrorPages
	hdLogs
	hdLiveLogs
	hdViewConfig
	hdBackupRestore
	hdDelete
)

// redirectHostActions are the only actions that make sense for a redirect
// host: it has no locations, and its server-level `return` answers before
// access lists or error pages would ever apply.
var redirectHostActions = map[int]bool{
	hdToggleEnable: true, hdRedirect: true, hdAliases: true, hdSSL: true, hdPresets: true,
	hdLogs: true, hdLiveLogs: true, hdViewConfig: true, hdBackupRestore: true, hdDelete: true,
}

func newHostDetailScreen(deps Deps, h nginx.ListedHost) *hostDetailScreen {
	s := &hostDetailScreen{deps: deps, host: h}
	s.rebuildMenu()
	return s
}

func (s *hostDetailScreen) rebuildMenu() {
	toggleLabel := "Enable Host"
	if s.host.Enabled {
		toggleLabel = "Disable Host"
	}
	all := []struct {
		action int
		item   menuItem
	}{
		{hdToggleEnable, menuItem{title: toggleLabel}},
		{hdMaintenance, menuItem{title: "Maintenance Mode", desc: maintenanceDesc(s.host)}},
		{hdRedirect, menuItem{title: "Redirect Configuration", desc: redirectDesc(s.host)}},
		{hdAliases, menuItem{title: "Domain Names", desc: aliasesDesc(s.host)}},
		{hdAddPath, menuItem{title: "Add New Path"}},
		{hdAddCustomPath, menuItem{title: "Add Custom Path"}},
		{hdSSL, menuItem{title: "SSL Configuration"}},
		{hdPresets, menuItem{title: "Presets", desc: presetsDesc(s.host)}},
		{hdWebSocket, menuItem{title: "WebSocket Configuration"}},
		{hdProxy, menuItem{title: "Proxy Configuration"}},
		{hdAccessList, menuItem{title: "Access List"}},
		{hdIPRules, menuItem{title: "IP Access Rules", desc: ipRulesDesc(s.host)}},
		{hdCountry, menuItem{title: "Country Access", desc: countryDesc(s.host)}},
		{hdRateLimit, menuItem{title: "Rate Limit", desc: rateLimitDesc(s.host)}},
		{hdErrorPages, menuItem{title: "Error Pages", desc: errorPagesDesc(s.host)}},
		{hdLogs, menuItem{title: "Access / Error Logs"}},
		{hdLiveLogs, menuItem{title: "Live Logs & Traffic", desc: "tail, filter, summarize, block clients"}},
		{hdViewConfig, menuItem{title: "View Configuration"}},
		{hdBackupRestore, menuItem{title: "Backup / Restore"}},
		{hdDelete, menuItem{title: "Delete Host"}},
	}

	isRedirectHost := s.host.Managed && s.host.Mode == nginx.ModeRedirect
	var items []menuItem
	s.actions = s.actions[:0]
	for _, e := range all {
		if isRedirectHost && !redirectHostActions[e.action] {
			continue
		}
		if !isRedirectHost && e.action == hdRedirect {
			continue
		}
		if !s.host.Managed && unmanagedBlocked[e.action] {
			e.item.desc = mutedStyle.Render("(unmanaged host)")
		}
		items = append(items, e.item)
		s.actions = append(s.actions, e.action)
	}
	if s.menu == nil {
		s.menu = newSimpleMenu(items)
	} else {
		s.menu.SetItems(items)
	}
}

// unmanagedBlocked lists the actions that edit a host's configuration and
// therefore are refused for hosts GoNix did not generate.
var unmanagedBlocked = map[int]bool{
	hdMaintenance: true, hdRedirect: true, hdAliases: true, hdAddPath: true, hdAddCustomPath: true, hdSSL: true, hdPresets: true,
	hdWebSocket: true, hdProxy: true, hdAccessList: true, hdIPRules: true, hdCountry: true, hdRateLimit: true, hdErrorPages: true, hdLogs: true,
}

// selectAction moves the menu cursor to action, reporting whether it is
// offered for this host.
func (s *hostDetailScreen) selectAction(action int) bool {
	for i, a := range s.actions {
		if a == action {
			s.menu.cursor = i
			return true
		}
	}
	return false
}

func redirectDesc(h nginx.ListedHost) string {
	if h.Mode != nginx.ModeRedirect {
		return ""
	}
	return fmt.Sprintf("%d → %s", h.Redirect.Code, h.Redirect.Target)
}

func aliasesDesc(h nginx.ListedHost) string {
	if len(h.ServerAlias) == 0 {
		return mutedStyle.Render("(no additional names)")
	}
	return strings.Join(h.ServerAlias, " ")
}

func ipRulesDesc(h nginx.ListedHost) string {
	var parts []string
	if h.IPListName != "" {
		parts = append(parts, h.IPListName)
	}
	for _, loc := range h.Locations {
		if loc.IPListName != "" {
			parts = append(parts, loc.Path+": "+loc.IPListName)
		}
	}
	if len(parts) == 0 {
		return mutedStyle.Render("(none)")
	}
	return strings.Join(parts, ", ")
}

func countryDesc(h nginx.ListedHost) string {
	if h.CountryList == "" {
		return mutedStyle.Render("(none)")
	}
	return h.CountryList
}

func errorPagesDesc(h nginx.ListedHost) string {
	if h.ErrorPagesSnippet == "" {
		return mutedStyle.Render("(none)")
	}
	return h.ErrorPagesSnippet
}

func (s *hostDetailScreen) Init() tea.Cmd { return nil }

func (s *hostDetailScreen) refreshHost() {
	hostList, err := s.deps.Manager.ListHosts()
	if err != nil {
		return
	}
	for _, h := range hostList {
		if h.FileName == s.host.FileName {
			s.host = h
			return
		}
	}
}

func (s *hostDetailScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if s.menu.HandleKey(msg) {
			return s, nil
		}
		switch msg.String() {
		case "esc", "q":
			return s, navPop()
		case "enter":
			if sel := s.menu.Selected(); sel < len(s.actions) {
				return s.dispatch(s.actions[sel])
			}
		}
	}
	return s, nil
}

func (s *hostDetailScreen) dispatch(choice int) (screen, tea.Cmd) {
	if !s.host.Managed && unmanagedBlocked[choice] {
		return newResultScreen(s.deps, "Unmanaged Host", false,
			"This host's configuration was not generated by GoNix and cannot be edited automatically. Use \"View Configuration\" to inspect it, or edit it manually with a text editor.",
			s), nil
	}

	switch choice {
	case hdToggleEnable:
		return s.toggleEnable()
	case hdMaintenance:
		return newMaintenanceWizard(s.deps, s.host), nil
	case hdRedirect:
		return newRedirectWizard(s.deps, s.host), nil
	case hdAliases:
		return newAliasesWizard(s.deps, s.host), nil
	case hdViewConfig:
		return s.viewConfig()
	case hdDelete:
		return s.confirmDelete()
	case hdBackupRestore:
		return newBackupScreen(s.deps, s.host), nil
	case hdAddPath:
		return newAddPathWizard(s.deps, s.host), nil
	case hdAddCustomPath:
		return newAddCustomPathWizard(s.deps, s.host), nil
	case hdSSL:
		return newSSLWizard(s.deps, s.host), nil
	case hdPresets:
		return newHostPresetsWizard(s.deps, s.host), nil
	case hdWebSocket:
		return s.toggleWebSocket()
	case hdProxy:
		return newProxyWizard(s.deps, s.host), nil
	case hdAccessList:
		return newHostAccessListScreen(s.deps, s.host), nil
	case hdIPRules:
		return newHostIPRulesWizard(s.deps, s.host), nil
	case hdRateLimit:
		return newHostRateLimitWizard(s.deps, s.host), nil
	case hdCountry:
		return newHostCountryWizard(s.deps, s.host), nil
	case hdErrorPages:
		return newHostErrorPagesScreen(s.deps, s.host), nil
	case hdLogs:
		return newLogsWizard(s.deps, s.host), nil
	case hdLiveLogs:
		return s, navPush(newLiveLogsScreen(s.deps, s.host))
	}
	return s, nil
}

func (s *hostDetailScreen) toggleEnable() (screen, tea.Cmd) {
	ctx := backgroundCtx()
	var err error
	if s.host.Enabled {
		err = s.deps.HostService.DisableHost(ctx, s.host.FileName)
	} else {
		err = s.deps.HostService.EnableHost(ctx, s.host.FileName)
	}
	if err != nil {
		return newResultScreen(s.deps, "Host State", false, err.Error(), s), nil
	}
	s.refreshHost()
	s.rebuildMenu()
	return s, nil
}

func (s *hostDetailScreen) toggleWebSocket() (screen, tea.Cmd) {
	h, err := s.deps.Manager.GetHost(s.host.FileName)
	if err != nil {
		return newResultScreen(s.deps, "WebSocket Configuration", false, err.Error(), s), nil
	}
	for i, loc := range h.Locations {
		if loc.Path == "/" && loc.Proxy != nil {
			h.Locations[i].Proxy.WebSocket = !loc.Proxy.WebSocket
		}
	}
	if _, err := s.deps.HostService.UpdateHost(backgroundCtx(), h); err != nil {
		return newResultScreen(s.deps, "WebSocket Configuration", false, err.Error(), s), nil
	}
	return newResultScreen(s.deps, "WebSocket Configuration", true, "WebSocket setting toggled for location /.", s), nil
}

func (s *hostDetailScreen) viewConfig() (screen, tea.Cmd) {
	content, err := s.deps.Manager.RawConfig(s.host.FileName)
	if err != nil {
		return newResultScreen(s.deps, "View Configuration", false, err.Error(), s), nil
	}
	return newViewerScreen("Configuration: "+s.host.ServerName, content, s), nil
}

func (s *hostDetailScreen) confirmDelete() (screen, tea.Cmd) {
	return newConfirmScreen(
		"Delete Host",
		fmt.Sprintf("Delete host %q? A backup will be kept, but this cannot be undone from the UI.", s.host.ServerName),
		func() (screen, tea.Cmd) {
			if err := s.deps.HostService.DeleteHost(backgroundCtx(), s.host.FileName); err != nil {
				return newResultScreen(s.deps, "Delete Host", false, err.Error(), s), nil
			}
			return newResultScreen(s.deps, "Delete Host", true, "Host deleted.", newHostsListScreen(s.deps)), nil
		},
		func() (screen, tea.Cmd) { return s, nil },
	), nil
}

func (s *hostDetailScreen) View(width, height int) string {
	dot := disabledDot
	state := "DISABLED"
	if s.host.Enabled {
		dot = enabledDot
		state = "ENABLED"
	}
	body := headerStyle.Render("Host: "+s.host.ServerName) + "\n"
	body += dot + " " + state + "\n\n"
	body += s.menu.View()
	help := [][2]string{{"↑↓", "Navigate"}, {"Enter", "Select"}, {"Esc", "Back"}}
	return renderFrame(width, height, "GONIX", body, help)
}

// --- Add New Path ---

func newAddPathWizard(deps Deps, h nginx.ListedHost) *wizardScreen {
	fields := []wizardField{
		{Key: "path", Label: "Location path (e.g. /api/):", Kind: fieldText, Placeholder: "/api/",
			Validate: func(v string) error { return validateLocationPath(v) }},
	}
	fields = append(fields, upstreamTargetField(deps, "", nil)...)
	fields = append(fields, []wizardField{
		{Key: "upstream_host", Label: "Upstream host/IP:", Kind: fieldText, Placeholder: "127.0.0.1", Validate: requireNonEmpty("upstream host"), ShowIf: isSingleUpstream},
		{Key: "upstream_port", Label: "Upstream port:", Kind: fieldText, Placeholder: "8080", Validate: validatePort, ShowIf: isSingleUpstream},
		{Key: "upstream_scheme", Label: "Upstream protocol:", Kind: fieldChoice, Options: []string{"http", "https"}},
		{Key: "websocket", Label: "Enable WebSocket Support?", Kind: fieldBool},
	}...)
	return newWizard("Add New Path — "+h.ServerName, fields, func(v map[string]string) (screen, tea.Cmd) {
		host, err := deps.Manager.GetHost(h.FileName)
		if err != nil {
			return newResultScreen(deps, "Add New Path", false, err.Error(), newHostDetailScreen(deps, h)), nil
		}
		port, _ := strconv.Atoi(v["upstream_port"])
		host.Locations = append(host.Locations, nginx.Location{
			Path: v["path"],
			Proxy: &nginx.ProxyConfig{
				UpstreamScheme: v["upstream_scheme"], UpstreamHost: v["upstream_host"], UpstreamPort: port,
				UpstreamGroup: upstreamGroupChoice(v),
				HTTPVersion:   "1.1", WebSocket: v["websocket"] == "yes",
				ForwardHost: true, ForwardFor: true, ForwardProto: true, BufferingEnabled: true,
			},
		})
		if _, err := deps.HostService.UpdateHost(backgroundCtx(), host); err != nil {
			return newResultScreen(deps, "Add New Path", false, err.Error(), newHostDetailScreen(deps, h)), nil
		}
		return newResultScreen(deps, "Add New Path", true, "Location added.", newHostDetailScreen(deps, h)), nil
	}, func() (screen, tea.Cmd) { return newHostDetailScreen(deps, h), nil })
}

func validateLocationPath(p string) error {
	if p == "" || p[0] != '/' {
		return fmt.Errorf("path must start with /")
	}
	return nil
}

// --- Add Custom Path ---

func newAddCustomPathWizard(deps Deps, h nginx.ListedHost) *wizardScreen {
	fields := []wizardField{
		{Key: "path", Label: "Location path (e.g. /custom/):", Kind: fieldText, Placeholder: "/custom/", Validate: validateLocationPath},
		{Key: "config", Label: "Custom location configuration (single line; edit the file later for more):", Kind: fieldText,
			Placeholder: "return 200 'ok';"},
	}
	return newWizard("Add Custom Location — "+h.ServerName, fields, func(v map[string]string) (screen, tea.Cmd) {
		host, err := deps.Manager.GetHost(h.FileName)
		if err != nil {
			return newResultScreen(deps, "Add Custom Location", false, err.Error(), newHostDetailScreen(deps, h)), nil
		}
		host.Locations = append(host.Locations, nginx.Location{
			Path: v["path"], IsCustom: true, CustomConfig: "        " + v["config"],
		})
		if _, err := deps.HostService.UpdateHost(backgroundCtx(), host); err != nil {
			return newResultScreen(deps, "Add Custom Location", false, err.Error(), newHostDetailScreen(deps, h)), nil
		}
		return newResultScreen(deps, "Add Custom Location", true,
			"Custom location added. You can edit "+host.ServerName+" directly with a text editor for advanced configuration.",
			newHostDetailScreen(deps, h)), nil
	}, func() (screen, tea.Cmd) { return newHostDetailScreen(deps, h), nil })
}

// --- SSL Configuration ---

func newSSLWizard(deps Deps, h nginx.ListedHost) *wizardScreen {
	certs, _ := certificates.Scan(deps.Config.Certificates.Directory)
	certOptions := make([]string, 0, len(certs))
	for _, c := range certs {
		certOptions = append(certOptions, c.Domain)
	}
	if len(certOptions) == 0 {
		certOptions = []string{h.ServerName}
	}

	fields := []wizardField{
		{Key: "enable", Label: "Enable SSL for this host?", Kind: fieldBool, BoolDefault: h.SSL.Enabled},
		{Key: "cert", Label: "Certificate (from centralized certs directory):", Kind: fieldChoice, Options: certOptions,
			ShowIf: func(v map[string]string) bool { return v["enable"] == "yes" }},
		{Key: "http2", Label: "Enable HTTP/2?", Kind: fieldBool, BoolDefault: true,
			ShowIf: func(v map[string]string) bool { return v["enable"] == "yes" }},
		{Key: "http3", Label: http3Label, Kind: fieldBool, BoolDefault: h.SSL.HTTP3,
			ShowIf: func(v map[string]string) bool {
				return v["enable"] == "yes" && deps.Manager.Renderer.Build.SupportsHTTP3()
			}},
		{Key: "force", Label: "Redirect HTTP to HTTPS?", Kind: fieldBool, BoolDefault: true,
			ShowIf: func(v map[string]string) bool { return v["enable"] == "yes" && h.Mode != nginx.ModeRedirect }},
	}
	return newWizard("SSL Configuration — "+h.ServerName, fields, func(v map[string]string) (screen, tea.Cmd) {
		host, err := deps.Manager.GetHost(h.FileName)
		if err != nil {
			return newResultScreen(deps, "SSL Configuration", false, err.Error(), newHostDetailScreen(deps, h)), nil
		}
		if v["enable"] == "yes" {
			cert := v["cert"]
			host.SSL = nginx.SSLConfig{
				Enabled:    true,
				CertFile:   deps.Config.Certificates.Directory + "/" + cert + "/fullchain.pem",
				KeyFile:    deps.Config.Certificates.Directory + "/" + cert + "/privkey.pem",
				HTTP2:      v["http2"] == "yes",
				HTTP3:      v["http3"] == "yes",
				ForceHTTPS: v["force"] == "yes",
			}
			if host.ListenSSL == 0 {
				host.ListenSSL = 443
			}
		} else {
			host.SSL = nginx.SSLConfig{}
		}
		host.Presets = presets.KeepApplicable(host)
		if _, err := deps.HostService.UpdateHost(backgroundCtx(), host); err != nil {
			return newResultScreen(deps, "SSL Configuration", false, err.Error(), newHostDetailScreen(deps, h)), nil
		}
		return newResultScreen(deps, "SSL Configuration", true, "SSL configuration updated.", newHostDetailScreen(deps, h)), nil
	}, func() (screen, tea.Cmd) { return newHostDetailScreen(deps, h), nil })
}

// --- Proxy Configuration (edits the "/" location) ---

func newProxyWizard(deps Deps, h nginx.ListedHost) *wizardScreen {
	// Prefill from the current "/" location.
	cur := nginx.ProxyConfig{UpstreamScheme: "http", ConnectTimeout: "60s", ReadTimeout: "60s", SendTimeout: "60s"}
	for _, loc := range h.Locations {
		if loc.Path == "/" && loc.Proxy != nil {
			cur = *loc.Proxy
		}
	}
	port := ""
	if cur.UpstreamPort > 0 {
		port = strconv.Itoa(cur.UpstreamPort)
	}
	orDefault := func(v, def string) string {
		if v == "" {
			return def
		}
		return v
	}
	fields := upstreamTargetField(deps, cur.UpstreamGroup, nil)
	fields = append(fields, []wizardField{
		{Key: "upstream_host", Label: "Upstream host/IP (for location /):", Kind: fieldText, Default: cur.UpstreamHost,
			Validate: requireNonEmpty("upstream host"), ShowIf: isSingleUpstream},
		{Key: "upstream_port", Label: "Upstream port:", Kind: fieldText, Default: port, Validate: validatePort, ShowIf: isSingleUpstream},
		{Key: "upstream_scheme", Label: "Upstream protocol:", Kind: fieldChoice, Options: []string{"http", "https"}, ChoiceDefault: cur.UpstreamScheme},
		{Key: "connect_timeout", Label: "Connect timeout:", Kind: fieldText, Placeholder: "60s", Default: orDefault(cur.ConnectTimeout, "60s")},
		{Key: "read_timeout", Label: "Read timeout:", Kind: fieldText, Placeholder: "60s", Default: orDefault(cur.ReadTimeout, "60s")},
		{Key: "send_timeout", Label: "Send timeout:", Kind: fieldText, Placeholder: "60s", Default: orDefault(cur.SendTimeout, "60s")},
		{Key: "max_body", Label: "Max request body size (e.g. 10m, empty = default):", Kind: fieldText, Placeholder: "10m", Default: cur.MaxBodySize},
	}...)
	return newWizard("Proxy Configuration — "+h.ServerName, fields, func(v map[string]string) (screen, tea.Cmd) {
		host, err := deps.Manager.GetHost(h.FileName)
		if err != nil {
			return newResultScreen(deps, "Proxy Configuration", false, err.Error(), newHostDetailScreen(deps, h)), nil
		}
		found := false
		port, _ := strconv.Atoi(v["upstream_port"])
		for i, loc := range host.Locations {
			if loc.Path == "/" && loc.Proxy != nil {
				host.Locations[i].Proxy.UpstreamHost = v["upstream_host"]
				host.Locations[i].Proxy.UpstreamPort = port
				host.Locations[i].Proxy.UpstreamScheme = v["upstream_scheme"]
				host.Locations[i].Proxy.UpstreamGroup = upstreamGroupChoice(v)
				host.Locations[i].Proxy.ConnectTimeout = v["connect_timeout"]
				host.Locations[i].Proxy.ReadTimeout = v["read_timeout"]
				host.Locations[i].Proxy.SendTimeout = v["send_timeout"]
				host.Locations[i].Proxy.MaxBodySize = v["max_body"]
				found = true
			}
		}
		if !found {
			return newResultScreen(deps, "Proxy Configuration", false, "host has no location / to configure", newHostDetailScreen(deps, h)), nil
		}
		if _, err := deps.HostService.UpdateHost(backgroundCtx(), host); err != nil {
			return newResultScreen(deps, "Proxy Configuration", false, err.Error(), newHostDetailScreen(deps, h)), nil
		}
		return newResultScreen(deps, "Proxy Configuration", true, "Proxy configuration updated.", newHostDetailScreen(deps, h)), nil
	}, func() (screen, tea.Cmd) { return newHostDetailScreen(deps, h), nil })
}

// --- Access / Error Logs ---

func newLogsWizard(deps Deps, h nginx.ListedHost) *wizardScreen {
	fields := []wizardField{
		{Key: "access", Label: "Enable Access Log?", Kind: fieldBool, BoolDefault: true},
		{Key: "error", Label: "Enable Error Log?", Kind: fieldBool, BoolDefault: true},
	}
	return newWizard("Access / Error Logs — "+h.ServerName, fields, func(v map[string]string) (screen, tea.Cmd) {
		host, err := deps.Manager.GetHost(h.FileName)
		if err != nil {
			return newResultScreen(deps, "Logs", false, err.Error(), newHostDetailScreen(deps, h)), nil
		}
		host.AccessLog = v["access"] == "yes"
		host.ErrorLog = v["error"] == "yes"
		if _, err := deps.HostService.UpdateHost(backgroundCtx(), host); err != nil {
			return newResultScreen(deps, "Logs", false, err.Error(), newHostDetailScreen(deps, h)), nil
		}
		return newResultScreen(deps, "Logs", true, "Log settings updated.", newHostDetailScreen(deps, h)), nil
	}, func() (screen, tea.Cmd) { return newHostDetailScreen(deps, h), nil })
}

// --- Redirect Configuration ---

func newRedirectWizard(deps Deps, h nginx.ListedHost) *wizardScreen {
	return newWizard("Redirect Configuration — "+h.ServerName, redirectFields(h.Redirect, nil), func(v map[string]string) (screen, tea.Cmd) {
		host, err := deps.Manager.GetHost(h.FileName)
		if err != nil {
			return newResultScreen(deps, "Redirect Configuration", false, err.Error(), newHostDetailScreen(deps, h)), nil
		}
		host.Redirect = redirectFromValues(v)
		if _, err := deps.HostService.UpdateHost(backgroundCtx(), host); err != nil {
			return newResultScreen(deps, "Redirect Configuration", false, err.Error(), newHostDetailScreen(deps, h)), nil
		}
		return newResultScreen(deps, "Redirect Configuration", true,
			fmt.Sprintf("Requests to %s now redirect to %s.", host.ServerName, host.Redirect.ReturnURL()),
			newHostDetailScreen(deps, h)), nil
	}, func() (screen, tea.Cmd) { return newHostDetailScreen(deps, h), nil })
}

// --- Domain Names (server_name aliases) ---

func newAliasesWizard(deps Deps, h nginx.ListedHost) *wizardScreen {
	fields := []wizardField{
		{
			Key:   "aliases",
			Label: "Additional domain names for " + h.ServerName + " (space-separated, empty for none):",
			Kind:  fieldText, Placeholder: "www." + h.ServerName, Default: strings.Join(h.ServerAlias, " "),
			Validate: func(v string) error {
				_, err := hosts.ParseAliases(v, h.ServerName)
				return err
			},
		},
	}
	return newWizard("Domain Names — "+h.ServerName, fields, func(v map[string]string) (screen, tea.Cmd) {
		host, err := deps.Manager.GetHost(h.FileName)
		if err != nil {
			return newResultScreen(deps, "Domain Names", false, err.Error(), newHostDetailScreen(deps, h)), nil
		}
		host.ServerAlias, err = hosts.ParseAliases(v["aliases"], host.ServerName)
		if err != nil {
			return newResultScreen(deps, "Domain Names", false, err.Error(), newHostDetailScreen(deps, h)), nil
		}
		if _, err := deps.HostService.UpdateHost(backgroundCtx(), host); err != nil {
			return newResultScreen(deps, "Domain Names", false, err.Error(), newHostDetailScreen(deps, h)), nil
		}
		return newResultScreen(deps, "Domain Names", true, "Domain names updated.", newHostDetailScreen(deps, h)), nil
	}, func() (screen, tea.Cmd) { return newHostDetailScreen(deps, h), nil })
}
