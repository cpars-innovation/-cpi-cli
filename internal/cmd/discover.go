package cmd

import (
	"github.com/cpars-innovation/cpicli/internal/config"
	"github.com/cpars-innovation/cpicli/internal/output"
	"github.com/cpars-innovation/cpicli/pkg/ops"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func NewDiscoverCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "discover",
		Short: "Inventory existing integration flows to derive conventions",
		Long: `Inventory the packages and integration flows of the tenant (or of a local
directory with --dir) and write the facts as JSON: adapters, steps, exception
subprocesses, log levels, scripts (identical scripts across flows), externalised
parameter keys, credential names, headers and properties that are set, and
naming patterns of packages and flows.

The tenant is only read; every integration flow is downloaded and analysed in
memory. The file is the input for writing a repository's conventions (see the
cpi-discover skill of the Claude Code plugin).`,
		Example: `  cpictl discover --output-file .cpi/discovery.json
  cpictl discover --package-ids SalesOrders,Finance
  cpictl discover --dir ./content   # local repository from 'sync' or 'snapshot', offline`,
		SilenceUsage: true,
		Annotations:  map[string]string{annotationOffline: "dir"},
		RunE: func(cmd *cobra.Command, args []string) error {
			var d *ops.Discovery
			var err error
			if dir := config.GetString(cmd, "dir"); dir != "" {
				d, err = ops.DiscoverDir(cmd.Context(), dir)
			} else {
				d, err = ops.DiscoverTenant(cmd.Context(), tenantExecuter(cmd), config.GetString(cmd, "tmn-host"), ops.DiscoverOptions{
					PackageIDs: config.GetStringSlice(cmd, "package-ids"), MaxIFlows: config.GetInt(cmd, "max-iflows"),
				})
			}
			if err != nil {
				return err
			}
			file := config.GetString(cmd, "output-file")
			if err := ops.WriteDiscovery(d, file); err != nil {
				return err
			}
			output.SetResult(cmd.Context(), map[string]any{"file": file, "source": d.Source, "summary": d.Summary, "errors": d.Errors})
			log.Info().Msgf("Discovered %d package(s) and %d integration flow(s), written to %s", d.Summary.Packages, d.Summary.IFlows, file)
			for _, e := range d.Errors {
				log.Warn().Msg(e)
			}
			return nil
		},
	}
	c.Flags().String("output-file", ".cpi/discovery.json", "JSON file to write")
	c.Flags().StringSlice("package-ids", nil, "Only these packages (default: all)")
	c.Flags().Int("max-iflows", 0, "Stop after this many integration flows (0: all)")
	c.Flags().String("dir", "", "Analyse this local directory instead of the tenant")
	return c
}
