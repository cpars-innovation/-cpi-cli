package cmd

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/cpars-innovation/cpicli/internal/cpitest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParamsAndStatusCommands(t *testing.T) {
	a := &cpitest.Artifact{Type: "Integration", DesignVersion: "1", Package: "P", Parameters: map[string]string{"Host": "old"},
		Runtime: &cpitest.Runtime{Version: "1", Status: "STARTED"}}
	mock := cpitest.NewTenant(t, map[string]*cpitest.Artifact{"A": a})
	mock.Packages = []cpitest.Package{{ID: "P", Name: "Pkg"}}

	res := runMain(t, append([]string{"params", "set", "--artifact-id", "A", "--param", "Host=new", "--output", "json"}, basicAuth(mock)...)...)
	require.Equal(t, 0, res.code, res.stderr)
	assert.Equal(t, "new", a.Parameters["Host"])

	res = runMain(t, append([]string{"params", "set", "--artifact-id", "A", "--param", "Typo=x", "--output", "json"}, basicAuth(mock)...)...)
	assert.Equal(t, 2, res.code)

	res = runMain(t, append([]string{"status", "--artifact-ids", "A", "--output", "json"}, basicAuth(mock)...)...)
	require.Equal(t, 0, res.code, res.stderr)
	var env struct {
		Result struct {
			Artifacts []struct {
				ID       string `json:"id"`
				Deployed bool   `json:"deployed"`
				Status   string `json:"status"`
			} `json:"artifacts"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal([]byte(res.stdout), &env))
	assert.Equal(t, "STARTED", env.Result.Artifacts[0].Status)

	res = runMain(t, append([]string{"artifacts", "--package-id", "P", "--output", "json"}, basicAuth(mock)...)...)
	require.Equal(t, 0, res.code, res.stderr)
	assert.Contains(t, res.stdout, `"id": "A"`)

	res = runMain(t, append([]string{"packages", "--output", "json"}, basicAuth(mock)...)...)
	require.Equal(t, 0, res.code, res.stderr)
	assert.Contains(t, res.stdout, `"name": "Pkg"`)
}

func TestLogsCommands(t *testing.T) {
	now := time.Now()
	mock := cpitest.NewTenant(t, nil)
	mock.MessageLogSteps = [][]cpitest.MessageLog{{
		{Guid: "g1", Artifact: "A", Status: "FAILED", Start: now, End: now, ErrorText: "boom\nstack"},
	}}
	res := runMain(t, append([]string{"logs", "--artifact-id", "A", "--since", "1d", "--errors", "--output", "json"}, basicAuth(mock)...)...)
	require.Equal(t, 0, res.code, res.stderr)
	assert.Contains(t, res.stdout, `"errorText": "boom\nstack"`)
	assert.Contains(t, mock.LastMessageLogQuery, "IntegrationFlowName")

	res = runMain(t, append([]string{"logs", "get", "--message-guid", "g1", "--output", "json"}, basicAuth(mock)...)...)
	require.Equal(t, 0, res.code, res.stderr)
	assert.Contains(t, res.stdout, `"messageGuid": "g1"`)
}
