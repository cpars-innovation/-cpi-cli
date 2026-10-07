package cmd

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"github.com/cpars-innovation/cpicli/pkg/httpclnt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/cpars-innovation/cpicli/internal/config"
	"github.com/cpars-innovation/cpicli/internal/deploy"
	"github.com/cpars-innovation/cpicli/internal/manifest"
	"github.com/cpars-innovation/cpicli/internal/models"
	"github.com/cpars-innovation/cpicli/internal/output"
	artifactsync "github.com/cpars-innovation/cpicli/internal/sync"
	"github.com/cpars-innovation/cpicli/internal/versioning"
	"github.com/cpars-innovation/cpicli/pkg/cpi"
	"github.com/cpars-innovation/cpicli/pkg/ops"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// OperationMode defines the orchestrator operation mode
type OperationMode string

const (
	ModeUpdateAndDeploy OperationMode = "update-and-deploy"
	ModeUpdateOnly      OperationMode = "update-only"
	ModeDeployOnly      OperationMode = "deploy-only"
)

// ProcessingStats tracks processing statistics
type ProcessingStats struct {
	PackagesUpdated          int `json:"packagesUpdated"`
	PackagesDeployed         int `json:"packagesDeployed"`
	PackagesFailed           int `json:"packagesFailed"`
	PackagesFiltered         int `json:"packagesFiltered"`
	ArtifactsTotal           int `json:"artifactsTotal"`
	ArtifactsDeployedSuccess int `json:"artifactsDeployedSuccess"`
	ArtifactsDeployedFailed  int `json:"artifactsDeployedFailed"`
	ArtifactsFiltered        int `json:"artifactsFiltered"`
	UpdateFailures           int `json:"updateFailures"`
	DeployFailures           int `json:"deployFailures"`
	// ArtifactsChanged were created or updated, ArtifactsUnchanged had the
	// tenant's content already. Existing artifacts were compared with the
	// snapshot state (ComparedWithSnapshot) or downloaded (DownloadedForComparison).
	ArtifactsChanged          int             `json:"artifactsChanged"`
	ArtifactsUnchanged        int             `json:"artifactsUnchanged"`
	ComparedWithSnapshot      int             `json:"comparedWithSnapshot"`
	DownloadedForComparison   int             `json:"downloadedForComparison"`
	SuccessfulPackageUpdates  map[string]bool `json:"successfulPackageUpdates"`
	SuccessfulArtifactUpdates map[string]bool `json:"successfulArtifactUpdates"`
	SuccessfulArtifactDeploys map[string]bool `json:"successfulArtifactDeploys"`
	FailedPackageUpdates      map[string]bool `json:"failedPackageUpdates"`
	FailedArtifactUpdates     map[string]bool `json:"failedArtifactUpdates"`
	FailedArtifactDeploys     map[string]bool `json:"failedArtifactDeploys"`
	// changed and redeploy are keyed by the final artifact ID: content
	// created or updated, and content changed with the running version (to
	// be deployed with force; the runtime was not undeployed).
	changed, redeploy map[string]bool
}

