package cmd

import (
	"github.com/cpars-innovation/cpicli/internal/config"
	"github.com/cpars-innovation/cpicli/internal/output"
	"github.com/cpars-innovation/cpicli/internal/repo"
	"github.com/cpars-innovation/cpicli/pkg/cpi"
	"github.com/cpars-innovation/cpicli/pkg/ops"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// NewPDCommand groups the Partner Directory inspection commands (pd-snapshot
// and pd-deploy stay top-level commands).
func NewPDCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "pd",
		Short: "Inspect Partner Directory parameters (get, diff)",
	}

	get := &cobra.Command{
		Use:   "get",
		Short: "Show the Partner Directory parameters of a partner ID on the tenant",
		Example: `  cpictl pd get --pid ONE_OMS
  cpictl pd get --pid ONE_OMS --key now_email --content`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			content, _ := cmd.Flags().GetBool("content")
			res, err := ops.GetPDParameters(cpi.NewPartnerDirectory(tenantExecuter(cmd)), config.GetString(cmd, "pid"),
				config.GetStringSlice(cmd, "key"), content, contentLimit(cmd))
			if err != nil {
				return err
			}
			output.SetResult(cmd.Context(), res)
			for _, p := range res.Strings {
				log.Info().Msgf("string %-30s %s", p.ID, p.Value)
			}
			for _, b := range res.Binaries {
				log.Info().Msgf("binary %-30s %s, %d bytes, sha256 %s", b.ID, b.ContentType, b.Size, b.SHA256)
			}
			for _, k := range res.Missing {
				log.Warn().Msgf("%s not found", k)
			}
			return nil
		},
	}
	get.Flags().String("pid", "", "Partner ID")
	get.Flags().StringSlice("key", nil, "Only these parameter IDs (repeatable)")
	get.Flags().Bool("content", false, "Include binary content")
	addContentFlags(get)
	_ = get.MarkFlagRequired("pid")

	diff := &cobra.Command{
		Use:   "diff",
		Short: "Compare local Partner Directory files with the tenant",
		Long: `Compare the local tree (layout of pd-snapshot) with the tenant: per parameter
create, update, unchanged or remote_only (only on the tenant: pd-deploy --full-sync
would delete it). Exit code 7 when a PID could not be read locally.`,
		Example:      `  cpictl pd diff --resources-path ./partner-directory --pids ONE_OMS`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := ops.PDDiff(cpi.NewPartnerDirectory(tenantExecuter(cmd)), repo.NewPartnerDirectory(config.GetString(cmd, "resources-path")),
				config.GetStringSlice(cmd, "pids"))
			if res != nil {
				output.SetResult(cmd.Context(), res)
				for _, it := range res.Items {
					if it.Change != ops.PDUnchanged {
						log.Info().Msgf("%-11s %s %s/%s %s", it.Change, it.Kind, it.Pid, it.ID, it.Detail)
					}
				}
				log.Info().Msgf("create %d, update %d, unchanged %d, remote_only %d", res.Summary[ops.PDCreate], res.Summary[ops.PDUpdate],
					res.Summary[ops.PDUnchanged], res.Summary[ops.PDRemoteOnly])
			}
			return err
		},
	}
	diff.Flags().String("resources-path", "./partner-directory", "Path to partner directory parameters")
	diff.Flags().StringSlice("pids", nil, "Only these partner IDs")

	c.AddCommand(get, diff)
	return c
}
