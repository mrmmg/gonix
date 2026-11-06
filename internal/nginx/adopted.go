package nginx

// Adopted hosts: hand-written configuration files GoNix manages without
// regenerating them.
//
// Adopting a file only prepends AdoptedMarker; nothing else changes, so
// Nginx serves exactly what it served before. GoNix reads the settings it
// understands (server names, certificates, proxy locations, its own
// snippets, ...) from a lossless syntax tree of the file (package ast), and
// writes a change by editing just the directives involved. Everything else
// — comments, ordering, directives GoNix has no setting for, certbot's own
// lines — stays byte-for-byte as it was. Every edit is verified by reading
// the result back: if it does not come out as the requested host, the edit
// is refused instead of being written half-applied.

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mrmmg/gonix/internal/nginx/ast"
)

// AdoptedMarker starts the first line of every adopted file.
const AdoptedMarker = "# Adopted by GoNix."

// adoptedHeader is what adopting prepends to a file.
const adoptedHeader = AdoptedMarker + " GoNix edits only the settings it manages; everything\n" +
	"# else in this file is kept exactly as written.\n"

// IsAdopted reports whether content is an adopted file.
func IsAdopted(content string) bool {
	return strings.HasPrefix(strings.TrimLeft(content, "\uFEFF \t\r\n"), AdoptedMarker)
}

// AdoptContent returns content marked as adopted.
func AdoptContent(content string) string { return adoptedHeader + content }

// ReleaseContent undoes AdoptContent, turning the file back into one GoNix
// does not manage. It removes the marker line, and the second header line
// when it is the one AdoptContent wrote.
func ReleaseContent(content string) (string, error) {
	if !IsAdopted(content) {
		return "", fmt.Errorf("file is not adopted")
	}
	if rest, ok := strings.CutPrefix(content, adoptedHeader); ok {
		return rest, nil
	}
	i := strings.Index(content, AdoptedMarker)
	end := strings.IndexByte(content[i:], '\n')
	if end < 0 {
		return content[:i], nil
	}
	return content[:i] + content[i+end+1:], nil
}

// AdoptionReport describes how GoNix reads a hand-written file.
type AdoptionReport struct {
	Host Host
	// Kept lists the server-level directives GoNix has no setting for;
	// they stay in the file as written.
	Kept []string
	// Custom lists the locations GoNix keeps as custom configuration
	// because they do more than a plain reverse proxy.
	Custom []string
	// HTTPBlock is set when a separate plain-HTTP server block redirects to
	// HTTPS (the layout certbot's nginx plugin writes).
	HTTPBlock bool
	// Certificates is the number of ssl_certificate directives (two for a
	// dual RSA + ECDSA setup, which GoNix cannot switch to another
	// certificate).
	Certificates int
}

// InspectAdoption reads a hand-written file the way GoNix would after
// adopting it. It fails when the file cannot be adopted: a syntax GoNix
// cannot represent faithfully, or a layout other than one server block
// (optionally plus its HTTP-to-HTTPS redirect block).
func (r *Renderer) InspectAdoption(fileName, content string) (AdoptionReport, error) {
	p, err := r.parseAdopted(content)
	if err != nil {
		return AdoptionReport{}, err
	}
	rep := AdoptionReport{Host: p.host, HTTPBlock: p.lay.redirect != nil, Certificates: p.certs}
	rep.Host.Adopted = &Adoption{FileName: fileName, Source: content}
	rep.Host.Managed = true
	for _, n := range p.lay.main.Block.Children {
		if n.Kind == ast.KindDirective && !p.mapped[n] {
			rep.Kept = append(rep.Kept, directiveSummary(n))
		}
	}
	for _, l := range p.host.Locations {
		if l.IsCustom {
			rep.Custom = append(rep.Custom, l.Path)
		}
	}
	return rep, nil
}

