package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cpars-innovation/cpicli/internal/cpitest"
	"github.com/cpars-innovation/cpicli/pkg/cpi"
	"github.com/cpars-innovation/cpicli/pkg/httpclnt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Agents choose tools by their descriptions: every tool and every argument
// must be described, and the instructions must mention every tool.
func TestToolsAreDescribed(t *testing.T) {
	for _, tool := range Tools(Config{}) {
		assert.GreaterOrEqual(t, len(tool.Description), 60, "%s: description too short to choose the tool", tool.Name)
		assert.NotEmpty(t, tool.Title, tool.Name)
		props, _ := tool.InputSchema["properties"].(map[string]any)
		for name, p := range props {
			desc, _ := p.(map[string]any)["description"].(string)
			assert.NotEmpty(t, desc, "%s.%s has no description", tool.Name, name)
		}
		assert.Contains(t, Instructions, tool.Name, "instructions do not mention %s", tool.Name)
		_, hasReadOnly := tool.Annotations["readOnlyHint"]
		assert.True(t, hasReadOnly, "%s: readOnlyHint missing", tool.Name)
	}
}

func TestCreatePackageTool(t *testing.T) {
	mock := cpitest.NewTenant(t, nil)
	resp := session(t, mock, t.TempDir(), call(1, "create_package", map[string]any{"package_id": "Orders", "name": "Orders"}))
	res := toolResult(t, resp["1"])
	require.False(t, res.IsError, res.Content[0].Text)
	assert.Equal(t, "CREATED", res.StructuredContent.Result.(map[string]any)["action"])
}

func TestSendTestMessageTool(t *testing.T) {
	mock := cpitest.NewTenant(t, map[string]*cpitest.Artifact{"Orders": {Type: "Integration", DesignVersion: "1"}})
	host, port := mock.HostPort()
	mock.Artifacts["Orders"].EndpointURL = fmt.Sprintf("http://%s:%d/http/orders", host, port)
	mock.Inbound = map[string]*cpitest.Inbound{"/http/orders": {MessageGuid: "G1", Response: "done"}}
	mock.MessageLogSteps = [][]cpitest.MessageLog{{{Guid: "G1", Artifact: "Orders", Status: "COMPLETED"}}}
	newExe := func(url string) (*httpclnt.HTTPExecuter, string, error) {
		return cpi.NewEndpointExecuter(&cpi.ServiceDetails{Userid: "u", Password: "p"}, url)
	}
	cfg := Config{Exe: mock.Executer(), Root: t.TempDir(), LogPollInterval: time.Millisecond, NewEndpointExecuter: newExe}

	resp := sessionWith(t, cfg,
		call(1, "send_test_message", map[string]any{"artifact_id": "Orders", "body": "<a/>", "content_type": "application/xml", "wait_seconds": 5}),
		call(2, "send_test_message", map[string]any{"artifact_id": "Orders", "headers": map[string]string{"Cookie": "x"}}),
	)
	res := toolResult(t, resp["1"])
	require.False(t, res.IsError, res.Content[0].Text)
	result := res.StructuredContent.Result.(map[string]any)
	assert.Equal(t, "G1", result["messageGuid"])
	assert.Equal(t, "COMPLETED", result["log"].(map[string]any)["status"])

	res = toolResult(t, resp["2"])
	assert.True(t, res.IsError)
	assert.Equal(t, "usage", res.StructuredContent.ErrorCategory)
	assert.Len(t, mock.Received, 1)

	// without runtime credentials configured the tool explains itself
	resp = session(t, mock, t.TempDir(), call(1, "send_test_message", map[string]any{"artifact_id": "Orders"}))
	res = toolResult(t, resp["1"])
	assert.True(t, res.IsError)
	assert.Contains(t, res.StructuredContent.Error, "not configured")
}

func TestDiscoverTenantTool(t *testing.T) {
	root := t.TempDir()
	flow := filepath.Join(root, "content", "Pkg", "Flow_A")
	files := map[string]string{
		"META-INF/MANIFEST.MF": "Bundle-SymbolicName: Flow_A\nSAP-BundleType: IntegrationFlow\n",
		"src/main/resources/scenarioflows/integrationflow/Flow_A.iflw": `<definitions><collaboration><extensionElements><property><key>log</key><value>Info</value></property></extensionElements></collaboration></definitions>`,
	}
	for name, content := range files {
		p := filepath.Join(flow, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}
	mock := cpitest.NewTenant(t, nil)
	resp := session(t, mock, root,
		call(1, "discover_tenant", map[string]any{"local_dir": "content"}),
		call(2, "discover_tenant", map[string]any{"local_dir": "content", "output_file": "../outside.json"}),
	)
	res := toolResult(t, resp["1"])
	require.False(t, res.IsError, res.Content[0].Text)
	summary := res.StructuredContent.Result.(map[string]any)["summary"].(map[string]any)
	assert.EqualValues(t, 1, summary["iflows"])

	data, err := os.ReadFile(filepath.Join(root, ".cpi", "discovery.json"))
	require.NoError(t, err)
	var d map[string]any
	require.NoError(t, json.Unmarshal(data, &d))
	assert.Equal(t, "content", d["source"])

	res = toolResult(t, resp["2"])
	assert.True(t, res.IsError)
	assert.True(t, strings.Contains(res.StructuredContent.Error, "outside"))
	assert.Empty(t, mock.Requests(), "local discovery does not contact the tenant")
}
