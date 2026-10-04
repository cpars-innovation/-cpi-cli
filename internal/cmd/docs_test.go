package cmd

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const commandReferencePath = "../../docs/commands.md"

// TestCommandReference keeps docs/commands.md in sync with the CLI.
// Regenerate with: go test ./internal/cmd -run TestCommandReference -update
func TestCommandReference(t *testing.T) {
	got := renderCommandReference(NewCLI("dev"))
	if *updateGolden {
		require.NoError(t, os.WriteFile(commandReferencePath, []byte(got), 0644))
	}
	want, err := os.ReadFile(commandReferencePath)
	require.NoError(t, err)
	assert.Equal(t, string(want), got, "docs/commands.md is out of date, run: go test ./internal/cmd -run TestCommandReference -update")
}

func renderCommandReference(root *cobra.Command) string {
	var b strings.Builder
	b.WriteString("# Command reference\n\n")
	b.WriteString("<!-- Generated from the CLI by `go test ./internal/cmd -run TestCommandReference -update`. Do not edit. -->\n\n")
	b.WriteString("Every flag can also be set with an environment variable (`FLASHPIPE_` + flag name in upper case, `-` replaced by `_`) or as a top-level key in the config file. ")
	b.WriteString("See [configuration.md](configuration.md).\n\n")

	var cmds []*cobra.Command
	walkCommands(root, func(c *cobra.Command) {
		if c != root && !c.Hidden && c.Name() != "help" && c.Name() != "completion" && c.Parent().Name() != "completion" {
			cmds = append(cmds, c)
		}
	})

	b.WriteString("| Command | Description |\n|---------|-------------|\n")
	for _, c := range cmds {
		path := commandName(c)
		fmt.Fprintf(&b, "| [`%s`](#%s) | %s |\n", path, strings.ReplaceAll(path, " ", "-"), c.Short)
	}

	b.WriteString("\n## Global flags\n\n```\n")
	b.WriteString(root.PersistentFlags().FlagUsages())
	b.WriteString("```\n")

	for _, c := range cmds {
		path := commandName(c)
		fmt.Fprintf(&b, "\n## %s\n\n%s\n", path, c.Short)
		if c.Long != "" {
			fmt.Fprintf(&b, "\n```\n%s\n```\n", strings.TrimSpace(c.Long))
		}
		if c.Runnable() {
			fmt.Fprintf(&b, "\n**Usage:** `%s`\n", c.UseLine())
		}
		if flags := c.LocalNonPersistentFlags().FlagUsages(); flags != "" {
			fmt.Fprintf(&b, "\n**Flags:**\n\n```\n%s```\n", flags)
		}
		if c.Example != "" {
			fmt.Fprintf(&b, "\n**Examples:**\n\n```\n%s\n```\n", c.Example)
		}
	}
	return b.String()
}
