package tui

import (
	"fmt"
	"strings"

	"github.com/mrmmg/gonix/internal/lint"
)

// newConfigCheckScreen runs every lint check, including upstream dials, and
// shows the findings most severe first.
func newConfigCheckScreen(deps Deps) screen {
	findings, err := newLintChecker(deps, true).Run(backgroundCtx())
	if err != nil {
		return newViewerScreen("Config Check", errorBoxStyle.Render(err.Error()), nil)
	}
	return newViewerScreen("Config Check", formatFindings(findings), nil)
}

func newLintChecker(deps Deps, upstreams bool) *lint.Checker {
	return &lint.Checker{
		Manager:        deps.Manager,
		ConfigDir:      deps.Config.Nginx.ConfigDir,
		HTTPDir:        deps.Config.Nginx.HTTPSnippetsDir,
		SnippetsDir:    deps.Config.ErrorPages.SnippetsDirectory,
		AccessListDir:  deps.Config.AccessLists.Directory,
		CheckUpstreams: upstreams,
	}
}

func formatFindings(findings []lint.Finding) string {
	if len(findings) == 0 {
		return successBoxStyle.Render("No problems found.")
	}
	counts := map[lint.Severity]int{}
	for _, f := range findings {
		counts[f.Severity]++
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d errors, %d warnings, %d notes\n\n", counts[lint.Error], counts[lint.Warning], counts[lint.Info])
	for _, f := range findings {
		label := fmt.Sprintf("%-5s", f.Severity)
		switch f.Severity {
		case lint.Error:
			label = dangerStyle.Render(label)
		case lint.Info:
			label = mutedStyle.Render(label)
		}
		fmt.Fprintf(&b, "%s %s\n      %s\n", label, f.Target, f.Message)
	}
	return b.String()
}
