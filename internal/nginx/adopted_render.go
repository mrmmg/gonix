package nginx

import (
	"bytes"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/mrmmg/gonix/internal/nginx/ast"
)

// renderAdopted applies the differences between the host as its file
// reads (h.Adopted.Source) and h to that file, directive by directive.
func (r *Renderer) renderAdopted(h Host) (string, error) {
	base, err := r.parseAdopted(h.Adopted.Source)
	if err != nil {
		return "", fmt.Errorf("re-reading %s: %w", h.Adopted.FileName, err)
	}
	e := newAdoptedEditor(r, base, h)
	for _, step := range []func() error{
		e.names, e.listens, e.ssl, e.mode, e.logs, e.accessList, e.satisfy,
		e.includes, e.country, e.maintenance, e.locations,
	} {
		if err := step(); err != nil {
			return "", fmt.Errorf("%s: %w", h.Adopted.FileName, err)
		}
	}
	out := ast.Print(base.lay.file)

	// Read the result back: it must be exactly the requested host, so a
	// change GoNix cannot express in this file is refused rather than
	// written half-applied.
	got, err := r.parseAdopted(out)
	if err != nil {
		return "", fmt.Errorf("%s: the edited file could not be read back (%v); nothing was written", h.Adopted.FileName, err)
	}
	if got.host.Mode != ModeRedirect && h.Mode != ModeRedirect {
		// Static and reverse proxy only describe the file (a root, a
		// proxy at /); removing location / may turn one into the other.
		got.host.Mode = h.Mode
	}
	if diff := r.hostDiff(got.host, h); len(diff) > 0 {
		return "", fmt.Errorf("%s: GoNix cannot apply this change to the hand-written file (%s); edit the file by hand",
			h.Adopted.FileName, strings.Join(diff, ", "))
	}
	return out, nil
}

// adoptedEditor edits an adopted file's syntax tree from base to want.
type adoptedEditor struct {
	r    *Renderer
	p    adoptedParse
	srv  *ast.List // direct children of the main server block
	base Host
	want Host
	ind  string // indentation of server-level directives
	unit string // one indentation step
}

func newAdoptedEditor(r *Renderer, p adoptedParse, want Host) *adoptedEditor {
	srv := &p.lay.main.Block.Children
	outer := p.lay.main.Name.Space
	if i := strings.LastIndexByte(outer, '\n'); i >= 0 {
		outer = outer[i+1:]
	}
	ind := srv.Indent(outer + "    ")
	unit := strings.TrimPrefix(ind, outer)
	if unit == "" || unit == ind && outer != "" {
		unit = "    "
	}
	return &adoptedEditor{r: r, p: p, srv: srv, base: p.host, want: want, ind: ind, unit: unit}
}

// step stands for one indentation step in the text given to fragment. It
// is a control character so it never collides with a tab in user content.
const step = "\x01"

// fragment parses text, which uses step for one indentation step and
// starts each line with "\n", into nodes indented under prefix.
func (e *adoptedEditor) fragment(prefix, text string) (ast.List, error) {
	text = strings.ReplaceAll(text, "\n", "\n"+prefix)
	text = strings.ReplaceAll(text, step, e.unit)
	text = strings.ReplaceAll(text, "\n"+prefix+"\n", "\n\n") // no trailing spaces on blank lines
	nodes, _, err := ast.ParseNodes(text)
	if err != nil {
		return nil, fmt.Errorf("internal error building %q: %w", text, err)
	}
	return nodes, nil
}

func (e *adoptedEditor) directive(name string, values ...string) *ast.Node {
	return ast.NewDirective("\n"+e.ind, name, values...)
}

// beforeLocations is where new server-level directives go: before the
// first location block, or at the end.
func (e *adoptedEditor) beforeLocations() int {
	for i, n := range *e.srv {
		if n.Is("location") {
			return trimLeadingComments(*e.srv, i)
		}
	}
	return len(*e.srv)
}

