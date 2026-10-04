package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cpars-innovation/cpicli/internal/api"
	"github.com/cpars-innovation/cpicli/internal/file"
	"github.com/cpars-innovation/cpicli/internal/httpclnt"
	"github.com/cpars-innovation/cpicli/internal/str"
	"github.com/go-errors/errors"
	"github.com/rs/zerolog/log"
)

type Syncer interface {
	Exec(request Request) error
}

type Request struct {
	WorkDir      string
	ArtifactsDir string
	IncludedIds  []string
	ExcludedIds  []string
	PackageFile  string
}

func NewSyncer(target string, functionType string, exe *httpclnt.HTTPExecuter) Syncer {
	switch functionType {
	case "APIProxy":
		switch target {
		case "git":
			return NewAPIProxyGitSynchroniser(exe)
		case "tenant":
			return NewAPIProxyTenantSynchroniser(exe)
		default:
			return nil
		}
	case "APIProduct":
		switch target {
		case "git":
			return NewAPIProductGitSynchroniser(exe)
		case "tenant":
			return NewAPIProductTenantSynchroniser(exe)
		default:
			return nil
		}
	case "CPIPackage":
		switch target {
		case "tenant":
			return NewCPIPackageTenantSynchroniser(exe)
		default:
			return nil
		}
		// TODO - refactor CPI syncer
	//case "CPI":
	//	return NewScriptCollection(exe)
	default:
		return nil
	}
}

type APIProxyGitSynchroniser struct {
	exe *httpclnt.HTTPExecuter
}

// NewAPIProxyGitSynchroniser returns an initialised APIProxyGitSynchroniser instance.
func NewAPIProxyGitSynchroniser(exe *httpclnt.HTTPExecuter) Syncer {
	s := new(APIProxyGitSynchroniser)
	s.exe = exe
	return s
}

func (s *APIProxyGitSynchroniser) Exec(request Request) error {
	log.Info().Msg("Sync API Proxy content to Git")

	proxy := api.NewAPIProxy(s.exe)
	// Get all APIProxies
	artifacts, err := proxy.List()
	if err != nil {
		return err
	}

	// Create temp directories in working dir
	targetRootDir := fmt.Sprintf("%v/download", request.WorkDir)
	err = os.MkdirAll(targetRootDir, os.ModePerm)
	if err != nil {
		return errors.Wrap(err, 0)
	}

	// Process through the artifacts
	for _, artifact := range artifacts {
		log.Info().Msg("---------------------------------------------------------------------------------")
		log.Info().Msgf("📢 Begin processing for APIProxy %v", artifact.Name)

		// Filter in/out artifacts
		if str.FilterIDs(artifact.Name, request.IncludedIds, request.ExcludedIds) {
			continue
		}

		// Download artifact content
		err = proxy.Download(artifact.Name, targetRootDir)
		if err != nil {
			return err
		}

		// Compare content and update Git if required
		gitArtifactPath := fmt.Sprintf("%v/%v", request.ArtifactsDir, artifact.Name)
		downloadedArtifactPath := fmt.Sprintf("%v/%v", targetRootDir, artifact.Name)
		if file.Exists(fmt.Sprintf("%v/manifest.json", gitArtifactPath)) {
			// (1) If artifact already exists in Git, then compare and update
			log.Info().Msg("Comparing content from tenant against Git")
			dirDiffer := file.DiffDirectories(downloadedArtifactPath, gitArtifactPath)

			if dirDiffer {
				log.Info().Msg("🏆 Changes detected and will be updated to Git")
				// Update the changes into the Git directory
				err := file.ReplaceDir(downloadedArtifactPath, gitArtifactPath)
				if err != nil {
					return err
				}
			} else {
				log.Info().Msg("🏆 No changes detected. Update to Git not required")
			}
		} else { // (2) If artifact does not exist in Git, then add it
			log.Info().Msgf("🏆 APIProxy %v does not exist, and will be added to Git", artifact.Name)
			err = file.ReplaceDir(downloadedArtifactPath, gitArtifactPath)
			if err != nil {
				return err
			}
		}
	}

	log.Info().Msg("---------------------------------------------------------------------------------")
	log.Info().Msgf("🏆 Completed processing of APIProxies")

	return nil
}

type APIProxyTenantSynchroniser struct {
	exe *httpclnt.HTTPExecuter
}

// NewAPIProxyTenantSynchroniser returns an initialised APIProxyTenantSynchroniser instance.
func NewAPIProxyTenantSynchroniser(exe *httpclnt.HTTPExecuter) Syncer {
	s := new(APIProxyTenantSynchroniser)
	s.exe = exe
	return s
}

