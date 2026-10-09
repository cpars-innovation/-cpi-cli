package ops

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/cpars-innovation/cpicli/internal/cpitest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An artifact in draft on the tenant (someone edits it in the Web UI) is
// neither overwritten nor deployed by upload_artifact and deploy.
func TestDraftIsLeftAlone(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "OrderIntake")
	writeIFlow(t, dir, "println 'v2'")
	mock := cpitest.NewTenant(t, map[string]*cpitest.Artifact{
		"OrderIntake": {Type: "Integration", DesignVersion: "Active", Package: "Orders", Name: "Order Intake",
			Runtime: &cpitest.Runtime{Version: "1.0.0", Status: "STARTED"}},
	})
	req := UploadRequest{ID: "OrderIntake", Type: "Integration", PackageID: "Orders", Dir: dir}

	_, err := UploadArtifact(mock.Executer(), req)
	require.Error(t, err, "without SkipDraft (CLI update artifact) a draft fails as before")

	req.SkipDraft = true
	res, err := UploadArtifact(mock.Executer(), req)
	require.NoError(t, err)
	assert.Equal(t, "SKIPPED", res.Action)
	assert.Equal(t, SkippedDraft, res.Skipped)
	assert.Contains(t, res.Reason, "draft on the tenant")
	assert.Equal(t, 0, mock.Artifacts["OrderIntake"].Uploads)

	results := Deploy(context.Background(), NewTenant(mock.Executer()), []Artifact{{ID: "OrderIntake", Type: "Integration"}}, fastOpts())
	require.Len(t, results, 1)
	assert.Equal(t, StatusSkipped, results[0].Status)
	assert.Equal(t, SkippedDraft, results[0].Skipped)
	assert.Equal(t, 0, mock.Artifacts["OrderIntake"].Deploys)

	plan, err := PlanDeploy(mock.Executer(), "Integration", []string{"OrderIntake"}, "", false)
	require.NoError(t, err)
	assert.False(t, plan[0].Deploy)
	assert.Equal(t, SkippedDraft, plan[0].Skipped)
}
