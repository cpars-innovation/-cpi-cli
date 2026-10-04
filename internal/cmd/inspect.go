package cmd

import (
	"errors"
	"strings"

	"github.com/cpars-innovation/cpicli/internal/config"
	"github.com/cpars-innovation/cpicli/internal/output"
	"github.com/cpars-innovation/cpicli/internal/str"
	"github.com/cpars-innovation/cpicli/pkg/cpi"
	"github.com/cpars-innovation/cpicli/pkg/httpclnt"
	"github.com/cpars-innovation/cpicli/pkg/ops"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func tenantExecuter(cmd *cobra.Command) *httpclnt.HTTPExecuter {
	return cpi.InitHTTPExecuter(serviceDetails(cmd))
}

func NewStatusCommand() *cobra.Command {
	c := &cobra.Command{
		Use:          "status",
		Short:        "Show runtime status of artifacts",
		SilenceUsage: true,
		Example:      `  cpictl status --artifact-ids MyIFlow,MyMapping --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ids := nonEmpty(str.TrimSlice(config.GetStringSlice(cmd, "artifact-ids")))
			if len(ids) == 0 {
				return output.Usagef("required flag \"artifact-ids\" not set")
			}
			statuses := ops.GetRuntimeStatus(tenantExecuter(cmd), ids)
			output.SetResult(cmd.Context(), map[string]any{"artifacts": statuses})
			var errs []error
			for _, s := range statuses {
				switch {
				case s.Err != nil:
					log.Error().Str("artifact", s.ID).Msgf("%s: %v", s.ID, s.Err)
					errs = append(errs, s.Err)
				case !s.Deployed:
					log.Info().Str("artifact", s.ID).Msgf("%s: NOT_DEPLOYED", s.ID)
				default:
					log.Info().Str("artifact", s.ID).Msgf("%s: %s (version %s)%s", s.ID, s.Status, s.Version, errSuffix(s.ErrorInfo))
				}
			}
			return errors.Join(errs...)
		},
	}
	c.Flags().StringSlice("artifact-ids", nil, "Comma separated list of artifact IDs")
	return c
}

func NewPackagesCommand() *cobra.Command {
	return &cobra.Command{
		Use:          "packages",
		Short:        "List integration packages",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			pkgs, err := ops.ListPackages(tenantExecuter(cmd))
			if err != nil {
				return err
			}
			output.SetResult(cmd.Context(), map[string]any{"packages": pkgs})
			for _, p := range pkgs {
				log.Info().Msgf("%s  %s  %s", p.ID, p.Version, p.Name)
			}
			return nil
		},
	}
}

func NewArtifactsCommand() *cobra.Command {
	c := &cobra.Command{
		Use:          "artifacts",
		Short:        "List designtime artifacts of a package",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			packageID := config.GetString(cmd, "package-id")
			if packageID == "" {
				return output.Usagef("required flag \"package-id\" not set")
			}
			arts, err := ops.ListArtifacts(tenantExecuter(cmd), packageID)
			if err != nil {
				return err
			}
			output.SetResult(cmd.Context(), map[string]any{"packageId": packageID, "artifacts": arts})
			for _, a := range arts {
				log.Info().Msgf("%s  %s  %s  %s", a.ID, a.Type, a.Version, a.Name)
			}
			return nil
		},
	}
	c.Flags().String("package-id", "", "Integration package ID")
	return c
}

func NewParamsCommand() *cobra.Command {
	params := &cobra.Command{
		Use:   "params",
		Short: "Read or change configuration parameters of an integration flow",
	}

	get := &cobra.Command{
		Use:          "get",
		Short:        "Show configuration parameters",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			id := config.GetString(cmd, "artifact-id")
			if id == "" {
				return output.Usagef("required flag \"artifact-id\" not set")
			}
			ps, err := ops.GetConfiguration(tenantExecuter(cmd), id, config.GetString(cmd, "version"))
			if err != nil {
				return err
			}
			output.SetResult(cmd.Context(), map[string]any{"artifactId": id, "parameters": ps})
			for _, p := range ps {
				log.Info().Msgf("%s = %s", p.Key, p.Value)
			}
			return nil
		},
	}
	get.Flags().String("artifact-id", "", "Integration flow ID")
	get.Flags().String("version", "active", "Designtime version")

	set := &cobra.Command{
		Use:          "set",
		Short:        "Set configuration parameters (deploy afterwards to activate)",
		SilenceUsage: true,
		Example:      `  cpictl params set --artifact-id MyIFlow --param Host=example.com --param Port=443`,
		RunE: func(cmd *cobra.Command, args []string) error {
			id := config.GetString(cmd, "artifact-id")
			if id == "" {
				return output.Usagef("required flag \"artifact-id\" not set")
			}
			values, err := parseKeyValues(paramFlag(cmd))
			if err != nil {
				return err
			}
			res, err := ops.UpdateConfiguration(tenantExecuter(cmd), id, config.GetString(cmd, "version"), values, config.GetBool(cmd, "dry-run"))
			if res != nil {
				output.SetResult(cmd.Context(), res)
				log.Info().Msgf("%s: updated %v, unchanged %v", id, res.Updated, res.Unchanged)
			}
			return err
		},
	}
	set.Flags().String("artifact-id", "", "Integration flow ID")
	set.Flags().String("version", "active", "Designtime version")
	set.Flags().StringArray("param", nil, "Parameter as key=value (repeatable)")
	set.Flags().Bool("dry-run", false, "Show what would change without writing")

	params.AddCommand(get, set)
	return params
}

func parseKeyValues(pairs []string) (map[string]string, error) {
	if len(pairs) == 0 {
		return nil, output.Usagef("at least one --param key=value is required")
	}
	values := make(map[string]string, len(pairs))
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		if !ok || strings.TrimSpace(k) == "" {
			return nil, output.Usagef("invalid --param %q, expected key=value", p)
		}
		values[strings.TrimSpace(k)] = v
	}
	return values, nil
}

func paramFlag(cmd *cobra.Command) []string {
	v, _ := cmd.Flags().GetStringArray("param")
	return v
}

func nonEmpty(in []string) []string {
	out := in[:0:0]
	for _, s := range in {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