func NewOrchestratorCommand() *cobra.Command {
	var (
		packagesDir         string
		deployConfig        string
		deploymentPrefix    string
		packageFilter       string
		artifactFilter      string
		keepTemp            bool
		configPattern       string
		mergeConfigs        bool
		updateMode          bool
		updateOnlyMode      bool
		deployOnlyMode      bool
		deployRetries       int
		deployDelaySeconds  int
		parallelDeployments int
	)

	orchestratorCmd := &cobra.Command{
		Use:          "orchestrator",
		Short:        "Update and deploy many packages from a local directory tree",
		SilenceUsage: true,
		Long: `Orchestrate the complete deployment lifecycle for SAP CPI artifacts.

This command handles:
  - Updates artifacts in SAP CPI tenant with modified MANIFEST.MF and parameters
  - Deploys artifacts to make them active (in parallel for faster execution)
  - Supports deployment prefixes for multi-environment scenarios
  - Intelligent artifact grouping by type for efficient deployment
  - Filter by specific packages or artifacts
  - Load configs from files, folders, or remote URLs
  - Configure via YAML file for repeatable deployments

Configuration Sources:
  The --deploy-config flag accepts:
  - Single file:      ./001-deploy-config.yml
  - Folder:           ./configs (processes all matching files alphabetically)
  - Remote URL:       https://raw.githubusercontent.com/org/repo/main/config.yml (public, no authentication)

  All flags can also be set in the global config file (--config) under
  the 'orchestrator' key; CLI flags override config file settings.

Operation Modes:
  --update          Update and deploy artifacts (default)
  --update-only     Only update artifacts, don't deploy
  --deploy-only     Only deploy artifacts, don't update

Deployment Strategy:
  1. Update Phase: All packages and artifacts are updated first
  2. Deploy Phase: All artifacts are deployed in parallel
     - Deployments are triggered concurrently per package
     - Status is polled for all deployments simultaneously
     - Configurable parallelism and retry settings

Configuration:
  Settings can be loaded from the global config file (--config) under the
  'orchestrator' section. CLI flags override config file settings.`,
		Example: `  # Update and deploy with config from global cpictl.yaml
  cpictl orchestrator --update

  # Load specific config file
  cpictl orchestrator --config ./my-config.yml --update

  # Override settings via CLI flags
  cpictl orchestrator --config ./my-config.yml \
    --deployment-prefix DEV --parallel-deployments 5`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Determine operation mode
			mode := ModeUpdateAndDeploy
			if updateOnlyMode {
				mode = ModeUpdateOnly
			} else if deployOnlyMode {
				mode = ModeDeployOnly
			}

			// Load from viper config if available (CLI flags override config file)
			if !cmd.Flags().Changed("packages-dir") && viper.IsSet("orchestrator.packagesDir") {
				packagesDir = viper.GetString("orchestrator.packagesDir")
			}
			if !cmd.Flags().Changed("deploy-config") && viper.IsSet("orchestrator.deployConfig") {
				deployConfig = viper.GetString("orchestrator.deployConfig")
			}
			if !cmd.Flags().Changed("deployment-prefix") && viper.IsSet("orchestrator.deploymentPrefix") {
				deploymentPrefix = viper.GetString("orchestrator.deploymentPrefix")
			}
			if !cmd.Flags().Changed("package-filter") && viper.IsSet("orchestrator.packageFilter") {
				packageFilter = viper.GetString("orchestrator.packageFilter")
			}
			if !cmd.Flags().Changed("artifact-filter") && viper.IsSet("orchestrator.artifactFilter") {
				artifactFilter = viper.GetString("orchestrator.artifactFilter")
			}
			if !cmd.Flags().Changed("config-pattern") && viper.IsSet("orchestrator.configPattern") {
				configPattern = viper.GetString("orchestrator.configPattern")
			}
			if !cmd.Flags().Changed("merge-configs") && viper.IsSet("orchestrator.mergeConfigs") {
				mergeConfigs = viper.GetBool("orchestrator.mergeConfigs")
			}
			if !cmd.Flags().Changed("keep-temp") && viper.IsSet("orchestrator.keepTemp") {
				keepTemp = viper.GetBool("orchestrator.keepTemp")
			}
			if !updateMode && !updateOnlyMode && !deployOnlyMode && viper.IsSet("orchestrator.mode") {
				switch viper.GetString("orchestrator.mode") {
				case "update-and-deploy":
					mode = ModeUpdateAndDeploy
				case "update-only":
					mode = ModeUpdateOnly
				case "deploy-only":
					mode = ModeDeployOnly
				}
			}
			if !cmd.Flags().Changed("deploy-retries") && viper.IsSet("orchestrator.deployRetries") {
				deployRetries = viper.GetInt("orchestrator.deployRetries")
			}
			if !cmd.Flags().Changed("deploy-delay") && viper.IsSet("orchestrator.deployDelaySeconds") {
				deployDelaySeconds = viper.GetInt("orchestrator.deployDelaySeconds")
			}
			if !cmd.Flags().Changed("parallel-deployments") && viper.IsSet("orchestrator.parallelDeployments") {
				parallelDeployments = viper.GetInt("orchestrator.parallelDeployments")
			}

			// Validate required parameters
			if deployConfig == "" {
				return output.Usagef("--deploy-config is required (set via CLI flag or in config file under 'orchestrator.deployConfig')")
			}

			// Set defaults for deployment settings
			if deployRetries == 0 {
				deployRetries = 5
			}
			if deployDelaySeconds == 0 {
				deployDelaySeconds = 15
			}
			if parallelDeployments == 0 {
				parallelDeployments = 3
			}

			return runOrchestrator(cmd, mode, packagesDir, deployConfig,
				deploymentPrefix, packageFilter, artifactFilter, keepTemp,
				configPattern, mergeConfigs, deployRetries, deployDelaySeconds, parallelDeployments)
		},
	}

	// Flags
	orchestratorCmd.Flags().StringVarP(&packagesDir, "packages-dir", "d", "", "Directory containing packages (config: orchestrator.packagesDir)")
	orchestratorCmd.Flags().StringVarP(&deployConfig, "deploy-config", "c", "", "Path to deployment config file/folder/URL (config: orchestrator.deployConfig)")
	orchestratorCmd.Flags().StringVarP(&deploymentPrefix, "deployment-prefix", "p", "", "Deployment prefix for package/artifact IDs (config: orchestrator.deploymentPrefix)")
	orchestratorCmd.Flags().StringVar(&packageFilter, "package-filter", "", "Comma-separated list of packages to include (config: orchestrator.packageFilter)")
	orchestratorCmd.Flags().StringVar(&artifactFilter, "artifact-filter", "", "Comma-separated list of artifacts to include (config: orchestrator.artifactFilter)")
	orchestratorCmd.Flags().BoolVar(&keepTemp, "keep-temp", false, "Keep temporary directory after execution (config: orchestrator.keepTemp)")
	orchestratorCmd.Flags().StringVar(&configPattern, "config-pattern", "*.y*ml", "File pattern for config files in folders (config: orchestrator.configPattern)")
	orchestratorCmd.Flags().BoolVar(&mergeConfigs, "merge-configs", false, "Merge multiple configs into single deployment (config: orchestrator.mergeConfigs)")
	orchestratorCmd.Flags().BoolVar(&updateMode, "update", false, "Update and deploy artifacts")
	orchestratorCmd.Flags().BoolVar(&updateOnlyMode, "update-only", false, "Only update artifacts, don't deploy")
	orchestratorCmd.Flags().BoolVar(&deployOnlyMode, "deploy-only", false, "Only deploy artifacts, don't update")
	orchestratorCmd.Flags().IntVar(&deployRetries, "deploy-retries", 0, "Number of retries for deployment status checks (config: orchestrator.deployRetries, default: 5)")
	orchestratorCmd.Flags().IntVar(&deployDelaySeconds, "deploy-delay", 0, "Delay in seconds between deployment status checks (config: orchestrator.deployDelaySeconds, default: 15)")
	addVersioningFlag(orchestratorCmd)
	orchestratorCmd.Flags().String("snapshot-state", "", "Snapshot state of the target tenant (written by snapshot) used instead of downloading artifacts for the comparison (config: orchestrator.snapshotState; default: .cpi/snapshot-state.json in the current directory or above --packages-dir; \"off\": always download)")
	addDeferFlags(orchestratorCmd)
	orchestratorCmd.Flags().Int("parallel", 8, "Artifacts uploaded at the same time, across all packages (config: orchestrator.parallel)")
	orchestratorCmd.Flags().Bool("plan", false, "Only show what would be uploaded and deployed, and why; nothing is written to the tenant")
	orchestratorCmd.Flags().Bool("verify-download", false, "Download every existing artifact for the comparison, even when the snapshot state covers it (config: orchestrator.verifyDownload)")
	orchestratorCmd.Flags().IntVar(&parallelDeployments, "parallel-deployments", 0, "Number of parallel deployments per package (config: orchestrator.parallelDeployments, default: 3)")

	return orchestratorCmd
}

