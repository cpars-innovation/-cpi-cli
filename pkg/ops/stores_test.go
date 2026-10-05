package ops

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cpars-innovation/cpicli/internal/cpitest"
	"github.com/cpars-innovation/cpicli/internal/exitcode"
	"github.com/cpars-innovation/cpicli/internal/output"
	"github.com/cpars-innovation/cpicli/pkg/cpi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDataStores(t *testing.T) {
	mock := cpitest.NewTenant(t, nil)
	entries := []map[string]any{
		{"Id": "e1", "DataStoreName": "Orders", "IntegrationFlow": "Flow_A", "Type": "", "Status": "Waiting", "Messageid": "G1", "CreatedAt": "/Date(1759651200000)/"},
		{"Id": "e2", "DataStoreName": "Orders", "IntegrationFlow": "Flow_A", "Type": "", "Status": "Overdue"},
	}
	mock.Raw = map[string]any{
		"/api/v1/DataStores(DataStoreName='Orders',IntegrationFlow='Flow_A',Type='')/Entries": entries,
		"/api/v1/DataStoreEntries": []map[string]any{entries[0], {"Id": "x", "DataStoreName": "Other", "IntegrationFlow": "Flow_B"}},
		"/api/v1/DataStoreEntries(Id='e1',DataStoreName='Orders',IntegrationFlow='Flow_A',Type='')/$value": []byte("<order/>"),
		"/api/v1/DataStoreEntries(Id='e1',DataStoreName='Orders',IntegrationFlow='Flow_A',Type='')":        map[string]any{},
		"/api/v1/Variables": []map[string]any{{"VariableName": "lastRun", "IntegrationFlow": "Flow_A", "Visibility": "Integration Flow"}, {"VariableName": "global", "IntegrationFlow": "", "Visibility": "Global"}, {"VariableName": "other", "IntegrationFlow": "Flow_B"}},
		"/api/v1/Variables(VariableName='lastRun',IntegrationFlow='Flow_A')/$value": []byte("2026-10-05"),
	}
	key := cpi.DataStoreKey{Name: "Orders", IntegrationFlow: "Flow_A"}

	list, err := ListDataStoreEntries(mock.Executer(), key, "", true, 1)
	require.NoError(t, err)
	assert.Equal(t, 2, list.Total)
	assert.True(t, list.Truncated)
	assert.Equal(t, "e1", list.Entries[0].ID)
	assert.Equal(t, "G1", list.Entries[0].MessageGuid)
	require.NotNil(t, list.Entries[0].CreatedAt)
	assert.Equal(t, "overdueonly=true", mock.RawQueries["/api/v1/DataStores(DataStoreName='Orders',IntegrationFlow='Flow_A',Type='')/Entries"])

	list, err = ListDataStoreEntries(mock.Executer(), cpi.DataStoreKey{IntegrationFlow: "Flow_A"}, "G1", false, 0)
	require.NoError(t, err)
	assert.Len(t, list.Entries, 1, "all stores, filtered by flow")
	assert.Equal(t, "messageid=G1", mock.RawQueries["/api/v1/DataStoreEntries"])

	c, err := GetDataStoreEntry(mock.Executer(), "e1", key, 0)
	require.NoError(t, err)
	assert.Equal(t, "<order/>", c.Text)

	_, err = DeleteDataStoreEntry(mock.Executer(), "e1", key)
	require.NoError(t, err)
	assert.Equal(t, []string{"/api/v1/DataStoreEntries(Id='e1',DataStoreName='Orders',IntegrationFlow='Flow_A',Type='')"}, mock.Deleted)

	vars, err := ListVariables(mock.Executer(), "Flow_A")
	require.NoError(t, err)
	assert.Len(t, vars, 2, "flow and global variables")
	v, err := GetVariable(mock.Executer(), "lastRun", "Flow_A", 0)
	require.NoError(t, err)
	assert.Equal(t, "2026-10-05", v.Text)

	_, err = GetDataStoreEntry(mock.Executer(), "", key, 0)
	assert.Equal(t, exitcode.Usage, output.ExitCode(err))
}

