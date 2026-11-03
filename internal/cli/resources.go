package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/mrmmg/gonix/internal/certificates"
	"github.com/mrmmg/gonix/internal/geoblock"
	"github.com/mrmmg/gonix/internal/iplist"
	"github.com/mrmmg/gonix/internal/presets"
	"github.com/mrmmg/gonix/internal/ratelimit"
	"github.com/mrmmg/gonix/internal/realip"
	"github.com/mrmmg/gonix/internal/streams"
	"github.com/mrmmg/gonix/internal/upstreams"
)

func init() {
	register(
		command{path: "iplist list", summary: "List IP access lists", run: ipListList},
		command{path: "iplist create", args: "<name> --mode allow|block --entries \"IP/CIDR ...\"", summary: "Create an IP access list", write: true, run: ipListCreate},
		command{path: "iplist add", args: "<name> <ip/cidr>", summary: "Add an address to a list", write: true, run: ipListEntry(true)},
		command{path: "iplist remove", args: "<name> <ip/cidr>", summary: "Remove an address from a list", write: true, run: ipListEntry(false)},
		command{path: "iplist delete", args: "<name> --yes", summary: "Delete an unused IP access list", write: true, run: ipListDelete},

		command{path: "ratelimit list", summary: "List rate limits", run: rateLimitList},
		command{path: "ratelimit create", args: "<name> [--preset general|api|login] [--rate N --per s|m --burst N --nodelay --conn N]",
			summary: "Create a rate limit", write: true, run: rateLimitCreate},
		command{path: "ratelimit delete", args: "<name> --yes", summary: "Delete an unused rate limit", write: true, run: rateLimitDelete},

		command{path: "upstream list", summary: "List upstream groups", run: upstreamList},
		command{path: "upstream create", args: "<name> --servers \"host:port[ weight=N][ backup], ...\" [--method round_robin|least_conn|ip_hash]",
			summary: "Create an upstream group", write: true, run: upstreamCreate},
		command{path: "upstream delete", args: "<name> --yes", summary: "Delete an unused upstream group", write: true, run: upstreamDelete},

		command{path: "stream list", summary: "List TCP/UDP stream proxies", run: streamList},
		command{path: "stream setup", summary: "Create the stream { } block (when nginx.conf has none)", write: true, run: streamSetup},
		command{path: "stream create", args: "<name> --port N --backend host:port [--udp] [--ip-list NAME]",
			summary: "Create a stream proxy", write: true, run: streamCreate},
		command{path: "stream enable", args: "<name>", summary: "Enable a stream proxy", write: true, run: streamToggle(true)},
		command{path: "stream disable", args: "<name>", summary: "Disable a stream proxy", write: true, run: streamToggle(false)},
		command{path: "stream delete", args: "<name> --yes", summary: "Delete a stream proxy", write: true, run: streamDelete},

		command{path: "country list", summary: "List country lists (GeoIP)", run: countryList},
		command{path: "country setup", summary: "Load the GeoIP database (needs the geoip2 module)", write: true, run: countrySetup},
		command{path: "country create", args: "<name> --mode allow|block --countries \"DE FR ...\"", summary: "Create a country list", write: true, run: countryCreate},
		command{path: "country delete", args: "<name> --yes", summary: "Delete an unused country list", write: true, run: countryDelete},

		command{path: "preset list", summary: "List host presets", run: presetList},
		command{path: "realip show", summary: "Show real client IP settings", run: realIPShow},
		command{path: "realip cloudflare", args: "[--refresh]", summary: "Trust Cloudflare (optionally download current ranges)", write: true, run: realIPCloudflare},
		command{path: "realip custom", args: "--proxies \"IP/CIDR ...\" --header X-Forwarded-For|X-Real-IP", summary: "Trust your own proxies", write: true, run: realIPCustom},
		command{path: "realip disable", summary: "Stop trusting proxy headers", write: true, run: realIPDisable},

		command{path: "accesslist list", summary: "List Basic Auth access lists", run: accessListList},
		command{path: "errorpages list", summary: "List error pages snippets", run: errorPagesList},
		command{path: "cert list", summary: "List certificates and their expiry", run: certList},
	)
}

func parseIPs(s string) ([]string, error) { return iplist.ParseEntries(s) }

func nameArg(fs *flag.FlagSet, args []string) (string, error) {
	pos, err := parseFlags(fs, args)
	if err != nil {
		return "", err
	}
	if err := exactArgs(pos, 1); err != nil {
		return "", err
	}
	return pos[0], nil
}

