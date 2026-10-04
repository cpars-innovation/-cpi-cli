package cmd

import (
	"fmt"
	"github.com/cpars-innovation/-cpi-cli/internal/api"
	"github.com/cpars-innovation/-cpi-cli/internal/output"
	"github.com/cpars-innovation/-cpi-cli/internal/repo"
	"github.com/cpars-innovation/-cpi-cli/internal/str"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"strings"
)

func NewPDDeployCommand() *cobra.Command {

	pdDeployCmd := &cobra.Command{
		Use:   "pd-deploy",
		Short: "Deploy partner directory parameters to SAP CPI",
		Long: `Upload all partner directory parameters from local files to SAP CPI.

This command reads partner directory parameters from a local directory structure
and uploads them to the SAP CPI Partner Directory:

  {PID}/
    String.properties    - String parameters as key=value pairs
    Binary/              - Binary parameters as individual files
      {ParamId}.{ext}    - Binary parameter files
      _metadata.json     - Content type metadata

The deploy operation supports several modes:
  - Replace mode (default): Updates existing parameters with local values
  - Add-only mode: Only creates new parameters, skips existing ones
  - Full sync mode: Deletes remote parameters not present locally (local is source of truth)

Authentication is performed using OAuth 2.0 client credentials flow or Basic Auth.`,
		Example: `  # Deploy with OAuth (environment variables)
  export FLASHPIPE_TMN_HOST="your-tenant.hana.ondemand.com"
  export FLASHPIPE_OAUTH_HOST="your-tenant.authentication.eu10.hana.ondemand.com"
  export FLASHPIPE_OAUTH_CLIENTID="your-client-id"
  export FLASHPIPE_OAUTH_CLIENTSECRET="your-client-secret"
  flashpipe pd-deploy

  # Deploy with explicit credentials and custom path
  flashpipe pd-deploy \
    --tmn-host "your-tenant.hana.ondemand.com" \
    --oauth-host "your-tenant.authentication.eu10.hana.ondemand.com" \
    --oauth-clientid "your-client-id" \
    --oauth-clientsecret "your-client-secret" \
    --resources-path "./partner-directory"

  # Deploy in add-only mode (don't update existing parameters)
  flashpipe pd-deploy --replace=false

  # Deploy with full sync (delete remote parameters not in local)
  flashpipe pd-deploy --full-sync

  # Deploy only specific PIDs
  flashpipe pd-deploy --pids "SAP_SYSTEM_001,CUSTOMER_API"

  # Dry run to see what would be changed
  flashpipe pd-deploy --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			if err = runPDDeploy(cmd); err != nil {
				cmd.SilenceUsage = true
			}
			return
		},
	}

	// Define flags
	// Note: These can be set in config file under 'pd-deploy' key
	pdDeployCmd.Flags().String("resources-path", "./partner-directory",
		"Path to partner directory parameters")
	pdDeployCmd.Flags().Bool("replace", true,
		"Replace existing values (false = add only missing values)")
	pdDeployCmd.Flags().Bool("full-sync", false,
		"Delete remote parameters not present locally (local is source of truth)")
	pdDeployCmd.Flags().Bool("dry-run", false,
		"Show what would be changed without making changes")
	pdDeployCmd.Flags().StringSlice("pids", nil,
		"Comma separated list of Partner IDs to deploy (e.g., 'PID1,PID2')")

	return pdDeployCmd
}

func runPDDeploy(cmd *cobra.Command) error {
	serviceDetails := api.GetServiceDetails(cmd)

	log.Info().Msg("Executing Partner Directory Deploy command")

	// Support reading from config file under 'pd-deploy' key
	resourcesPath := getConfigStringWithFallback(cmd, "resources-path", "pd-deploy.resources-path")
	replace := getConfigBoolWithFallback(cmd, "replace", "pd-deploy.replace")
	fullSync := getConfigBoolWithFallback(cmd, "full-sync", "pd-deploy.full-sync")
	dryRun := getConfigBoolWithFallback(cmd, "dry-run", "pd-deploy.dry-run")
	pids := getConfigStringSliceWithFallback(cmd, "pids", "pd-deploy.pids")

	log.Info().Msgf("Resources Path: %s", resourcesPath)
	log.Info().Msgf("Replace Mode: %v", replace)
	log.Info().Msgf("Full Sync Mode: %v", fullSync)
	log.Info().Msgf("Dry Run: %v", dryRun)
	if len(pids) > 0 {
		log.Info().Msgf("Filter PIDs: %v", pids)
	}

	// Initialise HTTP executer
	exe := api.InitHTTPExecuter(serviceDetails)

	// Initialise Partner Directory API
	pdAPI := api.NewPartnerDirectory(exe)

	// Initialise Partner Directory Repository
	pdRepo := repo.NewPartnerDirectory(resourcesPath)

	// Trim PIDs
	pids = str.TrimSlice(pids)

	// Execute deploy
	summary, err := deployPartnerDirectory(pdAPI, pdRepo, replace, fullSync, dryRun, pids)
	if summary != nil {
		output.SetResult(cmd.Context(), summary)
	}
	if err != nil {
		return err
	}

	log.Info().Msg("🏆 Partner Directory Deploy completed successfully")
	return nil
}

// pdDeploySummary is the JSON result of pd-deploy.
type pdDeploySummary struct {
	DryRun  bool             `json:"dryRun"`
	String  *api.BatchResult `json:"string"`
	Binary  *api.BatchResult `json:"binary"`
	Deleted *api.BatchResult `json:"deleted,omitempty"`
}

func deployPartnerDirectory(pdAPI *api.PartnerDirectory, pdRepo *repo.PartnerDirectory, replace bool, fullSync bool, dryRun bool, pidsFilter []string) (*pdDeploySummary, error) {
	log.Info().Msg("Starting Partner Directory Deploy...")

	// Get locally managed PIDs
	managedPIDs, err := pdRepo.GetLocalPIDs()
	if err != nil {
		return nil, fmt.Errorf("failed to get local PIDs: %w", err)
	}

	// Filter managed PIDs if filter is specified
	if len(pidsFilter) > 0 {
		filteredPIDs := filterPIDs(managedPIDs, pidsFilter)
		if len(filteredPIDs) == 0 {
			return nil, output.Usagef("no PIDs match the filter: %v", pidsFilter)
		}
		managedPIDs = filteredPIDs
		log.Info().Msgf("Filtered to %d PIDs: %v", len(managedPIDs), managedPIDs)
	}

	if fullSync && len(managedPIDs) > 0 {
		log.Warn().Msg("Full sync will delete remote parameters not in local files!")
		log.Warn().Msgf("Managed PIDs (only these will be affected):\n  - %s",
			strings.Join(managedPIDs, "\n  - "))
		log.Warn().Msg("Parameters in other PIDs will NOT be touched.")

		if dryRun {
			log.Info().Msg("DRY RUN MODE: No deletions will be performed")
		}
	}

	// Push string parameters
	stringResults, err := deployStringParameters(pdAPI, pdRepo, replace, dryRun, pidsFilter)
	if err != nil {
		return nil, fmt.Errorf("failed to deploy string parameters: %w", err)
	}

	// Push binary parameters
	binaryResults, err := deployBinaryParameters(pdAPI, pdRepo, replace, dryRun, pidsFilter)
	if err != nil {
		return nil, fmt.Errorf("failed to deploy binary parameters: %w", err)
	}

	// Collected failures make the command exit non-zero (partial failure)
	var failures []string

	// Full sync - delete remote entries not in local (only for managed PIDs)
	var deletionResults *api.BatchResult
	if fullSync && !dryRun {
		log.Info().Msg("Executing full sync - deleting remote entries not present locally...")
		var deletionErr error
		deletionResults, deletionErr = deleteRemoteEntriesNotInLocal(pdAPI, pdRepo, managedPIDs)
		if deletionErr != nil {
			log.Error().Msgf("Error during full sync deletion: %v", deletionErr)
			failures = append(failures, fmt.Sprintf("full sync: %v", deletionErr))
		} else {
			log.Info().Msgf("Parameters Deleted: %d", len(deletionResults.Deleted))
			if len(deletionResults.Deleted) > 0 {
				log.Info().Msg("Deleted parameters:")
				for _, deleted := range deletionResults.Deleted {
					log.Info().Msgf("  - %s", deleted)
				}
			}
			if len(deletionResults.Errors) > 0 {
				log.Info().Msgf("Deletion Errors: %d", len(deletionResults.Errors))
				for _, err := range deletionResults.Errors {
					log.Warn().Msg(err)
				}
				failures = append(failures, deletionResults.Errors...)
			}
		}
	} else if fullSync && dryRun {
		log.Info().Msg("DRY RUN: Would execute full sync deletion")
		log.Warn().Msgf("Would delete remote parameters not in local for PIDs:):\n  - %s",
			strings.Join(managedPIDs, "\n  - "))
	}

	// Log summary
	log.Info().Msgf("String Parameters - Created: %d, Updated: %d, Unchanged: %d, Errors: %d",
		len(stringResults.Created), len(stringResults.Updated), len(stringResults.Unchanged), len(stringResults.Errors))
	log.Info().Msgf("Binary Parameters - Created: %d, Updated: %d, Unchanged: %d, Errors: %d",
		len(binaryResults.Created), len(binaryResults.Updated), len(binaryResults.Unchanged), len(binaryResults.Errors))

	if fullSync && deletionResults != nil {
		log.Info().Msgf("Full Sync - Deleted: %d, Errors: %d",
			len(deletionResults.Deleted), len(deletionResults.Errors))
		if len(deletionResults.Deleted) > 0 {
			log.Info().Msgf("Deleted: %s", strings.Join(deletionResults.Deleted, ", "))
		}
	}

	if len(stringResults.Errors) > 0 || len(binaryResults.Errors) > 0 {
		log.Warn().Msg("Errors encountered during deploy:")
		for _, err := range stringResults.Errors {
			log.Warn().Msgf("String: %s", err)
		}
		for _, err := range binaryResults.Errors {
			log.Warn().Msgf("Binary: %s", err)
		}
	}

	if dryRun {
		log.Info().Msg("DRY RUN completed - no changes were made!")
	}

	for _, e := range stringResults.Errors {
		failures = append(failures, "string: "+e)
	}
	for _, e := range binaryResults.Errors {
		failures = append(failures, "binary: "+e)
	}
	summary := &pdDeploySummary{DryRun: dryRun, String: stringResults, Binary: binaryResults, Deleted: deletionResults}
	if len(failures) > 0 {
		return summary, output.Partial(fmt.Errorf("partner directory deploy completed with %d error(s): %s",
			len(failures), strings.Join(failures, "; ")))
	}

	return summary, nil
}

func deployStringParameters(pdAPI *api.PartnerDirectory, pdRepo *repo.PartnerDirectory, replace bool, dryRun bool, pidsFilter []string) (*api.BatchResult, error) {
	log.Debug().Msg("Loading string parameters from local files")

	// Get local PIDs
	localPIDs, err := pdRepo.GetLocalPIDs()
	if err != nil {
		return nil, err
	}

	// Filter if needed
	if len(pidsFilter) > 0 {
		localPIDs = filterPIDs(localPIDs, pidsFilter)
	}

	results := &api.BatchResult{
		Created:   []string{},
		Updated:   []string{},
		Unchanged: []string{},
		Errors:    []string{},
	}

	// Load and deploy parameters for each PID
	for _, pid := range localPIDs {
		parameters, err := pdRepo.ReadStringParameters(pid)
		if err != nil {
			results.Errors = append(results.Errors, fmt.Sprintf("Failed to read %s: %v", pid, err))
			continue
		}

		for _, param := range parameters {
			key := fmt.Sprintf("%s/%s", param.Pid, param.ID)

			if dryRun {
				// Just check if it exists and report what would happen
				existing, err := pdAPI.GetStringParameter(param.Pid, param.ID)
				if err != nil {
					results.Errors = append(results.Errors, fmt.Sprintf("%s: %v", key, err))
					continue
				}

				if existing == nil {
					results.Created = append(results.Created, key)
					log.Info().Msgf("[DRY RUN] Would create: %s", key)
				} else if replace && existing.Value != param.Value {
					results.Updated = append(results.Updated, key)
					log.Info().Msgf("[DRY RUN] Would update: %s", key)
				} else {
					results.Unchanged = append(results.Unchanged, key)
				}
				continue
			}

			// Check if parameter exists
			existing, err := pdAPI.GetStringParameter(param.Pid, param.ID)
			if err != nil {
				results.Errors = append(results.Errors, fmt.Sprintf("%s: %v", key, err))
				continue
			}

			if existing == nil {
				// Create new parameter
				if err := pdAPI.CreateStringParameter(param); err != nil {
					results.Errors = append(results.Errors, fmt.Sprintf("%s: %v", key, err))
				} else {
					results.Created = append(results.Created, key)
					log.Debug().Msgf("Created: %s", key)
				}
			} else if replace && existing.Value != param.Value {
				// Update existing parameter
				if err := pdAPI.UpdateStringParameter(param); err != nil {
					results.Errors = append(results.Errors, fmt.Sprintf("%s: %v", key, err))
				} else {
					results.Updated = append(results.Updated, key)
					log.Debug().Msgf("Updated: %s", key)
				}
			} else {
				results.Unchanged = append(results.Unchanged, key)
			}
		}
	}

	return results, nil
}

func deployBinaryParameters(pdAPI *api.PartnerDirectory, pdRepo *repo.PartnerDirectory, replace bool, dryRun bool, pidsFilter []string) (*api.BatchResult, error) {
	log.Debug().Msg("Loading binary parameters from local files")

	// Get local PIDs
	localPIDs, err := pdRepo.GetLocalPIDs()
	if err != nil {
		return nil, err
	}

	// Filter if needed
	if len(pidsFilter) > 0 {
		localPIDs = filterPIDs(localPIDs, pidsFilter)
	}

	results := &api.BatchResult{
		Created:   []string{},
		Updated:   []string{},
		Unchanged: []string{},
		Errors:    []string{},
	}

	// Load and deploy parameters for each PID
	for _, pid := range localPIDs {
		parameters, err := pdRepo.ReadBinaryParameters(pid)
		if err != nil {
			results.Errors = append(results.Errors, fmt.Sprintf("Failed to read %s: %v", pid, err))
			continue
		}

		for _, param := range parameters {
			key := fmt.Sprintf("%s/%s", param.Pid, param.ID)

			if dryRun {
				// Just check if it exists and report what would happen
				existing, err := pdAPI.GetBinaryParameter(param.Pid, param.ID)
				if err != nil {
					results.Errors = append(results.Errors, fmt.Sprintf("%s: %v", key, err))
					continue
				}

				if existing == nil {
					results.Created = append(results.Created, key)
					log.Info().Msgf("[DRY RUN] Would create: %s", key)
				} else if replace && existing.Value != param.Value {
					results.Updated = append(results.Updated, key)
					log.Info().Msgf("[DRY RUN] Would update: %s", key)
				} else {
					results.Unchanged = append(results.Unchanged, key)
				}
				continue
			}

			// Check if parameter exists
			existing, err := pdAPI.GetBinaryParameter(param.Pid, param.ID)
			if err != nil {
				results.Errors = append(results.Errors, fmt.Sprintf("%s: %v", key, err))
				continue
			}

			if existing == nil {
				// Create new parameter
				if err := pdAPI.CreateBinaryParameter(param); err != nil {
					results.Errors = append(results.Errors, fmt.Sprintf("%s: %v", key, err))
				} else {
					results.Created = append(results.Created, key)
					log.Debug().Msgf("Created: %s", key)
				}
			} else if replace && existing.Value != param.Value {
				// Update existing parameter
				if err := pdAPI.UpdateBinaryParameter(param); err != nil {
					results.Errors = append(results.Errors, fmt.Sprintf("%s: %v", key, err))
				} else {
					results.Updated = append(results.Updated, key)
					log.Debug().Msgf("Updated: %s", key)
				}
			} else {
				results.Unchanged = append(results.Unchanged, key)
			}
		}
	}

	return results, nil
}

func deleteRemoteEntriesNotInLocal(pdAPI *api.PartnerDirectory, pdRepo *repo.PartnerDirectory, managedPIDs []string) (*api.BatchResult, error) {
	results := &api.BatchResult{
		Deleted: []string{},
		Errors:  []string{},
	}

	// Load local parameters for managed PIDs. A PID is only eligible for deletion
	// when BOTH its string and binary parameters were read successfully: an
	// unreadable local file must never be interpreted as "no local parameters",
	// which would delete every remote parameter of that PID.
	localStringParams := make(map[string]map[string]bool) // PID -> ID -> exists
	localBinaryParams := make(map[string]map[string]bool)
	var syncablePIDs []string

	for _, pid := range managedPIDs {
		stringParams, err := pdRepo.ReadStringParameters(pid)
		if err != nil {
			msg := fmt.Sprintf("Full sync aborted for PID %s: failed to read local string parameters: %v", pid, err)
			log.Error().Msg(msg)
			results.Errors = append(results.Errors, msg)
			continue
		}
		binaryParams, err := pdRepo.ReadBinaryParameters(pid)
		if err != nil {
			msg := fmt.Sprintf("Full sync aborted for PID %s: failed to read local binary parameters: %v", pid, err)
			log.Error().Msg(msg)
			results.Errors = append(results.Errors, msg)
			continue
		}

		localStringParams[pid] = make(map[string]bool, len(stringParams))
		for _, param := range stringParams {
			localStringParams[pid][param.ID] = true
		}
		localBinaryParams[pid] = make(map[string]bool, len(binaryParams))
		for _, param := range binaryParams {
			localBinaryParams[pid][param.ID] = true
		}
		syncablePIDs = append(syncablePIDs, pid)
	}

	if len(syncablePIDs) == 0 {
		return results, nil
	}

	// Get all remote string parameters
	remoteStringParams, err := pdAPI.GetStringParameters("Pid,Id")
	if err != nil {
		return nil, fmt.Errorf("failed to get remote string parameters: %w", err)
	}

	// Delete string parameters not in local for managed PIDs
	for _, param := range remoteStringParams {
		if !contains(syncablePIDs, param.Pid) {
			continue // Skip PIDs we don't manage or could not read locally
		}

		if !localStringParams[param.Pid][param.ID] {
			key := fmt.Sprintf("%s/%s", param.Pid, param.ID)
			if err := pdAPI.DeleteStringParameter(param.Pid, param.ID); err != nil {
				results.Errors = append(results.Errors, fmt.Sprintf("Failed to delete string %s: %v", key, err))
			} else {
				results.Deleted = append(results.Deleted, key)
				log.Debug().Msgf("Deleted string parameter: %s", key)
			}
		}
	}

	// Get all remote binary parameters
	remoteBinaryParams, err := pdAPI.GetBinaryParameters("Pid,Id")
	if err != nil {
		return nil, fmt.Errorf("failed to get remote binary parameters: %w", err)
	}

	// Delete binary parameters not in local for managed PIDs
	for _, param := range remoteBinaryParams {
		if !contains(syncablePIDs, param.Pid) {
			continue // Skip PIDs we don't manage or could not read locally
		}

		if !localBinaryParams[param.Pid][param.ID] {
			key := fmt.Sprintf("%s/%s", param.Pid, param.ID)
			if err := pdAPI.DeleteBinaryParameter(param.Pid, param.ID); err != nil {
				results.Errors = append(results.Errors, fmt.Sprintf("Failed to delete binary %s: %v", key, err))
			} else {
				results.Deleted = append(results.Deleted, key)
				log.Debug().Msgf("Deleted binary parameter: %s", key)
			}
		}
	}

	return results, nil
}

func filterPIDs(pids []string, filter []string) []string {
	if len(filter) == 0 {
		return pids
	}

	result := make([]string, 0)
	for _, pid := range pids {
		if contains(filter, pid) {
			result = append(result, pid)
		}
	}
	return result
}