// trimLeadingComments moves i back over the own-line comments right
// before it, which describe the node at i.
func trimLeadingComments(l ast.List, i int) int {
	for i > 0 && l[i-1].Kind == ast.KindComment && !l[i-1].IsTrailingComment() {
		i--
	}
	return i
}

// afterNamed returns the index just past the first directive called name
// (and its same-line comment), or -1.
func (e *adoptedEditor) afterNamed(name string) int {
	if n := e.srv.First(name); n != nil {
		return e.srv.After(n)
	}
	return -1
}

// headerPoint is where SSL and HTTP/2 directives go: after server_name,
// else after the last listen, else at the top.
func (e *adoptedEditor) headerPoint() int {
	if i := e.afterNamed("server_name"); i >= 0 {
		return i
	}
	if l := e.srv.Find("listen"); len(l) > 0 {
		return e.srv.After(l[len(l)-1])
	}
	return 0
}

// rewritePoint is where a server-level `if`/`set` that must run before
// the file's own rewrite directives goes: before the first set, if,
// return, rewrite or break (and GoNix's country list include), but after
// the HTTP-to-HTTPS redirect.
func (e *adoptedEditor) rewritePoint() int {
	for i, n := range *e.srv {
		if n.Kind != ast.KindDirective || isSchemeRedirect(n) {
			continue
		}
		switch n.Name.Text {
		case "set", "if", "return", "rewrite", "break", "location":
			return trimLeadingComments(*e.srv, i)
		case "include":
			if k, _ := e.r.snippetRef(n.Arg(0)); k == snipCountry {
				return trimLeadingComments(*e.srv, i)
			}
		}
	}
	return len(*e.srv)
}

// setDirective gives the first `name` directive new arguments, or inserts
// one at the point at.
func (e *adoptedEditor) setDirective(at func() int, name string, values ...string) *ast.Node {
	if n := e.srv.First(name); n != nil {
		n.SetArgs(values...)
		return n
	}
	n := e.directive(name, values...)
	e.srv.InsertAt(at(), n)
	return n
}

func unsupported(what string) error {
	return fmt.Errorf("%s is not supported for an adopted host; edit the file by hand", what)
}

// --- server names ---

func (e *adoptedEditor) names() error {
	if e.want.ServerName != e.base.ServerName {
		return unsupported("renaming")
	}
	if slices.Equal(e.want.ServerAlias, e.base.ServerAlias) {
		return nil
	}
	names := append([]string{e.want.ServerName}, e.want.ServerAlias...)
	e.srv.First("server_name").SetArgs(names...)
	// The plain-HTTP block follows only when it served the same names; one
	// that redirects other names (e.g. www to the bare domain) is its own.
	if rd := e.p.lay.redirect; rd != nil {
		l := &rd.Block.Children
		n := l.First("server_name")
		old := append([]string{e.base.ServerName}, e.base.ServerAlias...)
		if n == nil || !sameNames(n.Values(), old) {
			return nil
		}
		n.SetArgs(names...)
		return e.syncHostIfs(l, names)
	}
	return nil
}

func sameNames(a, b []string) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(slices.Compact(a), slices.Compact(b))
}

// syncHostIfs keeps certbot's per-name redirects (`if ($host = NAME)`) in
// step with the server names, so a new alias is redirected to HTTPS too
// instead of getting the block's final `return 404`.
func (e *adoptedEditor) syncHostIfs(l *ast.List, names []string) error {
	var ifs []*ast.Node
	have := map[string]bool{}
	for _, n := range *l {
		if name, ok := hostIfName(n); ok {
			ifs = append(ifs, n)
			have[name] = true
		}
	}
	if len(ifs) == 0 {
		return nil
	}
	ind := l.Indent(e.ind)
	at := l.Index(ifs[0])
	for _, name := range names {
		if have[name] {
			continue
		}
		nodes, err := e.fragment(ind, "\nif ($host = "+name+") {\n"+step+"return 301 "+e.want.HTTPSBase()+"$request_uri;\n}")
		if err != nil {
			return err
		}
		l.InsertAt(at, nodes...)
		at += len(nodes)
	}
	for _, n := range ifs {
		if name, _ := hostIfName(n); !slices.Contains(names, name) {
			l.RemoveNode(n)
		}
	}
	return nil
}

