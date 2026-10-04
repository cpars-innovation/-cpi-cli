package cmd

import (
	"context"
	"time"

	"github.com/cpars-innovation/cpicli/pkg/httpclnt"
	"github.com/cpars-innovation/cpicli/pkg/ops"
	"github.com/rs/zerolog/log"
)

// DeploymentTask represents an artifact ready for deployment
type DeploymentTask struct {
	ArtifactID   string
	ArtifactType string
	PackageID    string
	DisplayName  string
}

// deployTasks deploys the tasks of configure and orchestrator through the
// shared deployer. Packages are processed one after another (in the order in
// which they first appear); within a package up to parallelDeployments
// artifacts are deployed concurrently. Results are returned in task order.
func deployTasks(ctx context.Context, exe *httpclnt.HTTPExecuter, tasks []DeploymentTask,
	compareVersions bool, maxChecks, delaySeconds, parallelDeployments int) []ops.Result {

	var packageOrder []string
	byPackage := make(map[string][]ops.Artifact)
	for _, t := range tasks {
		if _, seen := byPackage[t.PackageID]; !seen {
			packageOrder = append(packageOrder, t.PackageID)
		}
		byPackage[t.PackageID] = append(byPackage[t.PackageID], ops.Artifact{
			ID:        t.ArtifactID,
			Type:      mapArtifactTypeForSync(t.ArtifactType),
			PackageID: t.PackageID,
		})
	}

	tenant := ops.NewTenant(exe)
	opts := ops.Options{
		Interval:        time.Duration(delaySeconds) * time.Second,
		MaxChecks:       maxChecks,
		CompareVersions: compareVersions,
		Parallelism:     parallelDeployments,
	}

	results := make([]ops.Result, 0, len(tasks))
	for _, packageID := range packageOrder {
		artifacts := byPackage[packageID]
		log.Info().Msgf("📦 Deploying %d artifact(s) for package %s (max %d concurrent)", len(artifacts), packageID, parallelDeployments)
		results = append(results, ops.Deploy(ctx, tenant, artifacts, opts)...)
	}
	logResults(results)
	return results
}
