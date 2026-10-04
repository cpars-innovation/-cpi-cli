package cpi

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessageLogQueryFilter(t *testing.T) {
	since := time.Date(2026, 10, 4, 10, 0, 0, 0, time.FixedZone("CEST", 2*3600))
	f, err := MessageLogQuery{
		ArtifactID: "Order'Intake", Statuses: []string{"failed", "RETRY"}, Since: since, CorrelationID: "c1",
	}.Filter()
	require.NoError(t, err)
	assert.Equal(t, "IntegrationFlowName eq 'Order''Intake' and (Status eq 'FAILED' or Status eq 'RETRY') and "+
		"CorrelationId eq 'c1' and LogEnd ge datetime'2026-10-04T08:00:00.000'", f)

	f, err = MessageLogQuery{}.Filter()
	require.NoError(t, err)
	assert.Empty(t, f)

	_, err = MessageLogQuery{Statuses: []string{"FAILED' or 1 eq 1"}}.Filter()
	assert.Error(t, err, "status values are validated, not interpolated")
}

func TestQueryEscape(t *testing.T) {
	assert.Equal(t, "Status%20eq%20%27FAILED%27", queryEscape("Status eq 'FAILED'"))
}
