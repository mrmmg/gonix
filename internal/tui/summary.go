package tui

import (
	"fmt"
	"strings"

	"github.com/mrmmg/gonix/internal/nginx"
)

// The summaries below describe a change in plain words, one line each, for
// the review step that comes before anything is applied. They never show
// Nginx configuration: they say what will be different for the people
// visiting the site.

func yes(v map[string]string, key string) bool { return v[key] == "yes" }

// addHostSummary describes what the Add New Host answers will create.
func addHostSummary(certs certChoices) func(map[string]string) []string {
	return func(v map[string]string) []string {
		name := v["domain"]
		line := "Create the host " + name
		if names := strings.Fields(v["aliases"]); len(names) > 0 {
			line += ", which also answers to " + strings.Join(names, ", ")
		}
		out := []string{line}
		switch v["mode"] {
		case modeStatic:
			out = append(out, "Visitors get the files in "+v["root"])
		case modeRedirect:
			out = append(out, redirectSummary(v)...)
		default:
			target := fmt.Sprintf("the app at %s:%s", v["upstream_host"], v["upstream_port"])
			if g := upstreamGroupChoice(v); g != "" {
				target = "the upstream group " + g + " (load balanced)"
			}
			out = append(out, "Visitors are passed on to "+target+" ("+v["upstream_scheme"]+")")
			if yes(v, "websocket") {
				out = append(out, "WebSocket connections are allowed")
			}
			if yes(v, "forward_headers") {
				out = append(out, "The app learns the visitor's real address and the original host name")
			}
		}
		out = append(out, sslSummary(v, "ssl_enable", "ssl_cert", "ssl_force", "ssl_http2", "ssl_http3", certs, v["mode"] == modeRedirect)...)
		if sn := v["error_pages"]; sn != "" && sn != "(none)" {
			out = append(out, "Error pages come from the snippet "+sn)
		}
		out = append(out, "The host is switched on right away")
		return out
	}
}

// sslSummary describes an SSL choice; the keys differ between the Add New
// Host wizard and the SSL Configuration one.
func sslSummary(v map[string]string, enable, cert, force, http2, http3 string, certs certChoices, redirectHost bool) []string {
	if !yes(v, enable) {
		return []string{"Plain HTTP only: no HTTPS"}
	}
	out := []string{"Served over HTTPS"}
	if ch, ok := certs.By[v[cert]]; ok {
		out[0] += " with " + ch.Describe()
		if ch.Cert != nil && ch.Cert.NeedsAttention() {
			out = append(out, "Warning: that certificate has "+ch.Cert.Short()+"; renew it from the Certificates menu")
		}
	} else {
		out[0] += " with a certificate GoNix will look for in its own directory (none exists yet)"
	}
	if yes(v, http2) {
		out = append(out, "HTTP/2 is on")
	}
	if yes(v, http3) {
		out = append(out, "HTTP/3 is on (UDP port 443 must be open in the firewall)")
	}
	if yes(v, force) && !redirectHost {
		out = append(out, "Visitors using plain HTTP are sent to HTTPS")
	}
	return out
}

func redirectSummary(v map[string]string) []string {
	r := redirectFromValues(v)
	kind := "temporarily"
	if r.Code == 301 || r.Code == 308 {
		kind = "permanently"
	}
	line := fmt.Sprintf("Visitors are sent %s to %s", kind, r.Target)
	if r.PreservePath {
		line += ", keeping the page and query they asked for"
	} else {
		line += ", always landing on its front page"
	}
	return []string{line}
}

func sslConfigSummary(h nginx.ListedHost, certs certChoices) func(map[string]string) []string {
	return func(v map[string]string) []string {
		out := sslSummary(v, "enable", "cert", "force", "http2", "http3", certs, h.Mode == nginx.ModeRedirect)
		if !yes(v, "enable") && h.SSL.Enabled {
			out = append(out, "Existing HTTPS settings are removed; visitors will only reach the site over plain HTTP")
		}
		return out
	}
}

func aliasesSummary(h nginx.ListedHost) func(map[string]string) []string {
	return func(v map[string]string) []string {
		names := strings.Fields(v["aliases"])
		if len(names) == 0 {
			return []string{h.ServerName + " answers only to its own name"}
		}
		return []string{h.ServerName + " also answers to " + strings.Join(names, ", ")}
	}
}

func redirectConfigSummary(h nginx.ListedHost) func(map[string]string) []string {
	return func(v map[string]string) []string {
		return append([]string{"Change where " + h.ServerName + " sends its visitors"}, redirectSummary(v)...)
	}
}
