package mcp

import (
	"testing"

	"github.com/cpars-innovation/cpicli/internal/cpitest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDataStoreTools(t *testing.T) {
	mock := cpitest.NewTenant(t, nil)
	entry := "/api/v1/DataStoreEntries(Id='e1',DataStoreName='Orders',IntegrationFlow='Flow_A',Type='')"
	mock.Raw = map[string]any{
		"/api/v1/DataStores(DataStoreName='Orders',IntegrationFlow='Flow_A',Type='')/Entries": []map[string]any{{"Id": "e1", "DataStoreName": "Orders", "IntegrationFlow": "Flow_A"}},
		entry: map[string]any{},
	}
	args := map[string]any{"data_store": "Orders", "artifact_id": "Flow_A"}
	resp := session(t, mock, t.TempDir(),
		call(1, "list_data_store_entries", args),
		call(2, "delete_data_store_entry", map[string]any{"data_store": "Orders", "artifact_id": "Flow_A", "id": "e1", "confirm": false}),
		call(3, "delete_data_store_entry", map[string]any{"data_store": "Orders", "artifact_id": "Flow_A", "id": "e1", "confirm": true}),
		call(4, "list_data_store_entries", map[string]any{"data_store": "Orders", "bogus": 1}),
	)
	list := toolResult(t, resp["1"]).StructuredContent
	require.True(t, list.OK, list.Error)
	assert.EqualValues(t, 1, list.Result.(map[string]any)["total"])
	assert.Equal(t, "usage", toolResult(t, resp["2"]).StructuredContent.ErrorCategory)
	assert.True(t, toolResult(t, resp["3"]).StructuredContent.OK)
	assert.Equal(t, []string{entry}, mock.Deleted, "deleted once, only with confirm")
	assert.Equal(t, "usage", toolResult(t, resp["4"]).StructuredContent.ErrorCategory, "unknown arguments are refused")
}
