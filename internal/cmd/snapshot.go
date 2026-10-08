package cmd

import (
	"cmp"
	"fmt"
	"github.com/cpars-innovation/cpicli/internal/deploy"
	"github.com/spf13/viper"
	"slices"
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
	snapshotCmd.Flags().Int("parallel", 8, "Artifacts downloaded at the same time, across all packages (config: snapshot.parallel)")
	snapshotCmd.Flags().Bool("dry-run", false, "Only report per artifact what the snapshot would do (new, changed, deleted, unchanged, local-modified, derived); writes no files and no state")
	snapshotCmd.Flags().Bool("overwrite-local", false, "Overwrite artifacts with local edits since the last snapshot (default: skip them as local-modified)")
	snapshotCmd.Flags().Bool("fail-on-local-modified", false, "Exit with code 5 when an artifact is local-modified (for CI)")
	snapshotCmd.Flags().Bool("prune", false, "Remove local artifact folders of artifacts and packages deleted on the tenant, and local folders of derived copies (never when edited locally)")
	snapshotCmd.Flags().String("deploy-config", "", "Deploy config file or folder of the orchestrator: its deployment copies are not written (config: snapshot.deployConfig, else orchestrator.deployConfig)")
	snapshotCmd.Flags().StringSlice("deployment-prefix", nil, "Additional deployment prefixes whose copies are not written (the deploy config's deploymentPrefix always counts)")
	snapshotCmd.Flags().Bool("include-derived", false, "Write deployment copies like any other artifact (ignore the deploy config)")
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

	snap := &sync.SnapshotOptions{
		DryRun:         config.GetBoolWithFallback(cmd, "dry-run", "snapshot.dryRun"),
		OverwriteLocal: config.GetBoolWithFallback(cmd, "overwrite-local", "snapshot.overwriteLocal"),
		Prune:          config.GetBoolWithFallback(cmd, "prune", "snapshot.prune"),
	}
	if snap.Derived, err = derivedIndex(cmd, artifactsBaseDir); err != nil {
		return err
	}
	failOnLocal := config.GetBoolWithFallback(cmd, "fail-on-local-modified", "snapshot.failOnLocalModified")
	if snap.DryRun {
		log.Info().Msg("DRY RUN: no files and no state are written")
	}

	serviceDetails := serviceDetails(cmd)
	if tenant := cpi.TenantID(serviceDetails.Host); state.Tenant != tenant {
		if state.Tenant != "" && len(state.Artifacts) > 0 {
			log.Warn().Msgf("Snapshot state %s belongs to tenant %s, not %s: starting a new state", stateFile, state.Tenant, tenant)
			state.Artifacts = map[string]sync.ArtifactState{}
		}
		state.Tenant = tenant
	}
	res, snapErr := getTenantSnapshot(serviceDetails, artifactsBaseDir, workDir, draftHandling, syncPackageLevelDetails, includedIds, excludedIds,
		snapshotOptions{incremental: incremental, parallel: parallel, state: state, snap: snap, includedIds: includedIds, excludedIds: excludedIds})
	if res != nil {
		output.SetResult(cmd.Context(), res)
	}
	if snapErr != nil && (res == nil || res.Succeeded == 0) {
		return snapErr
	}
	localErr := error(nil)
	if failOnLocal && res != nil && res.Counts[sync.SnapLocalModified] > 0 {
		localErr = output.Failed(fmt.Errorf("%d artifact(s) local-modified", res.Counts[sync.SnapLocalModified]))
	}
	if snap.DryRun {
		if snapErr != nil {
			return snapErr
		}
		return localErr
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
	if snapErr != nil {
		return snapErr
	}
	return localErr
}

type snapshotOptions struct {
	incremental bool
	parallel    int
	state       *sync.SnapshotState
	snap        *sync.SnapshotOptions
	// the package filters, for packages that exist only locally
	includedIds, excludedIds []string
}

// snapshotResult is the JSON result of snapshot.
type snapshotResult struct {
	Packages   int      `json:"packages"`
	Succeeded  int      `json:"succeeded"`
	Downloaded int64    `json:"artifactsDownloaded"`
	Skipped    int64    `json:"artifactsSkipped"`
	Seconds    float64  `json:"seconds"`
	Failed     []string `json:"failed,omitempty"`
	DryRun     bool     `json:"dryRun,omitempty"`
	// Counts per status, Artifacts per artifact (sorted), Warnings: derived
	// copies edited on the tenant and similar.
	Counts    map[string]int      `json:"counts"`
	Warnings  int                 `json:"warnings"`
	Artifacts []sync.SnapshotItem `json:"artifacts"`
}

func getTenantSnapshot(serviceDetails *cpi.ServiceDetails, artifactsBaseDir string, workDir string, draftHandling string, syncPackageLevelDetails bool,
	includedIds []string, excludedIds []string, opts snapshotOptions) (*snapshotResult, error) {
	log.Info().Msg("---------------------------------------------------------------------------------")
	log.Info().Msg("📢 Begin taking a snapshot of the tenant")

	// Initialise HTTP executer; many parallel reads: retry when the tenant
	// throttles (429) or a gateway fails
	exe := cpi.InitHTTPExecuter(serviceDetails)

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
	synchroniser.Snap = opts.snap
	if synchroniser.Snap == nil {
		synchroniser.Snap = &sync.SnapshotOptions{}
	}
	snap := synchroniser.Snap
	tenantPackages := map[string]bool{}
	// the artifacts of all packages share the download slots, so one large
	// package does not run alone at the end
	synchroniser.ArtifactSlots = make(chan struct{}, opts.parallel)
	started := time.Now()

	res := &snapshotResult{}
	var mu gosync.Mutex
	sem := make(chan struct{}, opts.parallel)
	var wg gosync.WaitGroup
	for i, packageDataFromTenant := range packages {
		id := packageDataFromTenant.Root.Id
		tenantPackages[id] = true
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
				_, derivedPackage := derivedPackages(snap)[id]
				if syncPackageLevelDetails && !snap.DryRun && !derivedPackage {
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
	// deployment copies are compared with their sources once those are written
	if err := synchroniser.CheckDerived(); err != nil {
		res.Failed = append(res.Failed, err.Error())
	}
	// packages that exist only locally
	if err := snapshotLocalPackages(synchroniser, artifactsBaseDir, tenantPackages, includedIds, excludedIds); err != nil {
		res.Failed = append(res.Failed, err.Error())
	}
	res.DryRun = snap.DryRun
	res.Artifacts = snap.Items()
	res.Counts = map[string]int{}
	for _, it := range res.Artifacts {
		res.Counts[it.Status]++
		if it.Warning != "" {
			res.Warnings++
		}
	}
	res.Skipped, res.Downloaded = synchroniser.Skipped(), synchroniser.Downloaded()
	res.Seconds = time.Since(started).Round(100 * time.Millisecond).Seconds()
	sort.Strings(res.Failed)

	log.Info().Msg("---------------------------------------------------------------------------------")
	log.Info().Msgf("🏆 Snapshot%s: %d package(s) done, %d failed; %d artifact(s) downloaded, %d unchanged and skipped; %.1fs (%d in parallel)",
		map[bool]string{true: " (dry run)", false: ""}[snap.DryRun], res.Succeeded, len(res.Failed), res.Downloaded, res.Skipped, res.Seconds, opts.parallel)
	log.Info().Msgf("Artifacts: %s; %d warning(s)", formatCounts(res.Counts), res.Warnings)
	if len(res.Failed) > 0 {
		err := fmt.Errorf("%d of %d package(s) failed: %s", len(res.Failed), res.Packages, strings.Join(res.Failed, "; "))
		if res.Succeeded > 0 {
			return res, output.Partial(err)
		}
		return res, err
	}
	return res, nil
}

func derivedPackages(o *sync.SnapshotOptions) map[string]string {
	if o == nil || o.Derived == nil {
		return nil
	}
	return o.Derived.Packages
}

func formatCounts(counts map[string]int) string {
	order := []string{sync.SnapNew, sync.SnapChanged, sync.SnapUnchanged, sync.SnapDeleted, sync.SnapLocalModified, sync.SnapLocalOnly, sync.SnapDerived}
	var parts []string
	for _, k := range order {
		if counts[k] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[k], k))
		}
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}

// snapshotLocalPackages reports (and with --prune removes) the artifacts of
// package folders whose package is no longer on the tenant.
func snapshotLocalPackages(s *sync.Synchroniser, base string, tenantPackages map[string]bool, included, excluded []string) error {
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		id := e.Name()
		if !e.IsDir() || strings.HasPrefix(id, ".") || tenantPackages[id] || str.FilterIDs(id, included, excluded) {
			continue
		}
		dir := filepath.Join(base, id)
		if err := s.SnapshotLeftovers(id, dir, nil); err != nil {
			return err
		}
		if s.Snap.Prune && !s.Snap.DryRun {
			// the package folder goes when only its details file is left
			rest, _ := os.ReadDir(dir)
			if len(rest) == 0 || (len(rest) == 1 && rest[0].Name() == id+".json") {
				if err := os.RemoveAll(dir); err != nil {
					return err
				}
				log.Info().Msgf("Package folder %s removed (--prune): the package is gone from the tenant", id)
			}
		}
	}
	return nil
}

