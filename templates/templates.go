// Package templates embeds the Nginx configuration templates shipped with
// GoNix so the compiled binary remains self-contained while the
// template sources stay editable as plain files in this directory.
package templates

import "embed"

//go:embed *.tmpl
var FS embed.FS

// Lab holds the files of the GoNix lab (see internal/lab): its Nginx
// configuration, systemd unit, guide, and one template per test host
// scenario under lab/sites.
//
//go:embed lab
var Lab embed.FS
