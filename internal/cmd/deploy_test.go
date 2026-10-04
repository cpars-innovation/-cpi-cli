package cmd

import (
	"testing"
	"time"

	"github.com/cpars-innovation/cpicli/internal/cpitest"
	"github.com/cpars-innovation/cpicli/internal/deployer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	var depErr *deployer.Error
	require.ErrorAs(t, err, &depErr)
	assert.Len(t, depErr.Failed(), 1)
	assert.Equal(t, "B", depErr.Failed()[0].ID)
	assert.Equal(t, 1, mock.Count("POST /api/v1/DeployIntegrationDesigntimeArtifact"))
}