// --- listen ports and SSL ---

func (e *adoptedEditor) listens() error {
	if e.want.Listen != e.base.Listen {
		return unsupported("changing the HTTP port")
	}
	if e.base.SSL.Enabled && e.want.SSL.Enabled && e.want.ListenSSL != e.base.ListenSSL {
		return unsupported("changing the HTTPS port")
	}
	return nil
}

func (e *adoptedEditor) ssl() error {
	b, w := e.base.SSL, e.want.SSL
	switch {
	case !b.Enabled && !w.Enabled:
		return nil
	case b.Enabled && !w.Enabled:
		return unsupported("turning SSL off")
	case !b.Enabled:
		return e.enableSSL()
	}
	if w.HTTP3 != b.HTTP3 {
		return unsupported("changing HTTP/3")
	}
	if w.CertFile != b.CertFile || w.KeyFile != b.KeyFile {
		if err := e.setCertificate(w.CertFile, w.KeyFile); err != nil {
			return err
		}
	}
	if w.HTTP2 != b.HTTP2 {
		e.setHTTP2(w.HTTP2)
	}
	if w.ForceHTTPS != b.ForceHTTPS {
		return e.setForceHTTPS(w.ForceHTTPS)
	}
	return nil
}

func (e *adoptedEditor) enableSSL() error {
	w := e.want
	if w.SSL.HTTP3 {
		return unsupported("HTTP/3")
	}
	port := w.ListenSSL
	if port == 0 {
		return fmt.Errorf("no HTTPS port given")
	}
	listen := []string{strconv.Itoa(port), "ssl"}
	if w.SSL.HTTP2 && !e.r.Build.HTTP2Directive() {
		listen = append(listen, "http2")
	}
	at := 0
	if l := e.srv.Find("listen"); len(l) > 0 {
		at = e.srv.After(l[len(l)-1])
	}
	e.srv.InsertAt(at, e.directive("listen", listen...))
	if w.SSL.HTTP2 && e.r.Build.HTTP2Directive() {
		e.srv.InsertAt(e.headerPoint(), e.directive("http2", "on"))
	}
	if err := e.setCertificate(w.SSL.CertFile, w.SSL.KeyFile); err != nil {
		return err
	}
	if w.SSL.ForceHTTPS {
		return e.setForceHTTPS(true)
	}
	return nil
}

func isCertbotPath(p string) bool { return strings.Contains(p, "/letsencrypt/live/") }

func (e *adoptedEditor) setCertificate(cert, key string) error {
	certs, keys := e.srv.Find("ssl_certificate"), e.srv.Find("ssl_certificate_key")
	if len(certs) > 1 || len(keys) > 1 {
		return unsupported(fmt.Sprintf("changing the certificate of a server block with %d certificates", len(certs)))
	}
	set := func(nodes []*ast.Node, name, path string) {
		if len(nodes) == 0 {
			e.srv.InsertAt(e.headerPoint(), e.directive(name, path))
			return
		}
		n := nodes[0]
		n.SetArgs(path)
		// "# managed by Certbot" would no longer be true.
		if c := e.srv.TrailingComment(n); c != nil && strings.Contains(c.Name.Text, "managed by Certbot") && !isCertbotPath(path) {
			e.srv.RemoveNode(c)
		}
	}
	// Inserted in reverse, so the certificate ends up first.
	set(keys, "ssl_certificate_key", key)
	set(certs, "ssl_certificate", cert)
	return nil
}

