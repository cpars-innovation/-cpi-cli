package cmd

import (
	"context"
	"time"

	"github.com/cpars-innovation/cpicli/internal/versioning"
	"github.com/cpars-innovation/cpicli/pkg/httpclnt"
	"github.com/cpars-innovation/cpicli/pkg/ops"
	"github.com/rs/zerolog/log"
)

// DeploymentTask represents an artifact ready for deployment
// Deployment defaults shared by deploy, undeploy, orchestrator, configure
// and the MCP server: a status check every 10 s for up to 5 minutes; a
// failed deployment ends at its first check.
const (
	defaultDeployChecks        = 30
	defaultDeployDelaySeconds  = 10
	defaultParallelDeployments = 5
)

type DeploymentTask struct {
	ArtifactID   string
	ArtifactType string
	PackageID    string
	DisplayName  string
	// Force deploys even when the runtime already has the designtime
	// version (configuration changes do not change the version).
	Force bool
	// AllowDowngrade, ModifiedAt and Versioning are passed to ops.Artifact.
	AllowDowngrade bool
	ModifiedAt     *time.Time
	Versioning     versioning.Mode
	// ExpectedVersion is the repository's Bundle-Version (versioning
	// manifest): the deployment refuses any other designtime version.
	ExpectedVersion string
}

// deployTasks deploys the tasks of configure and orchestrator through the
// shared deployer. Packages are processed one after another (in the order in
// which they first appear); within a package up to parallelDeployments
// artifacts are deployed concurrently (forced tasks first). Results are returned per package.
func deployTasks(ctx context.Context, exe *httpclnt.HTTPExecuter, tasks []DeploymentTask,
	compareVersions bool, maxChecks, delaySeconds, parallelDeployments int) []ops.Result {

	// per package: forced tasks and tasks that follow compareVersions
	type group struct{ forced, compared []ops.Artifact }
	var packageOrder []string
	byPackage := make(map[string]*group)
	for _, t := range tasks {
		g, seen := byPackage[t.PackageID]
		if !seen {
			packageOrder = append(packageOrder, t.PackageID)
			g = &group{}
			byPackage[t.PackageID] = g
		}
		a := ops.Artifact{ID: t.ArtifactID, Type: mapArtifactTypeForSync(t.ArtifactType), PackageID: t.PackageID,
			AllowDowngrade: t.AllowDowngrade, ModifiedAt: t.ModifiedAt, Versioning: t.Versioning, ExpectedVersion: t.ExpectedVersion}
		if t.Force || !compareVersions {
			g.forced = append(g.forced, a)
		} else {
			g.compared = append(g.compared, a)
		}
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
		g := byPackage[packageID]
		log.Info().Msgf("📦 Deploying %d artifact(s) for package %s (max %d concurrent)", len(g.forced)+len(g.compared), packageID, parallelDeployments)
		if len(g.forced) > 0 {
			forced := opts
			forced.CompareVersions = false
			results = append(results, ops.Deploy(ctx, tenant, g.forced, forced)...)
		}
		if len(g.compared) > 0 {
			results = append(results, ops.Deploy(ctx, tenant, g.compared, opts)...)
		}
	}
	logResults(results)
	return results
}
