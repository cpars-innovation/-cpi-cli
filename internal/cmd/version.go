package cmd

import (
	"github.com/cpars-innovation/cpicli/internal/config"
	"github.com/cpars-innovation/cpicli/internal/output"
	"github.com/cpars-innovation/cpicli/pkg/ops"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func NewVersionCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "version",
		Short: "Artifact versions in the repository (Bundle-Version)",
		Long: `Versions of artifacts live in the repository: Bundle-Version in each artifact's
META-INF/MANIFEST.MF. With --versioning manifest, upload and deploy use exactly
that version on every tenant (docs/versioning.md).`,
		Annotations: map[string]string{annotationOffline: "true"},
	}
	bump := &cobra.Command{
		Use:   "bump",
		Short: "Raise Bundle-Version of (changed) artifacts",
		Long: `Raise Bundle-Version in META-INF/MANIFEST.MF of the artifacts below --dir
(layout <package>/<artifact>). With --changed only artifacts whose directory
changed since the commit that last set their version: committed, staged,
unstaged and untracked changes count. Artifacts never committed keep their
version (new), artifacts whose version was raised since that commit are left
alone (already_bumped), so running it twice does not bump twice.

Run it before the pull request to the development branch and commit the
manifests with the change. No tenant access.`,
		Example: `  cpictl version bump --changed --dir packages
  cpictl version bump --changed --level minor --package UtilitiesBaseEDM
  cpictl version bump --artifact UtilitiesBase_MDX_to_EDM_Outbound --dry-run`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		Annotations:  map[string]string{annotationOffline: "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			changed, _ := cmd.Flags().GetBool("changed")
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			res, err := ops.BumpVersions(cmd.Context(), ops.BumpOptions{
				Dir: config.GetString(cmd, "dir"), Changed: changed, Level: config.GetString(cmd, "level"),
				Packages: config.GetStringSlice(cmd, "package"), Artifacts: config.GetStringSlice(cmd, "artifact"), DryRun: dryRun,
			})
			if res != nil {
				output.SetResult(cmd.Context(), res)
				for _, it := range res.Items {
					switch it.Status {
					case ops.BumpBumped:
						log.Info().Str("artifact", it.Artifact).Msgf("%-50s %s -> %s", it.Artifact, it.Old, it.New)
					case ops.BumpFailed:
						log.Error().Str("artifact", it.Artifact).Msgf("%-50s %s: %s", it.Artifact, it.Old, it.Error)
					default:
						log.Debug().Str("artifact", it.Artifact).Msgf("%-50s %s (%s)", it.Artifact, it.Old, it.Status)
					}
				}
				suffix := ""
				if dryRun {
					suffix = " (dry run, nothing written)"
				}
				log.Info().Msgf("%d bumped, %d unchanged, %d already bumped, %d new%s", res.Counts[ops.BumpBumped], res.Counts[ops.BumpUnchanged],
					res.Counts[ops.BumpAlreadyBumped], res.Counts[ops.BumpNew], suffix)
			}
			return err
		},
	}
	bump.Flags().String("dir", ".", "Content tree (<package>/<artifact>)")
	bump.Flags().Bool("changed", false, "Only artifacts changed since their version was last set (Git)")
	bump.Flags().String("level", "patch", "patch, minor or major")
	bump.Flags().StringSlice("package", nil, "Only these package folders (names or patterns)")
	bump.Flags().StringSlice("artifact", nil, "Only these artifact IDs (names or patterns)")
	bump.Flags().Bool("dry-run", false, "Show the new versions without writing")
	c.AddCommand(bump)
	return c
}