func table(w io.Writer, header string, rows [][]string) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, header)
	for _, r := range rows {
		fmt.Fprintln(tw, strings.Join(r, "\t"))
	}
	tw.Flush()
}

// --- IP access lists ---

type ipListJSON struct {
	Name    string   `json:"name"`
	Mode    string   `json:"mode"`
	Entries []string `json:"entries"`
	UsedBy  []string `json:"used_by"`
}

func ipListList(e *env, args []string) error {
	if _, err := parseFlags(flag.NewFlagSet("iplist list", flag.ContinueOnError), args); err != nil {
		return err
	}
	lists, err := e.svc.IPLists.Store.List()
	if err != nil {
		return err
	}
	out := []ipListJSON{}
	for _, l := range lists {
		users, _ := e.svc.IPLists.UsedBy(l.Name)
		out = append(out, ipListJSON{l.Name, string(l.Mode), l.Entries, users})
	}
	return e.show(out, func(w io.Writer) {
		var rows [][]string
		for _, l := range out {
			rows = append(rows, []string{l.Name, l.Mode, strings.Join(l.Entries, " "), strings.Join(l.UsedBy, ", ")})
		}
		table(w, "NAME\tMODE\tENTRIES\tUSED BY", rows)
	})
}

func parseMode(v string) (iplist.Mode, error) {
	switch v {
	case "allow", "allowlist":
		return iplist.Allowlist, nil
	case "block", "blocklist":
		return iplist.Blocklist, nil
	}
	return "", usageErr("--mode must be allow or block")
}

func ipListCreate(e *env, args []string) error {
	fs := flag.NewFlagSet("iplist create", flag.ContinueOnError)
	mode := fs.String("mode", "", "allow or block")
	entries := fs.String("entries", "", "IP addresses or CIDR ranges")
	name, err := nameArg(fs, args)
	if err != nil {
		return err
	}
	m, err := parseMode(*mode)
	if err != nil {
		return err
	}
	list, err := iplist.ParseEntries(*entries)
	if err != nil {
		return err
	}
	if err := e.svc.IPLists.Create(context.Background(), iplist.List{Name: name, Mode: m, Entries: list}); err != nil {
		return err
	}
	return e.done("IP access list " + name + " created; apply it with: gonix host set <domain> --ip-list " + name)
}

func ipListEntry(add bool) func(*env, []string) error {
	return func(e *env, args []string) error {
		pos, err := parseFlags(flag.NewFlagSet("iplist", flag.ContinueOnError), args)
		if err != nil {
			return err
		}
		if err := exactArgs(pos, 2); err != nil {
			return err
		}
		if add {
			err = e.svc.IPLists.AddEntry(context.Background(), pos[0], pos[1])
		} else {
			err = e.svc.IPLists.RemoveEntry(context.Background(), pos[0], pos[1])
		}
		if err != nil {
			return err
		}
		return e.done("IP access list " + pos[0] + " updated.")
	}
}

func ipListDelete(e *env, args []string) error {
	name, err := nameArg(flag.NewFlagSet("iplist delete", flag.ContinueOnError), args)
	if err != nil {
		return err
	}
	if err := e.confirm("deleting IP access list " + name); err != nil {
		return err
	}
	if err := e.svc.IPLists.Delete(context.Background(), name); err != nil {
		return err
	}
	return e.done("IP access list " + name + " deleted.")
}

// --- Rate limits ---

func rateLimitList(e *env, args []string) error {
	if _, err := parseFlags(flag.NewFlagSet("ratelimit list", flag.ContinueOnError), args); err != nil {
		return err
	}
	limits, err := e.svc.RateLimits.List()
	if err != nil {
		return err
	}
	type limitJSON struct {
		ratelimit.Limit
		Description string   `json:"description"`
		UsedBy      []string `json:"used_by"`
	}
	out := []limitJSON{}
	for _, l := range limits {
		users, _ := e.svc.RateLimits.UsedBy(l.Name)
		out = append(out, limitJSON{l, l.Describe(), users})
	}
	return e.show(out, func(w io.Writer) {
		var rows [][]string
		for _, l := range out {
			rows = append(rows, []string{l.Name, l.Description, strings.Join(l.UsedBy, ", ")})
		}
		table(w, "NAME\tLIMIT\tUSED BY", rows)
	})
}

