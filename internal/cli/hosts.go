package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/mrmmg/gonix/internal/hosts"
	"github.com/mrmmg/gonix/internal/nginx"
	"github.com/mrmmg/gonix/internal/presets"
)

func init() {
	register(
		command{path: "host list", summary: "List hosts", run: hostList},
		command{path: "host show", args: "<domain> [--raw]", summary: "Show a host's settings (or its raw config)", run: hostShow},
		command{path: "host add", args: "<domain> (--upstream URL | --group NAME | --root DIR | --redirect URL) [flags]",
			summary: "Create and enable a host", write: true, run: hostAdd},
		command{path: "host set", args: "<domain> [flags]", summary: "Change a host's settings", write: true, run: hostSet},
		command{path: "host enable", args: "<domain>", summary: "Enable a host", write: true, run: hostToggle(true)},
		command{path: "host disable", args: "<domain>", summary: "Disable a host (its file is kept)", write: true, run: hostToggle(false)},
		command{path: "host delete", args: "<domain> --yes", summary: "Delete a host (a backup is kept)", write: true, run: hostDelete},
		command{path: "host path add", args: "<domain> <path> (--upstream URL | --group NAME) [--websocket]",
			summary: "Add a reverse-proxy location", write: true, run: pathAdd},
		command{path: "host path remove", args: "<domain> <path>", summary: "Remove a location", write: true, run: pathRemove},
	)
}

type proxyJSON struct {
	Upstream       string `json:"upstream,omitempty"`
	Group          string `json:"group,omitempty"`
	WebSocket      bool   `json:"websocket"`
	ConnectTimeout string `json:"connect_timeout,omitempty"`
	ReadTimeout    string `json:"read_timeout,omitempty"`
	SendTimeout    string `json:"send_timeout,omitempty"`
	MaxBodySize    string `json:"max_body_size,omitempty"`
}

type locationJSON struct {
	Path       string     `json:"path"`
	Custom     string     `json:"custom,omitempty"`
	Proxy      *proxyJSON `json:"proxy,omitempty"`
	AccessList string     `json:"access_list,omitempty"`
	IPList     string     `json:"ip_list,omitempty"`
	RateLimit  string     `json:"rate_limit,omitempty"`
}

type hostJSON struct {
	Name        string         `json:"name"`
	File        string         `json:"file"`
	Aliases     []string       `json:"aliases,omitempty"`
	Managed     bool           `json:"managed"`
	Enabled     bool           `json:"enabled"`
	Mode        string         `json:"mode,omitempty"`
	Root        string         `json:"root,omitempty"`
	Redirect    *redirectJSON  `json:"redirect,omitempty"`
	SSL         *sslJSON       `json:"ssl,omitempty"`
	Locations   []locationJSON `json:"locations,omitempty"`
	AccessList  string         `json:"access_list,omitempty"`
	IPList      string         `json:"ip_list,omitempty"`
	SatisfyAny  bool           `json:"satisfy_any,omitempty"`
	RateLimit   string         `json:"rate_limit,omitempty"`
	CountryList string         `json:"country_list,omitempty"`
	ErrorPages  string         `json:"error_pages,omitempty"`
	Presets     []string       `json:"presets,omitempty"`
	Maintenance bool           `json:"maintenance"`
	AccessLog   bool           `json:"access_log"`
	ErrorLog    bool           `json:"error_log"`
	Broken      bool           `json:"broken_symlink,omitempty"`
}

type redirectJSON struct {
	Target       string `json:"target"`
	Code         int    `json:"code"`
	PreservePath bool   `json:"preserve_path"`
}

type sslJSON struct {
	Certificate string `json:"certificate"`
	Key         string `json:"key"`
	HTTP2       bool   `json:"http2"`
	HTTP3       bool   `json:"http3"`
	ForceHTTPS  bool   `json:"force_https"`
}

