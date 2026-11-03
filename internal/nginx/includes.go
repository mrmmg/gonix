package nginx

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var reInclude = regexp.MustCompile(`(?m)^\s*include\s+([^;]+);`)

// IncludesDir reports whether configDir/nginx.conf has an include directive
// whose pattern matches .conf files in dir, e.g. `include
// /etc/nginx/conf.d/*.conf;` for dir /etc/nginx/conf.d. Files GoNix writes
// to dir only take effect when this is true. It is a best-effort check of
// nginx.conf itself, not of nested includes.
func IncludesDir(configDir, dir string) (bool, error) {
	data, err := os.ReadFile(filepath.Join(configDir, "nginx.conf"))
	if err != nil {
		return false, fmt.Errorf("reading nginx.conf: %w", err)
	}
	probe := filepath.Join(dir, "gonix-probe.conf")
	for _, m := range reInclude.FindAllStringSubmatch(stripComments(string(data)), -1) {
		pattern := strings.Trim(strings.TrimSpace(m[1]), `"'`)
		if !filepath.IsAbs(pattern) {
			pattern = filepath.Join(configDir, pattern)
		}
		if ok, _ := filepath.Match(pattern, probe); ok {
			return true, nil
		}
	}
	return false, nil
}

func stripComments(content string) string {
	lines := strings.Split(content, "\n")
	for i, l := range lines {
		if idx := strings.IndexByte(l, '#'); idx >= 0 {
			lines[i] = l[:idx]
		}
	}
	return strings.Join(lines, "\n")
}