func runOrchestrator(cmd *cobra.Command, mode OperationMode, packagesDir, deployConfigPath,
	deploymentPrefix, packageFilterStr, artifactFilterStr string, keepTemp bool,
	configPattern string, mergeConfigs bool, deployRetries, deployDelaySeconds, parallelDeployments int) error {

	log.Info().Msg("Starting orchestrator")
	versionMode, err := versioningMode(cmd)
	if err != nil {
		return err
	}
	log.Info().Msgf("Deployment Strategy: Two-phase with parallel deployment")
	log.Info().Msgf("  Phase 1: Update all artifacts")
	log.Info().Msgf("  Phase 2: Deploy all artifacts in parallel (max %d concurrent)", parallelDeployments)

	// Validate deployment prefix
	if err := deploy.ValidateDeploymentPrefix(deploymentPrefix); err != nil {
		return output.Usage(err)
	}

	// Parse filters
	packageFilter := parseFilter(packageFilterStr)
	artifactFilter := parseFilter(artifactFilterStr)

	// Initialize stats
	stats := ProcessingStats{
		SuccessfulArtifactUpdates: make(map[string]bool),
		SuccessfulPackageUpdates:  make(map[string]bool),
		SuccessfulArtifactDeploys: make(map[string]bool),
		FailedArtifactUpdates:     make(map[string]bool),
		FailedPackageUpdates:      make(map[string]bool),
		FailedArtifactDeploys:     make(map[string]bool),
		changed:                   make(map[string]bool),
		redeploy:                  make(map[string]bool),
	}

	// Setup config loader
	configLoader := deploy.NewConfigLoader()
	configLoader.Debug = viper.GetBool("debug")
	configLoader.FilePattern = configPattern

	if err := configLoader.DetectSource(deployConfigPath); err != nil {
		return output.Usage(fmt.Errorf("failed to detect config source: %w", err))
	}

	log.Info().Msgf("Loading config from: %s (type: %s)", deployConfigPath, configLoader.Source)
	configFiles, err := configLoader.LoadConfigs()
	if err != nil {
		return output.Usage(fmt.Errorf("failed to load deployment config: %w", err))
	}

	log.Info().Msgf("Loaded %d config file(s)", len(configFiles))

	// Create temporary work directory if needed
	var workDir string
	if mode != ModeDeployOnly {
		tempDir, err := os.MkdirTemp("", "cpictl-orchestrator-*")
		if err != nil {
			return fmt.Errorf("failed to create temp directory: %w", err)
		}
		workDir = tempDir

		if !keepTemp {
			defer os.RemoveAll(tempDir)
		} else {
			log.Info().Msgf("Temporary directory: %s", tempDir)
		}
	}

	log.Info().Msgf("Mode: %s", mode)
	log.Info().Msgf("Packages Directory: %s", packagesDir)

	if len(packageFilter) > 0 {
		log.Info().Msgf("Package filter: %s", strings.Join(packageFilter, ", "))
	}
	if len(artifactFilter) > 0 {
		log.Info().Msgf("Artifact filter: %s", strings.Join(artifactFilter, ", "))
	}

	// Get service details once (shared across all operations). Values from the
	// config file and environment are already bound to the flags.
	serviceDetails := serviceDetails(cmd)
	if serviceDetails.Host == "" {
		return output.Usagef("CPI host (tmn-host) is required but not provided")
	}

	log.Debug().Msg("CPI credentials successfully loaded:")
	log.Debug().Msgf("  Host: %s", serviceDetails.Host)
	if serviceDetails.OauthHost != "" {
		log.Debug().Msgf("  OAuth Host: %s", serviceDetails.OauthHost)
		log.Debug().Msg("  Auth Method: OAuth")
	} else {
		log.Debug().Msg("  Auth Method: Basic Auth")
	}

	parallel := config.GetIntWithFallback(cmd, "parallel", "orchestrator.parallel")
	if parallel < 1 {
		return output.Usagef("--parallel must be at least 1")
	}
	upload := uploadOptions{versionMode: versionMode, slots: make(chan struct{}, parallel), wg: &sync.WaitGroup{}, mu: &sync.Mutex{}}
	if plan, _ := cmd.Flags().GetBool("plan"); plan {
		upload.plan = &planCollector{index: map[string]int{}}
		log.Info().Msg("PLAN: nothing is written to the tenant")
	}
	if mode != ModeDeployOnly {
		if upload.baseline, err = loadBaseline(cmd, packagesDir, serviceDetails.Host); err != nil {
			return err
		}
		upload.verifyDownload = config.GetBoolWithFallback(cmd, "verify-download", "orchestrator.verifyDownload")
	}

	// Collect all deployment tasks (will be executed in phase 2)
	var deploymentTasks []DeploymentTask

	// Process configs
	if mergeConfigs && len(configFiles) > 1 {
		log.Info().Msg("Merging multiple configs into single deployment")

		if deploymentPrefix != "" {
			log.Warn().Msg("Note: --deployment-prefix is ignored when merging configs with their own prefixes")
		}

		mergedConfig, err := deploy.MergeConfigs(configFiles)
		if err != nil {
			return output.Usage(fmt.Errorf("failed to merge configs: %w", err))
		}

		tasks, err := processPackages(mergedConfig, false, mode, packagesDir, workDir,
			packageFilter, artifactFilter, &stats, serviceDetails, upload)
		if err != nil {
			return err
		}
		deploymentTasks = append(deploymentTasks, tasks...)
	} else {
		for _, configFile := range configFiles {
			if len(configFiles) > 1 {
				log.Info().Msgf("Processing Config: %s", configFile.FileName)
			}

			// Override deployment prefix if specified via CLI
			if deploymentPrefix != "" {
				configFile.Config.DeploymentPrefix = deploymentPrefix
			}

			log.Info().Msgf("Deployment Prefix: %s", configFile.Config.DeploymentPrefix)

			tasks, err := processPackages(configFile.Config, true, mode, packagesDir, workDir,
				packageFilter, artifactFilter, &stats, serviceDetails, upload)
			if err != nil {
				log.Error().Msgf("Failed to process config %s: %v", configFile.FileName, err)
				continue
			}
			deploymentTasks = append(deploymentTasks, tasks...)
		}
	}

	if deferDeploy, _ := cmd.Flags().GetBool("defer-deploy"); deferDeploy && upload.plan == nil && mode != ModeUpdateOnly {
		printSummary(&stats)
		output.SetResult(cmd.Context(), orchestratorResult{Mode: string(mode), Stats: &stats, Deployments: []ops.Result{}})
		err := deferDeployments(pendingFile(cmd), serviceDetails.Host, deploymentTasks, func(t DeploymentTask) string {
			switch {
			case stats.redeploy[t.ArtifactID]:
				return "orchestrator: content changed, same version as running"
			case stats.changed[t.ArtifactID]:
				return "orchestrator: content changed"
			}
			return "orchestrator: deploy if the runtime lacks the designtime version"
		})
		if err != nil {
			return err
		}
		return orchestratorErr(&stats)
	}

	if upload.plan != nil {
		if mode != ModeUpdateOnly {
			planDeployments(deploymentTasks, upload.plan, serviceDetails)
		}
		res := orchestratorResult{Mode: string(mode), Stats: &stats, Deployments: []ops.Result{}, Plan: upload.plan.items}
		output.SetResult(cmd.Context(), res)
		return logPlan(upload.plan.items)
	}

	// Phase 2: Deploy all artifacts in parallel (if not update-only mode)
	deployments := []ops.Result{}
	if mode != ModeUpdateOnly && len(deploymentTasks) > 0 {
		log.Info().Msg("")
		log.Info().Msg("═══════════════════════════════════════════════════════════════════════")
		log.Info().Msg("PHASE 2: DEPLOYING ALL ARTIFACTS IN PARALLEL")
		log.Info().Msg("═══════════════════════════════════════════════════════════════════════")
		log.Info().Msgf("Total artifacts to deploy: %d", len(deploymentTasks))
		log.Info().Msgf("Max concurrent deployments: %d", parallelDeployments)
		log.Info().Msg("")

		deployments = deployAllArtifactsParallel(cmd.Context(), deploymentTasks, parallelDeployments, deployRetries,
			deployDelaySeconds, &stats, serviceDetails)
	}

	// Print summary
	printSummary(&stats)
	output.SetResult(cmd.Context(), orchestratorResult{Mode: string(mode), Stats: &stats, Deployments: deployments})

	return orchestratorErr(&stats)
}

