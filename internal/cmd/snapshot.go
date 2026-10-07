package cmd

import (
	"fmt"
	"sort"
	gosync "sync"

	"github.com/cpars-innovation/cpicli/internal/config"
	"github.com/cpars-innovation/cpicli/internal/output"
	"github.com/cpars-innovation/cpicli/internal/repo"
	"github.com/cpars-innovation/cpicli/internal/str"
	"github.com/cpars-innovation/cpicli/internal/sync"
	"github.com/cpars-innovation/cpicli/pkg/cpi"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func NewSnapshotCommand() *cobra.Command {

	snapshotCmd := &cobra.Command{
		Use:          "snapshot",
		Short:        "Save all integration packages of the tenant to a Git repository",
		SilenceUsage: true,
		Long: `Snapshot all editable integration packages from SAP Integration Suite
tenant to a Git repository.

Configuration:
  Settings can be loaded from the global config file (--config) under the
  'snapshot' section. CLI flags override config file settings.`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Validate Draft Handling
			draftHandling := config.GetStringWithFallback(cmd, "draft-handling", "snapshot.draftHandling")
			switch draftHandling {
			case "SKIP", "ADD", "ERROR":
			default:
				return fmt.Errorf("invalid value for --draft-handling = %v", draftHandling)
			}
			// If artifacts directory is provided, validate that is it a subdirectory of Git repo
			gitRepoDir, err := config.GetStringWithEnvExpandAndFallback(cmd, "dir-git-repo", "snapshot.dirGitRepo")
			if err != nil {
				return fmt.Errorf("security alert for --dir-git-repo: %w", err)
			}

			if gitRepoDir != "" {
				artifactsDir, err := config.GetStringWithEnvExpandAndFallback(cmd, "dir-artifacts", "snapshot.dirArtifacts")
				if err != nil {
					return fmt.Errorf("security alert for --dir-artifacts: %w", err)
				}
				gitRepoDirClean := filepath.Clean(gitRepoDir) + string(os.PathSeparator)
				if artifactsDir != "" && !strings.HasPrefix(artifactsDir, gitRepoDirClean) {
					return fmt.Errorf("--dir-artifacts [%v] should be a subdirectory of --dir-git-repo [%v]", artifactsDir, gitRepoDirClean)
				}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			if err = runSnapshot(cmd); err != nil {
				cmd.SilenceUsage = true
			}
			return
		},
	}

	// Define cobra flags, the default value has the lowest (least significant) precedence
	// Note: These can be set in config file under 'snapshot' key
	snapshotCmd.PersistentFlags().String("dir-git-repo", "", "Directory of Git repository (config: snapshot.dirGitRepo)")
	snapshotCmd.PersistentFlags().String("dir-artifacts", "", "Directory containing contents of artifacts (grouped into packages) (config: snapshot.dirArtifacts)")
	snapshotCmd.PersistentFlags().String("dir-work", "/tmp", "Working directory for in-transit files (config: snapshot.dirWork)")
	snapshotCmd.Flags().String("draft-handling", "SKIP", "Handling when artifact is in draft version. Allowed values: SKIP, ADD, ERROR (config: snapshot.draftHandling)")
	snapshotCmd.PersistentFlags().StringSlice("ids-include", nil, "List of included package IDs (config: snapshot.idsInclude)")
	snapshotCmd.PersistentFlags().StringSlice("ids-exclude", nil, "List of excluded package IDs (config: snapshot.idsExclude)")

	snapshotCmd.Flags().String("git-commit-msg", "", "Message used in commit (config: snapshot.gitCommitMsg) (default \"Tenant snapshot of <current time>\")")
	snapshotCmd.Flags().String("git-commit-user", "github-actions[bot]", "User used in commit (config: snapshot.gitCommitUser)")
	snapshotCmd.Flags().String("git-commit-email", "41898282+github-actions[bot]@users.noreply.github.com", "Email used in commit (config: snapshot.gitCommitEmail)")
	snapshotCmd.Flags().Bool("git-skip-commit", false, "Skip committing changes to Git repository (config: snapshot.gitSkipCommit)")
	snapshotCmd.Flags().Bool("sync-package-details", true, "Sync details of Integration Packages (config: snapshot.syncPackageDetails)")
	snapshotCmd.Flags().Bool("incremental", false, "Skip the download of artifacts whose version, ModifiedAt, configured parameters and local copy did not change since the last snapshot (config: snapshot.incremental)")
	snapshotCmd.Flags().Int("parallel", 4, "Packages processed at the same time (config: snapshot.parallel)")
	snapshotCmd.Flags().String("state-file", "", "State of the last snapshot (default: <dir-git-repo>/.cpi/snapshot-state.json, committed with the snapshot) (config: snapshot.stateFile)")

	_ = snapshotCmd.MarkFlagRequired("dir-git-repo")
	snapshotCmd.MarkFlagsMutuallyExclusive("ids-include", "ids-exclude")

	return snapshotCmd
}

func runSnapshot(cmd *cobra.Command) error {
	log.Info().Msg("Executing snapshot command")

	// Support reading from config file under 'snapshot' key
	gitRepoDir, err := config.GetStringWithEnvExpandAndFallback(cmd, "dir-git-repo", "snapshot.dirGitRepo")
	if err != nil {
		return fmt.Errorf("security alert for --dir-git-repo: %w", err)
	}
	artifactsBaseDir := config.GetStringWithFallback(cmd, "dir-artifacts", "snapshot.dirArtifacts")
	if artifactsBaseDir == "" {
		artifactsBaseDir = gitRepoDir
	}
	workDir, err := config.GetStringWithEnvExpandAndFallback(cmd, "dir-work", "snapshot.dirWork")
	if err != nil {
		return fmt.Errorf("security alert for --dir-work: %w", err)
	}
	draftHandling := config.GetStringWithFallback(cmd, "draft-handling", "snapshot.draftHandling")
	includedIds := str.TrimSlice(config.GetStringSliceWithFallback(cmd, "ids-include", "snapshot.idsInclude"))
	excludedIds := str.TrimSlice(config.GetStringSliceWithFallback(cmd, "ids-exclude", "snapshot.idsExclude"))
	commitMsg := config.GetStringWithFallback(cmd, "git-commit-msg", "snapshot.gitCommitMsg")
	if commitMsg == "" {
		commitMsg = "Tenant snapshot of " + time.Now().Format(time.UnixDate)
	}
	commitUser := config.GetStringWithFallback(cmd, "git-commit-user", "snapshot.gitCommitUser")
	commitEmail := config.GetStringWithFallback(cmd, "git-commit-email", "snapshot.gitCommitEmail")
	skipCommit := config.GetBoolWithFallback(cmd, "git-skip-commit", "snapshot.gitSkipCommit")
	syncPackageLevelDetails := config.GetBoolWithFallback(cmd, "sync-package-details", "snapshot.syncPackageDetails")

	incremental := config.GetBoolWithFallback(cmd, "incremental", "snapshot.incremental")
	parallel := config.GetIntWithFallback(cmd, "parallel", "snapshot.parallel")
	stateFile := config.GetStringWithFallback(cmd, "state-file", "snapshot.stateFile")
	if stateFile == "" {
		stateFile = filepath.Join(gitRepoDir, ".cpi", "snapshot-state.json")
	}
	state, err := sync.LoadSnapshotState(stateFile)
	if err != nil {
		return output.Usagef("cannot read the snapshot state %s: %v (delete it for a full snapshot)", stateFile, err)
	}

	serviceDetails := serviceDetails(cmd)
	res, snapErr := getTenantSnapshot(serviceDetails, artifactsBaseDir, workDir, draftHandling, syncPackageLevelDetails, includedIds, excludedIds,
		snapshotOptions{incremental: incremental, parallel: parallel, state: state})
	if res != nil {
		output.SetResult(cmd.Context(), res)
	}
	if snapErr != nil && (res == nil || res.Succeeded == 0) {
		return snapErr
	}
	// what succeeded is kept, also when some packages failed
	if err := state.Save(stateFile); err != nil {
		return err
	}
	if !skipCommit {
		err = repo.CommitToRepo(gitRepoDir, commitMsg, commitUser, commitEmail)
		if err != nil {
			return err
		}
	}
	return snapErr
}

type snapshotOptions struct {
	incremental bool
	parallel    int
	state       *sync.SnapshotState
}

// snapshotResult is the JSON result of snapshot.
type snapshotResult struct {
	Packages  int      `json:"packages"`
	Succeeded int      `json:"succeeded"`
	Skipped   int64    `json:"artifactsSkipped"`
	Failed    []string `json:"failed,omitempty"`
}

func getTenantSnapshot(serviceDetails *cpi.ServiceDetails, artifactsBaseDir string, workDir string, draftHandling string, syncPackageLevelDetails bool,
	includedIds []string, excludedIds []string, opts snapshotOptions) (*snapshotResult, error) {
	log.Info().Msg("---------------------------------------------------------------------------------")
	log.Info().Msg("📢 Begin taking a snapshot of the tenant")

	// Initialise HTTP executer; many parallel reads: retry when the tenant
	// throttles (429) or a gateway fails
	exe := cpi.InitHTTPExecuter(serviceDetails).RetryReads(3, 2*time.Second)

	// Get packages from the tenant - details of all packages are returned in this single call,
	// so no additional call per package is needed
	ip := cpi.NewIntegrationPackage(exe)
	packages, err := ip.GetPackagesData()
	if err != nil {
		return nil, err
	}
	if len(packages) == 0 {
		return nil, fmt.Errorf("No packages found in the tenant")
	}

	if opts.parallel < 1 {
		opts.parallel = 1
	}
	mode := "full"
	if opts.incremental {
		mode = "incremental"
	}
	log.Info().Msgf("Processing %d packages (%s, %d in parallel)", len(packages), mode, opts.parallel)
	synchroniser := sync.New(exe)
	synchroniser.State, synchroniser.Incremental = opts.state, opts.incremental

	res := &snapshotResult{}
	var mu gosync.Mutex
	sem := make(chan struct{}, opts.parallel)
	var wg gosync.WaitGroup
	for i, packageDataFromTenant := range packages {
		id := packageDataFromTenant.Root.Id
		// Filter in/out packages before any call to the tenant
		if str.FilterIDs(id, includedIds, excludedIds) {
			continue
		}
		if packageDataFromTenant.Root.Mode == "READ_ONLY" {
			log.Warn().Msgf("Skipping package %v as it is Configure-only and cannot be downloaded", id)
			continue
		}
		res.Packages++
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			log.Info().Str("package", id).Msgf("Processing package %d/%d - ID: %v", i+1, len(packages), id)
			packageWorkingDir := fmt.Sprintf("%v/%v", workDir, id)
			packageArtifactsDir := fmt.Sprintf("%v/%v", artifactsBaseDir, id)
			err := func() error {
				if syncPackageLevelDetails {
					if err := synchroniser.PackageToGit(packageDataFromTenant, id, packageWorkingDir, packageArtifactsDir); err != nil {
						return err
					}
				}
				return synchroniser.ArtifactsToGit(id, packageWorkingDir, packageArtifactsDir, nil, nil, draftHandling, "ID", nil)
			}()
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				log.Error().Str("package", id).Msgf("❌ Package %v failed: %v", id, err)
				res.Failed = append(res.Failed, fmt.Sprintf("%s: %v", id, err))
				return
			}
			res.Succeeded++
		}()
	}
	wg.Wait()
	res.Skipped = synchroniser.Skipped()
	sort.Strings(res.Failed)

	log.Info().Msg("---------------------------------------------------------------------------------")
	log.Info().Msgf("🏆 Snapshot: %d package(s) done, %d failed, %d artifact(s) unchanged and skipped", res.Succeeded, len(res.Failed), res.Skipped)
	if len(res.Failed) > 0 {
		err := fmt.Errorf("%d of %d package(s) failed: %s", len(res.Failed), res.Packages, strings.Join(res.Failed, "; "))
		if res.Succeeded > 0 {
			return res, output.Partial(err)
		}
		return res, err
	}
	return res, nil
}