// ParseAdoptedHost reads an adopted file into a Host whose changes
// Renderer.Render applies in place.
func (r *Renderer) ParseAdoptedHost(fileName, content string) (Host, error) {
	p, err := r.parseAdopted(content)
	if err != nil {
		return Host{}, err
	}
	h := p.host
	h.Managed = true
	h.Adopted = &Adoption{FileName: fileName, Source: content}
	return h, nil
}

// AdoptedServerNames returns every name in the server_name of an adopted
// file's main server block, or nil when it cannot be read.
func AdoptedServerNames(content string) []string {
	f, err := ast.Parse(content)
	if err != nil {
		return nil
	}
	lay, err := layoutOf(f)
	if err != nil {
		return nil
	}
	if n := lay.main.Block.Children.First("server_name"); n != nil {
		return n.Values()
	}
	return nil
}

func directiveSummary(n *ast.Node) string {
	parts := []string{n.Name.Text}
	for _, a := range n.Args {
		parts = append(parts, a.Text)
	}
	s := strings.Join(parts, " ")
	if n.Block != nil {
		s += " { ... }"
	}
	if len(s) > 80 {
		s = s[:77] + "..."
	}
	return s
}

// adoptedLayout locates the server blocks of an adopted file.
type adoptedLayout struct {
	file *ast.File
	main *ast.Node // the server block GoNix edits
	// redirect is a plain-HTTP server block that only redirects to HTTPS,
	// e.g. the one certbot's nginx plugin adds; nil when there is none.
	redirect *ast.Node
}

func layoutOf(f *ast.File) (adoptedLayout, error) {
	servers := f.Servers()
	switch len(servers) {
	case 0:
		return adoptedLayout{}, fmt.Errorf("the file has no server block")
	case 1:
		return adoptedLayout{file: f, main: servers[0]}, nil
	case 2:
		a, b := servers[0], servers[1]
		if hasSSLListen(a) && !isHTTPSRedirect(a) && isHTTPSRedirect(b) {
			return adoptedLayout{file: f, main: a, redirect: b}, nil
		}
		if hasSSLListen(b) && !isHTTPSRedirect(b) && isHTTPSRedirect(a) {
			return adoptedLayout{file: f, main: b, redirect: a}, nil
		}
	}
	return adoptedLayout{}, fmt.Errorf("the file has %d server blocks; GoNix adopts a file with one server block, "+
		"optionally plus the plain-HTTP block that redirects it to HTTPS (split the others into their own files)", len(servers))
}

// listenInfo is what GoNix reads from a listen directive.
type listenInfo struct {
	port             int
	ssl, http2, quic bool
}

func parseListen(n *ast.Node) (listenInfo, bool) {
	if len(n.Args) == 0 {
		return listenInfo{}, false
	}
	addr := n.Arg(0)
	if strings.HasPrefix(addr, "unix:") {
		return listenInfo{}, false
	}
	portText := addr
	if i := strings.LastIndexByte(addr, ':'); i >= 0 && !strings.HasSuffix(addr, "]") {
		portText = addr[i+1:]
	}
	li := listenInfo{port: 80}
	if p, err := strconv.Atoi(portText); err == nil {
		li.port = p
	} else if !strings.ContainsAny(addr, ".:[") && addr != "localhost" && addr != "*" {
		return listenInfo{}, false
	}
	for _, a := range n.Args[1:] {
		switch a.Value() {
		case "ssl":
			li.ssl = true
		case "http2":
			li.http2 = true
		case "quic":
			li.quic = true
		}
	}
	return li, true
}

func hasSSLListen(srv *ast.Node) bool {
	for _, n := range srv.Block.Children.Find("listen") {
		if li, ok := parseListen(n); ok && li.ssl {
			return true
		}
	}
	return false
}

var redirectCodes = map[string]bool{"301": true, "302": true, "307": true, "308": true}

// isHTTPSReturn matches `return 301 https://...;`.
func isHTTPSReturn(n *ast.Node) bool {
	return n.Is("return") && n.Block == nil && len(n.Args) == 2 && redirectCodes[n.Arg(0)] && strings.HasPrefix(n.Arg(1), "https://")
}