// orchestratorErr is the error of a run with failures (partial when
// anything succeeded).
func orchestratorErr(stats *ProcessingStats) error {
	if stats.PackagesFailed > 0 || stats.UpdateFailures > 0 || stats.DeployFailures > 0 {
		err := fmt.Errorf("deployment completed with failures")
		if len(stats.SuccessfulArtifactUpdates) > 0 || stats.ArtifactsDeployedSuccess > 0 {
			return output.Partial(err)
		}
		return output.Failed(err)
	}
	return nil
}

func processPackages(config *models.DeployConfig, applyPrefix bool, mode OperationMode,
	packagesDir, workDir string, packageFilter, artifactFilter []string,
	stats *ProcessingStats, serviceDetails *cpi.ServiceDetails, upload uploadOptions) ([]DeploymentTask, error) {
	versionMode := upload.versionMode

	var deploymentTasks []DeploymentTask
	var collect []func()

	// Phase 1: Update all packages and artifacts
	if mode != ModeDeployOnly {
		log.Info().Msg("")
		log.Info().Msg("═══════════════════════════════════════════════════════════════════════")
		log.Info().Msg("PHASE 1: UPDATING ALL PACKAGES AND ARTIFACTS")
		log.Info().Msg("═══════════════════════════════════════════════════════════════════════")
		log.Info().Msg("")
	}

	for _, pkg := range config.Packages {
		// Apply package filter
		if !shouldInclude(pkg.ID, packageFilter) {
			log.Debug().Msgf("Skipping package %s (filtered)", pkg.ID)
			stats.PackagesFiltered++
			continue
		}

		if !pkg.Sync && !pkg.Deploy {
			log.Info().Msgf("Skipping package %s (sync=false, deploy=false)", pkg.ID)
			continue
		}

		log.Info().Msgf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		log.Info().Msgf("📦 Package: %s", pkg.ID)

		packageDir := filepath.Join(packagesDir, pkg.PackageDir)
		if !deploy.DirExists(packageDir) {
			log.Warn().Msgf("Package directory not found: %s", packageDir)
			continue
		}

		// Calculate final package ID and name
		finalPackageID := pkg.ID
		finalPackageName := pkg.DisplayName
		if finalPackageName == "" {
			finalPackageName = pkg.ID
		}

		// Apply prefix if needed
		if applyPrefix && config.DeploymentPrefix != "" {
			finalPackageID = config.DeploymentPrefix + "" + pkg.ID
			finalPackageName = config.DeploymentPrefix + " - " + finalPackageName
		}

		log.Info().Msgf("Package ID: %s", finalPackageID)
		log.Info().Msgf("Package Name: %s", finalPackageName)

		// Update package metadata
		if mode != ModeDeployOnly && upload.plan != nil {
			log.Info().Msgf("[PLAN] package %s would be created or updated", finalPackageID)
		} else if mode != ModeDeployOnly {
			err := updatePackage(&pkg, finalPackageID, finalPackageName, workDir, serviceDetails)
			if err != nil {
				log.Error().Msgf("Failed to update package %s: %v", pkg.ID, err)
				stats.FailedPackageUpdates[pkg.ID] = true
				stats.PackagesFailed++
				continue
			}
			stats.SuccessfulPackageUpdates[pkg.ID] = true
			stats.PackagesUpdated++
		}

		// Process artifacts for update
		if pkg.Sync && mode != ModeDeployOnly {
			if err := updateArtifacts(&pkg, packageDir, finalPackageID, finalPackageName,
				config.DeploymentPrefix, workDir, artifactFilter, stats, serviceDetails, upload); err != nil {
				log.Error().Msgf("Failed to update artifacts for package %s: %v", pkg.ID, err)
				upload.mu.Lock()
				stats.UpdateFailures++
				upload.mu.Unlock()
			}
		}

		// Collect deployment tasks (will be executed in phase 2) once the
		// uploads are done
		if pkg.Deploy && mode != ModeUpdateOnly {
			pkg, packageDir, finalPackageID := pkg, packageDir, finalPackageID
			collect = append(collect, func() {
				tasks := collectDeploymentTasks(&pkg, packageDir, finalPackageID, config.DeploymentPrefix,
					artifactFilter, stats, versionMode)
				deploymentTasks = append(deploymentTasks, tasks...)
			})
		}
	}

	upload.wg.Wait()
	for _, c := range collect {
		c()
	}
	return deploymentTasks, nil
}

