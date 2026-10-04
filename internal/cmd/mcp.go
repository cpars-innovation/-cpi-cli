package cmd

import (
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

Tools: list_packages, list_artifacts, get_runtime_status, get_parameters,
set_parameters, upload_artifact, deploy, undeploy (requires confirm=true),
pd_deploy (dry run unless dry_run=false).

Local paths given to tools are resolved against --root and may not leave it.`,
		Example: `  # Claude Code / any MCP client configuration
  {"mcpServers": {"cpi": {"command": "cpictl", "args": ["mcp", "--root", "/path/to/repo"],
    "env": {"CPICTL_TMN_HOST": "...", "CPICTL_OAUTH_HOST": "...",
            "CPICTL_OAUTH_CLIENTID": "...", "CPICTL_OAUTH_CLIENTSECRET": "..."}}}}`,
		SilenceUsage: true,
		Annotations:  map[string]string{annotationNoEnvelope: "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			// stdout is the protocol channel: logs always as JSON lines on stderr
			logger.Init(cmd.ErrOrStderr(), true, viper.GetBool("debug"))

			tools := mcp.Tools(mcp.Config{
				Exe:          tenantExecuter(cmd),
				Root:         config.GetString(cmd, "root"),
				PollInterval: time.Duration(config.GetInt(cmd, "poll-interval")) * time.Second,
				MaxChecks:    config.GetInt(cmd, "max-checks"),
			})
			server := mcp.NewServer("cpicli", version, mcp.Instructions, tools)
			log.Info().Msgf("MCP server started with %d tools", len(tools))
			return server.Serve(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout())
		},
	}
	c.Flags().String("root", ".", "Directory that local paths of tool calls are confined to")
	c.Flags().Int("poll-interval", 10, "Default seconds between deploy/undeploy status checks")
	c.Flags().Int("max-checks", 30, "Default maximum number of deploy/undeploy status checks")
	return c
}
