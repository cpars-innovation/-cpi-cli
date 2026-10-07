package cmd

import (
	"testing"
	"time"

	"github.com/cpars-innovation/cpicli/internal/cpitest"
	"github.com/cpars-innovation/cpicli/internal/models"
	"github.com/cpars-innovation/cpicli/internal/versioning"
	"github.com/cpars-innovation/cpicli/pkg/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestDeployCommand_MultipleArtifactIDs(t *testing.T) {
	started := []*cpitest.Runtime{{Version: "1", Status: "STARTED", DeployedOn: time.Now()}}
	mock := cpitest.NewTenant(t, map[string]*cpitest.Artifact{
		"A": {Type: "Integration", DesignVersion: "1", TaskStatuses: []string{"SUCCESS"}, AfterDeploy: started},
		"B": {Type: "Integration", DesignVersion: "1", TaskStatuses: []string{"SUCCESS"}, AfterDeploy: started},
	})

	_, _, err := runCLI(t, mock, "deploy", "--artifact-ids", "A, B", "--delay-length", "0", "--max-check-limit", "3")
	require.NoError(t, err)
	assert.Equal(t, 2, mock.Count("POST /api/v1/DeployIntegrationDesigntimeArtifact"))
}

func TestDeployCommand_PartialFailureReturnsError(t *testing.T) {
	started := []*cpitest.Runtime{{Version: "1", Status: "STARTED", DeployedOn: time.Now()}}
	mock := cpitest.NewTenant(t, map[string]*cpitest.Artifact{
		"A": {Type: "Integration", DesignVersion: "1", AfterDeploy: started},
		"B": {Type: "Integration"},
	})
	_, _, err := runCLI(t, mock, "deploy", "--artifact-ids", "A,B", "--delay-length", "0", "--max-check-limit", "3")
	var depErr *ops.Error
	require.ErrorAs(t, err, &depErr)
	assert.Len(t, depErr.Failed(), 1)
	assert.Equal(t, "B", depErr.Failed()[0].ID)
	assert.Equal(t, 1, mock.Count("POST /api/v1/DeployIntegrationDesigntimeArtifact"))
}

func TestOrchestratorVersioningPerArtifact(t *testing.T) {
	var cfg models.DeployConfig
	require.NoError(t, yaml.Unmarshal([]byte(`packages:
  - integrationSuiteId: EDM
    deploy: true
    versioning: keep
    artifacts:
      - {artifactId: A, type: Integration, deploy: true}
      - {artifactId: B, type: Integration, deploy: true, versioning: manifest}
      - {artifactId: C, type: Integration, deploy: true, versioning: wrong}
`), &cfg))
	stats := &ProcessingStats{FailedArtifactUpdates: map[string]bool{}, FailedArtifactDeploys: map[string]bool{}}
	tasks := collectDeploymentTasks(&cfg.Packages[0], "EDM", "", nil, stats, versioning.TenantBump)
	require.Len(t, tasks, 2)
	assert.Equal(t, versioning.Keep, tasks[0].Versioning, "package wins over the flag")
	assert.Equal(t, versioning.Manifest, tasks[1].Versioning, "artifact wins over the package")
	assert.True(t, stats.FailedArtifactDeploys["C"])
}
