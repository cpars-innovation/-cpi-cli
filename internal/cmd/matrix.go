package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/cpars-innovation/cpicli/internal/config"
	"github.com/cpars-innovation/cpicli/internal/output"
	"github.com/cpars-innovation/cpicli/internal/str"
	"github.com/cpars-innovation/cpicli/pkg/ops"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func NewMatrixCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "matrix <tier>...",
		Short: "Version matrix: every artifact's version in Git and on each tier, and what is ready to promote",
		Long: `Show per artifact the Bundle-Version in the content tree (--dir) and on each tier the
designtime version, draft flag, running version and status, and who changed it last
where the tenant reports it. Tiers are given in promotion order:

  tenant                 the configured tenant
  tenant:<profile>       the tenant of a profile (named after the profile)
  <name>=tenant:<profile>

"behind" lists the tiers whose version is lower than the tier before them (or Git):
the next promotions. One pass per tenant (package lists and the runtime list), no
downloads.`,
		Example: `  cpictl matrix tenant:dev tenant:test tenant:prod --dir packages
  cpictl matrix DEV=tenant:dev PROD=tenant:prod --package Orders --differences --output json`,
		Args:         cobra.MinimumNArgs(1),
		SilenceUsage: true,
		Annotations:  map[string]string{annotationOffline: "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			var tiers []ops.MatrixTier
			for _, a := range args {
				name, spec, ok := strings.Cut(a, "=")
				if !ok {
					spec, name = a, ""
				}
				if spec != "tenant" && !strings.HasPrefix(spec, "tenant:") {
					return output.Usagef("tier %s: tenant or tenant:<profile>", a)
				}
				side, err := compareSide(cmd, spec, "")
				if err != nil {
					return err
				}
				if side.Exe == nil {
					return output.Usagef("tier %s: tenant or tenant:<profile>", a)
				}
				if name == "" {
					name = side.Label
				}
				tiers = append(tiers, ops.MatrixTier{Name: name, Exe: side.Exe})
			}
			dir, _ := cmd.Flags().GetString("dir")
			if dir == "" {
				if info, err := os.Stat("packages"); err == nil && info.IsDir() {
					dir = "packages"
				}
			}
			m, err := ops.BuildVersionMatrix(cmd.Context(), tiers, ops.MatrixOptions{Dir: dir,
				Packages:  str.TrimSlice(config.GetStringSlice(cmd, "package")),
				Artifacts: str.TrimSlice(config.GetStringSlice(cmd, "artifact"))})
			if err != nil {
				return err
			}
			if only, _ := cmd.Flags().GetBool("differences"); only {
				rows := m.Rows[:0]
				for _, r := range m.Rows {
					if r.Differs {
						rows = append(rows, r)
					}
				}
				m.Rows = rows
			}
			output.SetResult(cmd.Context(), m)
			logMatrix(m, dir != "")
			for _, e := range m.Errors {
				log.Error().Msg(e)
			}
			if len(m.Errors) > 0 {
				return output.Partial(fmt.Errorf("%d tier(s) could not be read completely", len(m.Errors)))
			}
			return nil
		},
	}
	c.Flags().String("dir", "", "Content tree with the Git versions (default: packages if it exists)")
	c.Flags().StringSlice("package", nil, "Only these packages (IDs or patterns)")
	c.Flags().StringSlice("artifact", nil, "Only these artifacts (IDs or patterns)")
	c.Flags().Bool("differences", false, "Only artifacts whose versions differ")
	return c
}

func logMatrix(m *ops.VersionMatrix, withGit bool) {
	header := fmt.Sprintf("%-40s", "artifact")
	if withGit {
		header += fmt.Sprintf(" %-12s", "git")
	}
	for _, t := range m.Tiers {
		header += fmt.Sprintf(" %-22s", t)
	}
	log.Info().Msg(header + " behind")
	for _, r := range m.Rows {
		line := fmt.Sprintf("%-40s", r.Package+"/"+r.Artifact)
		if withGit {
			line += fmt.Sprintf(" %-12s", orDash(r.Git))
		}
		for _, t := range m.Tiers {
			c, ok := r.Tiers[t]
			cell := "-"
			switch {
			case !ok:
			case c.Draft:
				cell = "draft"
			default:
				cell = c.Designtime
				if c.Running == "" {
					cell += " (not deployed)"
				} else if c.Running != c.Designtime || c.Status != "STARTED" {
					cell += fmt.Sprintf(" (run %s %s)", c.Running, c.Status)
				}
			}
			line += fmt.Sprintf(" %-22s", cell)
		}
		ev := log.Info()
		if r.Differs {
			ev = log.Warn()
		}
		ev.Msg(line + " " + strings.Join(r.Behind, ","))
	}
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
