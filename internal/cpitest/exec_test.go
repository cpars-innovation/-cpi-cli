package cpitest_test

import (
	"context"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cpars-innovation/cpicli/internal/cpitest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func logsOf(m *cpitest.Tenant) []cpitest.MessageLog { return m.MessageLogs() }

// One order travels Orders_In -> (ProcessDirect) Orders_Route -> (JMS)
// Billing_In -> (ProcessDirect) Billing_Post with one correlation ID, the
// links and the key header where the scripts write it.
func TestExecutionFollowsTheModel(t *testing.T) {
	now := time.Now()
	m := cpitest.NewTenant(t, nil)
	require.NoError(t, cpitest.SeedDemo(m, "dev", now))
	byCorr := map[string][]cpitest.MessageLog{}
	for _, l := range logsOf(m) {
		assert.False(t, l.End.After(now), "no log ends in the future: %s %s", l.Artifact, l.End)
		byCorr[l.CorrelationID] = append(byCorr[l.CorrelationID], l)
	}
	var chain []cpitest.MessageLog
	for _, runs := range byCorr {
		if len(runs) == 4 && runs[0].Status == "COMPLETED" && runs[3].Status == "COMPLETED" {
			chain = runs
			break
		}
	}
	require.NotNil(t, chain, "a completed order")
	slices.SortFunc(chain, func(a, b cpitest.MessageLog) int { return a.Start.Compare(b.Start) })
	got := []string{}
	for _, l := range chain {
		got = append(got, l.Artifact)
	}
	assert.Equal(t, []string{"Orders_In", "Orders_Route", "Billing_In", "Billing_Post"}, got)
	in, route, bin, post := chain[0], chain[1], chain[2], chain[3]
	assert.Equal(t, in.Guid, route.Predecessor, "ProcessDirect links the run")
	assert.Empty(t, bin.Predecessor, "JMS does not")
	assert.Equal(t, bin.Guid, post.Predecessor)
	assert.NotEmpty(t, in.ApplicationID, "setKeys.groovy sets SAP_ApplicationID")
	assert.Equal(t, in.ApplicationID, in.Headers["OrderNo"])
	assert.Equal(t, in.ApplicationID, bin.Headers["OrderNo"])
	assert.Empty(t, route.Headers, "Orders_Route writes no custom header")
	assert.True(t, bin.Start.After(in.End), "the JMS consumer runs after the producer")
	assert.NotEmpty(t, in.Steps, "steps of the model")
}

// A failing receiver fails the run at its step with a trace, and the
// ProcessDirect caller started by JMS goes to RETRY with the same error.
func TestExecutionFailures(t *testing.T) {
	m := cpitest.NewTenant(t, nil)
	require.NoError(t, cpitest.SeedDemo(m, "dev", time.Now()))
	var post *cpitest.MessageLog
	logs := logsOf(m)
	for i := range logs {
		if logs[i].Artifact == "Billing_Post" && logs[i].Status == "FAILED" {
			post = &logs[i]
			break
		}
	}
	require.NotNil(t, post, "postings fail now and then")
	assert.Contains(t, post.ErrorText, "finance-dev.example.com", "{host} from the configured URL")
	assert.NotContains(t, post.ErrorText, "{key}")
	last := post.Steps[len(post.Steps)-1]
	assert.Equal(t, "FAILED", last.Status)
	assert.Equal(t, "ServiceTask_0", last.ModelStepID)
	require.Len(t, last.Traces, 1)
	assert.Contains(t, last.Traces[0].Payload, "<OrderNo>")
	for _, l := range logs {
		if l.Guid == post.Predecessor {
			assert.Equal(t, "Billing_In", l.Artifact)
			assert.Equal(t, "RETRY", l.Status)
			assert.Equal(t, post.ErrorText, l.ErrorText)
		}
	}
}

// Tiers generate different traffic; the same tier generates the same.
func TestTrafficIsReproduciblePerTier(t *testing.T) {
	now := time.Now()
	statuses := func(tier string) string {
		m := cpitest.NewTenant(t, nil)
		require.NoError(t, cpitest.SeedDemo(m, tier, now))
		var b strings.Builder
		for _, l := range logsOf(m) {
			b.WriteString(l.Artifact + l.Status + "\n")
		}
		return b.String()
	}
	assert.Equal(t, statuses("prod"), statuses("prod"))
	assert.NotEqual(t, statuses("test"), statuses("prod"), "test and prod do not share their failures")
}

func TestLiveTraffic(t *testing.T) {
	m := cpitest.NewTenant(t, nil)
	require.NoError(t, cpitest.SeedDemo(m, "test", time.Now()))
	before := len(logsOf(m))
	ctx, cancel := context.WithCancel(context.Background())
	m.StartTraffic(ctx, 3600*20) // 5 orders an hour, 20 a second
	require.Eventually(t, func() bool {
		return len(logsOf(m)) > before+8
	}, 5*time.Second, 50*time.Millisecond)
	cancel()
}

// A receiver whose credential is missing on the tenant fails.
func TestExecutionMissingCredential(t *testing.T) {
	m := cpitest.NewTenant(t, nil)
	require.NoError(t, cpitest.SeedDemo(m, "dev", time.Now()))
	delete(m.Credentials["UserCredentials"], "Partner_SFTP")
	logs := runOnce(t, m, "Partner_Notify")
	require.Len(t, logs, 1)
	assert.Equal(t, "FAILED", logs[0].Status)
	assert.Contains(t, logs[0].ErrorText, "credential Partner_SFTP not found")
}

// A huge rate must not make the ticker interval zero (time.NewTicker panics).
func TestLiveTrafficWithHugeSpeed(t *testing.T) {
	m := cpitest.NewTenant(t, nil)
	require.NoError(t, cpitest.SeedDemo(m, "dev", time.Now()))
	before := len(m.MessageLogs())
	ctx, cancel := context.WithCancel(context.Background())
	m.StartTraffic(ctx, 1e15)
	time.Sleep(200 * time.Millisecond)
	cancel()
	assert.Greater(t, len(m.MessageLogs()), before, "traffic ran, at most one message per ms and rule")
	m.StartTraffic(context.Background(), math.NaN()) // ignored
}
