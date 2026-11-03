package nginx

import (
	"context"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// BuildInfo describes the installed Nginx binary, as reported by `nginx -V`.
// The zero value means "unknown" and makes the renderer fall back to syntax
// every supported Nginx version accepts.
type BuildInfo struct {
	Major, Minor, Patch int
	// Modules holds every --with-* configure flag, e.g.
	// "http_v3_module" or "http_geoip_module=dynamic".
	Modules map[string]bool
}

var reNginxVersion = regexp.MustCompile(`nginx/(\d+)\.(\d+)\.(\d+)`)

// DetectBuild runs `nginx -V` and parses its output.
func DetectBuild(ctx context.Context, binaryPath string) (BuildInfo, error) {
	if binaryPath == "" {
		binaryPath = "nginx"
	}
	out, err := exec.CommandContext(ctx, binaryPath, "-V").CombinedOutput()
	if err != nil {
		return BuildInfo{}, err
	}
	return ParseBuild(string(out)), nil
}

// ParseBuild parses `nginx -V` output.
func ParseBuild(out string) BuildInfo {
	b := BuildInfo{Modules: map[string]bool{}}
	if m := reNginxVersion.FindStringSubmatch(out); m != nil {
		b.Major, _ = strconv.Atoi(m[1])
		b.Minor, _ = strconv.Atoi(m[2])
		b.Patch, _ = strconv.Atoi(m[3])
	}
	for _, f := range strings.Fields(out) {
		if mod, ok := strings.CutPrefix(f, "--with-"); ok {
			b.Modules[mod] = true
		}
	}
	return b
}

// AtLeast reports whether the version is major.minor.patch or newer.
func (b BuildInfo) AtLeast(major, minor, patch int) bool {
	if b.Major != major {
		return b.Major > major
	}
	if b.Minor != minor {
		return b.Minor > minor
	}
	return b.Patch >= patch
}

// HasModule reports whether module (e.g. "http_v3_module") was compiled in,
// statically or as a dynamic module.
func (b BuildInfo) HasModule(module string) bool {
	return b.Modules[module] || b.Modules[module+"=dynamic"]
}

// HTTP2Directive reports whether HTTP/2 is enabled with the `http2 on;`
// directive (1.25.1+); older versions use `listen ... http2`, which newer
// ones still accept but warn about.
func (b BuildInfo) HTTP2Directive() bool { return b.AtLeast(1, 25, 1) }

// SupportsHTTP3 reports whether QUIC/HTTP/3 listeners can be configured.
func (b BuildInfo) SupportsHTTP3() bool { return b.AtLeast(1, 25, 0) && b.HasModule("http_v3_module") }
