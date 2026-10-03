package nginx

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	reUserDirective = regexp.MustCompile(`(?m)^\s*user\s+([^\s;]+)(?:\s+([^\s;]+))?\s*;`)
	reConfigureUser = regexp.MustCompile(`--user=(\S+)`)
	reConfigureGrp  = regexp.MustCompile(`--group=(\S+)`)
)

// DetectWorkerGroup returns the group Nginx worker processes run as, which
// is the group that must be able to read files Nginx opens per request
// (e.g. auth_basic_user_file). It checks, in order: the `user` directive in
// <configDir>/nginx.conf, the compile-time --user/--group reported by
// `nginx -V`, and finally the "nobody"/"nogroup" built-in defaults. The
// returned group is guaranteed to exist on this system.
func DetectWorkerGroup(ctx context.Context, binaryPath, configDir string) (string, error) {
	var candidates []string

	if data, err := os.ReadFile(filepath.Join(configDir, "nginx.conf")); err == nil {
		if g := groupFromUserDirective(string(data)); g != "" {
			candidates = append(candidates, g)
		}
	}
	if binaryPath == "" {
		binaryPath = "nginx"
	}
	if out, err := exec.CommandContext(ctx, binaryPath, "-V").CombinedOutput(); err == nil {
		if g := groupFromConfigureArgs(string(out)); g != "" {
			candidates = append(candidates, g)
		}
	}
	candidates = append(candidates, "nobody", "nogroup")

	for _, g := range candidates {
		if _, err := user.LookupGroup(g); err == nil {
			return g, nil
		}
	}
	return "", fmt.Errorf("could not determine the Nginx worker group (tried %s)", strings.Join(candidates, ", "))
}

// groupFromUserDirective extracts the worker group from a `user` directive.
// Nginx uses a group named like the user when the group is omitted.
func groupFromUserDirective(conf string) string {
	m := reUserDirective.FindStringSubmatch(conf)
	if m == nil {
		return ""
	}
	if m[2] != "" {
		return m[2]
	}
	return m[1]
}

// groupFromConfigureArgs extracts the compile-time default worker group from
// `nginx -V` output, falling back to the compile-time user's name.
func groupFromConfigureArgs(out string) string {
	if m := reConfigureGrp.FindStringSubmatch(out); m != nil {
		return m[1]
	}
	if m := reConfigureUser.FindStringSubmatch(out); m != nil {
		return m[1]
	}
	return ""
}
