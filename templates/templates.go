// Package templates embeds the Nginx configuration templates shipped with
// GoNix so the compiled binary remains self-contained while the
// template sources stay editable as plain files in this directory.
package templates

import "embed"

//go:embed *.tmpl
var FS embed.FS