func rateLimitCreate(e *env, args []string) error {
	fs := flag.NewFlagSet("ratelimit create", flag.ContinueOnError)
	preset := fs.String("preset", "", "general, api or login")
	rate := fs.Int("rate", 10, "requests per --per")
	per := fs.String("per", "s", "s (second) or m (minute)")
	burst := fs.Int("burst", 0, "extra requests tolerated in a spike")
	nodelay := fs.Bool("nodelay", false, "serve burst requests immediately")
	conn := fs.Int("conn", 0, "max concurrent connections per client (0 = unlimited)")
	name, err := nameArg(fs, args)
	if err != nil {
		return err
	}
	l := ratelimit.Limit{Rate: *rate, Per: *per, Burst: *burst, NoDelay: *nodelay, Conn: *conn}
	if *preset != "" {
		found := false
		for _, p := range ratelimit.Presets {
			if p.Name == *preset {
				l, found = p, true
			}
		}
		if !found {
			return usageErr("unknown preset %q", *preset)
		}
	}
	l.Name = name
	if err := e.svc.RateLimits.Create(context.Background(), l); err != nil {
		return err
	}
	return e.done(fmt.Sprintf("Rate limit %s created (%s); apply it with: gonix host set <domain> --rate-limit %s", name, l.Describe(), name))
}

func rateLimitDelete(e *env, args []string) error {
	name, err := nameArg(flag.NewFlagSet("ratelimit delete", flag.ContinueOnError), args)
	if err != nil {
		return err
	}
	if err := e.confirm("deleting rate limit " + name); err != nil {
		return err
	}
	if err := e.svc.RateLimits.Delete(context.Background(), name); err != nil {
		return err
	}
	return e.done("Rate limit " + name + " deleted.")
}

// --- Upstream groups ---

func upstreamList(e *env, args []string) error {
	if _, err := parseFlags(flag.NewFlagSet("upstream list", flag.ContinueOnError), args); err != nil {
		return err
	}
	groups, err := e.svc.Upstreams.List()
	if err != nil {
		return err
	}
	type groupJSON struct {
		Name        string   `json:"name"`
		Method      string   `json:"method"`
		Servers     string   `json:"servers"`
		MaxFails    int      `json:"max_fails"`
		FailTimeout string   `json:"fail_timeout"`
		UsedBy      []string `json:"used_by"`
	}
	out := []groupJSON{}
	for _, g := range groups {
		users, _ := e.svc.Upstreams.UsedBy(g.Name)
		out = append(out, groupJSON{g.Name, g.Method, upstreams.FormatServers(g.Servers), g.MaxFails, g.FailTimeout, users})
	}
	return e.show(out, func(w io.Writer) {
		var rows [][]string
		for _, g := range out {
			rows = append(rows, []string{g.Name, g.Method, g.Servers, strings.Join(g.UsedBy, ", ")})
		}
		table(w, "NAME\tMETHOD\tSERVERS\tUSED BY", rows)
	})
}

func upstreamCreate(e *env, args []string) error {
	fs := flag.NewFlagSet("upstream create", flag.ContinueOnError)
	servers := fs.String("servers", "", "comma separated host:port [weight=N] [backup]")
	method := fs.String("method", upstreams.RoundRobin, "round_robin, least_conn or ip_hash")
	maxFails := fs.Int("max-fails", 3, "failed attempts before a server is skipped")
	failTimeout := fs.String("fail-timeout", "10s", "how long a failed server is skipped")
	name, err := nameArg(fs, args)
	if err != nil {
		return err
	}
	list, err := upstreams.ParseServers(*servers)
	if err != nil {
		return err
	}
	g := upstreams.Group{Name: name, Method: *method, Servers: list, MaxFails: *maxFails, FailTimeout: *failTimeout}
	if err := e.svc.Upstreams.Create(context.Background(), g); err != nil {
		return err
	}
	return e.done("Upstream group " + name + " created; use it with: gonix host add <domain> --group " + name)
}

func upstreamDelete(e *env, args []string) error {
	name, err := nameArg(flag.NewFlagSet("upstream delete", flag.ContinueOnError), args)
	if err != nil {
		return err
	}
	if err := e.confirm("deleting upstream group " + name); err != nil {
		return err
	}
	if err := e.svc.Upstreams.Delete(context.Background(), name); err != nil {
		return err
	}
	return e.done("Upstream group " + name + " deleted.")
}

// --- Stream proxies ---

