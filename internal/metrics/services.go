package metrics

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/mrmmg/gonix/internal/app"
	"github.com/mrmmg/gonix/internal/certificates"
	"github.com/mrmmg/gonix/internal/health"
	"github.com/mrmmg/gonix/internal/lint"
	"github.com/mrmmg/gonix/internal/system"
	"github.com/mrmmg/gonix/internal/upstreams"
)

// FromServices builds the exporter of an installation, with the filters,
// collectors and cache time of its monitoring settings.
func FromServices(svc *app.Services, version string) *Exporter {
	cfg := svc.Config
	m := cfg.Monitoring
	b := svc.Manager.Renderer.Build
	ssURL := svc.Exporter.StubStatusURL()

	src := Sources{
		Hosts: svc.Manager.ListHosts,
		Certs: func() []certificates.Certificate {
			certs, _ := certificates.Scan(cfg.Certificates.Directory)
			return append(certs, certificates.ScanCertbot(certificates.DefaultLetsEncryptDir)...)
		},
		Lint: func(ctx context.Context) ([]lint.Finding, error) {
			c := &lint.Checker{
				Manager: svc.Manager, ConfigDir: cfg.Nginx.ConfigDir, HTTPDir: cfg.Nginx.HTTPSnippetsDir,
				SnippetsDir: cfg.ErrorPages.SnippetsDirectory, AccessListDir: cfg.AccessLists.Directory,
			}
			return c.Run(ctx)
		},
		Groups: func() ([]upstreams.Group, error) { return svc.Upstreams.List() },
		Probe: func(ctx context.Context, addrs []string) map[string]health.Probe {
			return health.Probes(ctx, addrs, probeTimeout)
		},
		NginxStatus: svc.SystemSvc.GetStatus,
		NginxTest: func(ctx context.Context) (bool, error) {
			res, err := svc.HostService.Tester.Test(ctx)
			return res.OK, err
		},
		NginxVersion: fmt.Sprintf("%d.%d.%d", b.Major, b.Minor, b.Patch),
		Processes: func(ctx context.Context) (system.ProcessInfo, error) {
			if pid, err := svc.SystemSvc.MainPID(ctx); err == nil {
				if pid == 0 {
					return system.ProcessInfo{}, nil
				}
				return system.ChildProcesses("", pid)
			}
			return system.FindProcesses("") // no systemd: any nginx on the machine
		},
		RSS:       func(pid int) (uint64, error) { return system.MemoryUsage("", pid) },
		Uptime:    func(pid int) (time.Duration, error) { return system.Uptime("", pid) },
		LogDir:    cfg.Logs.NginxDirectory,
		AuditFile: cfg.Logs.AuditFile,
	}
	if ssURL != "" {
		src.StubStatus = func(ctx context.Context) (string, error) { return fetch(ctx, ssURL) }
	}
	return New(Options{
		Instance: cfg.Instance, Version: version,
		Filter:     Filter{Include: m.Hosts, Exclude: m.ExcludeHosts},
		Collectors: m.Collectors, CacheTTL: m.CacheTTL,
	}, src)
}

func fetch(ctx context.Context, url string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s answered %s", url, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	return string(body), err
}