func toHostJSON(h nginx.ListedHost) hostJSON {
	j := hostJSON{Name: h.ServerName, File: h.FileName, Aliases: h.ServerAlias, Managed: h.Managed, Enabled: h.Enabled, Broken: h.BrokenSymlink}
	if !h.Managed {
		return j
	}
	j.Mode, j.AccessList, j.IPList, j.SatisfyAny = string(h.Mode), h.AccessListName, h.IPListName, h.SatisfyAny
	j.RateLimit, j.CountryList, j.ErrorPages, j.Presets = h.RateLimit, h.CountryList, h.ErrorPagesSnippet, h.Presets
	j.Maintenance, j.AccessLog, j.ErrorLog = h.Maintenance, h.AccessLog, h.ErrorLog
	switch h.Mode {
	case nginx.ModeStatic:
		j.Root = h.Root
	case nginx.ModeRedirect:
		j.Redirect = &redirectJSON{h.Redirect.Target, h.Redirect.Code, h.Redirect.PreservePath}
	}
	if h.SSL.Enabled {
		j.SSL = &sslJSON{h.SSL.CertFile, h.SSL.KeyFile, h.SSL.HTTP2, h.SSL.HTTP3, h.SSL.ForceHTTPS}
	}
	for _, l := range h.Locations {
		lj := locationJSON{Path: l.Path, AccessList: l.AccessListName, IPList: l.IPListName, RateLimit: l.RateLimit}
		if l.IsCustom {
			lj.Custom = strings.TrimSpace(l.CustomConfig)
		} else if p := l.Proxy; p != nil {
			lj.Proxy = &proxyJSON{Group: p.UpstreamGroup, WebSocket: p.WebSocket, ConnectTimeout: p.ConnectTimeout,
				ReadTimeout: p.ReadTimeout, SendTimeout: p.SendTimeout, MaxBodySize: p.MaxBodySize}
			if p.UpstreamGroup == "" {
				lj.Proxy.Upstream = p.UpstreamURL()
			}
		}
		j.Locations = append(j.Locations, lj)
	}
	return j
}

func hostList(e *env, args []string) error {
	if _, err := parseFlags(flag.NewFlagSet("host list", flag.ContinueOnError), args); err != nil {
		return err
	}
	list, err := e.svc.Manager.ListHosts()
	if err != nil {
		return err
	}
	out := make([]hostJSON, 0, len(list))
	for _, h := range list {
		out = append(out, toHostJSON(h))
	}
	return e.show(out, func(w io.Writer) {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "HOST\tSTATE\tMODE\tTARGET")
		for _, h := range out {
			state := "disabled"
			if h.Enabled {
				state = "enabled"
			}
			mode, target := h.Mode, ""
			switch {
			case !h.Managed:
				mode = "unmanaged"
			case h.Redirect != nil:
				target = h.Redirect.Target
			case h.Root != "":
				target = h.Root
			case len(h.Locations) > 0 && h.Locations[0].Proxy != nil:
				target = h.Locations[0].Proxy.Upstream + h.Locations[0].Proxy.Group
			}
			if h.SSL != nil {
				mode += "+ssl"
			}
			if h.Maintenance {
				state += " (maintenance)"
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", h.Name, state, mode, target)
		}
		tw.Flush()
	})
}

// findHost resolves a domain (server name, alias or file name) to its host.
func (e *env) findHost(domain string) (nginx.ListedHost, error) {
	list, err := e.svc.Manager.ListHosts()
	if err != nil {
		return nginx.ListedHost{}, err
	}
	for _, h := range list {
		if h.FileName == domain || strings.EqualFold(h.ServerName, domain) {
			return h, nil
		}
	}
	for _, h := range list {
		for _, a := range h.ServerAlias {
			if strings.EqualFold(a, domain) {
				return h, nil
			}
		}
	}
	return nginx.ListedHost{}, fmt.Errorf("host %q not found", domain)
}

// managedHost loads a host for editing.
func (e *env) managedHost(domain string) (nginx.ListedHost, nginx.Host, error) {
	lh, err := e.findHost(domain)
	if err != nil {
		return lh, nginx.Host{}, err
	}
	if !lh.Managed {
		return lh, nginx.Host{}, fmt.Errorf("host %s was not created by GoNix and cannot be edited automatically", lh.ServerName)
	}
	h, err := e.svc.Manager.GetHost(lh.FileName)
	return lh, h, err
}