func (e *adoptedEditor) setHTTP2(on bool) {
	if !on {
		e.srv.Remove("http2")
		for _, n := range e.srv.Find("listen") {
			if li, ok := parseListen(n); ok && li.ssl {
				n.Args = slices.DeleteFunc(n.Args, func(t ast.Token) bool { return t.Value() == "http2" })
			}
		}
		return
	}
	if e.r.Build.HTTP2Directive() {
		e.setDirective(e.headerPoint, "http2", "on")
		return
	}
	for _, n := range e.srv.Find("listen") {
		if li, ok := parseListen(n); ok && li.ssl && !li.http2 {
			n.Args = append(n.Args, ast.Token{Space: " ", Text: "http2"})
		}
	}
}

func (e *adoptedEditor) setForceHTTPS(on bool) error {
	if e.p.lay.redirect != nil {
		return unsupported("changing the HTTP-to-HTTPS redirect of a file that has a separate plain-HTTP server block")
	}
	if !on {
		for _, n := range *e.srv {
			if isSchemeRedirect(n) {
				e.srv.RemoveNode(n)
			}
		}
		return nil
	}
	nodes, err := e.fragment(e.ind, "\n\nif ($scheme != \"https\") {\n"+step+"return 301 "+e.want.HTTPSBase()+"$request_uri;\n}")
	if err != nil {
		return err
	}
	at := e.headerPoint()
	for i, n := range *e.srv {
		if i >= at && (n.Is("ssl_certificate") || n.Is("ssl_certificate_key") || n.Is("http2")) {
			at = e.srv.After(n)
		}
	}
	e.srv.InsertAt(at, nodes...)
	return nil
}

// --- mode, root, redirect ---

func (e *adoptedEditor) mode() error {
	b, w := e.base, e.want
	if w.Mode != b.Mode {
		return unsupported(fmt.Sprintf("changing the kind of host (%s to %s)", b.Mode, w.Mode))
	}
	if w.Root != b.Root {
		switch {
		case w.Root == "":
			e.srv.Remove("root")
		default:
			e.setDirective(e.beforeLocations, "root", w.Root)
		}
	}
	if w.Mode == ModeRedirect && w.Redirect != b.Redirect {
		// A hand-written target (e.g. https://$host) is kept as it is when
		// only the code or path handling changes.
		if w.Redirect.Target != b.Redirect.Target {
			if err := ValidateRedirectTarget(w.Redirect.Target); err != nil {
				return err
			}
		}
		if !validRedirectCode(w.Redirect.Code) {
			return fmt.Errorf("status code %d is not one of %v", w.Redirect.Code, RedirectCodes)
		}
		for _, n := range *e.srv {
			if n.Is("return") && len(n.Args) == 2 && redirectCodes[n.Arg(0)] {
				n.SetArgs(strconv.Itoa(w.Redirect.Code), w.Redirect.ReturnURL())
				break
			}
		}
	}
	return nil
}

// --- logs ---

func (e *adoptedEditor) logs() error {
	// Both ways, every directive for the log is replaced by a single one,
	// so no other directive keeps logging (or keeps logging off).
	set := func(name string, old, want bool, on, off string) {
		if old == want {
			return
		}
		value := off
		if want {
			value = on
		}
		at := e.beforeLocations()
		if n := e.srv.First(name); n != nil {
			at = e.srv.Index(n)
		}
		e.srv.Remove(name)
		e.srv.InsertAt(min(at, len(*e.srv)), e.directive(name, value))
	}
	set("access_log", e.base.AccessLog, e.want.AccessLog, filepath.Join(e.r.accessLogDir, e.want.ServerName+".access.log"), "off")
	set("error_log", e.base.ErrorLog, e.want.ErrorLog, filepath.Join(e.r.errorLogDir, e.want.ServerName+".error.log"), "/dev/null")
	return nil
}

// --- access lists, satisfy ---