// condition returns the words of an `if (...)` condition.
func condition(n *ast.Node) []string {
	parts := make([]string, len(n.Args))
	for i, a := range n.Args {
		parts[i] = a.Text
	}
	c := strings.TrimSpace(strings.Join(parts, " "))
	if !strings.HasPrefix(c, "(") || !strings.HasSuffix(c, ")") {
		return nil
	}
	words := strings.Fields(c[1 : len(c)-1])
	for i, w := range words {
		words[i] = strings.Trim(w, `"'`)
	}
	return words
}

// onlyHTTPSReturn reports whether a block holds just a redirect to HTTPS
// (and comments).
func onlyHTTPSReturn(n *ast.Node) bool {
	if n.Block == nil || n.Block.HasRawBody {
		return false
	}
	found := false
	for _, c := range n.Block.Children {
		if c.Kind == ast.KindComment {
			continue
		}
		if !isHTTPSReturn(c) || found {
			return false
		}
		found = true
	}
	return found
}

// isSchemeRedirect matches `if ($scheme != "https") { return 301 https://...; }`,
// the plain-HTTP redirect of a server block that listens on both ports.
func isSchemeRedirect(n *ast.Node) bool {
	if !n.Is("if") {
		return false
	}
	w := condition(n)
	return len(w) == 3 && w[0] == "$scheme" && w[1] == "!=" && w[2] == "https" && onlyHTTPSReturn(n)
}

// hostIfName returns NAME for certbot's `if ($host = NAME) { return 301 https://...; }`.
func hostIfName(n *ast.Node) (string, bool) {
	if !n.Is("if") {
		return "", false
	}
	w := condition(n)
	if len(w) != 3 || w[0] != "$host" || w[1] != "=" || !onlyHTTPSReturn(n) {
		return "", false
	}
	return w[2], true
}

// isHTTPSRedirect reports whether srv is a plain-HTTP server block that
// does nothing but send clients to HTTPS.
func isHTTPSRedirect(srv *ast.Node) bool {
	redirects := false
	for _, n := range srv.Block.Children {
		if n.Kind == ast.KindComment {
			continue
		}
		switch n.Name.Text {
		case "listen":
			li, ok := parseListen(n)
			if !ok || li.ssl || li.quic {
				return false
			}
		case "server_name", "access_log", "error_log":
		case "return":
			switch {
			case isHTTPSReturn(n):
				redirects = true
			case len(n.Args) == 1 && n.Arg(0) == "404":
			default:
				return false
			}
		case "if":
			if _, ok := hostIfName(n); !ok {
				return false
			}
			redirects = true
		default:
			return false
		}
	}
	return redirects
}

// adoptedParse is a file read as an adopted host.
type adoptedParse struct {
	host   Host
	lay    adoptedLayout
	mapped map[*ast.Node]bool // server-level nodes a host setting came from
	certs  int
}

func (r *Renderer) parseAdopted(content string) (adoptedParse, error) {
	f, err := ast.Parse(content)
	if err != nil {
		return adoptedParse{}, fmt.Errorf("cannot read the file: %w", err)
	}
	lay, err := layoutOf(f)
	if err != nil {
		return adoptedParse{}, err
	}
	p := adoptedParse{lay: lay, mapped: map[*ast.Node]bool{}}
	r.mapServer(&p)
	return p, nil
}

// snippetKind classifies an include of one of GoNix's own snippets.
type snippetKind int

const (
	snipNone snippetKind = iota
	snipIPList
	snipPreset
	snipRateLimit
	snipCountry
	snipErrorPages
)

