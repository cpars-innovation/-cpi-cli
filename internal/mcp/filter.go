package mcp

import (
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/cpars-innovation/cpicli/internal/output"
)

// Effect is what a tool can change.
type Effect string

const (
	// EffectRead changes nothing.
	EffectRead Effect = "read"
	// EffectLocal writes files inside the server root only.
	EffectLocal Effect = "local"
	// EffectTenant changes the tenant or triggers processing on it.
	EffectTenant Effect = "tenant"
)

// toolEffects classifies every tool. TestEveryToolHasAnEffect fails for a
// tool missing here, so a new tool cannot slip into read-only servers.
var toolEffects = map[string]Effect{
	"list_packages": EffectRead, "list_artifacts": EffectRead, "get_runtime_status": EffectRead,
	"list_message_logs": EffectRead, "get_message_log": EffectRead, "get_message_steps": EffectRead,
	"get_message_attachment": EffectRead, "get_message_store_entry": EffectRead,
	"get_message_trace": EffectRead, "get_trace_message": EffectRead,
	"list_runtime_artifacts": EffectRead, "list_service_endpoints": EffectRead,
	"validate_artifact": EffectRead, "check_guidelines": EffectRead,
	"list_resources": EffectRead, "get_resource": EffectRead,
	"list_credentials": EffectRead, "list_keystore": EffectRead, "get_parameters": EffectRead,
	"get_pd_parameters": EffectRead, "pd_diff": EffectRead, "get_trace_tree": EffectRead,

	"download_artifact": EffectLocal, "discover_tenant": EffectLocal,

	"create_package": EffectTenant, "upload_artifact": EffectTenant, "set_parameters": EffectTenant,
	"deploy": EffectTenant, "undeploy": EffectTenant, "pd_deploy": EffectTenant,
	"send_test_message": EffectTenant, "set_log_level": EffectTenant,
}

// ToolFilter limits the tools a server offers.
type ToolFilter struct {
	// ReadOnly drops every tool that changes the tenant or triggers
	// processing (local file writes stay available).
	ReadOnly bool
	// Allow keeps only matching tools (names or path.Match patterns such as
	// "list_*"); empty keeps all.
	Allow []string
	// Deny drops matching tools; it wins over Allow.
	Deny []string
}

// Filter applies f and returns the remaining tools and the names of the
// removed ones. Patterns that match no tool are usage errors, so a typo never
// silently leaves a tool enabled.
func Filter(tools []Tool, f ToolFilter) (kept []Tool, removed []string, err error) {
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		names = append(names, t.Name)
	}
	for _, p := range append(slices.Clone(f.Allow), f.Deny...) {
		if _, err := path.Match(p, ""); err != nil {
			return nil, nil, output.Usagef("invalid tool pattern %q", p)
		}
		if !slices.ContainsFunc(names, func(n string) bool { return match(p, n) }) {
			return nil, nil, output.Usagef("tool pattern %q matches no tool (tools: %s)", p, strings.Join(names, ", "))
		}
	}
	for _, t := range tools {
		keep := (len(f.Allow) == 0 || slices.ContainsFunc(f.Allow, func(p string) bool { return match(p, t.Name) })) &&
			!slices.ContainsFunc(f.Deny, func(p string) bool { return match(p, t.Name) }) &&
			!(f.ReadOnly && EffectOf(t.Name) == EffectTenant)
		if keep {
			kept = append(kept, t)
		} else {
			removed = append(removed, t.Name)
		}
	}
	if len(kept) == 0 {
		return nil, nil, output.Usagef("the tool filter leaves no tools")
	}
	return kept, removed, nil
}

func match(pattern, name string) bool {
	ok, _ := path.Match(pattern, name)
	return ok
}

// EffectOf returns what a tool can change; unknown tools count as changing
// the tenant.
func EffectOf(name string) Effect {
	if e, ok := toolEffects[name]; ok {
		return e
	}
	return EffectTenant
}

// FilteredInstructions appends a note on removed tools to the instructions,
// so that the agent does not plan with tools it cannot call.
func FilteredInstructions(instructions string, f ToolFilter, removed []string) string {
	if len(removed) == 0 {
		return instructions
	}
	note := fmt.Sprintf("\n\nNot available on this server (disabled by configuration): %s.", strings.Join(removed, ", "))
	if f.ReadOnly {
		note += " This server is read-only: it cannot change the tenant or send messages; ask the user to make changes."
	}
	return instructions + note
}
