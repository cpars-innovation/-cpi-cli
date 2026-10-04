package deployer

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/cpars-innovation/cpicli/internal/api"
	"github.com/cpars-innovation/cpicli/internal/cpitest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type rtArtifact = api.RuntimeArtifact

var (
	t0 = time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	t1 = t0.Add(time.Hour)
)

func fastOpts() Options {
	return Options{Interval: time.Millisecond, MaxChecks: 5, Parallelism: 0}
}

func run(t *testing.T, artifacts map[string]*cpitest.Artifact, ids []string, opts Options) ([]Result, *cpitest.Tenant) {
	t.Helper()
	mock := cpitest.NewTenant(t, artifacts)
	var list []Artifact
	for _, id := range ids {
		list = append(list, Artifact{ID: id, Type: "Integration"})
	}
	return Deploy(context.Background(), NewTenant(mock.Executer()), list, opts), mock
}

func TestDeploy_PollsTaskStatusThenRuntime(t *testing.T) {
	results, mock := run(t, map[string]*cpitest.Artifact{
		"A": {
			Type: "Integration", DesignVersion: "1.0.1",
			TaskStatuses: []string{"DEPLOYING", "SUCCESS"},
			AfterDeploy:  []*cpitest.Runtime{{Version: "1.0.1", Status: "STARTING"}, {Version: "1.0.1", Status: "STARTED", DeployedOn: t1}},
		},
	}, []string{"A"}, fastOpts())

	require.Len(t, results, 1)
	assert.Equal(t, Result{ID: "A", Type: "Integration", TaskID: "task-A", Status: StatusDeployed, Version: "1.0.1"}, results[0])
	assert.Equal(t, 2, mock.Count("GET /api/v1/BuildAndDeployStatus"))
	assert.NoError(t, Err(results))
}

// TestDeploy_SameVersionRedeployIsNotFalseSuccess reproduces the race where the
// previous STARTED runtime artifact (same version) is still visible after the
// redeploy was triggered. It must not be reported as deployed until DeployedOn
// moves past the previous deployment.
func TestDeploy_SameVersionRedeployIsNotFalseSuccess(t *testing.T) {
	old := &cpitest.Runtime{Version: "1.0.0", Status: "STARTED", DeployedOn: t0}
	newer := &cpitest.Runtime{Version: "1.0.0", Status: "STARTED", DeployedOn: t1}

	t.Run("waits for the fresh deployment", func(t *testing.T) {
		results, mock := run(t, map[string]*cpitest.Artifact{
			"A": {Type: "Integration", DesignVersion: "1.0.0", Runtime: old,
				TaskStatuses: []string{"SUCCESS"},
				AfterDeploy:  []*cpitest.Runtime{old, old, newer}},
		}, []string{"A"}, fastOpts())
		assert.Equal(t, StatusDeployed, results[0].Status)
		// 1 baseline read + 3 polls (2 stale)
		assert.Equal(t, 4, mock.Count("GET /api/v1/IntegrationRuntimeArtifacts('A')"))
	})

	t.Run("times out if only the old deployment is visible", func(t *testing.T) {
		results, _ := run(t, map[string]*cpitest.Artifact{
			"A": {Type: "Integration", DesignVersion: "1.0.0", Runtime: old,
				AfterDeploy: []*cpitest.Runtime{old}},
		}, []string{"A"}, fastOpts())
		assert.Equal(t, StatusTimeout, results[0].Status)
		assert.Contains(t, results[0].Error, "remained unfinished after 5 checks")
	})

	t.Run("a stale ERROR from the previous deployment is not a failure", func(t *testing.T) {
		oldErr := &cpitest.Runtime{Version: "1.0.0", Status: "ERROR", DeployedOn: t0}
		results, _ := run(t, map[string]*cpitest.Artifact{
			"A": {Type: "Integration", DesignVersion: "1.0.0", Runtime: oldErr,
				AfterDeploy: []*cpitest.Runtime{oldErr, newer}},
		}, []string{"A"}, fastOpts())
		assert.Equal(t, StatusDeployed, results[0].Status)
	})
}

func TestDeploy_TaskFailure(t *testing.T) {
	results, _ := run(t, map[string]*cpitest.Artifact{
		"A": {Type: "Integration", DesignVersion: "1", TaskStatuses: []string{"DEPLOYING", "FAIL"}, ErrorInfo: "boom"},
	}, []string{"A"}, fastOpts())
	assert.Equal(t, StatusFailed, results[0].Status)
	assert.Contains(t, results[0].Error, "ended with status FAIL")
	assert.Contains(t, results[0].Error, "boom")
}

func TestDeploy_RuntimeError(t *testing.T) {
	results, _ := run(t, map[string]*cpitest.Artifact{
		"A": {Type: "Integration", DesignVersion: "1", ErrorInfo: "invalid mapping",
			AfterDeploy: []*cpitest.Runtime{{Version: "1", Status: "ERROR", DeployedOn: t1}}},
	}, []string{"A"}, fastOpts())
	assert.Equal(t, StatusFailed, results[0].Status)
	assert.Contains(t, results[0].Error, "invalid mapping")
}

func TestDeploy_FallsBackToRuntimeWhenTaskStatusUnavailable(t *testing.T) {
	results, mock := run(t, map[string]*cpitest.Artifact{
		"A": {Type: "Integration", DesignVersion: "1", // no TaskStatuses: endpoint returns 404
			AfterDeploy: []*cpitest.Runtime{{Version: "1", Status: "STARTED", DeployedOn: t1}}},
	}, []string{"A"}, fastOpts())
	assert.Equal(t, StatusDeployed, results[0].Status)
	assert.Equal(t, 1, mock.Count("GET /api/v1/BuildAndDeployStatus"))
}