func streamList(e *env, args []string) error {
	if _, err := parseFlags(flag.NewFlagSet("stream list", flag.ContinueOnError), args); err != nil {
		return err
	}
	proxies, err := e.svc.Streams.List()
	if err != nil {
		return err
	}
	ready, why := e.svc.Streams.Ready()
	type proxyJSON struct {
		Name     string `json:"name"`
		Protocol string `json:"protocol"`
		Port     int    `json:"port"`
		Backend  string `json:"backend"`
		IPList   string `json:"ip_list,omitempty"`
		Enabled  bool   `json:"enabled"`
	}
	out := struct {
		Ready   bool        `json:"ready"`
		Setup   string      `json:"setup,omitempty"`
		Proxies []proxyJSON `json:"proxies"`
	}{Ready: ready, Setup: why, Proxies: []proxyJSON{}}
	for _, p := range proxies {
		out.Proxies = append(out.Proxies, proxyJSON{p.Name, p.Protocol(), p.Port, p.Upstream, p.IPListName, p.Enabled})
	}
	return e.show(out, func(w io.Writer) {
		if !ready {
			fmt.Fprintf(w, "Stream support is not active: %s\n\n", why)
		}
		var rows [][]string
		for _, p := range out.Proxies {
			state := "enabled"
			if !p.Enabled {
				state = "disabled"
			}
			rows = append(rows, []string{p.Name, state, fmt.Sprintf("%s/%d", p.Protocol, p.Port), p.Backend})
		}
		table(w, "NAME\tSTATE\tLISTEN\tBACKEND", rows)
	})
}

func streamSetup(e *env, args []string) error {
	if _, err := parseFlags(flag.NewFlagSet("stream setup", flag.ContinueOnError), args); err != nil {
		return err
	}
	if ready, _ := e.svc.Streams.Ready(); ready {
		return e.done("Stream support is already active.")
	}
	if err := e.svc.Streams.SetUp(context.Background()); err != nil {
		return err
	}
	return e.done("Stream support is set up.")
}

func streamCreate(e *env, args []string) error {
	fs := flag.NewFlagSet("stream create", flag.ContinueOnError)
	port := fs.Int("port", 0, "port Nginx listens on")
	backend := fs.String("backend", "", "host:port to forward to")
	udp := fs.Bool("udp", false, "UDP instead of TCP")
	ipList := fs.String("ip-list", "", "IP access list")
	connect := fs.String("connect-timeout", "10s", "connect timeout")
	idle := fs.String("timeout", "10m", "idle timeout")
	name, err := nameArg(fs, args)
	if err != nil {
		return err
	}
	if *ipList != "" && !e.svc.IPLists.Store.Exists(*ipList) {
		return fmt.Errorf("IP access list %q does not exist", *ipList)
	}
	p := streams.Proxy{Name: name, Port: *port, UDP: *udp, Upstream: *backend, ConnectTimeout: *connect, Timeout: *idle, IPListName: *ipList, Enabled: true}
	if err := e.svc.Streams.Save(context.Background(), p, true); err != nil {
		return err
	}
	msg := fmt.Sprintf("Stream proxy %s created: %s port %d -> %s.", name, p.Protocol(), p.Port, p.Upstream)
	if ready, why := e.svc.Streams.Ready(); !ready {
		msg += " It takes effect once stream support is set up: " + why
	}
	return e.done(msg)
}

func streamToggle(enable bool) func(*env, []string) error {
	return func(e *env, args []string) error {
		name, err := nameArg(flag.NewFlagSet("stream", flag.ContinueOnError), args)
		if err != nil {
			return err
		}
		if err := e.svc.Streams.SetEnabled(context.Background(), name, enable); err != nil {
			return err
		}
		return e.done("Stream proxy " + name + " updated.")
	}
}

func streamDelete(e *env, args []string) error {
	name, err := nameArg(flag.NewFlagSet("stream delete", flag.ContinueOnError), args)
	if err != nil {
		return err
	}
	if err := e.confirm("deleting stream proxy " + name); err != nil {
		return err
	}
	if err := e.svc.Streams.Delete(context.Background(), name); err != nil {
		return err
	}
	return e.done("Stream proxy " + name + " deleted.")
}

// --- Country lists ---

