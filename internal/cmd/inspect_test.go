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

func TestContentCommands(t *testing.T) {
	now := time.Now()
	mock := cpitest.NewTenant(t, map[string]*cpitest.Artifact{
		"A": {Type: "Integration", DesignVersion: "1", Runtime: &cpitest.Runtime{Version: "1", Status: "ERROR"}, ErrorInfo: "boom",
			EndpointURL: "https://tenant/http/a",
			Resources:   map[string]cpitest.Resource{"s.groovy": {Type: "groovy", Content: []byte("println 1")}}},
	})
	mock.MessageLogSteps = [][]cpitest.MessageLog{{{Guid: "g1", Artifact: "A", Status: "FAILED", Start: now, End: now,
		Attachments: map[string]string{"in.xml": "<in/>"},
		Steps:       []cpitest.Step{{StepID: "s1", ModelStepID: "Mapping_1", Status: "FAILED", Error: "bad"}}}}}

	// text mode: resource content goes to stdout
	res := runMain(t, append([]string{"resources", "get", "--artifact-id", "A", "--name", "s.groovy", "--type", "groovy"}, basicAuth(mock)...)...)
	require.Equal(t, 0, res.code, res.stderr)
	assert.Equal(t, "println 1", res.stdout)

	out := filepath.Join(t.TempDir(), "in.xml")
	res = runMain(t, append([]string{"logs", "attachment", "--id", "att-g1-in.xml", "--out", out, "--output", "json"}, basicAuth(mock)...)...)
	require.Equal(t, 0, res.code, res.stderr)
	data, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Equal(t, "<in/>", string(data))

	res = runMain(t, append([]string{"logs", "steps", "--message-guid", "g1", "--output", "json"}, basicAuth(mock)...)...)
	require.Equal(t, 0, res.code, res.stderr)
	assert.Contains(t, res.stdout, `"modelStepId": "Mapping_1"`)

	res = runMain(t, append([]string{"status", "--runtime-status", "ERROR", "--output", "json"}, basicAuth(mock)...)...)
	require.Equal(t, 0, res.code, res.stderr)
	assert.Contains(t, res.stdout, `"errorInfo": "boom"`)

	res = runMain(t, append([]string{"endpoints", "--output", "json"}, basicAuth(mock)...)...)
	require.Equal(t, 0, res.code, res.stderr)
	assert.Contains(t, res.stdout, "https://tenant/http/a")

	res = runMain(t, append([]string{"validate", "--artifact-id", "A", "--output", "json"}, basicAuth(mock)...)...)
	require.Equal(t, 0, res.code, res.stderr)
	assert.Contains(t, res.stdout, `"status": "PASSED"`)
}
