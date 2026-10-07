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

// Artifacts without parameters (script collections, flows that only need a
// deployment) are not read from the configuration API; they are deployed
// unless the runtime already has their designtime version.
func TestConfigureArtifactsWithoutParameters(t *testing.T) {
	deployed := func(v string) []*cpitest.Runtime {
		return []*cpitest.Runtime{{Version: v, Status: "STARTED", DeployedOn: time.Now().Add(-time.Hour)}}
	}
	// the runtime after the deployment must be newer than the trigger
	redeployed := func(v string) []*cpitest.Runtime {
		return []*cpitest.Runtime{{Version: v, Status: "STARTED", DeployedOn: time.Now().Add(time.Minute)}}
	}
	mock := cpitest.NewTenant(t, map[string]*cpitest.Artifact{
		// script collection, not deployed yet
		"SC": {Type: "ScriptCollection", DesignVersion: "1.0.1", TaskStatuses: []string{"SUCCESS"}, AfterDeploy: redeployed("1.0.1")},
		// flow without parameters, runtime already on the designtime version
		"Same": {Type: "Integration", DesignVersion: "1.0.0", Runtime: deployed("1.0.0")[0]},
		// unchanged parameters but a newer designtime version (uploaded before)
		"Newer": {Type: "Integration", DesignVersion: "1.0.2", Parameters: map[string]string{"Host": "h"}, Runtime: deployed("1.0.1")[0],
			TaskStatuses: []string{"SUCCESS"}, AfterDeploy: redeployed("1.0.2")},
	})
	path := filepath.Join(t.TempDir(), "dev.yml")
	require.NoError(t, os.WriteFile(path, []byte(`packages:
  - integrationSuiteId: Utilities
    deploy: false
    artifacts:
      - artifactId: SC
        type: ScriptCollection
        deploy: true
      - artifactId: Same
        type: Integration
        deploy: true
        parameters: []
      - artifactId: Newer
        type: Integration
        deploy: true
        parameters:
          - {key: Host, value: h}
`), 0o644))

	r := runMain(t, append([]string{"configure", "--config-path", path, "--disable-batch", "--deploy-delay", "1"}, basicAuth(mock)...)...)
	require.Equal(t, 0, r.code, r.stderr)
	assert.Equal(t, 0, mock.Count("GET /api/v1/IntegrationDesigntimeArtifacts(Id='SC',Version='active')/Configurations"), "no configuration read for a script collection")
	assert.Equal(t, 0, mock.Count("GET /api/v1/IntegrationDesigntimeArtifacts(Id='Same',Version='active')/Configurations"), "no configuration read without parameters")
	assert.Equal(t, 1, mock.Count("GET /api/v1/IntegrationDesigntimeArtifacts(Id='Newer',Version='active')/Configurations"), "the URL the assertions above use")
	assert.Equal(t, 0, mock.Count("PUT "))
	assert.Equal(t, 1, mock.Count("POST /api/v1/DeployScriptCollectionDesigntimeArtifact"))
	assert.Equal(t, 1, mock.Count("POST /api/v1/DeployIntegrationDesigntimeArtifact"), "only Newer: Same already runs its version")

	// parameters on a script collection are a configuration error, reported before anything is written
	require.NoError(t, os.WriteFile(path, []byte(`packages:
  - integrationSuiteId: Utilities
    artifacts:
      - artifactId: SC
        type: ScriptCollection
        deploy: true
        parameters:
          - {key: X, value: y}
`), 0o644))
	r = runMain(t, append([]string{"configure", "--config-path", path, "--disable-batch"}, basicAuth(mock)...)...)
	assert.NotEqual(t, 0, r.code)
	assert.Contains(t, r.stderr, "no configurable parameters")
	assert.Equal(t, 1, mock.Count("POST /api/v1/DeployScriptCollectionDesigntimeArtifact"), "nothing deployed")
}

