package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// The MCP server exposes GoNix to AI agents as typed tools over stdio
// (JSON-RPC 2.0, one message per line). Every tool is translated into a
// CLI command run with --json, so agents get exactly the CLI's behavior:
// the same validation, nginx -t, rollback, audit log and history.

func init() {
	register(command{path: "mcp", summary: "Run an MCP server on stdio for AI agents (tools map to these commands)", run: mcpCmd})
}

// mcpRunner runs a CLI command line and returns its stdout and exit code.
type mcpRunner func(args []string) (string, int)

func mcpCmd(e *env, args []string) error {
	if err := noFlags("mcp", args); err != nil {
		return err
	}
	if e.Config != "" {
		mcpGlobal = append(mcpGlobal, "--config", e.Config)
	}
	if e.Instance != "" {
		mcpGlobal = append(mcpGlobal, "--instance", e.Instance)
	}
	if os.Getenv("GONIX_ACTOR") == "" {
		os.Setenv("GONIX_ACTOR", "mcp")
	}
	run := func(a []string) (string, int) {
		var out, errOut bytes.Buffer
		code := Run(a, &out, &errOut, mcpBuild, mcpConfig)
		if out.Len() == 0 {
			return errOut.String(), code
		}
		return out.String(), code
	}
	return serveMCP(os.Stdin, e.out, run)
}

// mcpBuild and mcpConfig are what the outer Run was started with, and
// mcpGlobal its --config/--instance flags, passed on to every tool call.
var (
	mcpBuild  Builder
	mcpConfig string
	mcpGlobal []string
)

// Version is reported to MCP clients; main sets it.
var Version = "dev"

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

func serveMCP(in io.Reader, out io.Writer, run mcpRunner) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	enc := json.NewEncoder(out)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			enc.Encode(rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{-32700, "parse error"}})
			continue
		}
		if len(req.ID) == 0 {
			continue // a notification, e.g. notifications/initialized
		}
		resp := rpcResponse{JSONRPC: "2.0", ID: req.ID}
		result, err := handleMCP(req, run)
		if err != nil {
			resp.Error = err
		} else {
			resp.Result = result
		}
		if e := enc.Encode(resp); e != nil {
			return e
		}
	}
	return sc.Err()
}

func handleMCP(req rpcRequest, run mcpRunner) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		json.Unmarshal(req.Params, &p)
		version := p.ProtocolVersion
		if version == "" {
			version = "2025-06-18"
		}
		return map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "gonix", "version": Version},
			"instructions": "GoNix manages Nginx safely: every change is validated with nginx -t, rolled back on failure, audited and versioned. " +
				"Inspect with list_hosts/show_host first, preview every change with dry_run=true and show the diff to the user, and apply only after they agree.",
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		var list []map[string]any
		for _, t := range mcpTools {
			list = append(list, map[string]any{"name": t.name, "description": t.description, "inputSchema": t.schema()})
		}
		return map[string]any{"tools": list}, nil
	case "tools/call":
		var p struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, &rpcError{-32602, "invalid params"}
		}
		for _, t := range mcpTools {
			if t.name != p.Name {
				continue
			}
			args, err := t.args(p.Arguments)
			if err != nil {
				return toolResult(err.Error(), true), nil
			}
			out, code := run(append(args, "--json"))
			return toolResult(out, code != 0), nil
		}
		return nil, &rpcError{-32602, "unknown tool " + p.Name}
	}
	return nil, &rpcError{-32601, "method not found: " + req.Method}
}

func toolResult(text string, isError bool) map[string]any {
	return map[string]any{"content": []map[string]any{{"type": "text", "text": text}}, "isError": isError}
}

// mcpParam is one tool argument and the CLI flag it becomes.
type mcpParam struct {
	name        string
	kind        string // string, boolean, integer, array
	description string
	required    bool
	flag        string // "" for a positional argument
	enum        []string
}

type mcpTool struct {
	name        string
	description string
	command     string // CLI command path
	params      []mcpParam
	extra       []string // fixed arguments, e.g. --raw
	write       bool     // gets a dry_run parameter
	confirm     bool     // destructive: needs confirm=true (adds --yes)
}

func (t mcpTool) schema() map[string]any {
	props := map[string]any{}
	required := []string{}
	all := t.params
	if t.write {
		all = append(all, mcpParam{name: "dry_run", kind: "boolean", description: "Only show the configuration diff, change nothing"})
	}
	if t.confirm {
		all = append(all, mcpParam{name: "confirm", kind: "boolean", description: "Must be true: the user explicitly approved this destructive action", required: true})
	}
	for _, p := range all {
		prop := map[string]any{"type": p.kind, "description": p.description}
		if p.kind == "array" {
			prop["items"] = map[string]any{"type": "string"}
		}
		if p.enum != nil {
			prop["enum"] = p.enum
		}
		props[p.name] = prop
		if p.required {
			required = append(required, p.name)
		}
	}
	return map[string]any{"type": "object", "properties": props, "required": required}
}

