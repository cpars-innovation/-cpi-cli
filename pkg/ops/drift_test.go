package ops

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cpars-innovation/cpicli/internal/cpitest"
	"github.com/cpars-innovation/cpicli/internal/exitcode"
	"github.com/cpars-innovation/cpicli/internal/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func flowFiles(id, version, script string) map[string]string {
	return map[string]string{
		"META-INF/MANIFEST.MF": "Bundle-SymbolicName: " + id + "\nBundle-Version: " + version + "\nSAP-BundleType: IntegrationFlow\n",
		"src/main/resources/scenarioflows/integrationflow/" + id + ".iflw": "<definitions/>",
		"src/main/resources/script/s.groovy":                               script,
	}
}

func TestDrift(t *testing.T) {
	root := t.TempDir()
	local := map[string]map[string]string{
		"Same":     flowFiles("Same", "1.0.0", "a"),
		"TenantUp": flowFiles("TenantUp", "1.0.0", "a"),
		"LocalUp":  flowFiles("LocalUp", "1.0.10", "new"),
		"Forked":   flowFiles("Forked", "1.0.0", "mine"),
		"NewFlow":  flowFiles("NewFlow", "1.0.0", "a"),
	}
	for id, files := range local {
		for name, content := range files {
			p := filepath.Join(root, "Orders", id, filepath.FromSlash(name))
			require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
			require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
		}
	}
	// the tenant copy of Same differs only in CR/LF and Origin headers
	sameTenant := flowFiles("Same", "1.0.0", "a\r\n")
	sameTenant["META-INF/MANIFEST.MF"] += "Origin-Bundle-Name: Same\n"
	mock := cpitest.NewTenant(t, map[string]*cpitest.Artifact{
		"Same":     {Type: "Integration", DesignVersion: "1.0.0", Zip: zipFiles(t, sameTenant), Runtime: &cpitest.Runtime{Version: "0.9.0", Status: "STARTED"}},
		"TenantUp": {Type: "Integration", DesignVersion: "1.0.1", Zip: zipFiles(t, flowFiles("TenantUp", "1.0.1", "edited on tenant"))},
		"LocalUp":  {Type: "Integration", DesignVersion: "1.0.9", Zip: zipFiles(t, flowFiles("LocalUp", "1.0.9", "a"))},
		"Forked":   {Type: "Integration", DesignVersion: "1.0.0", Zip: zipFiles(t, flowFiles("Forked", "1.0.0", "theirs"))},
	})

	res, err := Drift(context.Background(), mock.Executer(), root, "Orders")
	require.NoError(t, err)
	states := map[string]DriftItem{}
	for _, it := range res.Items {
		states[it.ArtifactID] = it
	}
	assert.Equal(t, DriftInSync, states["Same"].State)
	assert.True(t, states["Same"].RuntimeOutdated, "runtime 0.9.0, designtime 1.0.0")
	assert.Equal(t, DriftTenantNewer, states["TenantUp"].State)
	assert.Equal(t, DriftLocalNewer, states["LocalUp"].State, "1.0.10 > 1.0.9")
	assert.Equal(t, DriftDiverged, states["Forked"].State)
	assert.Equal(t, DriftNotOnTenant, states["NewFlow"].State)
	assert.Equal(t, 1, res.Summary[DriftInSync])

	// read only
	for _, r := range mock.Requests() {
		assert.Regexp(t, `^GET `, r)
	}
	_, err = Drift(context.Background(), mock.Executer(), root, "Other")
	assert.Equal(t, exitcode.Usage, output.ExitCode(err))
}