func TestDeploy_CompareVersionsSkips(t *testing.T) {
	opts := fastOpts()
	opts.CompareVersions = true
	results, mock := run(t, map[string]*cpitest.Artifact{
		"A": {Type: "Integration", DesignVersion: "2", Runtime: &cpitest.Runtime{Version: "2", Status: "STARTED", DeployedOn: t0}},
	}, []string{"A"}, opts)
	assert.Equal(t, StatusSkipped, results[0].Status)
	assert.Zero(t, mock.Count("POST "))
}

func TestDeploy_MultipleArtifactsKeepOrderAndIsolateFailures(t *testing.T) {
	started := []*cpitest.Runtime{{Version: "1", Status: "STARTED", DeployedOn: t1}}
	results, _ := run(t, map[string]*cpitest.Artifact{
		"A": {Type: "Integration", DesignVersion: "1", AfterDeploy: started},
		"B": {Type: "Integration"}, // designtime missing
		"C": {Type: "Integration", DesignVersion: "1", DeployStatusCode: http.StatusInternalServerError},
		"D": {Type: "Integration", DesignVersion: "1", AfterDeploy: started},
	}, []string{"A", "B", "C", "D"}, fastOpts())

	require.Len(t, results, 4)
	var got []string
	for _, r := range results {
		got = append(got, r.ID+"="+string(r.Status))
	}
	assert.Equal(t, []string{"A=DEPLOYED", "B=FAILED", "C=FAILED", "D=DEPLOYED"}, got)
	assert.Contains(t, results[1].Error, "does not exist")

	var depErr *Error
	require.ErrorAs(t, Err(results), &depErr)
	assert.Len(t, depErr.Failed(), 2)
}

func TestDeploy_AuthErrorFailsFast(t *testing.T) {
	mock := cpitest.NewTenant(t, map[string]*cpitest.Artifact{})
	mock.StatusOverride = http.StatusUnauthorized
	results := Deploy(context.Background(), NewTenant(mock.Executer()), []Artifact{{ID: "A", Type: "Integration"}}, fastOpts())
	assert.Equal(t, StatusFailed, results[0].Status)
	assert.Contains(t, results[0].Error, "401")
}

func TestDeploy_InvalidType(t *testing.T) {
	mock := cpitest.NewTenant(t, map[string]*cpitest.Artifact{})
	results := Deploy(context.Background(), NewTenant(mock.Executer()), []Artifact{{ID: "A", Type: "Bogus"}}, fastOpts())
	assert.Equal(t, StatusFailed, results[0].Status)
	assert.Empty(t, mock.Requests())
}

func TestIsFresh(t *testing.T) {
	before := &rtArtifact{Version: "1", Status: "STARTED", DeployedOn: t0}
	now := t1
	cases := []struct {
		name          string
		rt, before    *rtArtifact
		taskConfirmed bool
		want          bool
	}{
		{"no previous deployment", &rtArtifact{Version: "1", DeployedOn: t0}, nil, false, true},
		{"newer DeployedOn", &rtArtifact{Version: "1", DeployedOn: t1}, before, false, true},
		{"same DeployedOn", &rtArtifact{Version: "1", DeployedOn: t0}, before, false, false},
		{"no baseline timestamp, after trigger", &rtArtifact{Version: "1", DeployedOn: now.Add(-time.Minute)}, &rtArtifact{Version: "1"}, false, true},
		{"no baseline timestamp, long before trigger", &rtArtifact{Version: "1", DeployedOn: t0}, &rtArtifact{Version: "1"}, false, false},
		{"no timestamps, version changed", &rtArtifact{Version: "2"}, &rtArtifact{Version: "1"}, false, true},
		{"no timestamps, same version, task confirmed", &rtArtifact{Version: "1"}, &rtArtifact{Version: "1"}, true, true},
		{"no timestamps, same version", &rtArtifact{Version: "1"}, &rtArtifact{Version: "1"}, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, isFresh(c.rt, c.before, now, 2*time.Minute, c.taskConfirmed))
		})
	}
}

func TestUndeploy(t *testing.T) {
	deployed := &cpitest.Runtime{Version: "1", Status: "STARTED", DeployedOn: t0}
	mock := cpitest.NewTenant(t, map[string]*cpitest.Artifact{
		"A": {Runtime: deployed, UndeployAfter: 2},
		"B": {}, // not deployed
		"C": {Runtime: deployed, UndeployAfter: -1},
	})
	results := Undeploy(context.Background(), NewTenant(mock.Executer()),
		[]Artifact{{ID: "A"}, {ID: "B"}, {ID: "C"}}, fastOpts())

	assert.Equal(t, StatusUndeployed, results[0].Status)
	assert.Equal(t, "1", results[0].Version)
	assert.Equal(t, StatusNotDeployed, results[1].Status)
	assert.Equal(t, StatusTimeout, results[2].Status)
	assert.Equal(t, 0, mock.Count("DELETE /api/v1/IntegrationRuntimeArtifacts('B')"))
	assert.Equal(t, 1, mock.Count("DELETE /api/v1/IntegrationRuntimeArtifacts('A')"))
}

func TestDeploy_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	mock := cpitest.NewTenant(t, map[string]*cpitest.Artifact{
		"A": {Type: "Integration", DesignVersion: "1", AfterDeploy: []*cpitest.Runtime{{Status: "STARTING"}}},
	})
	opts := fastOpts()
	opts.Interval = time.Hour
	results := Deploy(ctx, NewTenant(mock.Executer()), []Artifact{{ID: "A", Type: "Integration"}}, opts)
	assert.Equal(t, StatusFailed, results[0].Status)
	assert.Contains(t, results[0].Error, "context canceled")
}
