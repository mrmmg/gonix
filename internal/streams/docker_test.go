package streams

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mrmmg/gonix/internal/iplist"
)

// TestStreamConfigInDocker validates the wrapper and proxies with `nginx -t`
// in the official nginx image, which has the stream module built in (most
// distribution packages ship it separately). Opt in with
// GONIX_DOCKER_TEST=1; it needs Docker and the nginx:latest image.
func TestStreamConfigInDocker(t *testing.T) {
	if os.Getenv("GONIX_DOCKER_TEST") != "1" {
		t.Skip("set GONIX_DOCKER_TEST=1 to run Docker-based checks")
	}
	root := t.TempDir()
	streamsDir := filepath.Join(root, "gonix-streams")
	snippets := filepath.Join(root, "snippets")
	for _, d := range []string{streamsDir, snippets, filepath.Join(root, "modules-enabled")} {
		os.MkdirAll(d, 0o755)
	}
	ipl, err := iplist.Render(iplist.List{Name: "office", Mode: iplist.Allowlist, Entries: []string{"10.0.0.0/8"}})
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(snippets, "gonix-ip-office.conf"), []byte(ipl), 0o644)
	for _, p := range []Proxy{
		{Name: "pg", Port: 15432, Upstream: "127.0.0.1:5432", ConnectTimeout: "5s", Timeout: "10m", IPListName: "office"},
		{Name: "dns", Port: 15353, UDP: true, Upstream: "127.0.0.1:53"},
	} {
		out, err := Render(p, "/t/snippets")
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(streamsDir, p.Name+".conf"), []byte(out), 0o644)
	}
	os.WriteFile(filepath.Join(root, "modules-enabled", WrapperFileName), []byte(RenderWrapper("/t/gonix-streams")), 0o644)
	os.WriteFile(filepath.Join(root, "nginx.conf"), []byte("include /t/modules-enabled/*.conf;\nevents {}\nhttp {}\n"), 0o644)

	out, err := exec.Command("docker", "run", "--rm", "-v", root+":/t:ro", "nginx:latest", "nginx", "-t", "-c", "/t/nginx.conf").CombinedOutput()
	if err != nil {
		t.Fatalf("nginx -t in Docker failed: %v\n%s", err, out)
	}
}