// setAccessList switches the auth_basic pair of l from old to want.
func (e *adoptedEditor) setAccessList(l *ast.List, at func() int, ind, old, want string) {
	if old == want {
		return
	}
	if want == "" {
		for _, n := range l.Find("auth_basic_user_file") {
			if e.r.accessListRef(n.Arg(0)) == old {
				l.RemoveNode(n)
			}
		}
		l.Remove("auth_basic")
		return
	}
	path := e.r.htpasswdPath(want)
	if n := l.First("auth_basic_user_file"); n != nil {
		n.SetArgs(path)
	} else {
		l.InsertAt(at(), ast.NewDirective("\n"+ind, "auth_basic_user_file", path))
	}
	switch n := l.First("auth_basic"); {
	case n == nil:
		l.InsertAt(l.Index(l.First("auth_basic_user_file")), ast.NewDirective("\n"+ind, "auth_basic", "Restricted"))
	case n.Arg(0) == "off":
		n.SetArgs("Restricted")
	}
}

func (e *adoptedEditor) accessList() error {
	e.setAccessList(e.srv, e.beforeLocations, e.ind, e.base.AccessListName, e.want.AccessListName)
	return nil
}

func (e *adoptedEditor) satisfy() error {
	if e.want.SatisfyAny == e.base.SatisfyAny {
		return nil
	}
	if e.want.SatisfyAny {
		e.setDirective(e.beforeLocations, "satisfy", "any")
	} else {
		for _, n := range e.srv.Find("satisfy") {
			if n.Arg(0) == "any" {
				e.srv.RemoveNode(n)
			}
		}
	}
	return nil
}

// --- GoNix snippets ---

// swapInclude replaces the include of snippet (kind, old) in l with one
// of (kind, want), keeping its position; a new include goes to at().
func (e *adoptedEditor) swapInclude(l *ast.List, at func() int, ind string, kind snippetKind, old, want, path string) {
	if old == want {
		return
	}
	var cur *ast.Node
	if old != "" {
		for _, n := range l.Find("include") {
			if k, name := e.r.snippetRef(n.Arg(0)); k == kind && name == old {
				cur = n
				break
			}
		}
	}
	switch {
	case want == "" && cur != nil:
		l.RemoveNode(cur)
	case want == "":
	case cur != nil:
		cur.SetArgs(path)
	default:
		l.InsertAt(at(), ast.NewDirective("\n"+ind, "include", path))
	}
}

func (e *adoptedEditor) includes() error {
	r, b, w := e.r, e.base, e.want
	e.swapInclude(e.srv, e.beforeLocations, e.ind, snipIPList, b.IPListName, w.IPListName, r.ipListPath(w.IPListName))
	e.swapInclude(e.srv, e.beforeLocations, e.ind, snipRateLimit, b.RateLimit, w.RateLimit, r.snippetPath(RateLimitFileName, w.RateLimit))
	e.swapInclude(e.srv, e.beforeLocations, e.ind, snipErrorPages, b.ErrorPagesSnippet, w.ErrorPagesSnippet, r.snippetPath(SnippetFileName, w.ErrorPagesSnippet))

	// Presets go after the last one already there, else at the header.
	for _, name := range b.Presets {
		if !slices.Contains(w.Presets, name) {
			e.swapInclude(e.srv, nil, e.ind, snipPreset, name, "", "")
		}
	}
	for _, name := range w.Presets {
		if slices.Contains(b.Presets, name) {
			continue
		}
		at := func() int {
			last := -1
			for i, n := range *e.srv {
				if k, _ := r.snippetRef(n.Arg(0)); n.Is("include") && k == snipPreset {
					last = i
				}
			}
			if last >= 0 {
				return e.srv.After((*e.srv)[last])
			}
			return e.beforeLocations()
		}
		e.swapInclude(e.srv, at, e.ind, snipPreset, "", name, r.snippetPath(PresetFileName, name))
	}
	return nil
}

