package cmd

import (
	"github.com/cpars-innovation/cpicli/internal/config"
	"github.com/cpars-innovation/cpicli/internal/output"
	"github.com/cpars-innovation/cpicli/pkg/ops"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func NewDriftCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "drift",
		Short: "Compare local artifacts with their designtime and runtime state on the tenant",
		Long: `For each artifact of a local content tree: the local Bundle-Version and content
against the designtime version and content on the tenant (compared like upload
does), and the deployed version.

  in_sync        same content
  tenant_newer   content differs and the tenant has the higher version
                 (edited on the tenant: download before uploading)
  local_newer    content differs and the local version is higher
  diverged       content differs with the same version
  not_on_tenant  the artifact does not exist on the tenant

runtimeOutdated marks artifacts whose deployed version differs from the
designtime version. The tenant is only read; every artifact is downloaded.`,
		Example:      `  cpictl drift --local-dir ./content --package-id Orders`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := ops.Drift(cmd.Context(), tenantExecuter(cmd), config.GetString(cmd, "local-dir"), config.GetString(cmd, "package-id"),
				config.GetIntWithFallback(cmd, "parallel", "drift.parallel"))
			if res != nil {
				output.SetResult(cmd.Context(), res)
				for _, it := range res.Items {
					note := ""
					if it.RuntimeOutdated {
						note = " (runtime " + it.RuntimeVersion + ")"
					}
					log.Info().Msgf("%-14s %-40s local %-8s tenant %-8s%s %s", it.State, it.ArtifactID, it.LocalVersion, it.DesigntimeVersion, note, it.Error)
				}
			}
			return err
		},
	}
	c.Flags().String("local-dir", ".", "Local content directory")
	c.Flags().String("package-id", "", "Only artifacts in this package folder")
	c.Flags().Int("parallel", 8, "Artifacts compared at the same time (config: drift.parallel)")
	return c
}
