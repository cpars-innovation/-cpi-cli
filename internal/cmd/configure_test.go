package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cpars-innovation/cpicli/internal/cpitest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeConfigureFile(t *testing.T, params string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dev.yml")
	require.NoError(t, os.WriteFile(path, []byte(`packages:
  - integrationSuiteId: Orders
    deploy: true
    artifacts:
      - artifactId: A
        type: Integration
        parameters:
`+params), 0o644))
	return path
}

func configureMock(t *testing.T) *cpitest.Tenant {
	t.Helper()
	return cpitest.NewTenant(t, map[string]*cpitest.Artifact{
		"A": {Type: "Integration", DesignVersion: "1.0.0", Parameters: map[string]string{"Host": "old", "Port": "443"},
			TaskStatuses: []string{"SUCCESS"}, AfterDeploy: []*cpitest.Runtime{{Version: "1.0.0", Status: "STARTED", DeployedOn: time.Now()}}},
	})
}

func TestConfigureWritesAndDeploysOnlyChanges(t *testing.T) {
	mock := configureMock(t)
	file := writeConfigureFile(t, "          - {key: Host, value: new}\n          - {key: Port, value: \"443\"}\n")
	args := append([]string{"configure", "--config-path", file, "--disable-batch", "--deploy-delay", "1", "--output", "json"}, basicAuth(mock)...)

	r := runMain(t, args...)
	require.Equal(t, 0, r.code, r.stderr)
	assert.Equal(t, 1, mock.Count("PUT /api/v1/IntegrationDesigntimeArtifacts(Id='A',Version='active')/$links/Configurations('Host')"), "only the changed key")
	assert.Equal(t, 0, mock.Count("PUT /api/v1/IntegrationDesigntimeArtifacts(Id='A',Version='active')/$links/Configurations('Port')"))
	assert.Equal(t, 1, mock.Count("POST /api/v1/DeployIntegrationDesigntimeArtifact"))
	var env struct {
		Result struct {
			Diff []struct{ Key, Change string }
		}
	}
	require.NoError(t, json.Unmarshal([]byte(r.stdout), &env))
	assert.Len(t, env.Result.Diff, 2)

	// second run: the tenant has the values, nothing is written or deployed
	r = runMain(t, args...)
	require.Equal(t, 0, r.code, r.stderr)
	assert.Equal(t, 1, mock.Count("PUT "), "no new PUT")
	assert.Equal(t, 1, mock.Count("POST /api/v1/DeployIntegrationDesigntimeArtifact"), "no new deploy")

	// --force restores the old behaviour: write everything and trigger the
	// deployment (its outcome against the static mock runtime is not checked)
	_ = runMain(t, append(args, "--force", "--deploy-retries", "1")...)
	assert.Equal(t, 3, mock.Count("PUT "))
	assert.Equal(t, 2, mock.Count("POST /api/v1/DeployIntegrationDesigntimeArtifact"))
}

func TestConfigureUnknownKeyIsNotWritten(t *testing.T) {
	mock := configureMock(t)
	file := writeConfigureFile(t, "          - {key: Host, value: new}\n          - {key: Hots, value: typo}\n")
	r := runMain(t, append([]string{"configure", "--config-path", file, "--disable-batch"}, basicAuth(mock)...)...)
	assert.NotEqual(t, 0, r.code)
	assert.Contains(t, r.stderr, "Hots")
	assert.Equal(t, 0, mock.Count("PUT "), "nothing written for the artifact")
	assert.Equal(t, 0, mock.Count("POST /api/v1/DeployIntegrationDesigntimeArtifact"))
}

func TestConfigureDryRunReadsTenantUnlessOffline(t *testing.T) {
	mock := configureMock(t)
	file := writeConfigureFile(t, "          - {key: Host, value: new}\n")
	r := runMain(t, append([]string{"configure", "--config-path", file, "--dry-run"}, basicAuth(mock)...)...)
	require.Equal(t, 0, r.code, r.stderr)
	assert.Contains(t, r.stderr, `"old" -> "new"`)
	gets := mock.Count("GET /api/v1/IntegrationDesigntimeArtifacts(Id='A',Version='active')/Configurations")
	assert.Equal(t, 1, gets)

	r = runMain(t, append([]string{"configure", "--config-path", file, "--dry-run", "--offline"}, basicAuth(mock)...)...)
	require.Equal(t, 0, r.code, r.stderr)
	assert.Equal(t, gets, mock.Count("GET /api/v1/IntegrationDesigntimeArtifacts(Id='A',Version='active')/Configurations"))
	assert.Equal(t, 0, mock.Count("PUT "))
}
