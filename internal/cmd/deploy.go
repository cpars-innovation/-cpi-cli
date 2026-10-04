package cmd

import (
	"fmt"
	"github.com/cpars-innovation/cpicli/internal/config"
	"github.com/cpars-innovation/cpicli/internal/output"
	"github.com/cpars-innovation/cpicli/internal/str"
	"github.com/cpars-innovation/cpicli/pkg/cpi"
	"github.com/cpars-innovation/cpicli/pkg/ops"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"time"
)

func NewDeployCommand() *cobra.Command {

	deployCmd := &cobra.Command{
		Use:          "deploy",
		Short:        "Deploy designtime artifact to runtime",
		SilenceUsage: true,
		Long: `Deploy artifact from designtime to
runtime of SAP Integration Suite tenant.

Configuration:
  Settings can be loaded from the global config file (--config) under the
  'deploy' section. CLI flags override config file settings.`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Validate the artifact type
			artifactType := config.GetStringWithFallback(cmd, "artifact-type", "deploy.artifactType")
			if !cpi.IsValidArtifactType(artifactType) {
				return fmt.Errorf("invalid value for --artifact-type = %v", artifactType)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			if err = runDeploy(cmd); err != nil {
				cmd.SilenceUsage = true
			}
			return
		},
	}

	// Define cobra flags, the default value has the lowest (least significant) precedence
	// Note: These can be set in config file under 'deploy' key
	deployCmd.Flags().StringSlice("artifact-ids", nil, "Comma separated list of artifact IDs (config: deploy.artifactIds)")
	deployCmd.Flags().Int("delay-length", 30, "Delay (in seconds) between each check of artifact deployment status (config: deploy.delayLength)")
	deployCmd.Flags().Int("max-check-limit", 10, "Max number of times to check for artifact deployment status (config: deploy.maxCheckLimit)")
	// To set to false, use --compare-versions=false
	deployCmd.Flags().Bool("compare-versions", true, "Perform version comparison of design time against runtime before deployment (config: deploy.compareVersions)")
	deployCmd.Flags().String("artifact-type", "Integration", "Artifact type. Allowed values: Integration, MessageMapping, ScriptCollection, ValueMapping (config: deploy.artifactType)")

	_ = deployCmd.MarkFlagRequired("artifact-ids")
	return deployCmd
}

func runDeploy(cmd *cobra.Command) error {
	serviceDetails := serviceDetails(cmd)

	// Support reading from config file under 'deploy' key
	artifactType := config.GetStringWithFallback(cmd, "artifact-type", "deploy.artifactType")
	log.Info().Msgf("Executing deploy %v command", artifactType)

	artifactIds := str.TrimSlice(config.GetStringSliceWithFallback(cmd, "artifact-ids", "deploy.artifactIds"))
	delayLength := config.GetIntWithFallback(cmd, "delay-length", "deploy.delayLength")
	maxCheckLimit := config.GetIntWithFallback(cmd, "max-check-limit", "deploy.maxCheckLimit")
	compareVersions := config.GetBoolWithFallback(cmd, "compare-versions", "deploy.compareVersions")

	artifacts := make([]ops.Artifact, 0, len(artifactIds))
	for _, id := range artifactIds {
		artifacts = append(artifacts, ops.Artifact{ID: id, Type: artifactType})
	}

	exe := cpi.InitHTTPExecuter(serviceDetails)
	results := ops.Deploy(cmd.Context(), ops.NewTenant(exe), artifacts, ops.Options{
		Interval:        time.Duration(delayLength) * time.Second,
		MaxChecks:       maxCheckLimit,
		CompareVersions: compareVersions,
		// All artifacts are triggered and polled concurrently, as before
		Parallelism: len(artifacts),
	})
	logResults(results)
	output.SetResult(cmd.Context(), artifactResults{Results: results})
	if err := ops.Err(results); err != nil {
		return err
	}
	log.Info().Msg("🏆 Artifact(s) deployment completed successfully")
	return nil
}

// artifactResults is the JSON result of deploy and undeploy.
type artifactResults struct {
	Results []ops.Result `json:"results"`
}

// logResults logs one line per artifact result.
func logResults(results []ops.Result) {
	for _, r := range results {
		event := log.Info()
		if !r.Status.Succeeded() {
			event = log.Error()
		}
		event.Str("artifact", r.ID).Str("status", string(r.Status)).Str("version", r.Version).Str("taskId", r.TaskID).
			Msgf("%s: %s%s", r.ID, r.Status, errSuffix(r.Error))
	}
}

func errSuffix(msg string) string {
	if msg == "" {
		return ""
	}
	return " - " + msg
}