func updatePackage(pkg *models.Package, finalPackageID, finalPackageName, workDir string,
	serviceDetails *cpi.ServiceDetails) error {

	if serviceDetails == nil {
		return fmt.Errorf("serviceDetails is nil - cannot update package")
	}

	log.Info().Msg("Updating package in tenant...")

	description := pkg.Description
	if description == "" {
		description = finalPackageName
	}

	shortText := pkg.ShortText
	if shortText == "" {
		shortText = finalPackageName
	}

	// Create package JSON
	packageJSON := map[string]any{
		"d": map[string]any{
			"Id":          finalPackageID,
			"Name":        finalPackageName,
			"Description": description,
			"ShortText":   shortText,
		},
	}

	jsonData, err := json.MarshalIndent(packageJSON, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal package JSON: %w", err)
	}

	// Write to temporary file
	packageJSONPath := filepath.Join(workDir, "modified", fmt.Sprintf("package_%s.json", pkg.ID))
	if err := os.MkdirAll(filepath.Dir(packageJSONPath), 0755); err != nil {
		return fmt.Errorf("failed to create package JSON directory: %w", err)
	}

	if err := os.WriteFile(packageJSONPath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write package JSON: %w", err)
	}

	// Use internal sync package update function
	exe := cpi.InitHTTPExecuter(serviceDetails)
	packageSynchroniser := artifactsync.NewSyncer("tenant", "CPIPackage", exe)

	err = packageSynchroniser.Exec(artifactsync.Request{PackageFile: packageJSONPath})
	if err != nil {
		log.Warn().Msgf("Package update warning (may not exist yet): %v", err)
		// Don't return error - package might not exist yet
		return nil
	}

	log.Info().Msg("  ✓ Package metadata updated")
	return nil
}

func updateArtifacts(pkg *models.Package, packageDir, finalPackageID, finalPackageName, prefix, workDir string,
	artifactFilter []string, stats *ProcessingStats, serviceDetails *cpi.ServiceDetails, upload uploadOptions) error {
	versionMode := upload.versionMode

	log.Info().Msg("Updating artifacts...")

	if serviceDetails == nil {
		return fmt.Errorf("serviceDetails is nil - cannot initialize HTTP executer")
	}
	if serviceDetails.Host == "" {
		return fmt.Errorf("serviceDetails.Host is empty - check CPI credentials in config file")
	}

	exe := cpi.InitHTTPExecuter(serviceDetails)
	locked := func(f func()) {
		upload.mu.Lock()
		defer upload.mu.Unlock()
		f()
	}

	for _, artifact := range pkg.Artifacts {
		// Apply artifact filter
		if !shouldInclude(artifact.Id, artifactFilter) {
			log.Debug().Msgf("Skipping artifact %s (filtered)", artifact.Id)
			locked(func() { stats.ArtifactsFiltered++ })
			continue
		}

		if !artifact.Sync {
			log.Debug().Msgf("Skipping artifact %s (sync=false)", artifact.DisplayName)
			continue
		}

		locked(func() { stats.ArtifactsTotal++ })

		artifactDir := filepath.Join(packageDir, artifact.ArtifactDir)
		if !deploy.DirExists(artifactDir) {
			log.Warn().Msgf("Artifact directory not found: %s", artifactDir)
			continue
		}

		// Calculate final artifact ID and name
		finalArtifactID := artifact.Id
		finalArtifactName := artifact.DisplayName
		if finalArtifactName == "" {
			finalArtifactName = artifact.Id
		}

		if prefix != "" {
			finalArtifactID = prefix + "_" + artifact.Id
		}

		// Uploads run on the shared pool (--parallel), each in its own work
		// directory with its own synchroniser
		artifact := artifact
		upload.wg.Add(1)
		go func() {
			defer upload.wg.Done()
			upload.slots <- struct{}{}
			defer func() { <-upload.slots }()
			jobDir := filepath.Join(workDir, "jobs", finalArtifactID)
			uploadOneArtifact(exe, pkg, &artifact, artifactDir, jobDir, finalPackageID, finalArtifactID, finalArtifactName,
				versionMode, stats, upload, locked)
		}()
	}
	return nil
}