func countryList(e *env, args []string) error {
	if _, err := parseFlags(flag.NewFlagSet("country list", flag.ContinueOnError), args); err != nil {
		return err
	}
	ready, db := e.svc.GeoBlock.Status()
	lists, err := e.svc.GeoBlock.List()
	if err != nil {
		return err
	}
	type listJSON struct {
		Name      string   `json:"name"`
		Mode      string   `json:"mode"`
		Countries []string `json:"countries"`
		UsedBy    []string `json:"used_by"`
	}
	out := struct {
		Ready    bool       `json:"geoip_ready"`
		Database string     `json:"database,omitempty"`
		Lists    []listJSON `json:"lists"`
	}{ready, db, []listJSON{}}
	for _, l := range lists {
		users, _ := e.svc.GeoBlock.UsedBy(l.Name)
		out.Lists = append(out.Lists, listJSON{l.Name, string(l.Mode), l.Countries, users})
	}
	return e.show(out, func(w io.Writer) {
		if !ready {
			fmt.Fprintln(w, "GeoIP is not set up (gonix country setup).")
		}
		var rows [][]string
		for _, l := range out.Lists {
			rows = append(rows, []string{l.Name, l.Mode, strings.Join(l.Countries, " "), strings.Join(l.UsedBy, ", ")})
		}
		table(w, "NAME\tMODE\tCOUNTRIES\tUSED BY", rows)
	})
}

func countrySetup(e *env, args []string) error {
	if _, err := parseFlags(flag.NewFlagSet("country setup", flag.ContinueOnError), args); err != nil {
		return err
	}
	if err := e.svc.GeoBlock.SetUp(context.Background()); err != nil {
		return err
	}
	return e.done("GeoIP lookups are set up.")
}

func countryCreate(e *env, args []string) error {
	fs := flag.NewFlagSet("country create", flag.ContinueOnError)
	mode := fs.String("mode", "", "allow or block")
	countries := fs.String("countries", "", "two-letter country codes")
	name, err := nameArg(fs, args)
	if err != nil {
		return err
	}
	l := geoblock.List{Name: name}
	switch *mode {
	case "allow", "allowlist":
		l.Mode = geoblock.Allowlist
	case "block", "blocklist":
		l.Mode = geoblock.Blocklist
	default:
		return usageErr("--mode must be allow or block")
	}
	if l.Countries, err = geoblock.ParseCountries(*countries); err != nil {
		return err
	}
	if err := e.svc.GeoBlock.Save(context.Background(), l, true); err != nil {
		return err
	}
	return e.done("Country list " + name + " created; apply it with: gonix host set <domain> --country-list " + name)
}

func countryDelete(e *env, args []string) error {
	name, err := nameArg(flag.NewFlagSet("country delete", flag.ContinueOnError), args)
	if err != nil {
		return err
	}
	if err := e.confirm("deleting country list " + name); err != nil {
		return err
	}
	if err := e.svc.GeoBlock.Delete(context.Background(), name); err != nil {
		return err
	}
	return e.done("Country list " + name + " deleted.")
}

// --- Presets, real IP, read-only lists ---

func presetList(e *env, args []string) error {
	if _, err := parseFlags(flag.NewFlagSet("preset list", flag.ContinueOnError), args); err != nil {
		return err
	}
	type presetJSON struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		SSLOnly     bool   `json:"ssl_only,omitempty"`
		ProxyOnly   bool   `json:"proxy_only,omitempty"`
	}
	out := []presetJSON{}
	for _, p := range presets.All {
		out = append(out, presetJSON{p.Name, p.Description, p.SSLOnly, p.ProxyOnly})
	}
	return e.show(out, func(w io.Writer) {
		var rows [][]string
		for _, p := range out {
			rows = append(rows, []string{p.Name, p.Description})
		}
		table(w, "NAME\tDESCRIPTION", rows)
	})
}

func realIPShow(e *env, args []string) error {
	if _, err := parseFlags(flag.NewFlagSet("realip show", flag.ContinueOnError), args); err != nil {
		return err
	}
	s, enabled, err := e.svc.RealIP.Get()
	if err != nil {
		return err
	}
	out := map[string]any{"enabled": enabled}
	if enabled {
		out["cloudflare"], out["header"], out["trusted_proxies"] = s.Cloudflare, s.Header, s.Proxies
	}
	return e.show(out, func(w io.Writer) {
		if !enabled {
			fmt.Fprintln(w, "Real client IP handling is disabled.")
			return
		}
		fmt.Fprintf(w, "Enabled: header %s from %d trusted ranges (cloudflare: %v)\n", s.Header, len(s.Proxies), s.Cloudflare)
	})
}

