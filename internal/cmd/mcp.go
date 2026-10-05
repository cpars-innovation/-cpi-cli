package cmd

import (
	"strings"
	"time"

	"github.com/cpars-innovation/cpicli/internal/config"
	"github.com/cpars-innovation/cpicli/internal/logger"
	"github.com/cpars-innovation/cpicli/internal/mcp"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// annotationNoEnvelope marks commands that own stdout (no JSON envelope).
const annotationNoEnvelope = "cpicli/no-envelope"

func NewMCPCommand(version string) *cobra.Command {
	c := &cobra.Command{
		Use:   "mcp",
		Short: "Run the MCP server (stdio) for AI agents",
		Long: `Run a Model Context Protocol server on stdin/stdout.

The server uses the same tenant settings as every other command (flags,
CPICTL_* environment variables or cpictl.yaml). stdout carries the
protocol only; logs go to stderr as JSON lines.

Tools: list packages, artifacts, resources and parameters; create packages;
download, upload, validate, guideline check, deploy, undeploy (requires
confirm=true); runtime status and endpoints; send test messages; message logs,
steps, attachments and persisted messages; discovery of existing content;
pd_deploy (dry run unless dry_run=false). See docs/mcp.md.

send_test_message uses the --runtime-* credentials (default: the API
credentials), see 'cpictl send --help'.

Local paths given to tools are resolved against --root and may not leave it.

Limit the tools per server, e.g. for a QA or production tenant:
  --read-only                     no tool that changes the tenant or sends messages
  --tools list_*,get_*            only matching tools
  --disable-tools undeploy,pd_*   everything except these
  --mode discover|operate|develop presets (combined with the above, the most
                                  restrictive wins)
Also as CPICTL_MODE, CPICTL_READ_ONLY, CPICTL_TOOLS, CPICTL_DISABLE_TOOLS. Disabled tools are not
listed and cannot be called; a pattern that matches no tool is an error.`,
		Example: `  # Claude Code / any MCP client configuration
  {"mcpServers": {"cpi": {"command": "cpictl", "args": ["mcp", "--root", "/path/to/repo"],
    "env": {"CPICTL_TMN_HOST": "...", "CPICTL_OAUTH_HOST": "...",
            "CPICTL_OAUTH_CLIENTID": "...", "CPICTL_OAUTH_CLIENTSECRET": "..."}}}}`,
		SilenceUsage: true,
		Annotations:  map[string]string{annotationNoEnvelope: "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			// stdout is the protocol channel: logs always as JSON lines on stderr
			logger.Init(cmd.ErrOrStderr(), true, viper.GetBool("debug"))

			mode := config.GetString(cmd, "mode")
			root := config.GetString(cmd, "root")
			ledger := mcp.NewLedger(root)
			all := ledger.Wrap(mcp.Tools(mcp.Config{
				Exe:          tenantExecuter(cmd),
				Root:         config.GetString(cmd, "root"),
				PollInterval: time.Duration(config.GetInt(cmd, "poll-interval")) * time.Second,
				MaxChecks:    config.GetInt(cmd, "max-checks"),
				TenantHost:   config.GetString(cmd, "tmn-host"),

				NewEndpointExecuter: endpointExecuter(cmd),
				DenyFullSync:        mode == "develop",
			}))
			readOnly, _ := cmd.Flags().GetBool("read-only")
			filter := mcp.ToolFilter{ReadOnly: readOnly, Allow: config.GetStringSlice(cmd, "tools"), Deny: config.GetStringSlice(cmd, "disable-tools")}
			tools, removed, err := mcp.ApplyFilters(all, mode, filter)
			if err != nil {
				return err
			}
			if len(removed) > 0 {
				log.Info().Msgf("Tools disabled by configuration: %s", strings.Join(removed, ", "))
			}
			server := mcp.NewServer("cpicli", version, mcp.FilteredInstructions(mcp.Instructions, filter, removed)+mcp.ModeInstructions(mode), tools)
			log.Info().Msgf("MCP server started with %d tools", len(tools))
			return server.Serve(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout())
		},
	}
	c.Flags().String("root", ".", "Directory that local paths of tool calls are confined to")
	c.Flags().Int("poll-interval", 10, "Default seconds between deploy/undeploy status checks")
	c.Flags().Int("max-checks", 30, "Default maximum number of deploy/undeploy status checks")
	c.Flags().String("mode", "", "Preset: discover (read-only), operate (read tools + set_log_level), develop (all tools, no pd_deploy full_sync)")
	c.Flags().Bool("read-only", false, "Offer only tools that do not change the tenant or trigger processing")
	c.Flags().StringSlice("tools", nil, "Offer only these tools (names or patterns such as list_*)")
	c.Flags().StringSlice("disable-tools", nil, "Do not offer these tools (names or patterns); wins over --tools")
	addRuntimeAuthFlags(c)
	return c
}
