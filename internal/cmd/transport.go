package cmd

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cpars-innovation/cpicli/internal/output"
	"github.com/cpars-innovation/cpicli/pkg/ops"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func NewTransportCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "transport",
		Short: "Prepare a transport between tiers: dependencies, pre-checks against the target, copy artifact folders",
		Long: `Helpers to move artifacts from one tier to the next. The deployment itself is the
orchestrator (or configure and deploy --pending) run against the target tier.

  deps   what the selected artifacts need: script collections and mappings they
         reference, flows they call (ProcessDirect / JMS), credentials and key
         aliases, Partner Directory parameters
  check  the same against the target tenant (read only): drafts, drift against
         the target's Git content, dependencies and called flows present,
         credentials, Partner Directory parameters, parameter values in the
         target's configure file; exit code 5 when a check fails
  copy   copy the artifact folders into another content tree (a branch or
         repository per tier), replacing them exactly

Artifacts are given by ID or folder name; --with-deps adds the script
collections and mappings they reference.`,
	}
	c.AddCommand(newTransportDepsCommand(), newTransportCheckCommand(), newTransportCopyCommand())
	return c
}

// transportDir is --dir, else packages when it exists, else ".".
func transportDir(cmd *cobra.Command) string {
	if dir, _ := cmd.Flags().GetString("dir"); dir != "" {
		return dir
	}
	if info, err := os.Stat("packages"); err == nil && info.IsDir() {
		return "packages"
	}
	return "."
}

func newTransportDepsCommand() *cobra.Command {
	c := &cobra.Command{
		Use:          "deps <artifact>...",
		Short:        "List what the selected artifacts depend on (local files)",
		Example:      `  cpictl transport deps Orders_In Billing --dir packages --with-deps`,
		Args:         cobra.MinimumNArgs(1),
		SilenceUsage: true,
		Annotations:  map[string]string{annotationOffline: "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			withDeps, _ := cmd.Flags().GetBool("with-deps")
			set, err := ops.ResolveTransport(cmd.Context(), transportDir(cmd), args, withDeps)
			if err != nil {
				return err
			}
			output.SetResult(cmd.Context(), set)
			logTransportSet(set)
			return nil
		},
	}
	c.Flags().String("dir", "", "Content tree (default: packages if it exists, else the current directory)")
	c.Flags().Bool("with-deps", false, "Add the script collections and mappings the artifacts reference")
	return c
}

func newTransportCheckCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "check <artifact>...",
		Short: "Check a transport against the target tenant (read only)",
		Example: `  cpictl transport check Orders_In --target tenant:prod --target-dir git:prod:packages \
    --configure config/prod.yaml --with-deps`,
		Args:         cobra.MinimumNArgs(1),
		SilenceUsage: true,
		Annotations:  map[string]string{annotationOffline: "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			work, err := os.MkdirTemp("", "cpictl-transport-*")
			if err != nil {
				return err
			}
			defer os.RemoveAll(work)
			target, _ := cmd.Flags().GetString("target")
			side, err := compareSide(cmd, target, filepath.Join(work, "unused"))
			if err != nil {
				return err
			}
			if side.Exe == nil {
				return output.Usagef("--target %s: tenant or tenant:<profile>", target)
			}
			o := ops.TransportCheckOptions{}
			o.WithDeps, _ = cmd.Flags().GetBool("with-deps")
			if td, _ := cmd.Flags().GetString("target-dir"); td != "" {
				s, err := compareSide(cmd, td, filepath.Join(work, "target"))
				if err != nil {
					return err
				}
				if s.Dir == "" {
					return output.Usagef("--target-dir %s: a directory or git:<ref>[:<path>]", td)
				}
				o.TargetDir = s.Dir
			}
			if cf, _ := cmd.Flags().GetString("configure"); cf != "" {
				files, err := ops.LoadConfigureFiles(cf)
				if err != nil {
					return output.Usage(err)
				}
				o.Configure = ops.MergeConfigureFiles(files, "")
			}
			res, err := ops.CheckTransport(cmd.Context(), side.Exe, transportDir(cmd), args, o)
			if err != nil {
				return err
			}
			output.SetResult(cmd.Context(), res)
			logTransportSet(res.TransportSet)
			for _, c := range res.Checks {
				ev := log.Info()
				switch c.Status {
				case ops.CheckFail:
					ev = log.Error()
				case ops.CheckWarn:
					ev = log.Warn()
				}
				ev.Str("check", c.Check).Msgf("%-4s %-10s %-24s %s", c.Status, c.Check, c.Artifact, c.Message)
			}
			log.Info().Msgf("Target %s: %d pass, %d warn, %d fail, %d skipped", side.Label,
				res.Summary[ops.CheckPass], res.Summary[ops.CheckWarn], res.Summary[ops.CheckFail], res.Summary[ops.CheckSkip])
			if n := res.Summary[ops.CheckFail]; n > 0 {
				return output.Failed(fmt.Errorf("%d check(s) failed", n))
			}
			return nil
		},
	}
	c.Flags().String("dir", "", "Source content tree (default: packages if it exists, else the current directory)")
	c.Flags().String("target", "tenant", "Target tenant: tenant (the configured one) or tenant:<profile>")
	c.Flags().String("target-dir", "", "The target tier's content in Git (a directory or git:<ref>[:<path>]): reports changes made on the target outside the pipeline")
	c.Flags().String("configure", "", "The target tier's configure file or folder: reports parameters without a value there")
	c.Flags().Bool("with-deps", false, "Add the script collections and mappings the artifacts reference")
	c.Flags().String("repo", ".", "Git repository for git: arguments")
	return c
}

func newTransportCopyCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "copy <artifact>...",
		Short: "Copy artifact folders into another content tree (local files)",
		Example: `  cpictl transport copy Orders_In --from git:dev:packages --to packages --with-deps
  cpictl transport copy Orders_In --from ../repo-dev/packages --to packages --dry-run`,
		Args:         cobra.MinimumNArgs(1),
		SilenceUsage: true,
		Annotations:  map[string]string{annotationOffline: "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			work, err := os.MkdirTemp("", "cpictl-transport-*")
			if err != nil {
				return err
			}
			defer os.RemoveAll(work)
			from, _ := cmd.Flags().GetString("from")
			to, _ := cmd.Flags().GetString("to")
			if from == "" || to == "" {
				return output.Usagef("--from and --to are required")
			}
			src, err := compareSide(cmd, from, filepath.Join(work, "from"))
			if err != nil {
				return err
			}
			if src.Dir == "" {
				return output.Usagef("--from %s: a directory or git:<ref>[:<path>]", from)
			}
			withDeps, _ := cmd.Flags().GetBool("with-deps")
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			res, err := ops.CopyArtifacts(cmd.Context(), src.Dir, to, args, withDeps, dryRun)
			if err != nil {
				return err
			}
			output.SetResult(cmd.Context(), res)
			prefix := map[bool]string{true: "[DRY RUN] ", false: ""}[dryRun]
			for _, a := range res.Artifacts {
				log.Info().Msgf("%s%-9s %s", prefix, a.Action, a.Path)
			}
			return nil
		},
	}
	c.Flags().String("from", "", "Source content tree: a directory or git:<ref>[:<path>]")
	c.Flags().String("to", "", "Target content tree (directory)")
	c.Flags().Bool("with-deps", false, "Also copy the script collections and mappings the artifacts reference")
	c.Flags().Bool("dry-run", false, "Only list what would be copied")
	c.Flags().String("repo", ".", "Git repository for git: arguments")
	return c
}

func logTransportSet(set *ops.TransportSet) {
	for _, a := range set.Artifacts {
		added := ""
		if a.AddedAsDependency {
			added = " (dependency)"
		}
		log.Info().Msgf("artifact   %-24s %-16s %s %s%s", a.ID, a.Type, a.Path, cmp.Or(a.Version, "-"), added)
	}
	for _, d := range set.Dependencies {
		where := ""
		switch {
		case d.InSelection:
			where = " [in the transport]"
		case d.Kind == ops.DepArtifact && !d.Local:
			where = " [not in the repository]"
		}
		log.Info().Msgf("needs %-10s %-30s %s, by %s%s", d.Kind, d.ID, d.Reason, strings.Join(d.By, ", "), where)
	}
}