// uploadOneArtifact prepares the artifact (final ID and name in the
// manifest, configOverrides in parameters.prop) in jobDir and uploads it.
func uploadOneArtifact(exe *httpclnt.HTTPExecuter, pkg *models.Package, artifact *models.Artifact, artifactDir, jobDir,
	finalPackageID, finalArtifactID, finalArtifactName string, versionMode versioning.Mode,
	stats *ProcessingStats, upload uploadOptions, locked func(func())) {

	log.Info().Msgf("  Updating: %s", finalArtifactID)
	fail := func(err error) {
		log.Error().Msgf("Update failed for %s: %v", finalArtifactName, err)
		locked(func() {
			stats.UpdateFailures++
			stats.FailedArtifactUpdates[artifact.Id] = true
		})
	}

	// Map artifact type for synchroniser (uses simple type names)
	artifactType := mapArtifactTypeForSync(artifact.Type)

	// Create temp directory for this artifact
	tempArtifactDir := filepath.Join(jobDir, "artifact")
	if err := deploy.CopyDir(artifactDir, tempArtifactDir); err != nil {
		log.Error().Msgf("Failed to copy artifact to temp: %v", err)
		locked(func() { stats.FailedArtifactUpdates[artifact.Id] = true })
		return
	}

	// Update MANIFEST.MF
	manifestPath := filepath.Join(tempArtifactDir, "META-INF", "MANIFEST.MF")
	modifiedManifestPath := filepath.Join(jobDir, "modified", "META-INF", "MANIFEST.MF")

	if deploy.FileExists(manifestPath) {
		if err := deploy.UpdateManifestBundleName(manifestPath, finalArtifactID, finalArtifactName, modifiedManifestPath); err != nil {
			log.Warn().Msgf("Failed to update MANIFEST.MF: %v", err)
		}
	}

	// Handle parameters.prop
	var modifiedParamsPath string
	paramsPath := deploy.FindParametersFile(tempArtifactDir)

	if paramsPath != "" && deploy.FileExists(paramsPath) {
		modifiedParamsPath = filepath.Join(jobDir, "modified", "parameters.prop")

		if len(artifact.ConfigOverrides) > 0 {
			if err := deploy.MergeParametersFile(paramsPath, artifact.ConfigOverrides, modifiedParamsPath); err != nil {
				log.Warn().Msgf("Failed to merge parameters: %v", err)
			} else {
				log.Debug().Msgf("Applied %d config overrides", len(artifact.ConfigOverrides))
			}
		} else {
			// No overrides, copy to modified location
			data, err := os.ReadFile(paramsPath)
			if err == nil {
				os.MkdirAll(filepath.Dir(modifiedParamsPath), 0755)
				os.WriteFile(modifiedParamsPath, data, 0644)
			}
		}
	}

	// Copy modified manifest to temp artifact dir for sync
	if deploy.FileExists(modifiedManifestPath) {
		targetManifestPath := filepath.Join(tempArtifactDir, "META-INF", "MANIFEST.MF")
		data, err := os.ReadFile(modifiedManifestPath)
		if err == nil {
			os.WriteFile(targetManifestPath, data, 0644)
		}
	}

	// Copy modified parameters if exists
	if modifiedParamsPath != "" && deploy.FileExists(modifiedParamsPath) {
		// Find the actual parameters location in the artifact
		actualParamsPath := deploy.FindParametersFile(tempArtifactDir)
		data, err := os.ReadFile(modifiedParamsPath)
		if err == nil {
			os.WriteFile(actualParamsPath, data, 0644)
		}
	}

	log.Debug().Msgf("Updating %s (type %s) in package %s", finalArtifactID, artifactType, finalPackageID)

	mode, err := resolveVersioning(artifact.Versioning, pkg.Versioning, versionMode)
	if err != nil {
		fail(err)
		return
	}
	synchroniser := artifactsync.New(exe)
	synchroniser.Baseline, synchroniser.VerifyDownload = upload.baseline, upload.verifyDownload
	synchroniser.DryRun = upload.plan != nil
	// a content change with the running version is deployed with force in
	// phase 2 (or by deploy --pending) instead of undeploying it now
	synchroniser.KeepRuntime = true
	synchroniser.Versioning = mode
	outcome, err := synchroniser.UploadArtifact(finalArtifactID, finalArtifactName, artifactType,
		finalPackageID, tempArtifactDir, jobDir, "", nil)

	locked(func() {
		switch outcome.Compared {
		case "snapshot":
			stats.ComparedWithSnapshot++
		case "download":
			stats.DownloadedForComparison++
		}
		if upload.plan != nil {
			upload.plan.upsert(finalArtifactID, func(it *PlanItem) {
				it.Package, it.Type = finalPackageID, artifactType
				it.Upload = map[string]string{"CREATED": "create", "UPDATED": "update", "UNCHANGED": "unchanged"}[outcome.Action]
				it.Compared, it.Designtime, it.VersionRule = outcome.Compared, outcome.Version, outcome.VersionRule
				if err != nil {
					it.Upload, it.Error = "fails", err.Error()
				}
			})
		}
		if err != nil {
			return
		}
		if outcome.Action == "UNCHANGED" {
			stats.ArtifactsUnchanged++
		} else {
			stats.ArtifactsChanged++
			stats.changed[finalArtifactID] = true
		}
		if outcome.Redeploy {
			stats.redeploy[finalArtifactID] = true
		}
		stats.SuccessfulArtifactUpdates[finalArtifactID] = true
	})
	if err != nil {
		fail(err)
		return
	}
	log.Info().Msgf("    ✓ %s: %s", finalArtifactID, strings.ToLower(outcome.Action))
}

func collectDeploymentTasks(pkg *models.Package, packageDir, finalPackageID, prefix string,
	artifactFilter []string, stats *ProcessingStats, versionMode versioning.Mode) []DeploymentTask {

	var tasks []DeploymentTask

	for _, artifact := range pkg.Artifacts {
		// Skip if update failed
		if stats.FailedArtifactUpdates[artifact.Id] {
			log.Debug().Msgf("Skipping artifact %s (due to failed update)", artifact.Id)
			continue
		}

		// Apply artifact filter
		if !shouldInclude(artifact.Id, artifactFilter) {
			log.Debug().Msgf("Skipping artifact %s (filtered)", artifact.Id)
			continue
		}

		if !artifact.Deploy {
			log.Debug().Msgf("Skipping artifact %s (deploy=false)", artifact.DisplayName)
			continue
		}

		finalArtifactID := artifact.Id
		if prefix != "" {
			finalArtifactID = prefix + "_" + artifact.Id
		}

		artifactType := artifact.Type
		if artifactType == "" {
			artifactType = "IntegrationFlow"
		}

		mode, err := resolveVersioning(artifact.Versioning, pkg.Versioning, versionMode)
		if err != nil {
			log.Error().Msgf("Skipping deployment of %s: %v", artifact.Id, err)
			stats.ArtifactsDeployedFailed++
			stats.DeployFailures++
			stats.FailedArtifactDeploys[artifact.Id] = true
			continue
		}
		// manifest: every variant of the artifact directory deploys exactly
		// its Bundle-Version
		expected := ""
		if mode == versioning.Manifest {
			v, err := manifest.Version(filepath.Join(packageDir, artifact.ArtifactDir))
			if err != nil || v == "" {
				log.Error().Msgf("Skipping deployment of %s: versioning manifest needs Bundle-Version in %s", finalArtifactID, filepath.Join(packageDir, artifact.ArtifactDir, "META-INF", "MANIFEST.MF"))
				stats.ArtifactsDeployedFailed++
				stats.DeployFailures++
				stats.FailedArtifactDeploys[artifact.Id] = true
				continue
			}
			expected = v
		}
		tasks = append(tasks, DeploymentTask{
			ArtifactID:      finalArtifactID,
			ArtifactType:    artifactType,
			PackageID:       finalPackageID,
			DisplayName:     artifact.DisplayName,
			Force:           stats.redeploy[finalArtifactID],
			Versioning:      mode,
			ExpectedVersion: expected,
		})
	}

	return tasks
}