// snippetRef tells which GoNix snippet an include path refers to. Only
// files in GoNix's snippets directory count, so a hand-written include that
// merely looks similar is never mistaken for one.
func (r *Renderer) snippetRef(path string) (snippetKind, string) {
	if r == nil || r.snippetsDir == "" || filepath.Clean(filepath.Dir(path)) != filepath.Clean(r.snippetsDir) {
		return snipNone, ""
	}
	base, ok := strings.CutSuffix(filepath.Base(path), ".conf")
	if !ok {
		return snipNone, ""
	}
	for _, k := range []struct {
		kind   snippetKind
		prefix string
	}{
		{snipIPList, IPListFilePrefix}, {snipPreset, PresetFilePrefix}, {snipRateLimit, RateLimitFilePrefix},
		{snipCountry, CountryListFilePrefix}, {snipErrorPages, SnippetFilePrefix},
	} {
		name, ok := strings.CutPrefix(base, k.prefix)
		if !ok {
			continue
		}
		valid := ValidSnippetName(name)
		if k.kind == snipCountry {
			valid = ValidCountryListName(name)
		}
		if valid {
			return k.kind, name
		}
	}
	return snipNone, ""
}

// accessListRef returns the GoNix access list an auth_basic_user_file path
// refers to, or "" for any other file.
func (r *Renderer) accessListRef(path string) string {
	if r == nil || r.accessListFileDir == "" || filepath.Clean(filepath.Dir(path)) != filepath.Clean(r.accessListFileDir) {
		return ""
	}
	name, ok := strings.CutSuffix(filepath.Base(path), ".htpasswd")
	if !ok || name == "" {
		return ""
	}
	return name
}

func (r *Renderer) mapServer(p *adoptedParse) {
	h := Host{Managed: true, AccessLog: true, ErrorLog: true}
	srv := p.lay.main.Block.Children
	mark := func(n *ast.Node) { p.mapped[n] = true }
	var redirect *ast.Node
	var certs, keys []*ast.Node
	accessOff, errorLogs, errorOff := 0, 0, 0

	for _, n := range srv {
		if n.Kind != ast.KindDirective {
			continue
		}
		switch n.Name.Text {
		case "listen":
			li, ok := parseListen(n)
			if !ok {
				continue
			}
			mark(n)
			switch {
			case li.quic:
				h.SSL.HTTP3 = true
			case li.ssl:
				h.ListenSSL, h.SSL.Enabled = li.port, true
				h.SSL.HTTP2 = h.SSL.HTTP2 || li.http2
			case h.Listen == 0:
				h.Listen = li.port
			}
		case "server_name":
			if h.ServerName == "" && len(n.Args) > 0 {
				names := n.Values()
				h.ServerName, h.ServerAlias = names[0], names[1:]
				mark(n)
			}
		case "http2":
			h.SSL.HTTP2 = h.SSL.HTTP2 || n.Arg(0) == "on"
			mark(n)
		case "ssl_certificate":
			certs = append(certs, n)
			mark(n)
		case "ssl_certificate_key":
			keys = append(keys, n)
			mark(n)
		case "root":
			if len(n.Args) == 1 {
				h.Root = n.Arg(0)
				mark(n)
			}
		case "return":
			if n.Block == nil && len(n.Args) == 2 && redirectCodes[n.Arg(0)] && redirect == nil {
				redirect = n
				mark(n)
			}
		case "access_log":
			if n.Arg(0) == "off" {
				accessOff++
			}
			mark(n)
		case "error_log":
			errorLogs++
			if v := n.Arg(0); v == "/dev/null" || v == "off" {
				errorOff++
			}
			mark(n)
		case "auth_basic":
			mark(n)
		case "auth_basic_user_file":
			if name := r.accessListRef(n.Arg(0)); name != "" && h.AccessListName == "" {
				h.AccessListName = name
				mark(n)
			}
		case "include":
			kind, name := r.snippetRef(n.Arg(0))
			switch kind {
			case snipIPList:
				h.IPListName = name
			case snipPreset:
				h.Presets = append(h.Presets, name)
			case snipRateLimit:
				h.RateLimit = name
			case snipCountry:
				h.CountryList = name
			case snipErrorPages:
				h.ErrorPagesSnippet = name
			default:
				continue
			}
			mark(n)
		case "satisfy":
			if n.Arg(0) == "any" {
				h.SatisfyAny = true
				mark(n)
			}
		case "set":
			if n.Arg(0) == "$gonix_maintenance" {
				h.Maintenance = true
				mark(n)
			}
		case "if":
			switch {
			case isSchemeRedirect(n):
				h.SSL.ForceHTTPS = true
				mark(n)
			case len(n.Args) > 0 && strings.Contains(n.Args[0].Text, "$gonix_maintenance"):
				mark(n)
			}
		case "location":
			h.Locations = append(h.Locations, r.mapLocation(n))
			mark(n)
		}
	}
	// auth_basic belongs to GoNix only together with one of its access
	// lists; otherwise it is the file's own.
	if h.AccessListName == "" {
		for _, n := range srv.Find("auth_basic") {
			delete(p.mapped, n)
		}
	}
	// `access_log off` cancels every access log of its level, while
	// error_log has no "off" and only stops when every target discards.
	h.AccessLog = accessOff == 0
	h.ErrorLog = errorLogs == 0 || errorOff < errorLogs
	if len(certs) > 0 {
		h.SSL.CertFile = certs[0].Arg(0)
	}
	if len(keys) > 0 {
		h.SSL.KeyFile = keys[0].Arg(0)
	}
	p.certs = len(certs)

	if rd := p.lay.redirect; rd != nil {
		h.SSL.ForceHTTPS = true
		for _, n := range rd.Block.Children.Find("listen") {
			if li, ok := parseListen(n); ok && h.Listen == 0 {
				h.Listen = li.port
			}
		}
	}
	if !h.SSL.Enabled {
		// Without SSL these have nothing to apply to; keeping them would
		// only make the host compare unequal to one built from scratch.
		h.SSL = SSLConfig{}
	}

	switch {
	case redirect != nil && len(h.Locations) == 0:
		h.Mode = ModeRedirect
		h.Redirect.Code, _ = strconv.Atoi(redirect.Arg(0))
		target := redirect.Arg(1)
		if t, ok := strings.CutSuffix(target, "$request_uri"); ok {
			h.Redirect.PreservePath = true
			target = t
		}
		h.Redirect.Target = target
	case h.Root != "" && proxyAt(h.Locations, "/") == nil:
		h.Mode = ModeStatic
	default:
		h.Mode = ModeReverseProxy
	}
	if h.Mode != ModeRedirect && redirect != nil {
		delete(p.mapped, redirect)
	}
	p.host = h
}

