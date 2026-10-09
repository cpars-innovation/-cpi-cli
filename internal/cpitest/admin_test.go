package cpitest_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/cpars-innovation/cpicli/internal/cpitest"
	"github.com/cpars-innovation/cpicli/pkg/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func admin(t *testing.T, m *cpitest.Tenant, method, path string, body any, out any) int {
	t.Helper()
	var b bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&b).Encode(body))
	}
	req, err := http.NewRequest(method, m.URL()+"/_mock"+path, &b)
	require.NoError(t, err)
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	if out != nil {
		require.NoError(t, json.NewDecoder(res.Body).Decode(out))
	}
	return res.StatusCode
}

func TestAdminSystemsAndRun(t *testing.T) {
	m := cpitest.NewTenant(t, nil)
	require.NoError(t, cpitest.SeedDemo(m, "dev", time.Now()))
	var systems map[string]cpitest.SystemSpec
	require.Equal(t, http.StatusOK, admin(t, m, http.MethodGet, "/systems", nil, &systems))
	require.Contains(t, systems, "finance")

	finance := systems["finance"]
	finance.FailRate, finance.Error = 1, "finance down for {key}"
	require.Equal(t, http.StatusOK, admin(t, m, http.MethodPut, "/systems/finance", finance, nil))
	var out struct {
		Runs []map[string]string `json:"runs"`
	}
	require.Equal(t, http.StatusOK, admin(t, m, http.MethodPost, "/run", map[string]any{"artifact": "Billing_Post", "key": "4711", "count": 2}, &out))
	require.Len(t, out.Runs, 2)
	assert.Equal(t, "FAILED", out.Runs[0]["status"])
	assert.Equal(t, "finance down for 4711", out.Runs[0]["error"])

	assert.Equal(t, http.StatusBadRequest, admin(t, m, http.MethodPost, "/run", map[string]any{"artifact": "Nope"}, nil))
	assert.Equal(t, http.StatusBadRequest, admin(t, m, http.MethodPut, "/systems/x", cpitest.SystemSpec{FailRate: 2}, nil))
	assert.Equal(t, http.StatusNotFound, admin(t, m, http.MethodGet, "/nothing", nil, nil))
}

func TestAdminFaultsAndLogs(t *testing.T) {
	m := cpitest.NewTenant(t, nil)
	require.NoError(t, cpitest.SeedDemo(m, "test", time.Now()))
	exe := m.Executer()

	require.Equal(t, http.StatusOK, admin(t, m, http.MethodPost, "/faults", map[string]any{"pathPrefix": "/api/v1/IntegrationPackages", "status": 503, "count": 1}, nil))
	_, err := ops.ListPackages(exe)
	require.Error(t, err, "one-shot fault")
	_, err = ops.ListPackages(exe)
	require.NoError(t, err, "the fault is used up")

	log := cpitest.MessageLog{Guid: "AGX1", Artifact: "Orders_In", Status: "ESCALATED", CorrelationID: "C-x", ErrorText: "escalated by test"}
	require.Equal(t, http.StatusOK, admin(t, m, http.MethodPost, "/messagelogs", []cpitest.MessageLog{log}, nil))
	got, err := ops.QueryMessageLogs(exe, ops.MessageLogQuery{Statuses: []string{"ESCALATED"}})
	require.NoError(t, err)
	require.Len(t, got.Logs, 1)
	assert.Equal(t, "AGX1", got.Logs[0].MessageGuid)
}

func TestAdminStateExportImport(t *testing.T) {
	src := cpitest.NewTenant(t, nil)
	require.NoError(t, cpitest.SeedDemo(src, "prod", time.Now()))
	var state json.RawMessage
	require.Equal(t, http.StatusOK, admin(t, src, http.MethodGet, "/state", nil, &state))

	dst := cpitest.NewTenant(t, nil)
	require.Equal(t, http.StatusNoContent, admin(t, dst, http.MethodPut, "/state", state, nil))
	assert.Equal(t, src.Packages, dst.Packages)
	assert.Len(t, dst.MessageLogs(), len(src.MessageLogs()))
	assert.Equal(t, src.Artifacts["Billing_Post"].Parameters, dst.Artifacts["Billing_Post"].Parameters)
	pkgs, err := ops.ListPackages(dst.Executer())
	require.NoError(t, err)
	assert.Len(t, pkgs, 3)
}

// runOnce runs a flow through the admin API and returns the new logs of that flow.
func runOnce(t *testing.T, m *cpitest.Tenant, artifact string) []cpitest.MessageLog {
	t.Helper()
	var out struct {
		Runs []map[string]string `json:"runs"`
	}
	require.Equal(t, http.StatusOK, admin(t, m, http.MethodPost, "/run", map[string]any{"artifact": artifact}, &out))
	var logs []cpitest.MessageLog
	for _, l := range m.MessageLogs() {
		for _, r := range out.Runs {
			if l.Guid == r["messageGuid"] {
				logs = append(logs, l)
			}
		}
	}
	return logs
}