func realIPCloudflare(e *env, args []string) error {
	fs := flag.NewFlagSet("realip cloudflare", flag.ContinueOnError)
	refresh := fs.Bool("refresh", false, "download Cloudflare's current ranges")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	ranges := realip.CloudflareRanges
	if *refresh {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		var err error
		if ranges, err = realip.FetchCloudflareRanges(ctx, &http.Client{Timeout: 15 * time.Second}); err != nil {
			return err
		}
	}
	if err := e.svc.RealIP.Save(context.Background(), realip.Settings{Cloudflare: true, Header: "CF-Connecting-IP", Proxies: ranges}); err != nil {
		return err
	}
	return e.done(fmt.Sprintf("Client addresses are taken from CF-Connecting-IP for %d Cloudflare ranges.", len(ranges)))
}

func realIPCustom(e *env, args []string) error {
	fs := flag.NewFlagSet("realip custom", flag.ContinueOnError)
	proxies := fs.String("proxies", "", "trusted proxy addresses or ranges")
	header := fs.String("header", "X-Forwarded-For", "X-Forwarded-For or X-Real-IP")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	list, err := iplist.ParseEntries(*proxies)
	if err != nil {
		return err
	}
	if err := e.svc.RealIP.Save(context.Background(), realip.Settings{Header: *header, Proxies: list}); err != nil {
		return err
	}
	return e.done("Client addresses are taken from " + *header + " for requests from the trusted proxies.")
}

func realIPDisable(e *env, args []string) error {
	if _, err := parseFlags(flag.NewFlagSet("realip disable", flag.ContinueOnError), args); err != nil {
		return err
	}
	if err := e.svc.RealIP.Disable(context.Background()); err != nil {
		return err
	}
	return e.done("Real client IP handling disabled.")
}

func accessListList(e *env, args []string) error {
	if _, err := parseFlags(flag.NewFlagSet("accesslist list", flag.ContinueOnError), args); err != nil {
		return err
	}
	lists, err := e.svc.AccessLists.List()
	if err != nil {
		return err
	}
	type listJSON struct {
		Name  string   `json:"name"`
		Users []string `json:"users"`
	}
	out := []listJSON{}
	for _, l := range lists {
		out = append(out, listJSON{l.Name, l.Users})
	}
	return e.show(out, func(w io.Writer) {
		var rows [][]string
		for _, l := range out {
			rows = append(rows, []string{l.Name, strings.Join(l.Users, " ")})
		}
		table(w, "NAME\tUSERS", rows)
	})
}

func errorPagesList(e *env, args []string) error {
	if _, err := parseFlags(flag.NewFlagSet("errorpages list", flag.ContinueOnError), args); err != nil {
		return err
	}
	snippets, err := e.svc.ErrorPages.Store.List()
	if err != nil {
		return err
	}
	type snJSON struct {
		Name      string `json:"name"`
		Directory string `json:"directory"`
		Codes     []int  `json:"codes"`
		Template  string `json:"template,omitempty"`
	}
	out := []snJSON{}
	for _, s := range snippets {
		out = append(out, snJSON{s.Name, s.Directory, s.Codes, s.Template})
	}
	return e.show(out, func(w io.Writer) {
		var rows [][]string
		for _, s := range out {
			rows = append(rows, []string{s.Name, s.Directory, fmt.Sprint(s.Codes)})
		}
		table(w, "NAME\tDIRECTORY\tCODES", rows)
	})
}

func certList(e *env, args []string) error {
	if _, err := parseFlags(flag.NewFlagSet("cert list", flag.ContinueOnError), args); err != nil {
		return err
	}
	certs, err := certificates.Scan(e.svc.Config.Certificates.Directory)
	if err != nil {
		return err
	}
	type certJSON struct {
		Name      string    `json:"name"`
		Status    string    `json:"status"`
		NotAfter  time.Time `json:"not_after"`
		Remaining string    `json:"remaining"`
		Names     []string  `json:"names"`
		Issuer    string    `json:"issuer"`
		KeyType   string    `json:"key_type"`
		KeyOK     bool      `json:"key_matches"`
		Error     string    `json:"error,omitempty"`
	}
	out := []certJSON{}
	for _, c := range certs {
		out = append(out, certJSON{c.Domain, string(c.Status), c.NotAfter, c.RemainingHuman(), c.SANs, c.Issuer, string(c.KeyType), c.KeyMatches, c.ParseError})
	}
	return e.show(out, func(w io.Writer) {
		var rows [][]string
		for _, c := range out {
			rows = append(rows, []string{c.Name, c.Status, c.NotAfter.Format("2006-01-02"), strings.Join(c.Names, " ")})
		}
		table(w, "NAME\tSTATUS\tEXPIRES\tNAMES", rows)
	})
}