// args converts tool arguments into a CLI command line. Values may never
// start with "-", so arguments cannot smuggle in flags such as --yes;
// only the generic "gonix" tool passes flags, and it is documented as such.
func (t mcpTool) args(in map[string]any) ([]string, error) {
	out := append(strings.Fields(t.command), t.extra...)
	var flags []string
	for _, p := range t.params {
		v, ok := in[p.name]
		if !ok || v == nil {
			if p.required {
				return nil, fmt.Errorf("missing required argument %q", p.name)
			}
			continue
		}
		var values []string
		switch x := v.(type) {
		case string:
			values = []string{x}
		case bool:
			if !x {
				continue
			}
			if p.flag == "" {
				return nil, fmt.Errorf("%s must not be a boolean", p.name)
			}
			flags = append(flags, "--"+p.flag)
			continue
		case float64:
			values = []string{strconv.Itoa(int(x))}
		case []any:
			for _, item := range x {
				s, ok := item.(string)
				if !ok {
					return nil, fmt.Errorf("%s must be a list of strings", p.name)
				}
				values = append(values, s)
			}
		default:
			return nil, fmt.Errorf("unsupported value for %s", p.name)
		}
		for _, val := range values {
			if t.name != "gonix" && strings.HasPrefix(val, "-") {
				return nil, fmt.Errorf("%s must not start with '-'", p.name)
			}
			if p.flag == "" {
				out = append(out, val)
			} else {
				flags = append(flags, "--"+p.flag, val)
			}
		}
	}
	if t.write {
		if b, _ := in["dry_run"].(bool); b {
			flags = append(flags, "--dry-run")
		}
	}
	if t.confirm {
		if b, _ := in["confirm"].(bool); !b {
			return nil, fmt.Errorf("this action is destructive: ask the user, then call again with confirm=true (or dry_run=true to preview)")
		}
		flags = append(flags, "--yes")
	}
	return append(append(out, flags...), mcpGlobal...), nil
}

var hostParams = []mcpParam{
	{name: "upstream", kind: "string", description: "Proxy / to this URL, e.g. http://127.0.0.1:3000", flag: "upstream"},
	{name: "group", kind: "string", description: "Proxy / to this upstream group", flag: "group"},
	{name: "ssl_certificate", kind: "string", description: "Certificate name from list_certificates, or none", flag: "ssl"},
	{name: "presets", kind: "string", description: "Comma-separated presets (security-headers, hsts, gzip, static-cache, proxy-cache, block-sensitive-files, block-bad-bots) or none", flag: "presets"},
	{name: "ip_list", kind: "string", description: "IP access list name, or none", flag: "ip-list"},
	{name: "rate_limit", kind: "string", description: "Rate limit name, or none", flag: "rate-limit"},
	{name: "country_list", kind: "string", description: "Country list name, or none", flag: "country-list"},
	{name: "access_list", kind: "string", description: "Basic Auth access list name, or none", flag: "access-list"},
	{name: "aliases", kind: "array", description: "Additional domain names", flag: "alias"},
}