func hostShow(e *env, args []string) error {
	fs := flag.NewFlagSet("host show", flag.ContinueOnError)
	raw := fs.Bool("raw", false, "print the configuration file")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if err := exactArgs(pos, 1); err != nil {
		return err
	}
	h, err := e.findHost(pos[0])
	if err != nil {
		return err
	}
	if *raw {
		content, err := e.svc.Manager.RawConfig(h.FileName)
		if err != nil {
			return err
		}
		return e.show(map[string]string{"file": filepath.Join(e.svc.Manager.SitesAvailable, h.FileName), "content": content},
			func(w io.Writer) { fmt.Fprint(w, content) })
	}
	j := toHostJSON(h)
	return e.show(j, func(w io.Writer) {
		writeJSON(w, j)
	})
}

// upstreamFlags holds the flags describing where "/" proxies to.
type hostFlags struct {
	aliases                                  stringsFlag
	aliasesSet                               string
	upstream, group, root, redirect          string
	code                                     int
	noPreservePath, websocket, noForward     bool
	ssl                                      string
	http2, http3, forceHTTPS                 bool
	disabled                                 bool
	presets, ipList, rateLimit, countryList  string
	accessList, errorPages, satisfy          string
	connectTimeout, readTimeout, sendTimeout string
	maxBody, maintenance, bypass, wsToggle   string
	accessLog, errorLog                      string
}

func (f *hostFlags) register(fs *flag.FlagSet, forSet bool) {
	fs.Var(&f.aliases, "alias", "additional domain name (repeatable)")
	fs.StringVar(&f.upstream, "upstream", "", "proxy / to this URL, e.g. http://127.0.0.1:3000")
	fs.StringVar(&f.group, "group", "", "proxy / to this upstream group")
	fs.StringVar(&f.root, "root", "", "serve static files from this directory")
	fs.StringVar(&f.redirect, "redirect", "", "redirect every request to this URL or domain")
	fs.IntVar(&f.code, "code", 301, "redirect status code (301, 302, 307, 308)")
	fs.BoolVar(&f.noPreservePath, "no-preserve-path", false, "redirect without the request path and query")
	fs.StringVar(&f.ssl, "ssl", "", "certificate name in the certificates directory (or none)")
	fs.BoolVar(&f.http2, "http2", true, "enable HTTP/2 with SSL")
	fs.BoolVar(&f.http3, "http3", false, "also listen for HTTP/3 (QUIC)")
	fs.BoolVar(&f.forceHTTPS, "force-https", true, "redirect HTTP to HTTPS when SSL is on")
	fs.StringVar(&f.presets, "presets", "", "comma-separated presets (or none)")
	fs.StringVar(&f.ipList, "ip-list", "", "IP access list (or none)")
	fs.StringVar(&f.rateLimit, "rate-limit", "", "rate limit (or none)")
	fs.StringVar(&f.countryList, "country-list", "", "country list (or none)")
	fs.StringVar(&f.accessList, "access-list", "", "Basic Auth access list (or none)")
	fs.StringVar(&f.errorPages, "error-pages", "", "error pages snippet (or none)")
	fs.StringVar(&f.satisfy, "satisfy", "", "with both an IP list and Basic Auth: any (either) or all (both)")
	fs.StringVar(&f.connectTimeout, "connect-timeout", "", "upstream connect timeout, e.g. 60s")
	fs.StringVar(&f.readTimeout, "read-timeout", "", "upstream read timeout")
	fs.StringVar(&f.sendTimeout, "send-timeout", "", "upstream send timeout")
	fs.StringVar(&f.maxBody, "max-body", "", "client_max_body_size, e.g. 10m")
	if forSet {
		fs.StringVar(&f.aliasesSet, "aliases", "", "replace all additional domain names (space separated, or none)")
		fs.StringVar(&f.wsToggle, "websocket", "", "on or off")
		fs.StringVar(&f.maintenance, "maintenance", "", "on or off")
		fs.StringVar(&f.bypass, "bypass", "", "with --maintenance on: addresses that still see the site")
		fs.StringVar(&f.accessLog, "access-log", "", "on or off")
		fs.StringVar(&f.errorLog, "error-log", "", "on or off")
	} else {
		fs.BoolVar(&f.websocket, "websocket", false, "enable WebSocket support")
		fs.BoolVar(&f.noForward, "no-forward-headers", false, "do not send Host/X-Forwarded-* headers")
		fs.BoolVar(&f.disabled, "disabled", false, "create the host without enabling it")
	}
}

