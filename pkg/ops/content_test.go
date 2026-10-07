package ops

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cpars-innovation/cpicli/internal/cpitest"
	"github.com/cpars-innovation/cpicli/internal/exitcode"
	"github.com/cpars-innovation/cpicli/internal/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateArtifact(t *testing.T) {
	m := cpitest.NewTenant(t, map[string]*cpitest.Artifact{
		"Good": {Type: "Integration", DesignVersion: "1"},
		"Bad":  {Type: "Integration", DesignVersion: "1", ValidationResult: "Check execution result: Failed\nReceiver adapter: address missing"},
	})
	res, err := ValidateArtifact(m.Executer(), "Good", "")
	require.NoError(t, err)
	assert.Equal(t, "PASSED", res.Status)

	res, err = ValidateArtifact(m.Executer(), "Bad", "")
	assert.Equal(t, exitcode.DeployFailed, output.ExitCode(err))
	assert.Equal(t, "FAILED", res.Status)
	assert.Contains(t, res.Details, "address missing")

	_, err = ValidateArtifact(m.Executer(), "Missing", "")
	assert.Equal(t, exitcode.TenantHTTP, output.ExitCode(err))
}

func TestCheckGuidelines(t *testing.T) {
	guidelines := []map[string]any{
		{"GuidelineId": "G1", "GuidelineName": "Use byte array", "Compliance": "Compliant", "IsGuidelineSkipped": "false"},
		{"GuidelineId": "G2", "GuidelineName": "Avoid synchronous loops", "Severity": "High", "Compliance": "Not Compliant", "IsGuidelineSkipped": "false", "ViolatedComponents": "Loop_1"},
		{"GuidelineId": "G3", "GuidelineName": "Skipped one", "Compliance": "Not Compliant", "IsGuidelineSkipped": true},
	}
	m := cpitest.NewTenant(t, map[string]*cpitest.Artifact{
		"A": {Type: "Integration", DesignVersion: "1", GuidelineStatuses: []string{"IN_PROGRESS", "FAIL"}, Guidelines: guidelines},
		"B": {Type: "Integration", DesignVersion: "1", GuidelineStatuses: []string{"PASS"}, Guidelines: guidelines[:1]},
	})
	report, err := CheckGuidelines(context.Background(), m.Executer(), "A", "", time.Second, time.Millisecond)
	assert.Equal(t, exitcode.DeployFailed, output.ExitCode(err))
	require.NotNil(t, report)
	assert.Equal(t, "exec-1", report.ExecutionID)
	assert.Equal(t, 3, report.Total)
	require.Len(t, report.Violations, 1, "skipped guidelines are not violations")
	assert.Equal(t, "Loop_1", report.Violations[0].ViolatedComponents)

	report, err = CheckGuidelines(context.Background(), m.Executer(), "B", "", time.Second, time.Millisecond)
	require.NoError(t, err)
	assert.Empty(t, report.Violations)
}

func TestEndpointsAndRuntimeList(t *testing.T) {
	m := cpitest.NewTenant(t, map[string]*cpitest.Artifact{
		"A": {Type: "Integration", DesignVersion: "1", EndpointURL: "https://tenant/http/orders", Runtime: &cpitest.Runtime{Version: "1", Status: "STARTED"}},
		"B": {Type: "Integration", DesignVersion: "1", Runtime: &cpitest.Runtime{Version: "1", Status: "ERROR"}, ErrorInfo: "bad config"},
	})
	eps, err := ListServiceEndpoints(m.Executer(), "A")
	require.NoError(t, err)
	require.Len(t, eps, 1)
	assert.Equal(t, "https://tenant/http/orders", eps[0].EntryPoints[0].URL)

	all, err := ListRuntimeArtifacts(m.Executer(), nil)
	require.NoError(t, err)
	assert.Len(t, all, 2)
	failed, err := ListRuntimeArtifacts(m.Executer(), []string{"error"})
	require.NoError(t, err)
	require.Len(t, failed, 1)
	assert.Equal(t, "bad config", failed[0].ErrorInfo)

	_, err = ListRuntimeArtifacts(m.Executer(), []string{"BOGUS"})
	assert.Equal(t, exitcode.Usage, output.ExitCode(err))
}

func TestResources(t *testing.T) {
	m := cpitest.NewTenant(t, map[string]*cpitest.Artifact{
		"A": {Type: "Integration", DesignVersion: "1", Resources: map[string]cpitest.Resource{
			"script1.groovy": {Type: "groovy", Content: []byte("println 'hi'")},
		}},
	})
	list, err := ListResources(m.Executer(), "A", "")
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "groovy", list[0].Type)
	assert.Equal(t, float64(12), list[0].Size, "ResourceSize comes as a string from the tenant")

	res, err := GetResource(m.Executer(), "A", "", "script1.groovy", "groovy", 0)
	require.NoError(t, err)
	assert.Equal(t, "println 'hi'", res.Text)

	_, err = GetResource(m.Executer(), "A", "", "missing.groovy", "groovy", 0)
	assert.Equal(t, exitcode.Usage, output.ExitCode(err))
}

func zipOf(t *testing.T, files map[string]string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, _ = w.Write([]byte(content))
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func TestDownloadArtifactToDir(t *testing.T) {
	m := cpitest.NewTenant(t, map[string]*cpitest.Artifact{
		"A": {Type: "Integration", DesignVersion: "1", Zip: zipOf(t, map[string]string{
			"META-INF/MANIFEST.MF": "Manifest-Version: 1.0\n", "src/main/resources/parameters.prop": "a=b\n"})},
		"Evil": {Type: "Integration", DesignVersion: "1", Zip: zipOf(t, map[string]string{"../escape.txt": "x"})},
	})
	dir := filepath.Join(t.TempDir(), "A")
	res, err := DownloadArtifactToDir(m.Executer(), "Integration", "A", "", dir, false)
	require.NoError(t, err)
	assert.Equal(t, 2, res.Files)
	assert.FileExists(t, filepath.Join(dir, "META-INF", "MANIFEST.MF"))

	_, err = DownloadArtifactToDir(m.Executer(), "Integration", "A", "", dir, false)
	assert.Equal(t, exitcode.Usage, output.ExitCode(err), "non-empty directory needs overwrite")
	_, err = DownloadArtifactToDir(m.Executer(), "Integration", "A", "", dir, true)
	require.NoError(t, err)

	evilDir := filepath.Join(t.TempDir(), "evil")
	_, err = DownloadArtifactToDir(m.Executer(), "Integration", "Evil", "", evilDir, false)
	assert.Error(t, err, "zip slip is rejected")
	_, statErr := os.Stat(filepath.Join(filepath.Dir(evilDir), "escape.txt"))
	assert.True(t, os.IsNotExist(statErr))
}
