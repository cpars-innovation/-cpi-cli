package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cpars-innovation/cpicli/internal/cpitest"
	"github.com/cpars-innovation/cpicli/pkg/cpi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The pipeline snapshot -> orchestrator --defer-deploy -> configure
// --defer-deploy -> deploy --pending deploys every artifact at most once,
// and a same-version content change is not undeployed in between.
func TestDeferredDeploymentsAreDeployedOnce(t *testing.T) {
	modified := time.Now().Add(-time.Hour).Truncate(time.Second)
	flow := func(id string) *cpitest.Artifact {
		return &cpitest.Artifact{Type: "Integration", DesignVersion: "1.0.5", Package: "Pkg", Name: id, ModifiedAt: modified,
			Zip: namedFlowZip(t, id, "v1"), Parameters: map[string]string{"Host": "old"},
			Runtime:      &cpitest.Runtime{Version: "1.0.5", Status: "STARTED", DeployedOn: modified},
			TaskStatuses: []string{"SUCCESS"},
			AfterDeploy:  []*cpitest.Runtime{{Version: "1.0.5", Status: "STARTED", DeployedOn: time.Now().Add(time.Hour)}}}
	}
	ids := []string{"Changed", "Param", "Both", "Same"}
	artifacts := map[string]*cpitest.Artifact{}
	for _, id := range ids {
		artifacts[id] = flow(id)
	}
	mock := cpitest.NewTenant(t, artifacts)
	mock.Packages = []cpitest.Package{{ID: "Pkg", Version: "1.0.0"}}
	repo := t.TempDir()
	t.Chdir(repo) // .cpi/pending-deploy.json is relative to the working directory

	r := runMain(t, append([]string{"snapshot", "--dir-git-repo", repo, "--dir-work", t.TempDir(), "--git-skip-commit",
		"--sync-package-details=false"}, basicAuth(mock)...)...)
	require.Equal(t, 0, r.code, r.stderr)
	for _, id := range []string{"Changed", "Both"} {
		require.NoError(t, os.WriteFile(filepath.Join(repo, "Pkg", id, "src", "main", "resources", "script", "s.groovy"), []byte("v2"), 0o644))
	}

	var lines string
	for _, id := range ids {
		lines += "      - {artifactId: " + id + ", artifactDir: " + id + ", type: IntegrationFlow}\n"
	}
	deployCfg := filepath.Join(t.TempDir(), "deploy.yml")
	require.NoError(t, os.WriteFile(deployCfg, []byte("packages:\n  - integrationSuiteId: Pkg\n    packageDir: Pkg\n    artifacts:\n"+lines), 0o644))
	r = runMain(t, append([]string{"orchestrator", "--packages-dir", repo, "--deploy-config", deployCfg, "--defer-deploy"}, basicAuth(mock)...)...)
	require.Equal(t, 0, r.code, r.stderr)
	assert.Equal(t, 0, mock.Count("POST /api/v1/DeployIntegrationDesigntimeArtifact"), "deferred")
	assert.Equal(t, 0, mock.Count("DELETE "), "a same-version change is not undeployed")

	configureCfg := filepath.Join(t.TempDir(), "configure.yml")
	require.NoError(t, os.WriteFile(configureCfg, []byte(`packages:
  - integrationSuiteId: Pkg
    deploy: true
    artifacts:
      - {artifactId: Param, type: Integration, parameters: [{key: Host, value: new}]}
      - {artifactId: Both, type: Integration, parameters: [{key: Host, value: new}]}
      - {artifactId: Same, type: Integration, parameters: [{key: Host, value: old}]}
`), 0o644))
	r = runMain(t, append([]string{"configure", "--config-path", configureCfg, "--disable-batch", "--defer-deploy"}, basicAuth(mock)...)...)
	require.Equal(t, 0, r.code, r.stderr)
	assert.Equal(t, 0, mock.Count("POST /api/v1/DeployIntegrationDesigntimeArtifact"), "deferred")

	data, err := os.ReadFile(filepath.Join(repo, ".cpi", "pending-deploy.json"))
	require.NoError(t, err)
	var pending pendingDeploy
	require.NoError(t, json.Unmarshal(data, &pending))
	require.Len(t, pending.Artifacts, 4)
	assert.True(t, pending.Artifacts["Changed"].Force)
	assert.True(t, pending.Artifacts["Param"].Force)
	assert.True(t, pending.Artifacts["Both"].Force)
	assert.Len(t, pending.Artifacts["Both"].Reasons, 2)
	assert.False(t, pending.Artifacts["Same"].Force)

	// the plan, then the deployment
	r = runMain(t, append([]string{"deploy", "--pending", "--plan", "--output", "json"}, basicAuth(mock)...)...)
	require.Equal(t, 0, r.code, r.stderr)
	assert.Equal(t, 0, mock.Count("POST /api/v1/DeployIntegrationDesigntimeArtifact"))
	assert.Contains(t, r.stdout, `running 1.0.5 already (STARTED)`)

	r = runMain(t, append([]string{"deploy", "--pending", "--delay-length", "0", "--output", "json"}, basicAuth(mock)...)...)
	require.Equal(t, 0, r.code, r.stderr)
	for _, id := range []string{"Changed", "Param", "Both"} {
		assert.Equal(t, 1, mock.Artifacts[id].Deploys, "%s deployed once", id)
	}
	assert.Equal(t, 0, mock.Artifacts["Same"].Deploys, "nothing changed")
	assert.True(t, strings.Contains(r.stdout, `"SKIPPED"`))
	assert.NoFileExists(t, filepath.Join(repo, ".cpi", "pending-deploy.json"), "all done")

	// nothing pending: nothing to do
	r = runMain(t, append([]string{"deploy", "--pending"}, basicAuth(mock)...)...)
	assert.Equal(t, 0, r.code, r.stderr)
}