// country goes where it runs before the file's own rewrite directives:
// it answers 403 with an `if`, which a server-level `rewrite ... last` or
// `return` placed first would otherwise skip.
func (e *adoptedEditor) country() error {
	e.swapInclude(e.srv, e.rewritePoint, e.ind, snipCountry, e.base.CountryList, e.want.CountryList,
		e.r.snippetPath(CountryListFileName, e.want.CountryList))
	return nil
}

// --- maintenance ---

var maintenanceComments = []string{"# GoNix maintenance mode", "# in " + MaintenanceFilePrefix, "# GoNix's own error page URIs"}

func (e *adoptedEditor) maintenance() error {
	w := e.want
	if w.Maintenance == e.base.Maintenance {
		return nil
	}
	if !w.Maintenance {
		var drop []*ast.Node
		for i, n := range *e.srv {
			isSet := n.Is("set") && n.Arg(0) == "$gonix_maintenance"
			isIf := n.Is("if") && len(n.Args) > 0 && strings.Contains(n.Args[0].Text, "$gonix_maintenance")
			if !isSet && !isIf {
				continue
			}
			drop = append(drop, n)
			for j := i - 1; j >= 0; j-- {
				c := (*e.srv)[j]
				if c.Kind != ast.KindComment || !slices.ContainsFunc(maintenanceComments, func(p string) bool { return strings.HasPrefix(c.Name.Text, p) }) {
					break
				}
				drop = append(drop, c)
			}
		}
		for _, n := range drop {
			e.srv.RemoveNode(n)
		}
		return nil
	}
	if w.Mode == ModeRedirect {
		return fmt.Errorf("redirect hosts have no content to put into maintenance")
	}
	text := "\n\n# GoNix maintenance mode: 503 for everyone except the bypass addresses" +
		"\n# in " + MaintenanceFileName(w.ServerName) + " (http level)." +
		"\n# GoNix's own error page URIs are exempt, so a custom 503 page is served." +
		"\nset $gonix_maintenance $" + MaintenanceVar(w.ServerName) + "$uri;" +
		"\nif ($gonix_maintenance ~ \"^1(?!" + ErrorPageURIPrefix + ")\") {\n" + step + "return 503;\n}"
	nodes, err := e.fragment(e.ind, text)
	if err != nil {
		return err
	}
	e.srv.InsertAt(e.rewritePoint(), nodes...)
	return nil
}

// --- locations ---

func (e *adoptedEditor) locations() error {
	baseBy := map[string]Location{}
	for _, l := range e.base.Locations {
		baseBy[l.Path] = l
	}
	nodeBy := map[string]*ast.Node{}
	for _, n := range e.srv.Find("location") {
		nodeBy[locationPath(n)] = n
	}
	wantPaths := map[string]bool{}
	for _, l := range e.want.Locations {
		if wantPaths[l.Path] {
			return fmt.Errorf("location %s is listed twice", l.Path)
		}
		wantPaths[l.Path] = true
	}

	for _, b := range e.base.Locations {
		if !wantPaths[b.Path] {
			e.srv.RemoveNode(nodeBy[b.Path])
		}
	}
	var last *ast.Node
	for _, w := range e.want.Locations {
		b, existed := baseBy[w.Path]
		if existed {
			last = nodeBy[w.Path]
			if e.r.locationsEqual(b, w) {
				continue
			}
			if err := e.patchLocation(nodeBy[w.Path], b, w); err != nil {
				return fmt.Errorf("location %s: %w", w.Path, err)
			}
			continue
		}
		n, err := e.newLocation(w)
		if err != nil {
			return fmt.Errorf("location %s: %w", w.Path, err)
		}
		if last != nil {
			e.srv.InsertAt(e.srv.After(last), n)
		} else if locs := e.srv.Find("location"); len(locs) > 0 {
			e.srv.InsertAt(e.srv.After(locs[len(locs)-1]), n)
		} else {
			*e.srv = append(*e.srv, n)
		}
		last = n
	}
	return nil
}