// parseUpstream turns http://host:port into a ProxyConfig target.
func parseUpstream(raw string) (scheme, host string, port int, err error) {
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || (u.Path != "" && u.Path != "/") {
		return "", "", 0, fmt.Errorf("upstream %q must look like http://127.0.0.1:3000", raw)
	}
	host = u.Hostname()
	if strings.ContainsAny(host, " ;{}\"'$") {
		return "", "", 0, fmt.Errorf("upstream host %q is invalid", host)
	}
	port = 80
	if u.Scheme == "https" {
		port = 443
	}
	if p := u.Port(); p != "" {
		port, err = strconv.Atoi(p)
		if err != nil || port < 1 || port > 65535 {
			return "", "", 0, fmt.Errorf("upstream port %q is invalid", p)
		}
	}
	return u.Scheme, host, port, nil
}

func (e *env) certPaths(name string) (string, string, error) {
	if strings.ContainsAny(name, "/\\ ") || name == "." || name == ".." {
		return "", "", fmt.Errorf("certificate name %q must be a directory name in %s", name, e.svc.Config.Certificates.Directory)
	}
	dir := filepath.Join(e.svc.Config.Certificates.Directory, name)
	return filepath.Join(dir, "fullchain.pem"), filepath.Join(dir, "privkey.pem"), nil
}

