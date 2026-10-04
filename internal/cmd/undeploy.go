package cmd

import (
	"fmt"
	"time"

	"github.com/cpars-innovation/-cpi-cli/internal/api"
	"github.com/cpars-innovation/-cpi-cli/internal/config"
	"github.com/cpars-innovation/-cpi-cli/internal/deployer"
	"github.com/cpars-innovation/-cpi-cli/internal/str"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func NewUndeployCommand() *cobra.Command {

	undeployCmd := &cobra.Command{
		Use:          "undeploy",
		Short:        "Undeploy runtime artifacts",
		SilenceUsage: true,
		Long: `Undeploy artifacts from the runtime of an SAP Integration Suite tenant.

For each artifact the runtime artifact is deleted and its status is polled
until the tenant no longer knows it (HTTP 404). Artifacts that are not
deployed are reported as NOT_DEPLOYED and are not treated as failures.
Designtime artifacts are not touched.

Configuration:
  Settings can be loaded from the global config file (--config) under the
  'undeploy' section. CLI flags override config file settings.`,
		Example: `  cpictl undeploy --artifact-ids MyIFlow,MyOtherIFlow`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUndeploy(cmd)
		},
	}

	undeployCmd.Flags().StringSlice("artifact-ids", nil, "Comma separated list of artifact IDs (config: undeploy.artifactIds)")
	undeployCmd.Flags().Int("delay-length", 10, "Delay (in seconds) between each check of artifact undeployment status (config: undeploy.delayLength)")
	undeployCmd.Flags().Int("max-check-limit", 30, "Max number of times to check for artifact undeployment status (config: undeploy.maxCheckLimit)")

	return undeployCmd
}

func runUndeploy(cmd *cobra.Command) error {
	artifactIds := str.TrimSlice(config.GetStringSliceWithFallback(cmd, "artifact-ids", "undeploy.artifactIds"))
	delayLength := config.GetIntWithFallback(cmd, "delay-length", "undeploy.delayLength")
	maxCheckLimit := config.GetIntWithFallback(cmd, "max-check-limit", "undeploy.maxCheckLimit")

	var artifacts []deployer.Artifact
	for _, id := range artifactIds {
		if id != "" {
			artifacts = append(artifacts, deployer.Artifact{ID: id})
		}
	}
	if len(artifacts) == 0 {
		return fmt.Errorf("required flag \"artifact-ids\" not set (or config undeploy.artifactIds)")
	}

	log.Info().Msgf("Executing undeploy command for %d artifact(s)", len(artifacts))
	exe := api.InitHTTPExecuter(api.GetServiceDetails(cmd))
	results := deployer.Undeploy(cmd.Context(), deployer.NewTenant(exe), artifacts, deployer.Options{
		Interval:    time.Duration(delayLength) * time.Second,
		MaxChecks:   maxCheckLimit,
		Parallelism: len(artifacts),
	})
	logResults(results)
	if err := deployer.Err(results); err != nil {
		return err
	}
	log.Info().Msg("🏆 Artifact(s) undeployment completed successfully")
	return nil
}