func (s *APIProxyTenantSynchroniser) Exec(request Request) error {
	// Get directory list
	baseSourceDir := filepath.Clean(request.ArtifactsDir)
	entries, err := os.ReadDir(baseSourceDir)
	if err != nil {
		return errors.Wrap(err, 0)
	}

	proxy := api.NewAPIProxy(s.exe)

	// Create temp directories in working dir
	uploadWorkDir := fmt.Sprintf("%v/upload", request.WorkDir)
	err = os.MkdirAll(uploadWorkDir, os.ModePerm)
	if err != nil {
		return errors.Wrap(err, 0)
	}
	downloadWorkDir := fmt.Sprintf("%v/download", request.WorkDir)
	err = os.MkdirAll(downloadWorkDir, os.ModePerm)
	if err != nil {
		return errors.Wrap(err, 0)
	}

	artifactDirFound := false
	for _, entry := range entries {
		artifactId := entry.Name()
		manifestPath := fmt.Sprintf("%v/%v/manifest.json", baseSourceDir, artifactId)
		if entry.IsDir() && file.Exists(manifestPath) {
			artifactDirFound = true
			gitArtifactDir := fmt.Sprintf("%v/%v", baseSourceDir, artifactId)

			log.Info().Msg("---------------------------------------------------------------------------------")
			log.Info().Msgf("Processing directory %v", gitArtifactDir)

			// Filter in/out artifacts
			if str.FilterIDs(artifactId, request.IncludedIds, request.ExcludedIds) {
				continue
			}

			log.Info().Msgf("📢 Begin processing for APIProxy %v", artifactId)
			proxyExists, err := proxy.Exists(artifactId)
			if err != nil {
				return err
			}
			if !proxyExists {
				log.Info().Msgf("APIProxy %v will be created", artifactId)

				err = proxy.Upload(gitArtifactDir, uploadWorkDir)
				if err != nil {
					return err
				}

				log.Info().Msg("🏆 APIProxy created successfully")
			} else {
				log.Info().Msg("Checking if APIProxy needs to be updated")

				err = proxy.Download(artifactId, downloadWorkDir)
				if err != nil {
					return err
				}

				log.Info().Msg("Comparing content from tenant against Git")
				downloadArtifactDir := fmt.Sprintf("%v/%v", downloadWorkDir, artifactId)
				dirDiffer := file.DiffDirectories(downloadArtifactDir, gitArtifactDir)
				if dirDiffer {
					log.Info().Msg("Changes found in APIProxy. APIProxy will be updated in tenant")

					err = proxy.Upload(gitArtifactDir, uploadWorkDir)
					if err != nil {
						return err
					}
					log.Info().Msg("🏆 APIProxy updated successfully")
				} else {
					log.Info().Msg("🏆 No changes detected. APIProxy does not need to be updated")
				}
			}
		}
	}
	if !artifactDirFound {
		log.Warn().Msgf("No directory with APIProxy contents found in %v", baseSourceDir)
	}
	log.Info().Msg("---------------------------------------------------------------------------------")
	log.Info().Msgf("🏆 Completed processing of APIProxies")
	return nil
}

type CPIPackageTenantSynchroniser struct {
	exe *httpclnt.HTTPExecuter
}

// NewCPIPackageTenantSynchroniser returns an initialised CPIPackageTenantSynchroniser instance.
func NewCPIPackageTenantSynchroniser(exe *httpclnt.HTTPExecuter) Syncer {
	s := new(CPIPackageTenantSynchroniser)
	s.exe = exe
	return s
}

func (s *CPIPackageTenantSynchroniser) Exec(request Request) error {
	var packageFile string
	if request.PackageFile != "" {
		packageFile = request.PackageFile
	} else {
		packageFile = fmt.Sprintf("%v/%v.json", request.ArtifactsDir, filepath.Base(request.ArtifactsDir))
	}
	// Get package details from JSON file
	log.Info().Msgf("Getting package details from %v file", packageFile)
	packageDetails, err := api.GetPackageDetails(packageFile)
	if err != nil {
		return err
	}

	ip := api.NewIntegrationPackage(s.exe)

	packageId := packageDetails.Root.Id
	_, _, exists, err := ip.Get(packageId)
	if err != nil {
		return err
	}
	if !exists {
		log.Info().Msgf("Package %v does not exist", packageId)
		err = ip.Create(packageDetails)
		if err != nil {
			return err
		}
		log.Info().Msgf("Package %v created", packageId)
	} else {
		// Update integration package
		err = ip.Update(packageDetails)
		if err != nil {
			return err
		}
		log.Info().Msgf("Package %v updated", packageId)
	}
	return nil
}

type APIProductGitSynchroniser struct {
	exe *httpclnt.HTTPExecuter
}

// NewAPIProductGitSynchroniser returns an initialised APIProductGitSynchroniser instance.
func NewAPIProductGitSynchroniser(exe *httpclnt.HTTPExecuter) Syncer {
	s := new(APIProductGitSynchroniser)
	s.exe = exe
	return s
}

