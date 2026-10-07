package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/cpars-innovation/cpicli/internal/config"
	"github.com/cpars-innovation/cpicli/internal/mcp"
	"github.com/cpars-innovation/cpicli/internal/output"
	"github.com/cpars-innovation/cpicli/internal/skills"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func NewSkillsCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "skills",
		Short: "List, read and install the cpi skills built into cpictl",
		Long: `The skills of the Claude Code plugin (cpi-discover, cpi-plan, cpi-build, cpi-test,
cpi-review) are built into cpictl, in the version of this binary. Use them in
agents other than Claude Code (Codex, Cursor, Gemini CLI, ...), or read them.
Claude Code users install the plugin instead (docs/plugin.md); over MCP, the
help tool shows the same skills.`,
		Annotations: map[string]string{annotationOffline: "true"},
	}
	list := &cobra.Command{
		Use:          "list",
		Short:        "List the skills and what they are for",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		Annotations:  map[string]string{annotationOffline: "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			res := map[string]any{"skills": skills.List(), "agents": skills.Agents()}
			output.SetResult(cmd.Context(), res)
			for _, s := range skills.List() {
				log.Info().Msgf("%-13s %s", s.Name, s.Description)
			}
			for _, a := range skills.Agents() {
				log.Info().Msgf("%-13s (Claude Code subagent) %s", a.Name, a.Description)
			}
			return nil
		},
	}
	show := &cobra.Command{
		Use:          "show SKILL [FILE]",
		Short:        "Print a skill's SKILL.md or one of its reference files",
		Example:      "  cpictl skills show cpi-build\n  cpictl skills show cpi-plan brief-template.md",
		Args:         cobra.RangeArgs(1, 2),
		SilenceUsage: true,
		Annotations:  map[string]string{annotationOffline: "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			file := ""
			if len(args) == 2 {
				file = args[1]
			}
			text, err := skills.Read(args[0], file)
			if err != nil {
				return err
			}
			if config.GetString(cmd, "output") == output.FormatJSON {
				output.SetResult(cmd.Context(), map[string]any{"skill": args[0], "file": file, "content": text})
				return nil
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), text)
			return err
		},
	}
	install := &cobra.Command{
		Use:   "install [REPO]",
		Short: "Copy the skills into the skill folder of an agent",
		Long: `Copy the skills into a content repository (default: the current directory) or,
with --user, into your home directory, in the folder the agent reads:

  agents, codex   .agents/skills   (Codex; Cursor and others read it too)
  cursor          .cursor/skills
  gemini          .gemini/skills
  opencode        .opencode/skills (--user: ~/.config/opencode/skills; OpenCode
                  also reads .agents/skills and .claude/skills)
  claude          .claude/skills   (Claude Code without the plugin)

Existing cpi-* skills there are replaced; other skills are kept. The
allowed-tools line is removed, it names tools as the Claude Code plugin sees
them. Run it again after updating cpictl.`,
		Example:      "  cpictl skills install --agent codex\n  cpictl skills install --agent cursor ../content-repo\n  cpictl skills install --agent gemini --user\n  cpictl skills install --agent opencode",
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		Annotations:  map[string]string{annotationOffline: "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			agent, _ := cmd.Flags().GetString("agent")
			user, _ := cmd.Flags().GetBool("user")
			base := "."
			if len(args) == 1 {
				base = args[0]
			}
			if user {
				if len(args) == 1 {
					return output.Usagef("give either REPO or --user")
				}
				home, err := os.UserHomeDir()
				if err != nil {
					return err
				}
				base = home
			} else if info, err := os.Stat(base); err != nil || !info.IsDir() {
				return output.Usagef("%s is not a directory", base)
			}
			res, err := skills.Install(agent, base, user)
			if err != nil {
				return err
			}
			output.SetResult(cmd.Context(), res)
			log.Info().Msgf("Installed %s into %s", strings.Join(res.Skills, ", "), res.Dir)
			return nil
		},
	}
	install.Flags().String("agent", "agents", "Agent whose skill folder to use: "+strings.Join(skills.AgentNames(), ", "))
	install.Flags().Bool("user", false, "Install into your home directory instead of a repository")
	c.AddCommand(list, show, install)
	return c
}

// commandInfos describes the CLI for the MCP help tool.
func commandInfos(root *cobra.Command) []mcp.CommandInfo {
	var res []mcp.CommandInfo
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			if sub.Hidden || sub.Name() == "help" || sub.Name() == "completion" {
				continue
			}
			res = append(res, mcp.CommandInfo{Command: strings.TrimPrefix(sub.CommandPath(), root.Name()+" "), Short: sub.Short,
				Long: sub.Long, Usage: sub.UseLine(), Flags: sub.LocalFlags().FlagUsages()})
			walk(sub)
		}
	}
	walk(root)
	return res
}
