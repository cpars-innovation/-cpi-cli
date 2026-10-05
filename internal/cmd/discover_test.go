package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/cpars-innovation/cpicli/internal/cpitest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// discover --dir works offline: no tenant settings needed.
func TestDiscoverDirOffline(t *testing.T) {
	dir := t.TempDir()
	flow := filepath.Join(dir, "Pkg", "Flow_A")
	require.NoError(t, os.MkdirAll(filepath.Join(flow, "META-INF"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(flow, "src/main/resources/scenarioflows/integrationflow"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(flow, "META-INF/MANIFEST.MF"), []byte("Bundle-SymbolicName: Flow_A\nSAP-BundleType: IntegrationFlow\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(flow, "src/main/resources/scenarioflows/integrationflow/Flow_A.iflw"), []byte("<definitions/>"), 0o644))
	out := filepath.Join(dir, "out", "discovery.json")

	r := runMain(t, "discover", "--dir", dir, "--output-file", out, "--output", "json")
	require.Equal(t, 0, r.code, r.stderr)
	var env struct {
		Ok     bool
		Result struct{ File string }
	}
	require.NoError(t, json.Unmarshal([]byte(r.stdout), &env))
	assert.True(t, env.Ok)
	assert.Equal(t, out, env.Result.File)
	_, err := os.Stat(out)
	require.NoError(t, err)

	// without --dir a tenant is required
	r = runMain(t, "discover", "--output-file", out)
	assert.Equal(t, 2, r.code)
	assert.Contains(t, r.stderr, "no tenant configured")
}

func TestPackagesCreateAndSend(t *testing.T) {
	mock := cpitest.NewTenant(t, map[string]*cpitest.Artifact{"Orders": {Type: "Integration", DesignVersion: "1"}})
	host, port := mock.HostPort()
	mock.Artifacts["Orders"].EndpointURL = fmt.Sprintf("http://%s:%d/http/orders", host, port)
	mock.Inbound = map[string]*cpitest.Inbound{"/http/orders": {MessageGuid: "G1", Status: 500}}

	_, _, err := runCLI(t, mock, "packages", "create", "--package-id", "Orders")
	require.NoError(t, err)
	assert.Equal(t, 1, mock.Count("POST /api/v1/IntegrationPackages"))

	r := runMain(t, append([]string{"send", "--artifact-id", "Orders", "--body", "x", "--header", "X-A=1", "--output", "json"}, basicAuth(mock)...)...)
	var env struct {
		ExitCode int
		Result   struct{ MessageGuid string }
	}
	require.NoError(t, json.Unmarshal([]byte(r.stdout), &env))
	assert.Equal(t, 5, r.code)
	assert.Equal(t, 5, env.ExitCode)
	assert.Equal(t, "G1", env.Result.MessageGuid)
	require.Len(t, mock.Received, 1)
	assert.Equal(t, "1", mock.Received[0].Header.Get("X-A"))
	// without runtime credentials the API credentials are used
	assert.Contains(t, mock.Received[0].Header.Get("Authorization"), "Basic ")
}