func deployAllArtifactsParallel(ctx context.Context, tasks []DeploymentTask, maxConcurrent int,
	retries int, delaySeconds int, stats *ProcessingStats, serviceDetails *cpi.ServiceDetails) []ops.Result {

	// Version comparison is kept from the previous implementation: artifacts
	// whose runtime version equals the designtime version are not redeployed
	// (the synchroniser undeploys same-version artifacts whose content changed).
	results := deployTasks(ctx, cpi.InitHTTPExecuter(serviceDetails), tasks, true, retries, delaySeconds, maxConcurrent)

	failedByPackage := make(map[string]int)
	var packageOrder []string
	for _, r := range results {
		if _, seen := failedByPackage[r.PackageID]; !seen {
			packageOrder = append(packageOrder, r.PackageID)
			failedByPackage[r.PackageID] = 0
		}
		if r.Status.Succeeded() {
			stats.ArtifactsDeployedSuccess++
			stats.SuccessfulArtifactDeploys[r.ID] = true
		} else {
			stats.ArtifactsDeployedFailed++
			stats.DeployFailures++
			stats.FailedArtifactDeploys[r.ID] = true
			failedByPackage[r.PackageID]++
		}
	}
	for _, packageID := range packageOrder {
		if failedByPackage[packageID] == 0 {
			stats.PackagesDeployed++
		} else {
			log.Warn().Msgf("⚠ Package %s: %d artifact deployment(s) failed", packageID, failedByPackage[packageID])
			stats.PackagesFailed++
		}
	}
	return results
}

// orchestratorResult is the JSON result of orchestrator.
type orchestratorResult struct {
	Mode        string           `json:"mode"`
	Stats       *ProcessingStats `json:"stats"`
	Deployments []ops.Result     `json:"deployments"`
	// Plan lists per artifact what would be done (--plan).
	Plan []PlanItem `json:"plan,omitempty"`
}

// uploadOptions are the settings of the update phase.
type uploadOptions struct {
	versionMode versioning.Mode
	// baseline is the snapshot state of the target tenant (nil: download
	// every existing artifact for the comparison).
	baseline       *artifactsync.SnapshotState
	verifyDownload bool
	// plan collects what would be done (--plan); nil: do it.
	plan *planCollector
	// slots limits the uploads running at the same time (--parallel), wg
	// waits for them, mu guards the statistics and the plan.
	slots chan struct{}
	wg    *sync.WaitGroup
	mu    *sync.Mutex
}

// PlanItem is what the orchestrator would do with one artifact.
type PlanItem struct {
	Package  string `json:"package,omitempty"`
	Artifact string `json:"artifact"`
	Type     string `json:"type,omitempty"`
	// Upload is create, update, unchanged, fails (see error) or empty (not
	// synchronised).
	Upload   string `json:"upload,omitempty"`
	Compared string `json:"compared,omitempty"`
	// Designtime is the designtime version after the upload.
	Designtime    string `json:"designtime,omitempty"`
	VersionRule   string `json:"versionRule,omitempty"`
	Running       string `json:"running,omitempty"`
	RuntimeStatus string `json:"runtimeStatus,omitempty"`
	Deploy        bool   `json:"deploy"`
	Reason        string `json:"reason,omitempty"`
	Error         string `json:"error,omitempty"`
}

type planCollector struct {
	items []PlanItem
	index map[string]int
}

func (p *planCollector) upsert(id string, f func(*PlanItem)) {
	i, ok := p.index[id]
	if !ok {
		i = len(p.items)
		p.index[id] = i
		p.items = append(p.items, PlanItem{Artifact: id})
	}
	f(&p.items[i])
}

// planDeployments predicts the deployment of each task (read only).
func planDeployments(tasks []DeploymentTask, plan *planCollector, serviceDetails *cpi.ServiceDetails) {
	exe := cpi.InitHTTPExecuter(serviceDetails)
	rt := cpi.NewRuntime(exe)
	for _, t := range tasks {
		plan.upsert(t.ArtifactID, func(it *PlanItem) {
			it.Package = cmp.Or(it.Package, t.PackageID)
			if it.Upload == "fails" {
				it.Reason = "not deployed: the upload fails"
				return
			}
			version := it.Designtime
			if it.Upload == "" { // not synchronised: the tenant's designtime version
				typ := mapArtifactTypeForSync(t.ArtifactType)
				it.Type = cmp.Or(it.Type, typ)
				v, _, exists, err := cpi.NewDesigntimeArtifact(typ, exe).Get(t.ArtifactID, "active")
				if err != nil {
					it.Error = err.Error()
					return
				}
				if !exists {
					it.Error = "designtime artifact does not exist"
					return
				}
				version = v
				it.Designtime = v
			}
			running, err := rt.GetArtifact(t.ArtifactID)
			if err != nil {
				it.Error = err.Error()
				return
			}
			if running != nil {
				it.Running, it.RuntimeStatus = running.Version, running.Status
			}
			changed := it.Upload == "create" || it.Upload == "update"
			var refused bool
			it.Deploy, it.Reason, refused = ops.DeployDecision(running, version, changed, t.Versioning, t.AllowDowngrade)
			if refused {
				it.Error = it.Reason
			} else if t.Force {
				it.Deploy, it.Reason = true, "configuration changed: deployed again"
			} else if t.ExpectedVersion != "" && version != "" && version != t.ExpectedVersion {
				it.Deploy, it.Error = false, fmt.Sprintf("refused: designtime %s is not the repository's Bundle-Version %s", version, t.ExpectedVersion)
			}
		})
	}
}