func hostAdd(e *env, args []string) error {
	fs := flag.NewFlagSet("host add", flag.ContinueOnError)
	var f hostFlags
	f.register(fs, false)
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if err := exactArgs(pos, 1); err != nil {
		return err
	}
	targets := 0
	for _, v := range []string{f.upstream, f.group, f.root, f.redirect} {
		if v != "" {
			targets++
		}
	}
	if targets != 1 {
		return usageErr("give exactly one of --upstream, --group, --root or --redirect")
	}
	if err := hosts.ValidateDomain(pos[0]); err != nil {
		return err
	}

	h := nginx.Host{ServerName: pos[0], Listen: 80, ListenSSL: 443, AccessLog: true, ErrorLog: true}
	if h.ServerAlias, err = hosts.ParseAliases(strings.Join(f.aliases, " "), h.ServerName); err != nil {
		return err
	}
	switch {
	case f.root != "":
		h.Mode, h.Root = nginx.ModeStatic, f.root
	case f.redirect != "":
		h.Mode = nginx.ModeRedirect
		h.Redirect = nginx.RedirectConfig{Target: nginx.NormalizeRedirectTarget(f.redirect), Code: f.code, PreservePath: !f.noPreservePath}
	default:
		h.Mode = nginx.ModeReverseProxy
		p := &nginx.ProxyConfig{UpstreamScheme: "http", HTTPVersion: "1.1", WebSocket: f.websocket, BufferingEnabled: true,
			ForwardHost: !f.noForward, ForwardFor: !f.noForward, ForwardProto: !f.noForward,
			ConnectTimeout: orDefault(f.connectTimeout, "60s"), ReadTimeout: orDefault(f.readTimeout, "60s"),
			SendTimeout: orDefault(f.sendTimeout, "60s"), MaxBodySize: f.maxBody}
		if err := e.setUpstream(p, f.upstream, f.group); err != nil {
			return err
		}
		h.Locations = []nginx.Location{{Path: "/", Proxy: p}}
	}
	if err := e.applyCommon(&h, f, setFlags(fs)); err != nil {
		return err
	}

	ctx := context.Background()
	if len(h.Presets) > 0 {
		if err := e.svc.Presets.Ensure(ctx, h.Presets); err != nil {
			return err
		}
	}
	if _, err := e.svc.HostService.CreateHost(ctx, h); err != nil {
		return err
	}
	msg := "Host " + h.ServerName + " created"
	if !f.disabled {
		if err := e.svc.HostService.EnableHost(ctx, h.ServerName); err != nil {
			return fmt.Errorf("host created but not enabled: %w", err)
		}
		msg += " and enabled"
	}
	return e.done(msg + ".")
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func (e *env) setUpstream(p *nginx.ProxyConfig, upstream, group string) error {
	if group != "" {
		if !e.svc.Upstreams.Exists(group) {
			return fmt.Errorf("upstream group %q does not exist", group)
		}
		p.UpstreamGroup, p.UpstreamHost, p.UpstreamPort = group, "", 0
		return nil
	}
	scheme, host, port, err := parseUpstream(upstream)
	if err != nil {
		return err
	}
	p.UpstreamScheme, p.UpstreamHost, p.UpstreamPort, p.UpstreamGroup = scheme, host, port, ""
	return nil
}

// applyCommon applies the flags shared by host add and host set; set says
// which flags were given.
func (e *env) applyCommon(h *nginx.Host, f hostFlags, set map[string]bool) error {
	if set["ssl"] {
		if noneOr(f.ssl) == "" {
			h.SSL = nginx.SSLConfig{}
			h.Presets = presets.KeepApplicable(*h)
		} else {
			cert, key, err := e.certPaths(f.ssl)
			if err != nil {
				return err
			}
			h.SSL = nginx.SSLConfig{Enabled: true, CertFile: cert, KeyFile: key, HTTP2: f.http2, HTTP3: f.http3, ForceHTTPS: f.forceHTTPS}
		}
	} else if h.SSL.Enabled {
		if set["http2"] {
			h.SSL.HTTP2 = f.http2
		}
		if set["http3"] {
			h.SSL.HTTP3 = f.http3
		}
		if set["force-https"] {
			h.SSL.ForceHTTPS = f.forceHTTPS
		}
	} else if set["http2"] || set["http3"] || set["force-https"] {
		return usageErr("--http2, --http3 and --force-https need SSL (--ssl CERT)")
	}
	if h.SSL.HTTP3 && !e.svc.Manager.Renderer.Build.SupportsHTTP3() {
		return fmt.Errorf("the installed Nginx does not support HTTP/3")
	}
	if set["presets"] {
		var names []string
		for _, n := range strings.Split(noneOr(f.presets), ",") {
			if n = strings.TrimSpace(n); n == "" {
				continue
			}
			if _, ok := presets.Get(n); !ok {
				return fmt.Errorf("unknown preset %q (see gonix preset list)", n)
			}
			names = append(names, n)
		}
		h.Presets = presets.Sort(names)
		allowed := map[string]bool{}
		for _, p := range presets.Applicable(*h) {
			allowed[p.Name] = true
		}
		for _, n := range h.Presets {
			if !allowed[n] {
				return fmt.Errorf("preset %q does not apply to this host", n)
			}
		}
	}
	refs := []struct {
		flag  string
		value string
		dst   *string
		check func(string) bool
		what  string
	}{
		{"ip-list", f.ipList, &h.IPListName, e.svc.IPLists.Store.Exists, "IP access list"},
		{"rate-limit", f.rateLimit, &h.RateLimit, e.svc.RateLimits.Exists, "rate limit"},
		{"country-list", f.countryList, &h.CountryList, e.svc.GeoBlock.Exists, "country list"},
		{"access-list", f.accessList, &h.AccessListName, e.svc.AccessLists.Exists, "access list"},
		{"error-pages", f.errorPages, &h.ErrorPagesSnippet, e.svc.ErrorPages.Store.Exists, "error pages snippet"},
	}
	for _, r := range refs {
		if !set[r.flag] {
			continue
		}
		v := noneOr(r.value)
		if v != "" && !r.check(v) {
			return fmt.Errorf("%s %q does not exist", r.what, v)
		}
		*r.dst = v
	}
	if set["satisfy"] {
		switch f.satisfy {
		case "any":
			h.SatisfyAny = true
		case "all":
			h.SatisfyAny = false
		default:
			return usageErr("--satisfy must be any or all")
		}
	}
	return nil
}

func onOff(flagName, v string) (bool, error) {
	switch v {
	case "on", "yes", "true":
		return true, nil
	case "off", "no", "false":
		return false, nil
	}
	return false, usageErr("--%s must be on or off", flagName)
}

func hostSet(e *env, args []string) error {
	fs := flag.NewFlagSet("host set", flag.ContinueOnError)
	var f hostFlags
	f.register(fs, true)
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if err := exactArgs(pos, 1); err != nil {
		return err
	}
	set := setFlags(fs)
	if len(set) == 0 {
		return usageErr("nothing to change; see gonix help host set")
	}
	lh, h, err := e.managedHost(pos[0])
	if err != nil {
		return err
	}

	if set["alias"] || set["aliases"] {
		names := strings.Join(f.aliases, " ")
		if set["aliases"] {
			names = noneOr(f.aliasesSet) + " " + names
		} else {
			names = strings.Join(h.ServerAlias, " ") + " " + names
		}
		if h.ServerAlias, err = hosts.ParseAliases(names, h.ServerName); err != nil {
			return err
		}
	}
	if set["redirect"] || set["code"] || set["no-preserve-path"] {
		if h.Mode != nginx.ModeRedirect {
			return fmt.Errorf("%s is not a redirect host", h.ServerName)
		}
		if set["redirect"] {
			h.Redirect.Target = nginx.NormalizeRedirectTarget(f.redirect)
		}
		if set["code"] {
			h.Redirect.Code = f.code
		}
		if set["no-preserve-path"] {
			h.Redirect.PreservePath = !f.noPreservePath
		}
	}
	if set["root"] {
		if h.Mode != nginx.ModeStatic {
			return fmt.Errorf("%s is not a static host", h.ServerName)
		}
		h.Root = f.root
	}
	proxyFlags := []string{"upstream", "group", "websocket", "connect-timeout", "read-timeout", "send-timeout", "max-body"}
	for _, name := range proxyFlags {
		if !set[name] {
			continue
		}
		p := rootProxy(&h)
		if p == nil {
			return fmt.Errorf("--%s needs a reverse proxy location /", name)
		}
		switch name {
		case "upstream", "group":
			if set["upstream"] && set["group"] {
				return usageErr("give either --upstream or --group")
			}
			if err := e.setUpstream(p, f.upstream, f.group); err != nil {
				return err
			}
		case "websocket":
			if p.WebSocket, err = onOff(name, f.wsToggle); err != nil {
				return err
			}
		case "connect-timeout":
			p.ConnectTimeout = f.connectTimeout
		case "read-timeout":
			p.ReadTimeout = f.readTimeout
		case "send-timeout":
			p.SendTimeout = f.sendTimeout
		case "max-body":
			p.MaxBodySize = noneOr(f.maxBody)
		}
	}
	for name, dst := range map[string]*bool{"access-log": &h.AccessLog, "error-log": &h.ErrorLog} {
		if set[name] {
			v := f.accessLog
			if name == "error-log" {
				v = f.errorLog
			}
			if *dst, err = onOff(name, v); err != nil {
				return err
			}
		}
	}
	if err := e.applyCommon(&h, f, set); err != nil {
		return err
	}

	ctx := context.Background()
	if set["presets"] && len(h.Presets) > 0 {
		if err := e.svc.Presets.Ensure(ctx, h.Presets); err != nil {
			return err
		}
	}

	// Maintenance mode writes its own http-level file, so it is applied
	// through its service after the other settings.
	maintenanceChange := false
	var maintenanceOn bool
	if set["maintenance"] {
		if maintenanceOn, err = onOff("maintenance", f.maintenance); err != nil {
			return err
		}
		maintenanceChange = maintenanceOn != h.Maintenance || set["bypass"]
	} else if set["bypass"] {
		return usageErr("--bypass needs --maintenance on")
	}
	if len(set) > countKeys(set, "maintenance", "bypass") {
		if _, err := e.svc.HostService.UpdateHost(ctx, h); err != nil {
			return err
		}
	}
	if maintenanceChange {
		if maintenanceOn {
			var bypass []string
			if strings.TrimSpace(f.bypass) != "" {
				if bypass, err = parseIPs(f.bypass); err != nil {
					return err
				}
			}
			err = e.svc.Maintenance.Enable(ctx, lh.FileName, bypass)
		} else {
			err = e.svc.Maintenance.Disable(ctx, lh.FileName)
		}
		if err != nil {
			return err
		}
	}
	return e.done("Host " + h.ServerName + " updated.")
}

func countKeys(m map[string]bool, keys ...string) int {
	n := 0
	for _, k := range keys {
		if m[k] {
			n++
		}
	}
	return n
}

// rootProxy returns the proxy settings of location /.
func rootProxy(h *nginx.Host) *nginx.ProxyConfig {
	for i := range h.Locations {
		if h.Locations[i].Path == "/" && h.Locations[i].Proxy != nil {
			return h.Locations[i].Proxy
		}
	}
	return nil
}

func hostToggle(enable bool) func(*env, []string) error {
	return func(e *env, args []string) error {
		pos, err := parseFlags(flag.NewFlagSet("host", flag.ContinueOnError), args)
		if err != nil {
			return err
		}
		if err := exactArgs(pos, 1); err != nil {
			return err
		}
		h, err := e.findHost(pos[0])
		if err != nil {
			return err
		}
		ctx := context.Background()
		if enable {
			err = e.svc.HostService.EnableHost(ctx, h.FileName)
		} else {
			err = e.svc.HostService.DisableHost(ctx, h.FileName)
		}
		if err != nil {
			return err
		}
		state := "disabled"
		if enable {
			state = "enabled"
		}
		return e.done("Host " + h.ServerName + " " + state + ".")
	}
}

func hostDelete(e *env, args []string) error {
	pos, err := parseFlags(flag.NewFlagSet("host delete", flag.ContinueOnError), args)
	if err != nil {
		return err
	}
	if err := exactArgs(pos, 1); err != nil {
		return err
	}
	h, err := e.findHost(pos[0])
	if err != nil {
		return err
	}
	if err := e.confirm("deleting " + h.ServerName); err != nil {
		return err
	}
	if err := e.svc.HostService.DeleteHost(context.Background(), h.FileName); err != nil {
		return err
	}
	return e.done("Host " + h.ServerName + " deleted (a backup was kept).")
}

func pathAdd(e *env, args []string) error {
	fs := flag.NewFlagSet("host path add", flag.ContinueOnError)
	upstream := fs.String("upstream", "", "proxy to this URL")
	group := fs.String("group", "", "proxy to this upstream group")
	websocket := fs.Bool("websocket", false, "enable WebSocket support")
	ipList := fs.String("ip-list", "", "IP access list for this path")
	rateLimit := fs.String("rate-limit", "", "rate limit for this path")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if err := exactArgs(pos, 2); err != nil {
		return err
	}
	if (*upstream == "") == (*group == "") {
		return usageErr("give exactly one of --upstream or --group")
	}
	if err := hosts.ValidatePath(pos[1]); err != nil {
		return err
	}
	_, h, err := e.managedHost(pos[0])
	if err != nil {
		return err
	}
	for _, l := range h.Locations {
		if l.Path == pos[1] {
			return fmt.Errorf("%s already has a location %s", h.ServerName, pos[1])
		}
	}
	p := &nginx.ProxyConfig{UpstreamScheme: "http", HTTPVersion: "1.1", WebSocket: *websocket, BufferingEnabled: true,
		ForwardHost: true, ForwardFor: true, ForwardProto: true}
	if err := e.setUpstream(p, *upstream, *group); err != nil {
		return err
	}
	loc := nginx.Location{Path: pos[1], Proxy: p}
	if *ipList != "" {
		if !e.svc.IPLists.Store.Exists(*ipList) {
			return fmt.Errorf("IP access list %q does not exist", *ipList)
		}
		loc.IPListName = *ipList
	}
	if *rateLimit != "" {
		if !e.svc.RateLimits.Exists(*rateLimit) {
			return fmt.Errorf("rate limit %q does not exist", *rateLimit)
		}
		loc.RateLimit = *rateLimit
	}
	h.Locations = append(h.Locations, loc)
	if _, err := e.svc.HostService.UpdateHost(context.Background(), h); err != nil {
		return err
	}
	return e.done("Location " + pos[1] + " added to " + h.ServerName + ".")
}

func pathRemove(e *env, args []string) error {
	pos, err := parseFlags(flag.NewFlagSet("host path remove", flag.ContinueOnError), args)
	if err != nil {
		return err
	}
	if err := exactArgs(pos, 2); err != nil {
		return err
	}
	_, h, err := e.managedHost(pos[0])
	if err != nil {
		return err
	}
	var kept []nginx.Location
	for _, l := range h.Locations {
		if l.Path != pos[1] {
			kept = append(kept, l)
		}
	}
	if len(kept) == len(h.Locations) {
		return fmt.Errorf("%s has no location %s", h.ServerName, pos[1])
	}
	h.Locations = kept
	if _, err := e.svc.HostService.UpdateHost(context.Background(), h); err != nil {
		return err
	}
	return e.done("Location " + pos[1] + " removed from " + h.ServerName + ".")
}