// The ENBW case: parameters already on the tenant, designtime 1.0.13 with the
// newer content, runtime 1.0.15 from a manual deployment of an older build.
func TestConfigureDowngrade(t *testing.T) {
	deployedAt := time.Now().Add(-2 * time.Hour)
	running := func() *cpitest.Runtime {
		return &cpitest.Runtime{Version: "1.0.15", Status: "STARTED", DeployedOn: deployedAt}
	}
	redeployed := []*cpitest.Runtime{{Version: "1.0.13", Status: "STARTED", DeployedOn: time.Now().Add(time.Minute)}}
	flow := func(modified time.Time) *cpitest.Artifact {
		return &cpitest.Artifact{Type: "Integration", DesignVersion: "1.0.13", Parameters: map[string]string{"Host": "h"},
			Runtime: running(), ModifiedAt: modified, TaskStatuses: []string{"SUCCESS"}, AfterDeploy: redeployed}
	}
	write := func(body string) string {
		path := filepath.Join(t.TempDir(), "dev.yml")
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
		return path
	}
	runConfig := func(mock *cpitest.Tenant, path string, extra ...string) cliRun {
		args := append([]string{"configure", "--config-path", path, "--disable-batch", "--deploy-delay", "1", "--output", "json"}, extra...)
		return runMain(t, append(args, basicAuth(mock)...)...)
	}
	results := func(r cliRun) map[string]map[string]any {
		var env struct {
			Result struct{ Deployments []map[string]any }
		}
		require.NoError(t, json.Unmarshal([]byte(r.stdout), &env), r.stdout)
		m := map[string]map[string]any{}
		for _, d := range env.Result.Deployments {
			m[d["id"].(string)] = d
		}
		return m
	}

	t.Run("changed after the manual deployment: deployed by timestamp", func(t *testing.T) {
		mock := cpitest.NewTenant(t, map[string]*cpitest.Artifact{
			"Fixed": flow(time.Now().Add(-time.Hour)), // edited after the deployment
			"Stale": flow(deployedAt.Add(-time.Hour)), // not edited since
		})
		r := runConfig(mock, write(`packages:
  - integrationSuiteId: EDM
    artifacts:
      - {artifactId: Fixed, type: Integration, deploy: true, parameters: [{key: Host, value: h}]}
      - {artifactId: Stale, type: Integration, deploy: true, parameters: [{key: Host, value: h}]}
`))
		assert.Equal(t, 7, r.code, "partial: one deployed, one refused")
		res := results(r)
		assert.Equal(t, "DEPLOYED", res["Fixed"]["status"])
		assert.Equal(t, "modified after deployment", res["Fixed"]["rule"])
		assert.Equal(t, "FAILED", res["Stale"]["status"])
		assert.Equal(t, "version", res["Stale"]["rule"])
		assert.Contains(t, res["Stale"]["error"], "allowDowngrade: true")
		assert.Contains(t, r.stderr, "[rule: modified after deployment: designtime 1.0.13 lower than running 1.0.15", "rule and reason are on the artifact's log line")
	})

	t.Run("allowDowngrade in the file: artifact wins over package", func(t *testing.T) {
		old := deployedAt.Add(-time.Hour)
		mock := cpitest.NewTenant(t, map[string]*cpitest.Artifact{"A": flow(old), "B": flow(old), "C": flow(old)})
		r := runConfig(mock, write(`packages:
  - integrationSuiteId: EDM
    allowDowngrade: true
    artifacts:
      - {artifactId: A, type: Integration, deploy: true, parameters: [{key: Host, value: h}]}
      - {artifactId: B, type: Integration, deploy: true, allowDowngrade: false, parameters: [{key: Host, value: h}]}
  - integrationSuiteId: Other
    artifacts:
      - {artifactId: C, type: Integration, deploy: true, allowDowngrade: true}
`))
		res := results(r)
		assert.Equal(t, "DEPLOYED", res["A"]["status"], "package allowDowngrade")
		assert.Equal(t, "allowDowngrade", res["A"]["rule"])
		assert.Equal(t, "FAILED", res["B"]["status"], "artifact false wins")
		assert.Equal(t, "DEPLOYED", res["C"]["status"], "artifact allowDowngrade")

		// the global flag covers artifacts without a setting, not B
		mock = cpitest.NewTenant(t, map[string]*cpitest.Artifact{"A": flow(old), "B": flow(old), "C": flow(old)})
		r = runConfig(mock, write(`packages:
  - integrationSuiteId: EDM
    artifacts:
      - {artifactId: A, type: Integration, deploy: true, parameters: [{key: Host, value: h}]}
      - {artifactId: B, type: Integration, deploy: true, allowDowngrade: false, parameters: [{key: Host, value: h}]}
`), "--allow-downgrade")
		res = results(r)
		assert.Equal(t, "DEPLOYED", res["A"]["status"])
		assert.Equal(t, "FAILED", res["B"]["status"])
	})

	t.Run("writing parameters does not count as a newer content", func(t *testing.T) {
		a := flow(deployedAt.Add(-time.Hour))
		a.ConfigBumpsModified = true // as if the tenant updated ModifiedAt on the parameter write
		mock := cpitest.NewTenant(t, map[string]*cpitest.Artifact{"A": a})
		r := runConfig(mock, write(`packages:
  - integrationSuiteId: EDM
    artifacts:
      - {artifactId: A, type: Integration, deploy: true, parameters: [{key: Host, value: new}]}
`))
		assert.Equal(t, 1, mock.Count("PUT "), "the parameter is written")
		res := results(r)
		assert.Equal(t, "FAILED", res["A"]["status"], "the time from before the write decides")
		assert.Equal(t, 0, mock.Count("POST /api/v1/DeployIntegrationDesigntimeArtifact"))
	})
}