// newLocation builds a location block the way host.tmpl would, indented
// like the rest of the file.
func (e *adoptedEditor) newLocation(w Location) (*ast.Node, error) {
	var text string
	if w.IsCustom {
		text = "location " + w.Path + " {" + reindent(w.CustomConfig, step) + "\n}"
	} else {
		resolved := w
		resolved.AccessListName = e.r.htpasswdPath(w.AccessListName)
		resolved.IPListName = e.r.ipListPath(w.IPListName)
		resolved.RateLimit = e.r.snippetPath(RateLimitFileName, w.RateLimit)
		var buf bytes.Buffer
		if err := e.r.tmpl.ExecuteTemplate(&buf, "location.tmpl", resolved); err != nil {
			return nil, fmt.Errorf("executing template: %w", err)
		}
		// The template indents with four spaces per level, starting at one.
		var lines []string
		for _, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
			trimmed := strings.TrimLeft(line, " ")
			level := (len(line) - len(trimmed)) / 4
			lines = append(lines, strings.Repeat(step, max(level-1, 0))+trimmed)
		}
		text = strings.Join(lines, "\n")
	}
	nodes, err := e.fragment(e.ind, "\n\n"+text)
	if err != nil {
		return nil, err
	}
	if len(nodes) != 1 || !nodes[0].Is("location") {
		return nil, fmt.Errorf("the location does not form a single block")
	}
	return nodes[0], nil
}

// reindent puts each non-empty line of a custom body on its own line at
// prefix, keeping the lines' indentation relative to each other.
func reindent(body, prefix string) string {
	lines := strings.Split(strings.Trim(body, "\n"), "\n")
	common := -1
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		n := len(l) - len(strings.TrimLeft(l, " \t"))
		if common < 0 || n < common {
			common = n
		}
	}
	var b strings.Builder
	for _, l := range lines {
		b.WriteByte('\n')
		if strings.TrimSpace(l) == "" {
			continue
		}
		b.WriteString(prefix + strings.TrimRight(l[common:], " \t"))
	}
	return b.String()
}

func (e *adoptedEditor) patchLocation(n *ast.Node, b, w Location) error {
	if b.IsCustom && w.IsCustom {
		children, tail, err := ast.ParseNodes(w.CustomConfig)
		if err != nil {
			return fmt.Errorf("the custom configuration does not parse: %w", err)
		}
		// Text typed on one line still gets the braces on lines of their own.
		if len(children) > 0 && !strings.Contains(children[0].Name.Space, "\n") {
			children[0].Name.Space = "\n" + e.ind + e.unit
		}
		if !strings.Contains(tail, "\n") {
			tail = "\n" + e.ind
		}
		n.Block.Children, n.Block.CloseSpace = children, tail
		return nil
	}
	if b.IsCustom != w.IsCustom {
		nn, err := e.newLocation(w)
		if err != nil {
			return err
		}
		nn.Name.Space = n.Name.Space
		e.srv.Replace(n, nn)
		return nil
	}
	l := &n.Block.Children
	ind := l.Indent(e.ind + e.unit)
	end := func() int { return len(*l) }
	bp, wp := b.Proxy, w.Proxy

	if bp.UpstreamURL() != wp.UpstreamURL() {
		l.First("proxy_pass").SetArgs(wp.UpstreamURL())
	}
	optional := func(name, old, want string) {
		switch {
		case old == want:
		case want == "":
			l.Remove(name)
		default:
			l.Set(name, want)
		}
	}
	optional("proxy_http_version", bp.HTTPVersion, wp.HTTPVersion)
	optional("proxy_connect_timeout", bp.ConnectTimeout, wp.ConnectTimeout)
	optional("proxy_read_timeout", bp.ReadTimeout, wp.ReadTimeout)
	optional("proxy_send_timeout", bp.SendTimeout, wp.SendTimeout)
	optional("client_max_body_size", bp.MaxBodySize, wp.MaxBodySize)
	if bp.BufferingEnabled != wp.BufferingEnabled {
		if wp.BufferingEnabled {
			l.Remove("proxy_buffering")
		} else {
			l.Set("proxy_buffering", "off")
		}
	}
	header := func(old, want bool, names ...string) {
		if old == want {
			return
		}
		for _, name := range names {
			if !want {
				for _, h := range l.Find("proxy_set_header") {
					if strings.EqualFold(h.Arg(0), name) {
						l.RemoveNode(h)
					}
				}
				continue
			}
			at := l.After(l.First("proxy_pass"))
			for _, h := range l.Find("proxy_set_header") {
				at = l.After(h)
			}
			l.InsertAt(at, ast.NewDirective("\n"+ind, "proxy_set_header", name, proxyHeaders[strings.ToLower(name)]))
		}
	}
	header(bp.ForwardHost, wp.ForwardHost, "Host")
	header(bp.ForwardFor, wp.ForwardFor, "X-Real-IP", "X-Forwarded-For")
	header(bp.ForwardProto, wp.ForwardProto, "X-Forwarded-Proto")
	header(bp.WebSocket, wp.WebSocket, "Upgrade", "Connection")

	e.setAccessList(l, end, ind, b.AccessListName, w.AccessListName)
	e.swapInclude(l, end, ind, snipIPList, b.IPListName, w.IPListName, e.r.ipListPath(w.IPListName))
	e.swapInclude(l, end, ind, snipRateLimit, b.RateLimit, w.RateLimit, e.r.snippetPath(RateLimitFileName, w.RateLimit))
	return nil
}