var mcpTools = []mcpTool{
	{name: "list_hosts", description: "List all Nginx hosts with their mode, state and target.", command: "host list"},
	{name: "show_host", description: "Show every setting of one host.", command: "host show",
		params: []mcpParam{{name: "domain", kind: "string", description: "Domain or alias", required: true}}},
	{name: "show_host_config", description: "Show the raw Nginx configuration file of a host.", command: "host show", extra: []string{"--raw"},
		params: []mcpParam{{name: "domain", kind: "string", description: "Domain", required: true}}},
	{name: "add_host", description: "Create and enable a host: a reverse proxy (upstream or group), static site (root) or redirect.",
		command: "host add", write: true, params: append([]mcpParam{
			{name: "domain", kind: "string", description: "Primary domain, e.g. app.example.com", required: true},
			{name: "root", kind: "string", description: "Serve static files from this directory", flag: "root"},
			{name: "redirect", kind: "string", description: "Redirect every request to this URL or domain", flag: "redirect"},
			{name: "redirect_code", kind: "integer", description: "301, 302, 307 or 308", flag: "code"},
			{name: "websocket", kind: "boolean", description: "Enable WebSocket support", flag: "websocket"},
		}, hostParams...)},
	{name: "update_host", description: "Change settings of an existing host; only the given arguments change.",
		command: "host set", write: true, params: append([]mcpParam{
			{name: "domain", kind: "string", description: "Domain or alias", required: true},
			{name: "websocket", kind: "string", description: "on or off", flag: "websocket", enum: []string{"on", "off"}},
			{name: "maintenance", kind: "string", description: "on or off", flag: "maintenance", enum: []string{"on", "off"}},
			{name: "bypass", kind: "string", description: "With maintenance on: space-separated addresses that still see the site", flag: "bypass"},
		}, hostParams...)},
	{name: "set_host_enabled", description: "Enable or disable a host (its file is kept).", command: "host", write: true,
		params: []mcpParam{{name: "action", kind: "string", description: "enable or disable", required: true, enum: []string{"enable", "disable"}},
			{name: "domain", kind: "string", description: "Domain", required: true}}},
	{name: "delete_host", description: "Delete a host (a backup is kept).", command: "host delete", write: true, confirm: true,
		params: []mcpParam{{name: "domain", kind: "string", description: "Domain", required: true}}},
	{name: "add_path", description: "Add a reverse-proxy location to a host.", command: "host path add", write: true,
		params: []mcpParam{{name: "domain", kind: "string", description: "Domain", required: true},
			{name: "path", kind: "string", description: "Location path, e.g. /api/", required: true},
			{name: "upstream", kind: "string", description: "Proxy to this URL", flag: "upstream"},
			{name: "group", kind: "string", description: "Proxy to this upstream group", flag: "group"},
			{name: "ip_list", kind: "string", description: "IP access list for this path", flag: "ip-list"},
			{name: "rate_limit", kind: "string", description: "Rate limit for this path", flag: "rate-limit"}}},
	{name: "list_ip_lists", description: "List IP access lists and where they are used.", command: "iplist list"},
	{name: "create_ip_list", description: "Create an IP allowlist or blocklist.", command: "iplist create", write: true,
		params: []mcpParam{{name: "name", kind: "string", description: "List name", required: true},
			{name: "mode", kind: "string", description: "allow or block", required: true, flag: "mode", enum: []string{"allow", "block"}},
			{name: "entries", kind: "string", description: "Space-separated IP addresses or CIDR ranges", required: true, flag: "entries"}}},
	{name: "add_to_ip_list", description: "Add an IP address or range to a list (e.g. to block a client).", command: "iplist add", write: true,
		params: []mcpParam{{name: "name", kind: "string", description: "List name", required: true}, {name: "entry", kind: "string", description: "IP or CIDR", required: true}}},
	{name: "list_rate_limits", description: "List rate limits.", command: "ratelimit list"},
	{name: "list_upstream_groups", description: "List upstream (load balancing) groups.", command: "upstream list"},
	{name: "adopt_host", description: "Let GoNix manage a hand-written host file. Only a marker comment is added; Nginx keeps serving the same configuration, certificates (e.g. certbot's) stay in place, and later edits change only the directives involved.",
		command: "adopt", write: true, params: []mcpParam{{name: "domain", kind: "string", description: "Domain or file name", required: true}}},
	{name: "list_certificates", description: "List TLS certificates with expiry and names.", command: "cert list"},
	{name: "check_config", description: "Find problems nginx -t does not: duplicate names, missing files, expiring certificates, dead upstreams.", command: "check"},
	{name: "host_logs", description: "Recent requests of a host, a traffic summary, or its error log.", command: "logs",
		params: []mcpParam{{name: "domain", kind: "string", description: "Domain", required: true},
			{name: "filter", kind: "string", description: "e.g. status:5xx ip:1.2.3.4 path:/api", flag: "filter"},
			{name: "summary", kind: "boolean", description: "Summarize instead of listing", flag: "summary"},
			{name: "error_log", kind: "boolean", description: "Show the error log", flag: "error"},
			{name: "lines", kind: "integer", description: "How many lines", flag: "lines"}}},
	{name: "nginx_status", description: "Whether Nginx is running, its version and HTTP/3 support.", command: "nginx status"},
	{name: "nginx_test", description: "Run nginx -t.", command: "nginx test"},
	{name: "audit_log", description: "Latest audited changes.", command: "audit",
		params: []mcpParam{{name: "lines", kind: "integer", description: "How many entries", flag: "lines"}}},
	{name: "history", description: "Configuration history commits (every change is one).", command: "history list",
		params: []mcpParam{{name: "lines", kind: "integer", description: "How many commits", flag: "lines"}}},
	{name: "gonix", description: "Run any other gonix command (see `gonix help`), e.g. [\"stream\",\"list\"]. Output is JSON. Destructive commands still need --yes, which must only be added after the user explicitly approved.",
		command: "", params: []mcpParam{{name: "args", kind: "array", description: "Command words and flags", required: true}}},
}