// --versioning comes from the pipeline; the file may override it per
// artifact or package.
func TestConfigureVersioning(t *testing.T) {
	deployedAt := time.Now().Add(-2 * time.Hour)
	flow := func() *cpitest.Artifact {
		return &cpitest.Artifact{Type: "Integration", DesignVersion: "1.0.13", Parameters: map[string]string{"Host": "h"},
			Runtime:      &cpitest.Runtime{Version: "1.0.15", Status: "STARTED", DeployedOn: deployedAt},
			ModifiedAt:   time.Now().Add(-time.Hour), // would pass the timestamp rule without a mode
			TaskStatuses: []string{"SUCCESS"}, AfterDeploy: []*cpitest.Runtime{{Version: "1.0.13", Status: "STARTED", DeployedOn: time.Now().Add(time.Minute)}}}
	}
	mock := cpitest.NewTenant(t, map[string]*cpitest.Artifact{"Strict": flow(), "Loose": flow(), "Bad": flow()})
	path := filepath.Join(t.TempDir(), "dev.yml")
	require.NoError(t, os.WriteFile(path, []byte(`packages:
  - integrationSuiteId: EDM
    artifacts:
      - {artifactId: Strict, type: Integration, deploy: true, parameters: [{key: Host, value: h}]}
      - {artifactId: Loose, type: Integration, deploy: true, versioning: keep, parameters: [{key: Host, value: h}]}
      - {artifactId: Bad, type: Integration, deploy: true, versioning: sometimes}
`), 0o644))
	r := runMain(t, append([]string{"configure", "--config-path", path, "--disable-batch", "--deploy-delay", "1", "--versioning", "manifest", "--output", "json"}, basicAuth(mock)...)...)
	var env struct {
		Result struct{ Deployments []map[string]any }
	}
	require.NoError(t, json.Unmarshal([]byte(r.stdout), &env), r.stdout)
	res := map[string]map[string]any{}
	for _, d := range env.Result.Deployments {
		res[d["id"].(string)] = d
	}
	assert.Equal(t, "FAILED", res["Strict"]["status"], "manifest: the version decides, not the timestamps")
	assert.Equal(t, "manifest", res["Strict"]["versioning"])
	assert.Equal(t, "DEPLOYED", res["Loose"]["status"], "keep on the artifact wins")
	assert.Equal(t, "keep", res["Loose"]["rule"])
	assert.NotContains(t, res, "Bad")
	assert.Contains(t, r.stderr, `invalid versioning \"sometimes\"`)

	r = runMain(t, append([]string{"configure", "--config-path", path, "--versioning", "always"}, basicAuth(mock)...)...)
	assert.Equal(t, 2, r.code)
}

// --plan (= --dry-run) says which artifacts would be deployed and why.
func TestConfigurePlanPredictsDeployments(t *testing.T) {
	mock := cpitest.NewTenant(t, map[string]*cpitest.Artifact{
		"A": {Type: "Integration", DesignVersion: "1.0.0", Parameters: map[string]string{"Host": "old"},
			Runtime: &cpitest.Runtime{Version: "1.0.0", Status: "STARTED"}},
		"B": {Type: "Integration", DesignVersion: "1.0.0", Parameters: map[string]string{"Host": "same"},
			Runtime: &cpitest.Runtime{Version: "1.0.0", Status: "STARTED"}},
		"C": {Type: "Integration", DesignVersion: "1.0.1", Parameters: map[string]string{"Host": "same"},
			Runtime: &cpitest.Runtime{Version: "1.0.0", Status: "STARTED"}},
	})
	path := filepath.Join(t.TempDir(), "dev.yml")
	require.NoError(t, os.WriteFile(path, []byte(`packages:
  - integrationSuiteId: Orders
    deploy: true
    artifacts:
      - {artifactId: A, type: Integration, parameters: [{key: Host, value: new}]}
      - {artifactId: B, type: Integration, parameters: [{key: Host, value: same}]}
      - {artifactId: C, type: Integration, parameters: [{key: Host, value: same}]}
`), 0o644))
	r := runMain(t, append([]string{"configure", "--config-path", path, "--plan", "--output", "json"}, basicAuth(mock)...)...)
	require.Equal(t, 0, r.code, r.stderr)
	for _, req := range mock.Requests() {
		assert.Regexp(t, `^GET `, req, "plan only reads")
	}
	var env struct {
		Result configureResult `json:"result"`
	}
	require.NoError(t, json.Unmarshal([]byte(r.stdout), &env))
	plan := map[string]PlanItem{}
	for _, it := range env.Result.Plan {
		plan[it.Artifact] = it
	}
	require.Len(t, plan, 3)
	assert.True(t, plan["A"].Deploy)
	assert.Equal(t, "configuration changed: deployed again", plan["A"].Reason)
	assert.False(t, plan["B"].Deploy, plan["B"].Reason)
	assert.True(t, plan["C"].Deploy)
	assert.Contains(t, plan["C"].Reason, "designtime 1.0.1, running 1.0.0")
}
