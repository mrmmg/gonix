// Package maintenance puts hosts into maintenance mode: every request gets
// 503 except from bypass addresses (e.g. the admins' office), so a site can
// be worked on behind closed doors. A custom page is served by adding 503
// to the host's error pages snippet.
//
// Bypass addresses are matched at http level, where geo understands CIDR
// ranges: geo $gonix_maint_<host> { default 1; <bypass> 0; }. The host
// then answers 503 when that is 1, except for GoNix's internal error page
// URIs (see nginx.ErrorPageURIPrefix), so the 503's own error page is not
// refused as well.
package maintenance

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mrmmg/gonix/internal/hosts"
	"github.com/mrmmg/gonix/internal/iplist"
	"github.com/mrmmg/gonix/internal/nginx"
	"github.com/mrmmg/gonix/internal/snippet"
)

// Render produces the http-level file for serverName.
func Render(serverName string, bypass []string) (string, error) {
	for _, b := range bypass {
		if err := iplist.ValidateEntry(b); err != nil {
			return "", fmt.Errorf("bypass address: %w", err)
		}
	}
	v := nginx.MaintenanceVar(serverName)
	var b strings.Builder
	fmt.Fprintf(&b, "%s Maintenance mode: %s\n", nginx.ManagedMarker, serverName)
	b.WriteString("# 1 = this client gets 503; bypass addresses get 0.\n")
	fmt.Fprintf(&b, "geo $%s {\n    default 1;\n", v)
	for _, a := range bypass {
		fmt.Fprintf(&b, "    %s 0;\n", a)
	}
	b.WriteString("}\n")
	return b.String(), nil
}

var reBypass = regexp.MustCompile(`(?m)^\s+(\S+)\s+0;`)

// ParseBypass extracts the bypass addresses from a file produced by Render.
func ParseBypass(content string) []string {
	var out []string
	for _, m := range reBypass.FindAllStringSubmatch(content, -1) {
		if m[1] != "default" {
			out = append(out, m[1])
		}
	}
	return out
}

// Service turns maintenance mode on and off.
type Service struct {
	HTTPDir string
	Hosts   *hosts.Service
	Writer  *snippet.Writer
}

// Path returns the http-level file of serverName.
func (s *Service) Path(serverName string) string {
	return filepath.Join(s.HTTPDir, nginx.MaintenanceFileName(serverName))
}

// Bypass returns the bypass addresses currently configured for serverName.
func (s *Service) Bypass(serverName string) []string {
	data, err := os.ReadFile(s.Path(serverName))
	if err != nil {
		return nil
	}
	return ParseBypass(string(data))
}

// Enable writes the http-level file, then switches the host on. If the host
// update fails, the file is removed again.
func (s *Service) Enable(ctx context.Context, fileName string, bypass []string) error {
	h, err := s.Hosts.Manager.GetHost(fileName)
	if err != nil {
		return err
	}
	if h.Mode == nginx.ModeRedirect {
		return fmt.Errorf("redirect hosts have no content to put into maintenance")
	}
	content, err := Render(h.ServerName, bypass)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.HTTPDir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", s.HTTPDir, err)
	}
	path := s.Path(h.ServerName)
	_, statErr := os.Stat(path)
	existed := statErr == nil
	if err := s.Writer.Write(ctx, "maintenance_configured", h.ServerName, path, content); err != nil {
		return err
	}
	h.Maintenance = true
	if _, err := s.Hosts.UpdateHost(ctx, h); err != nil {
		if !existed {
			_ = s.Writer.Remove(ctx, "maintenance_configured", h.ServerName, path)
		}
		return err
	}
	return nil
}

// Disable switches the host off, then removes its http-level file.
func (s *Service) Disable(ctx context.Context, fileName string) error {
	h, err := s.Hosts.Manager.GetHost(fileName)
	if err != nil {
		return err
	}
	h.Maintenance = false
	if _, err := s.Hosts.UpdateHost(ctx, h); err != nil {
		return err
	}
	return s.Writer.Remove(ctx, "maintenance_disabled", h.ServerName, s.Path(h.ServerName))
}