// derivedIndex reads the deploy config (--deploy-config, snapshot.deployConfig
// or orchestrator.deployConfig): the deployment copies it creates on the
// tenant, and the configOverrides keys of the artifacts deployed as they are.
func derivedIndex(cmd *cobra.Command, base string) (*sync.DerivedIndex, error) {
	path := config.GetStringWithFallback(cmd, "deploy-config", "snapshot.deployConfig")
	if path == "" {
		path = viper.GetString("orchestrator.deployConfig")
	}
	if path == "" {
		return nil, nil
	}
	loader := deploy.NewConfigLoader()
	if err := loader.DetectSource(path); err != nil {
		return nil, output.Usagef("deploy config %s: %v", path, err)
	}
	files, err := loader.LoadConfigs()
	if err != nil {
		return nil, output.Usagef("deploy config %s: %v", path, err)
	}
	extra := str.TrimSlice(config.GetStringSliceWithFallback(cmd, "deployment-prefix", "snapshot.deploymentPrefix"))
	includeDerived := config.GetBoolWithFallback(cmd, "include-derived", "snapshot.includeDerived")
	idx := &sync.DerivedIndex{Artifacts: map[string]sync.DerivedSource{}, Packages: map[string]string{}, Overrides: map[string]map[string]bool{}}
	sources := map[string]bool{}
	for _, f := range files {
		prefixes := slices.Clone(extra)
		if p := f.Config.DeploymentPrefix; p != "" && !slices.Contains(prefixes, p) {
			prefixes = append(prefixes, p)
		}
		for _, pkg := range f.Config.Packages {
			pkgDir := cmp.Or(pkg.PackageDir, pkg.ID)
			for _, p := range prefixes {
				idx.Packages[p+pkg.ID] = pkg.ID
			}
			for _, a := range pkg.Artifacts {
				artDir := filepath.ToSlash(filepath.Clean(cmp.Or(a.ArtifactDir, a.Id)))
				src := sync.DerivedSource{Label: pkgDir + "/" + artDir, Dir: filepath.Join(base, filepath.FromSlash(pkgDir), filepath.FromSlash(artDir))}
				if artDir != a.Id {
					idx.Artifacts[a.Id] = sync.DerivedSource{Label: src.Label, Dir: src.Dir, Reason: "artifactDir"}
				} else if f.Config.DeploymentPrefix == "" {
					sources[a.Id] = true
					if len(a.ConfigOverrides) > 0 {
						if idx.Overrides[a.Id] == nil {
							idx.Overrides[a.Id] = map[string]bool{}
						}
						for k := range a.ConfigOverrides {
							idx.Overrides[a.Id][k] = true
						}
					}
				}
				for _, p := range prefixes {
					idx.Artifacts[p+"_"+a.Id] = sync.DerivedSource{Label: src.Label, Dir: src.Dir, Reason: "deploymentPrefix " + p}
				}
			}
		}
	}
	// an ID that is deployed from its own folder is no copy
	for id := range sources {
		delete(idx.Artifacts, id)
	}
	if includeDerived {
		idx.Artifacts, idx.Packages = nil, nil
	}
	log.Info().Msgf("Deploy config %s: %d deployment copies and %d prefixed package(s) are not written%s",
		path, len(idx.Artifacts), len(idx.Packages), map[bool]string{true: " (--include-derived: all are written)", false: ""}[includeDerived])
	return idx, nil
}