func (s *APIProductGitSynchroniser) Exec(request Request) error {
	log.Info().Msg("Sync API Product content to Git")

	product := api.NewAPIProduct(s.exe)
	// Get all APIProducts
	artifacts, err := product.List()
	if err != nil {
		return err
	}

	// Create temp directories in working dir
	targetRootDir := fmt.Sprintf("%v/download", request.WorkDir)
	err = os.MkdirAll(targetRootDir, os.ModePerm)
	if err != nil {
		return errors.Wrap(err, 0)
	}

	// Process through the artifacts
	for _, artifact := range artifacts {
		log.Info().Msg("---------------------------------------------------------------------------------")
		log.Info().Msgf("📢 Begin processing for APIProduct %v", artifact.Name)

		// Filter in/out artifacts
		if str.FilterIDs(artifact.Name, request.IncludedIds, request.ExcludedIds) {
			continue
		}

		// Download artifact content
		err = product.Download(artifact.Name, targetRootDir)
		if err != nil {
			return err
		}

		// Compare content and update Git if required
		gitArtifactPath := fmt.Sprintf("%v/%v.json", request.ArtifactsDir, artifact.Name)
		downloadedArtifactPath := fmt.Sprintf("%v/%v.json", targetRootDir, artifact.Name)
		if file.Exists(gitArtifactPath) {
			// (1) If artifact already exists in Git, then compare and update
			log.Info().Msg("Comparing content from tenant against Git")
			fileDiffer := file.DiffFile(downloadedArtifactPath, gitArtifactPath)

			if fileDiffer {
				log.Info().Msg("🏆 Changes detected and will be updated to Git")
				// Update the changes into the Git
				err := file.CopyFile(downloadedArtifactPath, gitArtifactPath)
				if err != nil {
					return err
				}
			} else {
				log.Info().Msg("🏆 No changes detected. Update to Git not required")
			}
		} else { // (2) If artifact does not exist in Git, then add it
			log.Info().Msgf("🏆 APIProduct %v does not exist, and will be added to Git", artifact.Name)
			err = file.CopyFile(downloadedArtifactPath, gitArtifactPath)
			if err != nil {
				return err
			}
		}
	}

	log.Info().Msg("---------------------------------------------------------------------------------")
	log.Info().Msgf("🏆 Completed processing of APIProducts")

	return nil
}

type APIProductTenantSynchroniser struct {
	exe *httpclnt.HTTPExecuter
}

// NewAPIProductTenantSynchroniser returns an initialised APIProductTenantSynchroniser instance.
func NewAPIProductTenantSynchroniser(exe *httpclnt.HTTPExecuter) Syncer {
	s := new(APIProductTenantSynchroniser)
	s.exe = exe
	return s
}

func (s *APIProductTenantSynchroniser) Exec(request Request) error {
	// Get directory list
	baseSourceDir := filepath.Clean(request.ArtifactsDir)
	entries, err := os.ReadDir(baseSourceDir)
	if err != nil {
		return errors.Wrap(err, 0)
	}

	product := api.NewAPIProduct(s.exe)

	// Create temp directories in working dir
	uploadWorkDir := fmt.Sprintf("%v/upload", request.WorkDir)
	err = os.MkdirAll(uploadWorkDir, os.ModePerm)
	if err != nil {
		return errors.Wrap(err, 0)
	}
	downloadWorkDir := fmt.Sprintf("%v/download", request.WorkDir)
	err = os.MkdirAll(downloadWorkDir, os.ModePerm)
	if err != nil {
		return errors.Wrap(err, 0)
	}

	artifactFileFound := false
	for _, entry := range entries {
		artifactFileName := entry.Name()
		if !entry.IsDir() && strings.Contains(artifactFileName, ".json") {
			artifactFileFound = true
			gitArtifactPath := fmt.Sprintf("%v/%v", baseSourceDir, artifactFileName)

			log.Info().Msg("---------------------------------------------------------------------------------")
			log.Info().Msgf("Processing file %v", gitArtifactPath)

			// Strip .json from the file name
			artifactId := strings.TrimSuffix(artifactFileName, ".json")

			// Filter in/out artifacts
			if str.FilterIDs(artifactId, request.IncludedIds, request.ExcludedIds) {
				continue
			}

			log.Info().Msgf("📢 Begin processing for APIProduct %v", artifactId)
			productExists, err := product.Exists(artifactId)
			if err != nil {
				return err
			}
			if !productExists {
				log.Info().Msgf("APIProduct %v will be created", artifactId)

				err = product.Upload(gitArtifactPath, uploadWorkDir)
				if err != nil {
					return err
				}

				log.Info().Msg("🏆 APIProduct created successfully")
			} else {
				log.Warn().Msgf("APIProduct %v already exists. Skipping as update not currently supported", artifactId)
			}
		}
	}
	if !artifactFileFound {
		log.Warn().Msgf("No directory with APIProduct contents found in %v", baseSourceDir)
	}
	log.Info().Msg("---------------------------------------------------------------------------------")
	log.Info().Msgf("🏆 Completed processing of APIProducts")
	return nil
}
