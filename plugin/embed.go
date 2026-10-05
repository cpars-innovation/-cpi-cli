// Package plugin embeds the skills and agents of the Claude Code plugin, so
// that cpictl can list, show and install them without a checkout of this
// repository (cpictl skills, MCP help).
package plugin

import "embed"

// FS holds skills/<name>/... and agents/<name>.md.
//
//go:embed skills agents
var FS embed.FS