func proxyAt(locs []Location, path string) *ProxyConfig {
	for _, l := range locs {
		if l.Path == path && l.Proxy != nil {
			return l.Proxy
		}
	}
	return nil
}

// locationPath is a location's arguments as written, e.g. "/api/" or
// "~ \.php$".
func locationPath(n *ast.Node) string {
	parts := make([]string, len(n.Args))
	for i, a := range n.Args {
		parts[i] = a.Text
	}
	return strings.Join(parts, " ")
}

// blockBody is the exact text between a block's braces.
func blockBody(n *ast.Node) string {
	var b strings.Builder
	for _, c := range n.Block.Children {
		b.WriteString(c.String())
	}
	b.WriteString(n.Block.CloseSpace)
	return b.String()
}

// mapLocation reads a location as a reverse proxy when it is one GoNix can
// edit without losing anything, and as custom configuration otherwise.
func (r *Renderer) mapLocation(n *ast.Node) Location {
	loc := Location{Path: locationPath(n)}
	if n.Block == nil || n.Block.HasRawBody {
		loc.IsCustom = true
		return loc
	}
	if r.mapProxyLocation(n.Block.Children, &loc) {
		return loc
	}
	return Location{Path: loc.Path, IsCustom: true, CustomConfig: blockBody(n)}
}

// proxyHeaders are the proxy_set_header lines GoNix renders, keyed by
// lower-cased header name.
var proxyHeaders = map[string]string{
	"host":              "$host",
	"x-real-ip":         "$remote_addr",
	"x-forwarded-for":   "$proxy_add_x_forwarded_for",
	"x-forwarded-proto": "$scheme",
	"upgrade":           "$http_upgrade",
	"connection":        "upgrade",
}

