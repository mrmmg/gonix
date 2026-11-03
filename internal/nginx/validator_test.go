package nginx

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidatorPassesConfigFile(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "fake-nginx")
	out := filepath.Join(dir, "args")
	script := "#!/bin/sh\necho \"$@\" > " + out + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	v := NewValidator(bin)
	v.ConfigFile = "/etc/nginx-staging/nginx.conf"
	if res, err := v.Test(context.Background()); err != nil || !res.OK {
		t.Fatalf("Test = %+v, %v", res, err)
	}
	args, _ := os.ReadFile(out)
	if strings.TrimSpace(string(args)) != "-t -c /etc/nginx-staging/nginx.conf" {
		t.Errorf("args = %q", args)
	}
}