func TestQueuesBrokerNumberRanges(t *testing.T) {
	mock := cpitest.NewTenant(t, nil)
	mock.Raw = map[string]any{
		"/api/v1/MessagingQueues":       []map[string]any{{"queueName": "ORDERS_IN", "numberOfMessages": "3", "active": true}, {"queueName": "ORDERS_ERR", "numberOfMessages": "12"}, {"queueName": "BILLING", "numberOfMessages": 0}},
		"/api/v1/JmsBrokers('Broker1')": map[string]any{"Capacity": "1200", "MaxCapacity": 9500, "QueueNumber": 3, "MaxQueueNumber": "30", "IsConsumersHigh": 1},
		"/api/v1/NumberRanges":          []map[string]any{{"Name": "INVOICE", "MinValue": "1", "MaxValue": "999999", "CurrentValue": "4711", "Rotate": "true"}},
	}
	queues, err := ListQueues(mock.Executer(), "ORDERS")
	require.NoError(t, err)
	require.Len(t, queues, 2)
	assert.Equal(t, cpi.Queue{Name: "ORDERS_ERR", Messages: 12}, queues[0], "fullest first")

	b, err := cpi.NewStores(mock.Executer()).Broker()
	require.NoError(t, err)
	assert.Equal(t, int64(1200), b.Capacity)
	assert.Equal(t, int64(9500), b.MaxCapacity)
	assert.True(t, b.ConsumersHigh)

	nr, err := cpi.NewStores(mock.Executer()).NumberRanges()
	require.NoError(t, err)
	assert.Equal(t, "4711", nr[0].CurrentValue)
}

func TestLogFilesAndIdempotency(t *testing.T) {
	mock := cpitest.NewTenant(t, nil)
	logLines := strings.Repeat("old line\n", 20) + "ERROR adapter failed\n"
	mock.Raw = map[string]any{
		"/api/v1/LogFiles": []map[string]any{
			{"Name": "http_access.log", "Application": "it-cpi", "LogFileType": "http", "Size": "123", "LastModified": "/Date(" + strconv.FormatInt(time.Now().UnixMilli(), 10) + ")/"},
			{"Name": "old.log", "Application": "it-cpi", "LogFileType": "http", "LastModified": "/Date(1000)/"},
		},
		"/api/v1/LogFiles(Name='http_access.log',Application='it-cpi')/$value": []byte(logLines),
		"/api/v1/IdempotentRepositoryEntries":                                  []map[string]any{{"Source": "sftp://host/in", "Entry": "in/order1.csv", "Component": "SFTP", "CreationTime": "1759651200000"}, {"Source": "xi", "Entry": "abc", "Component": "XI"}},
		"/api/v1/IdMapFromIds('SRC-1')/ToIds":                                  []map[string]any{{"ToId": "TGT-9", "Mapper": "m"}},
	}
	files, err := ListLogFiles(mock.Executer(), "http", time.Now().Add(-time.Hour))
	require.NoError(t, err)
	require.Len(t, files.Files, 1, "older files are left out")
	assert.Contains(t, mock.RawQueries["/api/v1/LogFiles"], "LogFileType%20eq%20%27http%27")

	tail, err := GetLogFile(mock.Executer(), "http_access.log", "it-cpi", 30)
	require.NoError(t, err)
	assert.True(t, tail.Truncated)
	assert.True(t, strings.HasSuffix(tail.Text, "ERROR adapter failed\n"))
	assert.False(t, strings.HasPrefix(tail.Text, "line"), "starts at a line boundary")
	assert.Equal(t, len(logLines), tail.Size)

	entries, err := ListIdempotentEntries(mock.Executer(), "in/order1.csv", "sftp", "")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.NotNil(t, entries[0].CreationTime)
	assert.Equal(t, "id=in%2Forder1.csv", mock.RawQueries["/api/v1/IdempotentRepositoryEntries"])

	m, err := ListIDMappings(mock.Executer(), "SRC-1", "")
	require.NoError(t, err)
	assert.Equal(t, []cpi.IDMapping{{FromID: "SRC-1", ToID: "TGT-9", Mapper: "m"}}, m)
	_, err = ListIDMappings(mock.Executer(), "", "")
	assert.Equal(t, exitcode.Usage, output.ExitCode(err))
}
