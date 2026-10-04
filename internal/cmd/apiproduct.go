package cmd

import (
	"fmt"
	"github.com/cpars-innovation/cpicli/internal/config"
	"github.com/cpars-innovation/cpicli/internal/repo"
	"github.com/cpars-innovation/cpicli/internal/str"
	"github.com/cpars-innovation/cpicli/internal/sync"
	"github.com/cpars-innovation/cpicli/pkg/cpi"
	"github.com/go-errors/errors"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"strings"
)

func NewAPIProductCommand() *cobra.Command {
	apiproductCmd := &cobra.Command{
		Use:   "apiproduct",
		Short: "Sync API Management products between tenant and Git",
		Long: `Synchronise API Management products between SAP Integration Suite
tenant and a Git repository.

Configuration:
  Settings can be loaded from the global config file (--config) under the
  'sync.apiproduct' section. CLI flags override config file settings.`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// If artifacts directory is provided, validate that is it a subdirectory of Git repo
			gitRepoDir, err := config.GetStringWithEnvExpandAndFallback(cmd, "dir-git-repo", "sync.apiproduct.dirGitRepo")
			if err != nil {
				return fmt.Errorf("security alert for --dir-git-repo: %w", err)
			}
			if gitRepoDir != "" {
				artifactsDir, err := config.GetStringWithEnvExpandAndFallback(cmd, "dir-artifacts", "sync.apiproduct.dirArtifacts")
				if err != nil {
					return fmt.Errorf("security alert for --dir-artifacts: %w", err)
				}
				gitRepoDirClean := filepath.Clean(gitRepoDir) + string(os.PathSeparator)
				if artifactsDir != "" && !strings.HasPrefix(artifactsDir, gitRepoDirClean) {
					return fmt.Errorf("--dir-artifacts [%v] should be a subdirectory of --dir-git-repo [%v]", artifactsDir, gitRepoDirClean)
				}
			}
			// Validate target
			target := config.GetStringWithFallback(cmd, "target", "sync.apiproduct.target")
			switch target {
			case "git", "tenant":
			default:
				return fmt.Errorf("invalid value for --target = %v", target)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			if err = runSyncAPIProduct(cmd); err != nil {
				cmd.SilenceUsage = true
			}
			return
		},
	}

	return apiproductCmd
}

func runSyncAPIProduct(cmd *cobra.Command) error {
	log.Info().Msg("Executing sync apiproduct command")

	// Support reading from config file under 'sync.apiproduct' key
	gitRepoDir, err := config.GetStringWithEnvExpandAndFallback(cmd, "dir-git-repo", "sync.apiproduct.dirGitRepo")
	if err != nil {
		return fmt.Errorf("security alert for --dir-git-repo: %w", err)
	}
	artifactsDir := config.GetStringWithFallback(cmd, "dir-artifacts", "sync.apiproduct.dirArtifacts")
	if artifactsDir == "" {
		artifactsDir = gitRepoDir
	}
	workDir, err := config.GetStringWithEnvExpandAndFallback(cmd, "dir-work", "sync.apiproduct.dirWork")
	if err != nil {
		return fmt.Errorf("security alert for --dir-work: %w", err)
	}
	includedIds := str.TrimSlice(config.GetStringSliceWithFallback(cmd, "ids-include", "sync.apiproduct.idsInclude"))
	excludedIds := str.TrimSlice(config.GetStringSliceWithFallback(cmd, "ids-exclude", "sync.apiproduct.idsExclude"))
	commitMsg := config.GetStringWithFallback(cmd, "git-commit-msg", "sync.apiproduct.gitCommitMsg")
	commitUser := config.GetStringWithFallback(cmd, "git-commit-user", "sync.apiproduct.gitCommitUser")
	commitEmail := config.GetStringWithFallback(cmd, "git-commit-email", "sync.apiproduct.gitCommitEmail")
	skipCommit := config.GetBoolWithFallback(cmd, "git-skip-commit", "sync.apiproduct.gitSkipCommit")
	target := config.GetStringWithFallback(cmd, "target", "sync.apiproduct.target")

	serviceDetails := serviceDetails(cmd)
	// Initialise HTTP executer
	exe := cpi.InitHTTPExecuter(serviceDetails)

	syncer := sync.NewSyncer(target, "APIProduct", exe)
	apiproductWorkDir := fmt.Sprintf("%v/apiproduct", workDir)
	err = syncer.Exec(sync.Request{WorkDir: apiproductWorkDir, ArtifactsDir: artifactsDir, IncludedIds: includedIds, ExcludedIds: excludedIds})
	if err != nil {
		return err
	}
	if target == "git" && !skipCommit {
		err = repo.CommitToRepo(gitRepoDir, commitMsg, commitUser, commitEmail)
		if err != nil {
			return err
		}
	}
	// Clean up working directory
	err = os.RemoveAll(apiproductWorkDir)
	if err != nil {
		return errors.Wrap(err, 0)
	}

	return nil
}
