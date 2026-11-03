package nginx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIncludesDir(t *testing.T) {
	for _, tc := range []struct {
		conf string
		want bool
	}{
		{"http {\n    include /etc/nginx/conf.d/*.conf;\n}\n", true},
		{"http {\n    include conf.d/*.conf;\n}\n", true},
		{"http {\n    include /etc/nginx/conf.d/*;\n}\n", true},
		{"http {\n    # include /etc/nginx/conf.d/*.conf;\n    include sites-enabled/*;\n}\n", false},
		{"http {\n    include mime.types;\n}\n", false},
	} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "nginx.conf"), []byte(tc.conf), 0o644); err != nil {
			t.Fatal(err)
		}
		// Relative includes are resolved against the config directory.
		target := "/etc/nginx/conf.d"
		if !filepath.IsAbs(firstIncludeArg(tc.conf)) {
			target = filepath.Join(dir, "conf.d")
		}
		got, err := IncludesDir(dir, target)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("IncludesDir(%q) = %v, want %v", tc.conf, got, tc.want)
		}
	}

	if _, err := IncludesDir(t.TempDir(), "/etc/nginx/conf.d"); err == nil {
		t.Error("expected an error when nginx.conf is missing")
	}
}

func firstIncludeArg(conf string) string {
	if m := reInclude.FindStringSubmatch(stripComments(conf)); m != nil {
		return m[1]
	}
	return "/"
}
