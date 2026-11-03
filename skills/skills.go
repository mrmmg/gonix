// Package skills embeds the Agent Skill that teaches AI agents to manage
// Nginx through the gonix CLI. `gonix skill install` writes the copy that
// matches the installed binary, so its instructions never drift from the
// commands it describes.
package skills

import _ "embed"

// GoNix is the content of gonix/SKILL.md.
//
//go:embed gonix/SKILL.md
var GoNix string
