package cmd

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/cpars-innovation/cpicli/internal/cpitest"
	"github.com/cpars-innovation/cpicli/pkg/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoctor(t *testing.T) {
	mock := cpitest.NewTenant(t, map[string]*cpitest.Artifact{})
	mock.ForbidPaths = []string{"/api/v1/UserCredentials", "/api/v1/KeystoreEntries"}

	r := runMain(t, append([]string{"doctor", "--output", "json"}, basicAuth(mock)...)...)
	require.Equal(t, 0, r.code, r.stderr)
	var env struct {
		Result doctorResult `json:"result"`
	}
	require.NoError(t, json.Unmarshal([]byte(r.stdout), &env))
	status := map[string]string{}
	for _, c := range env.Result.Tenant.Checks {
		status[c.Name] = c.Status
	}
	assert.True(t, env.Result.Tenant.OK)
	assert.Equal(t, ops.CheckOK, status["designtime"])
	assert.Equal(t, ops.CheckOK, status["runtime"])
	assert.Equal(t, ops.CheckForbidden, status["security material"])
	assert.Equal(t, ops.CheckForbidden, status["keystore"])
	assert.NotEmpty(t, env.Result.Local)
	for _, req := range mock.Requests() {
		assert.Regexp(t, `^GET `, req, "doctor only reads")
	}

	// wrong credentials: exit code 3
	mock.StatusOverride = http.StatusUnauthorized
	r = runMain(t, append([]string{"doctor"}, basicAuth(mock)...)...)
	assert.Equal(t, 3, r.code, r.stderr)

	// designtime forbidden: the tenant cannot be used (3: roles)
	mock.StatusOverride = 0
	mock.ForbidPaths = []string{"/api/v1/IntegrationPackages"}
	r = runMain(t, append([]string{"doctor"}, basicAuth(mock)...)...)
	assert.Equal(t, 3, r.code, r.stderr)

	// no connection: exit code 4
	r = runMain(t, "doctor", "--tmn-host", "http://127.0.0.1:1", "--tmn-userid", "u", "--tmn-password", "p", "--read-retries", "0")
	assert.Equal(t, 4, r.code, r.stderr)

	// no host: usage
	r = runMain(t, "doctor")
	assert.Equal(t, 2, r.code, r.stderr)
}
