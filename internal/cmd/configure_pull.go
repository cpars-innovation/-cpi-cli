package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cpars-innovation/cpicli/internal/api"
	"github.com/cpars-innovation/cpicli/internal/models"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func NewConfigurePullCommand() *cobra.Command {
	var outputDir string
	var packageIDs []string

	cmd := &cobra.Command{
		Use:          "pull",
		Short:        "Pull artifact parameters into configuration YAML files",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputDir = getConfigStringWithFallback(cmd, "output-dir", "configure.pull.outputDir")
			packageIDs = getConfigStringSliceWithFallback(cmd, "package-ids", "configure.pull.packageIds")
			return runConfigurePull(cmd, outputDir, packageIDs)
		},
	}
	cmd.Flags().StringVarP(&outputDir, "output-dir", "o", ".", "Directory for one YAML file per package")
	cmd.Flags().StringSliceVar(&packageIDs, "package-ids", nil, "Package IDs to pull (default: all packages)")
	return cmd
}

func runConfigurePull(cmd *cobra.Command, outputDir string, packageIDs []string) error {
	exe := api.InitHTTPExecuter(api.GetServiceDetails(cmd))
	packages := api.NewIntegrationPackage(exe)
	configuration := api.NewConfiguration(exe)

	availablePackageIDs, err := packages.GetPackagesList()
	if err != nil {
		return err
	}
	if len(packageIDs) == 0 {
		packageIDs = availablePackageIDs
	} else {
		var missing []string
		packageIDs, missing = filterExistingPackages(packageIDs, availablePackageIDs)
		for _, packageID := range missing {
			log.Warn().Msgf("Skipping package %s because it does not exist", packageID)
		}
	}
	sort.Strings(packageIDs)

	type output struct {
		path string
		data []byte
	}
	outputs := make([]output, 0, len(packageIDs))
	for _, packageID := range packageIDs {
		if packageID == "" || filepath.Base(packageID) != packageID || strings.ContainsAny(packageID, `<>:"/\|?*`) {
			return fmt.Errorf("invalid package ID %q", packageID)
		}
		artifacts, err := packages.GetArtifactsData(packageID, "Integration")
		if err != nil {
			return fmt.Errorf("get artifacts for package %s: %w", packageID, err)
		}
		cfg, err := pulledConfigureConfig(packageID, artifacts, configuration.Get)
		if err != nil {
			return err
		}
		data, err := yaml.Marshal(cfg)
		if err != nil {
			return err
		}
		outputs = append(outputs, output{filepath.Join(outputDir, packageID+".yml"), data})
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}
	for _, output := range outputs {
		if err := os.WriteFile(output.path, output.data, 0644); err != nil {
			return err
		}
		log.Info().Msgf("Configuration written to %s", output.path)
	}
	return nil
}

func filterExistingPackages(requested, available []string) (existing, missing []string) {
	availableSet := make(map[string]bool, len(available))
	for _, packageID := range available {
		availableSet[packageID] = true
	}
	for _, packageID := range requested {
		if availableSet[packageID] {
			existing = append(existing, packageID)
		} else {
			missing = append(missing, packageID)
		}
	}
	return
}

func pulledConfigureConfig(packageID string, artifacts []*api.ArtifactDetails, getConfiguration func(string, string) (*api.ParametersData, error)) (*models.ConfigureConfig, error) {
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].Id < artifacts[j].Id })
	pkg := models.ConfigurePackage{ID: packageID}
	for _, artifact := range artifacts {
		parameters, err := getConfiguration(artifact.Id, "active")
		if err != nil {
			return nil, fmt.Errorf("get configuration for artifact %s: %w", artifact.Id, err)
		}
		sort.Slice(parameters.Root.Results, func(i, j int) bool {
			return parameters.Root.Results[i].ParameterKey < parameters.Root.Results[j].ParameterKey
		})
		pulled := models.ConfigureArtifact{ID: artifact.Id, Type: "Integration"}
		for _, parameter := range parameters.Root.Results {
			pulled.Parameters = append(pulled.Parameters, models.ConfigurationParameter{
				Key: parameter.ParameterKey, Value: parameter.ParameterValue,
			})
		}
		pkg.Artifacts = append(pkg.Artifacts, pulled)
	}
	return &models.ConfigureConfig{Packages: []models.ConfigurePackage{pkg}}, nil
}
