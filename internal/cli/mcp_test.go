package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

type mcpSession struct {
	t   *testing.T
	h   *harness
	in  []string
	ids int
}

func (s *mcpSession) call(method string, params any) int {
	s.ids++
	p, _ := json.Marshal(params)
	s.in = append(s.in, `{"jsonrpc":"2.0","id":`+itoa(s.ids)+`,"method":"`+method+`","params":`+string(p)+`}`)
	return s.ids
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func (s *mcpSession) run() map[int]map[string]any {
	var out bytes.Buffer
	runner := func(args []string) (string, int) {
		stdout, stderr, code := s.h.run(args...)
		if stdout == "" {
			return stderr, code
		}
		return stdout, code
	}
	if err := serveMCP(strings.NewReader(strings.Join(s.in, "\n")+"\n"), &out, runner); err != nil {
		s.t.Fatal(err)
	}
	responses := map[int]map[string]any{}
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var r map[string]any
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			s.t.Fatalf("bad response %q: %v", line, err)
		}
		id, _ := r["id"].(float64)
		responses[int(id)] = r
	}
	return responses
}

func toolText(t *testing.T, r map[string]any) (string, bool) {
	t.Helper()
	res, ok := r["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", r)
	}
	content := res["content"].([]any)[0].(map[string]any)
	return content["text"].(string), res["isError"].(bool)
}

func TestMCPSession(t *testing.T) {
	h := newHarness(t)
	s := &mcpSession{t: t, h: h}
	initID := s.call("initialize", map[string]any{"protocolVersion": "2025-06-18"})
	s.in = append(s.in, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	listID := s.call("tools/list", map[string]any{})
	dryID := s.call("tools/call", map[string]any{"name": "add_host", "arguments": map[string]any{
		"domain": "app.example.com", "upstream": "http://127.0.0.1:3000", "websocket": true, "aliases": []string{"www.app.example.com"}, "dry_run": true}})
	addID := s.call("tools/call", map[string]any{"name": "add_host", "arguments": map[string]any{
		"domain": "app.example.com", "upstream": "http://127.0.0.1:3000", "websocket": true, "aliases": []string{"www.app.example.com"}}})
	showID := s.call("tools/call", map[string]any{"name": "show_host", "arguments": map[string]any{"domain": "www.app.example.com"}})
	rawID := s.call("tools/call", map[string]any{"name": "show_host_config", "arguments": map[string]any{"domain": "app.example.com"}})
	noConfirmID := s.call("tools/call", map[string]any{"name": "delete_host", "arguments": map[string]any{"domain": "app.example.com"}})
	injectID := s.call("tools/call", map[string]any{"name": "show_host", "arguments": map[string]any{"domain": "--yes"}})
	genericID := s.call("tools/call", map[string]any{"name": "gonix", "arguments": map[string]any{"args": []string{"preset", "list"}}})
	unknownID := s.call("tools/call", map[string]any{"name": "nope", "arguments": map[string]any{}})
	badMethodID := s.call("resources/list", map[string]any{})
	deleteID := s.call("tools/call", map[string]any{"name": "delete_host", "arguments": map[string]any{"domain": "app.example.com", "confirm": true}})
	r := s.run()

	if len(r) != s.ids {
		t.Errorf("got %d responses for %d requests (notifications must not be answered)", len(r), s.ids)
	}
	info := r[initID]["result"].(map[string]any)
	if info["protocolVersion"] != "2025-06-18" || info["serverInfo"].(map[string]any)["name"] != "gonix" {
		t.Errorf("initialize = %v", info)
	}
	tools := r[listID]["result"].(map[string]any)["tools"].([]any)
	if len(tools) != len(mcpTools) {
		t.Errorf("tools/list returned %d tools", len(tools))
	}

	if text, isErr := toolText(t, r[dryID]); isErr || !strings.Contains(text, `"dry_run": true`) || !strings.Contains(text, "server_name app.example.com www.app.example.com;") {
		t.Errorf("dry run = %s", text)
	}
	if text, isErr := toolText(t, r[addID]); isErr {
		t.Errorf("add_host failed: %s", text)
	}
	if text, _ := toolText(t, r[showID]); !strings.Contains(text, `"websocket": true`) {
		t.Errorf("show_host = %s", text)
	}
	if text, _ := toolText(t, r[rawID]); !strings.Contains(text, "proxy_pass http://127.0.0.1:3000;") {
		t.Errorf("show_host_config = %s", text)
	}
	if text, isErr := toolText(t, r[noConfirmID]); !isErr || !strings.Contains(text, "confirm=true") {
		t.Errorf("delete without confirm = %s", text)
	}
	if text, isErr := toolText(t, r[injectID]); !isErr || !strings.Contains(text, "must not start with '-'") {
		t.Errorf("flag injection = %s", text)
	}
	if text, isErr := toolText(t, r[genericID]); isErr || !strings.Contains(text, "security-headers") {
		t.Errorf("generic tool = %s", text)
	}
	if r[unknownID]["error"] == nil || r[badMethodID]["error"] == nil {
		t.Errorf("unknown tool/method must be JSON-RPC errors: %v / %v", r[unknownID], r[badMethodID])
	}
	if text, isErr := toolText(t, r[deleteID]); isErr {
		t.Errorf("delete with confirm failed: %s", text)
	}
	if text, _, _ := h.run("host", "list", "--json"); strings.Contains(text, "app.example.com") {
		t.Error("host still exists after delete_host")
	}
}

// Every tool must translate into a real command, and write tools into
// commands that accept --dry-run.
func TestMCPToolsMapToCommands(t *testing.T) {
	for _, tool := range mcpTools {
		if tool.name == "gonix" {
			continue
		}
		words := strings.Fields(tool.command)
		if tool.name == "set_host_enabled" {
			words = append(words, "enable")
		}
		cmd, _, ok := find(words)
		if !ok {
			t.Errorf("tool %s maps to unknown command %q", tool.name, tool.command)
			continue
		}
		if tool.write != cmd.write {
			t.Errorf("tool %s: write=%v but command %q write=%v", tool.name, tool.write, cmd.path, cmd.write)
		}
		if tool.confirm && !strings.Contains(cmd.args, "--yes") {
			t.Errorf("tool %s needs confirmation but %q does not take --yes", tool.name, cmd.path)
		}
	}
}
