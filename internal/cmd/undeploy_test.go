package cmd

import (
	"testing"

	"github.com/cpars-innovation/cpicli/internal/cpitest"
	"github.com/cpars-innovation/cpicli/internal/deployer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUndeployCommand(t *testing.T) {
	deployed := &cpitest.Runtime{Version: "1.0.3", Status: "STARTED"}

	t.Run("deletes and waits until gone", func(t *testing.T) {
		mock := cpitest.NewTenant(t, map[string]*cpitest.Artifact{
			"A": {Runtime: deployed, UndeployAfter: 2},
			"B": {}, // not deployed: not a failure, no DELETE
		})
		_, _, err := runCLI(t, mock, "undeploy", "--artifact-ids", "A,B", "--delay-length", "0", "--max-check-limit", "5")
		require.NoError(t, err)
		assert.Equal(t, 1, mock.Count("DELETE /api/v1/IntegrationRuntimeArtifacts('A')"))
		assert.Zero(t, mock.Count("DELETE /api/v1/IntegrationRuntimeArtifacts('B')"))
	})

	t.Run("times out while still present", func(t *testing.T) {
		mock := cpitest.NewTenant(t, map[string]*cpitest.Artifact{"A": {Runtime: deployed, UndeployAfter: -1}})
		_, _, err := runCLI(t, mock, "undeploy", "--artifact-ids", "A", "--delay-length", "0", "--max-check-limit", "3")
		var depErr *deployer.Error
		require.ErrorAs(t, err, &depErr)
		assert.Equal(t, deployer.StatusTimeout, depErr.Failed()[0].Status)
	})

	t.Run("requires artifact ids", func(t *testing.T) {
		mock := cpitest.NewTenant(t, nil)
		_, _, err := runCLI(t, mock, "undeploy")
		require.Error(t, err)
		assert.Empty(t, mock.Requests())
	})
}
