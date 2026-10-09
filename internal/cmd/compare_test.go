package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/cpars-innovation/cpicli/internal/cpitest"
	"github.com/cpars-innovation/cpicli/pkg/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeProfile writes ~/.cpictl/<name>.yaml for a mock tenant (HOME is the
// test's).
func writeProfile(t *testing.T, name string, mock *cpitest.Tenant) {
	host, port := mock.HostPort()
	dir := filepath.Join(os.Getenv("HOME"), ".cpictl")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, name+".yaml"),
		[]byte("tmn-host: http://"+host+":"+strconv.Itoa(port)+"\ntmn-userid: user\ntmn-password: secret\n"), 0o600))
}

func TestCompareTiers(t *testing.T) {
	tenant := func(script, version string) *cpitest.Tenant {
		m := cpitest.NewTenant(t, map[string]*cpitest.Artifact{
			"Orders":  {Type: "Integration", DesignVersion: version, Package: "Pkg", Name: "Orders", Zip: namedFlowZip(t, "Orders", script)},
			"Billing": {Type: "Integration", DesignVersion: "1.0.0", Package: "Pkg", Name: "Billing", Zip: namedFlowZip(t, "Billing", "same")},
		})
		m.Packages = []cpitest.Package{{ID: "Pkg", Version: "1.0.0"}}
		return m
	}
	test, prod := tenant("new", "1.0.6"), tenant("old", "1.0.5")
	compare := func(code int, args ...string) (ops.CompareResult, cliRun) {
		t.Helper()
		r := runMainWith(t, func() {
			writeProfile(t, "test", test)
			writeProfile(t, "prod", prod)
		}, append([]string{"compare", "--output", "json"}, args...)...)
		require.Equal(t, code, r.code, r.stderr)
		var env struct {
			Result ops.CompareResult `json:"result"`
		}
		if r.stdout != "" {
			require.NoError(t, json.Unmarshal([]byte(r.stdout), &env), r.stdout)
		}
		return env.Result, r
	}
	res, _ := compare(0, "tenant:test", "tenant:prod", "--diff")
	assert.Equal(t, "test", res.A)
	assert.Equal(t, map[string]int{ops.CompareSame: 1, ops.CompareContentDiffers: 1}, res.Summary)
	assert.Equal(t, 0, test.Artifacts["Orders"].Uploads+prod.Artifacts["Orders"].Uploads, "read only")

	_, r := compare(5, "tenant:test", "tenant:prod", "--fail-on-diff", "--artifact", "Orders")
	assert.Contains(t, r.stderr, "1 artifact(s) differ")
	res, _ = compare(0, "tenant:test", "tenant:prod", "--fail-on-diff", "--artifact", "Billing")
	assert.Equal(t, 1, res.Summary[ops.CompareSame])

	// a local tree (as snapshot wrote it) against a tenant
	dir := t.TempDir()
	r = runMain(t, append([]string{"snapshot", "--dir-git-repo", dir, "--dir-work", t.TempDir(), "--git-skip-commit",
		"--sync-package-details=false"}, basicAuth(prod)...)...)
	require.Equal(t, 0, r.code, r.stderr)
	res, _ = compare(0, dir, "tenant:prod")
	assert.Equal(t, map[string]int{ops.CompareSame: 2}, res.Summary, "a snapshot equals its tenant")
	res, _ = compare(0, dir, "tenant:test")
	assert.Equal(t, ops.CompareContentDiffers, res.Items[1].Status)

	_, r = compare(2, "tenant:nope", "tenant:prod")
	assert.Contains(t, r.stderr, "nope")
	assert.Contains(t, r.stderr, "not found")
}

func TestMatrixCommand(t *testing.T) {
	tenant := func(version string) *cpitest.Tenant {
		m := cpitest.NewTenant(t, map[string]*cpitest.Artifact{
			"Orders": {Type: "Integration", DesignVersion: version, Package: "Pkg", Name: "Orders", Runtime: &cpitest.Runtime{Version: version, Status: "STARTED"}},
		})
		m.Packages = []cpitest.Package{{ID: "Pkg", Version: "1.0.0"}}
		return m
	}
	test, prod := tenant("1.0.6"), tenant("1.0.5")
	r := runMainWith(t, func() {
		writeProfile(t, "test", test)
		writeProfile(t, "prod", prod)
	}, "matrix", "TEST=tenant:test", "tenant:prod", "--output", "json")
	require.Equal(t, 0, r.code, r.stderr)
	var env struct {
		Result ops.VersionMatrix `json:"result"`
	}
	require.NoError(t, json.Unmarshal([]byte(r.stdout), &env))
	assert.Equal(t, []string{"TEST", "prod"}, env.Result.Tiers)
	require.Len(t, env.Result.Rows, 1)
	assert.Equal(t, []string{"prod"}, env.Result.Rows[0].Behind)

	r = runMain(t, "matrix", "git:main")
	assert.Equal(t, 2, r.code)
}