// A pending artifact that is in draft on the tenant by the time of deploy
// --pending is not deployed and stays in the file for the next run.
func TestPendingDraftStays(t *testing.T) {
	mock := cpitest.NewTenant(t, map[string]*cpitest.Artifact{
		"Ok":    {Type: "Integration", DesignVersion: "1.0.6", Package: "Pkg", Runtime: &cpitest.Runtime{Version: "1.0.6", Status: "STARTED"}},
		"Draft": {Type: "Integration", DesignVersion: "Active", Package: "Pkg", Runtime: &cpitest.Runtime{Version: "1.0.5", Status: "STARTED"}},
	})
	dir := t.TempDir()
	t.Chdir(dir)
	host, port := mock.HostPort()
	p := &pendingDeploy{Format: 1, Tenant: cpi.TenantID("http://" + host + ":" + strconv.Itoa(port)), Artifacts: map[string]*pendingArtifact{
		"Ok":    {Type: "IntegrationFlow", Package: "Pkg", Reasons: []string{"test"}},
		"Draft": {Type: "IntegrationFlow", Package: "Pkg", Reasons: []string{"test"}, Seq: 1},
	}}
	path := filepath.Join(dir, ".cpi", "pending-deploy.json")
	require.NoError(t, p.save(path))

	r := runMain(t, append([]string{"deploy", "--pending", "--plan", "--output", "json"}, basicAuth(mock)...)...)
	require.Equal(t, 0, r.code, r.stderr)
	assert.Contains(t, r.stdout, "draft on the tenant")

	r = runMain(t, append([]string{"deploy", "--pending", "--delay-length", "0", "--output", "json"}, basicAuth(mock)...)...)
	require.Equal(t, 0, r.code, r.stderr)
	assert.Equal(t, 0, mock.Artifacts["Draft"].Deploys)
	assert.Contains(t, r.stdout, `"skipped": "draft"`)
	left, err := loadPending(path)
	require.NoError(t, err)
	assert.Len(t, left.Artifacts, 1)
	assert.NotNil(t, left.Artifacts["Draft"], "deployed by a later run")
}