// logPlan prints the plan and returns an error when anything would fail.
func logPlan(items []PlanItem) error {
	uploads, deploys, failures := 0, 0, 0
	for _, it := range items {
		if it.Upload == "create" || it.Upload == "update" {
			uploads++
		}
		if it.Deploy {
			deploys++
		}
		what := cmp.Or(it.Upload, "-")
		dep := "no deploy"
		if it.Deploy {
			dep = "DEPLOY"
		}
		ev := log.Info()
		if it.Error != "" {
			failures++
			ev = log.Error()
		}
		ev.Msgf("[PLAN] %-40s upload: %-9s %-9s %s%s", it.Artifact, what, dep, it.Reason, map[bool]string{true: " ❌ " + it.Error, false: ""}[it.Error != ""])
	}
	log.Info().Msgf("[PLAN] %d artifact(s): %d upload(s), %d deployment(s), %d failure(s)", len(items), uploads, deploys, failures)
	if failures > 0 {
		return output.Failed(fmt.Errorf("plan: %d artifact(s) would fail", failures))
	}
	return nil
}

// loadBaseline reads the snapshot state used instead of downloads: the
// --snapshot-state file, else .cpi/snapshot-state.json in the current
// directory or a parent of packagesDir. A state of another tenant is not used.
func loadBaseline(cmd *cobra.Command, packagesDir, host string) (*artifactsync.SnapshotState, error) {
	path := config.GetStringWithFallback(cmd, "snapshot-state", "orchestrator.snapshotState")
	if path == "off" {
		return nil, nil
	}
	explicit := path != ""
	if !explicit {
		candidates := []string{filepath.Join(".cpi", "snapshot-state.json")}
		if abs, err := filepath.Abs(packagesDir); err == nil {
			for dir := abs; ; dir = filepath.Dir(dir) {
				candidates = append(candidates, filepath.Join(dir, ".cpi", "snapshot-state.json"))
				if filepath.Dir(dir) == dir {
					break
				}
			}
		}
		for _, c := range candidates {
			if deploy.FileExists(c) {
				path = c
				break
			}
		}
		if path == "" {
			log.Info().Msg("No snapshot state found: existing artifacts are downloaded for the comparison (run snapshot first to skip that)")
			return nil, nil
		}
	}
	if explicit && !deploy.FileExists(path) {
		return nil, output.Usagef("--snapshot-state %s does not exist", path)
	}
	state, err := artifactsync.LoadSnapshotState(path)
	if err != nil {
		return nil, output.Usagef("cannot read the snapshot state %s: %v", path, err)
	}
	if tenant := cpi.TenantID(host); state.Tenant != tenant {
		log.Warn().Msgf("Snapshot state %s is of tenant %q, not %s: existing artifacts are downloaded for the comparison", path, state.Tenant, tenant)
		return nil, nil
	}
	log.Info().Msgf("Comparing with the snapshot state %s (%d artifacts) instead of downloading", path, len(state.Artifacts))
	return state, nil
}

// mapArtifactTypeForSync maps artifact types for synchroniser (NewDesigntimeArtifact)
func mapArtifactTypeForSync(artifactType string) string {
	switch strings.ToLower(artifactType) {
	case "integrationflow", "integration flow", "iflow":
		return "Integration"
	case "valuemapping", "value mapping":
		return "ValueMapping"
	case "messagemapping", "message mapping":
		return "MessageMapping"
	case "scriptcollection", "script collection":
		return "ScriptCollection"
	default:
		// Default to integration flow
		return "Integration"
	}
}

func parseFilter(filterStr string) []string {
	if filterStr == "" {
		return nil
	}
	parts := strings.Split(filterStr, ",")
	var result []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func shouldInclude(id string, filter []string) bool {
	if len(filter) == 0 {
		return true
	}
	return slices.Contains(filter, id)
}

func printSummary(stats *ProcessingStats) {
	log.Info().Msg("")
	log.Info().Msg("═══════════════════════════════════════════════════════════════════════")
	log.Info().Msg("📊 DEPLOYMENT SUMMARY")
	log.Info().Msg("═══════════════════════════════════════════════════════════════════════")
	log.Info().Msgf("Packages Updated:   %d", stats.PackagesUpdated)
	log.Info().Msgf("Packages Deployed:  %d", stats.PackagesDeployed)
	log.Info().Msgf("Packages Failed:    %d", stats.PackagesFailed)
	log.Info().Msgf("Packages Filtered:  %d", stats.PackagesFiltered)
	log.Info().Msg("───────────────────────────────────────────────────────────────────────")
	log.Info().Msgf("Artifacts Total:         %d", stats.ArtifactsTotal)
	log.Info().Msgf("Artifacts Updated:       %d (%d changed, %d unchanged)", len(stats.SuccessfulArtifactUpdates), stats.ArtifactsChanged, stats.ArtifactsUnchanged)
	if stats.ComparedWithSnapshot+stats.DownloadedForComparison > 0 {
		log.Info().Msgf("Compared:                %d with the snapshot, %d downloaded", stats.ComparedWithSnapshot, stats.DownloadedForComparison)
	}
	log.Info().Msgf("Artifacts Deployed OK:   %d", stats.ArtifactsDeployedSuccess)
	log.Info().Msgf("Artifacts Deployed Fail: %d", stats.ArtifactsDeployedFailed)
	log.Info().Msgf("Artifacts Filtered:      %d", stats.ArtifactsFiltered)
	log.Info().Msg("───────────────────────────────────────────────────────────────────────")

	if stats.UpdateFailures > 0 {
		log.Warn().Msgf("⚠ Update Failures: %d", stats.UpdateFailures)
		log.Info().Msg("Failed Artifact Updates:")
		for artifactID := range stats.FailedArtifactUpdates {
			log.Info().Msgf("  - %s", artifactID)
		}
	}

	if stats.DeployFailures > 0 {
		log.Warn().Msgf("⚠ Deploy Failures: %d", stats.DeployFailures)
		log.Info().Msg("Failed Artifact Deployments:")
		for artifactID := range stats.FailedArtifactDeploys {
			log.Info().Msgf("  - %s", artifactID)
		}
	}

	if stats.UpdateFailures == 0 && stats.DeployFailures == 0 {
		log.Info().Msg("✓ All operations completed successfully!")
	}

	log.Info().Msg("═══════════════════════════════════════════════════════════════════════")
}