// --- comparison ---

func (r *Renderer) locationsEqual(a, b Location) bool {
	return reflect.DeepEqual(r.normalizeLocation(a), r.normalizeLocation(b))
}

// normalizeLocation reads a custom location the way it would read back
// from a file: a body that is just a plain proxy becomes one, and the
// whitespace of anything else does not matter.
func (r *Renderer) normalizeLocation(l Location) Location {
	if !l.IsCustom {
		return l
	}
	if nodes, _, err := ast.ParseNodes("location x {" + l.CustomConfig + "\n}"); err == nil && len(nodes) == 1 {
		if m := r.mapLocation(nodes[0]); !m.IsCustom {
			m.Path = l.Path
			return m
		}
	}
	l.CustomConfig = strings.Join(strings.Fields(l.CustomConfig), " ")
	l.Proxy = nil
	return l
}

// normalizeForDiff drops what an adopted file cannot or need not carry,
// so two hosts that render the same compare equal.
func (r *Renderer) normalizeForDiff(h Host) Host {
	h.Adopted, h.Enabled, h.Managed = nil, false, false
	if len(h.ServerAlias) == 0 {
		h.ServerAlias = nil
	}
	if !h.SSL.Enabled {
		h.SSL, h.ListenSSL = SSLConfig{}, 0
	}
	h.SSL.KeyType = ""
	if h.Mode != ModeRedirect {
		h.Redirect = RedirectConfig{}
	}
	if len(h.Presets) == 0 {
		h.Presets = nil
	} else {
		h.Presets = slices.Clone(h.Presets)
		sort.Strings(h.Presets)
	}
	locs := make([]Location, 0, len(h.Locations))
	for _, l := range h.Locations {
		locs = append(locs, r.normalizeLocation(l))
	}
	sort.SliceStable(locs, func(i, j int) bool { return locs[i].Path < locs[j].Path })
	h.Locations = locs
	return h
}

// hostDiff names the Host fields that differ between got and want.
func (r *Renderer) hostDiff(got, want Host) []string {
	a, b := reflect.ValueOf(r.normalizeForDiff(got)), reflect.ValueOf(r.normalizeForDiff(want))
	var out []string
	for i := 0; i < a.NumField(); i++ {
		if !reflect.DeepEqual(a.Field(i).Interface(), b.Field(i).Interface()) {
			out = append(out, a.Type().Field(i).Name)
		}
	}
	return out
}