func (r *Renderer) mapProxyLocation(children ast.List, loc *Location) bool {
	p := &ProxyConfig{BufferingEnabled: true}
	seen := map[string]bool{}
	authBasic := false
	for _, n := range children {
		if n.Kind == ast.KindComment {
			continue
		}
		name := n.Name.Text
		key := name
		if name == "proxy_set_header" {
			key += " " + strings.ToLower(n.Arg(0))
		}
		if seen[key] || n.Block != nil {
			return false
		}
		seen[key] = true
		switch name {
		case "proxy_pass":
			if len(n.Args) != 1 || !parseProxyPass(n.Arg(0), p) {
				return false
			}
		case "proxy_http_version", "proxy_connect_timeout", "proxy_read_timeout", "proxy_send_timeout", "client_max_body_size":
			if len(n.Args) != 1 {
				return false
			}
			v := n.Arg(0)
			switch name {
			case "proxy_http_version":
				p.HTTPVersion = v
			case "proxy_connect_timeout":
				p.ConnectTimeout = v
			case "proxy_read_timeout":
				p.ReadTimeout = v
			case "proxy_send_timeout":
				p.SendTimeout = v
			case "client_max_body_size":
				p.MaxBodySize = v
			}
		case "proxy_set_header":
			header := strings.ToLower(n.Arg(0))
			want, ok := proxyHeaders[header]
			if !ok || len(n.Args) != 2 || n.Arg(1) != want {
				return false
			}
		case "proxy_buffering":
			switch n.Arg(0) {
			case "off":
				p.BufferingEnabled = false
			case "on":
			default:
				return false
			}
		case "auth_basic":
			if len(n.Args) != 1 || n.Arg(0) == "off" {
				return false
			}
			authBasic = true
		case "auth_basic_user_file":
			if loc.AccessListName = r.accessListRef(n.Arg(0)); loc.AccessListName == "" {
				return false
			}
		case "include":
			switch kind, ref := r.snippetRef(n.Arg(0)); kind {
			case snipIPList:
				loc.IPListName = ref
			case snipRateLimit:
				loc.RateLimit = ref
			default:
				return false
			}
		default:
			return false
		}
	}
	h := func(name string) bool { return seen["proxy_set_header "+name] }
	if !seen["proxy_pass"] || h("x-real-ip") != h("x-forwarded-for") || h("upgrade") != h("connection") ||
		authBasic != (loc.AccessListName != "") {
		return false
	}
	p.ForwardHost, p.ForwardFor, p.ForwardProto, p.WebSocket = h("host"), h("x-forwarded-for"), h("x-forwarded-proto"), h("upgrade")
	loc.Proxy = p
	return true
}

// parseProxyPass accepts the proxy_pass targets GoNix renders: scheme and
// host[:port] (no URI part, which would change how paths are passed on,
// and no variables), or one of its upstream groups.
func parseProxyPass(target string, p *ProxyConfig) bool {
	scheme, rest, ok := strings.Cut(target, "://")
	if !ok || (scheme != "http" && scheme != "https") || rest == "" || strings.ContainsAny(rest, "/$?#") {
		return false
	}
	p.UpstreamScheme = scheme
	if group, ok := strings.CutPrefix(rest, UpstreamGroupPrefix); ok && ValidSnippetName(group) {
		p.UpstreamGroup = group
		return true
	}
	host, port := rest, 80
	if scheme == "https" {
		port = 443
	}
	if i := strings.LastIndexByte(rest, ':'); i >= 0 && !strings.HasSuffix(rest, "]") {
		n, err := strconv.Atoi(rest[i+1:])
		if err != nil || n < 1 || n > 65535 {
			return false
		}
		host, port = rest[:i], n
	}
	if host == "" || strings.HasPrefix(host, "unix:") {
		return false
	}
	p.UpstreamHost, p.UpstreamPort = host, port
	return true
}
