package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cpars-innovation/cpicli/internal/cpitest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJobSummary(t *testing.T) {
	mock := cpitest.NewTenant(t, map[string]*cpitest.Artifact{
		"A": {Type: "Integration", DesignVersion: "1.0.2", TaskStatuses: []string{"SUCCESS"},
			AfterDeploy: []*cpitest.Runtime{{Version: "1.0.2", Status: "STARTED", DeployedOn: time.Now().Add(time.Hour)}}},
	})
	summary := filepath.Join(t.TempDir(), "summary.md")
	r := runMain(t, append([]string{"deploy", "--artifact-ids", "A,B", "--delay-length", "0", "--max-check-limit", "2", "--summary", summary}, basicAuth(mock)...)...)
	assert.Equal(t, 7, r.code)
	data, err := os.ReadFile(summary)
	require.NoError(t, err)
	text := string(data)
	assert.Contains(t, text, "### ❌ cpictl deploy (exit code 7")
	assert.Contains(t, text, "| Artifact | Status | Version | Rule | Error |")
	assert.Regexp(t, `\| B \| FAILED \|.*\n\| A \| DEPLOYED`, text, "failures first")

	// appended, from $GITHUB_STEP_SUMMARY too; "off" writes nothing
	t.Setenv("GITHUB_STEP_SUMMARY", summary)
	r = runMainKeepEnv(t, append([]string{"deploy", "--artifact-ids", "A", "--plan"}, basicAuth(mock)...)...)
	require.Equal(t, 0, r.code, r.stderr)
	data, _ = os.ReadFile(summary)
	assert.Contains(t, string(data), "cpictl deploy (exit code 0")
	assert.Contains(t, string(data), "**Plan** (nothing was triggered)")
	size := len(data)
	r = runMainKeepEnv(t, append([]string{"deploy", "--artifact-ids", "A", "--plan", "--summary", "off"}, basicAuth(mock)...)...)
	require.Equal(t, 0, r.code)
	data, _ = os.ReadFile(summary)
	assert.Len(t, data, size)

	// commands without a summary write nothing
	r = runMainKeepEnv(t, "skills", "list")
	data, _ = os.ReadFile(summary)
	assert.Len(t, data, size)
}

// runMainKeepEnv is runMain without clearing GITHUB_STEP_SUMMARY.
func runMainKeepEnv(t *testing.T, args ...string) cliRun {
	t.Helper()
	keep := os.Getenv("GITHUB_STEP_SUMMARY")
	r := runMainWith(t, func() { t.Setenv("GITHUB_STEP_SUMMARY", keep) }, args...)
	return r
}
