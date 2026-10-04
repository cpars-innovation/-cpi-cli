package mcp

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cpars-innovation/cpicli/internal/cpitest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type rpcResp struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

// session sends the messages to a fresh server and returns the responses by id.
func session(t *testing.T, mock *cpitest.Tenant, root string, msgs ...string) map[string]rpcResp {
	t.Helper()
	srv := NewServer("cpicli", "test", Instructions, Tools(Config{Exe: mock.Executer(), Root: root, PollInterval: time.Millisecond, MaxChecks: 3}))
	var out bytes.Buffer
	require.NoError(t, srv.Serve(context.Background(), strings.NewReader(strings.Join(msgs, "\n")+"\n"), &out))

	byID := map[string]rpcResp{}
	for line := range strings.SplitSeq(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		var r rpcResp
		require.NoError(t, json.Unmarshal([]byte(line), &r), "every stdout line must be a JSON-RPC message: %q", line)
		byID[string(r.ID)] = r
	}
	return byID
}

func call(id int, tool string, args any) string {
	a, _ := json.Marshal(args)
	return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, id, tool, a)
}

type toolCallResult struct {
	IsError           bool       `json:"isError"`
	StructuredContent ToolResult `json:"structuredContent"`
	Content           []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

func toolResult(t *testing.T, r rpcResp) toolCallResult {
	t.Helper()
	require.Nil(t, r.Error, "unexpected JSON-RPC error")
	var res toolCallResult
	require.NoError(t, json.Unmarshal(r.Result, &res))
	require.Len(t, res.Content, 1)
	assert.True(t, json.Valid([]byte(res.Content[0].Text)))
	return res
}

func TestProtocol(t *testing.T) {
	mock := cpitest.NewTenant(t, nil)
	resp := session(t, mock, t.TempDir(),
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":4,"method":"bogus"}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"nope","arguments":{}}}`,
		`not json`,
	)
	require.Len(t, resp, 6, "the notification gets no response")

	var init struct {
		ProtocolVersion string                `json:"protocolVersion"`
		ServerInfo      struct{ Name string } `json:"serverInfo"`
		Capabilities    map[string]any        `json:"capabilities"`
	}
	require.NoError(t, json.Unmarshal(resp["1"].Result, &init))
	assert.Equal(t, "2025-03-26", init.ProtocolVersion)
	assert.Equal(t, "cpicli", init.ServerInfo.Name)
	assert.Contains(t, init.Capabilities, "tools")

	var list struct {
		Tools []struct {
			Name        string         `json:"name"`
			InputSchema map[string]any `json:"inputSchema"`
		} `json:"tools"`
	}
	require.NoError(t, json.Unmarshal(resp["2"].Result, &list))
	var names []string
	for _, tool := range list.Tools {
		names = append(names, tool.Name)
		assert.Equal(t, "object", tool.InputSchema["type"], tool.Name)
	}
	assert.Equal(t, []string{"list_packages", "list_artifacts", "get_runtime_status", "list_message_logs", "get_message_log",
		"get_message_steps", "get_message_attachment", "get_message_store_entry", "list_runtime_artifacts", "list_service_endpoints",
		"validate_artifact", "check_guidelines", "list_resources", "get_resource", "download_artifact",
		"get_parameters", "set_parameters", "upload_artifact", "deploy", "undeploy", "pd_deploy"}, names)

	assert.Nil(t, resp["3"].Error)
	assert.Equal(t, codeMethodNotFound, resp["4"].Error.Code)
	assert.Equal(t, codeInvalidParams, resp["5"].Error.Code)
	assert.Equal(t, codeParseError, resp["null"].Error.Code)
}

func TestUnknownProtocolVersionGetsLatest(t *testing.T) {
	resp := session(t, cpitest.NewTenant(t, nil), t.TempDir(),
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"1999-01-01"}}`)
	assert.Contains(t, string(resp["1"].Result), SupportedProtocolVersions[0])
}

func TestDeployAndStatusTools(t *testing.T) {
	mock := cpitest.NewTenant(t, map[string]*cpitest.Artifact{
		"A": {Type: "Integration", DesignVersion: "1.0.1", TaskStatuses: []string{"SUCCESS"},
			AfterDeploy: []*cpitest.Runtime{{Version: "1.0.1", Status: "STARTED", DeployedOn: time.Now()}}},
		"B": {Type: "Integration", DesignVersion: "1", TaskStatuses: []string{"FAIL"}, ErrorInfo: "script error"},
	})
	resp := session(t, mock, t.TempDir(),
		call(1, "deploy", map[string]any{"artifact_ids": []string{"A", "B"}}),
	)
	res := toolResult(t, resp["1"])
	assert.True(t, res.IsError)
	assert.Equal(t, "partial", res.StructuredContent.ErrorCategory)
	assert.Equal(t, 7, res.StructuredContent.ExitCode)
	results := res.StructuredContent.Result.(map[string]any)["results"].([]any)
	assert.Equal(t, "DEPLOYED", results[0].(map[string]any)["status"])
	assert.Equal(t, "FAILED", results[1].(map[string]any)["status"])
	assert.Contains(t, results[1].(map[string]any)["error"], "script error")

	resp = session(t, mock, t.TempDir(), call(2, "get_runtime_status", map[string]any{"artifact_ids": []string{"A"}}))
	res = toolResult(t, resp["2"])
	assert.False(t, res.IsError)
	assert.True(t, res.StructuredContent.OK)
}

func TestToolArgumentErrorsAreUsage(t *testing.T) {
	mock := cpitest.NewTenant(t, map[string]*cpitest.Artifact{"A": {Runtime: &cpitest.Runtime{Status: "STARTED"}}})
	root := t.TempDir()
	resp := session(t, mock, root,
		call(1, "undeploy", map[string]any{"artifact_ids": []string{"A"}, "confirm": false}),
		call(2, "deploy", map[string]any{"artifact_ids": []string{"A"}, "artifactIds": []string{"typo"}}),
		call(3, "deploy", map[string]any{"artifact_ids": []string{}}),
		call(4, "upload_artifact", map[string]any{"artifact_id": "A", "type": "Integration", "package_id": "P", "dir": "../../etc"}),
		call(5, "deploy", map[string]any{"artifact_ids": []string{"A"}, "artifact_type": "Bogus"}),
	)
	for id := 1; id <= 5; id++ {
		res := toolResult(t, resp[fmt.Sprint(id)])
		assert.True(t, res.IsError, "call %d", id)
		assert.Equal(t, "usage", res.StructuredContent.ErrorCategory, "call %d: %s", id, res.StructuredContent.Error)
	}
	assert.Zero(t, mock.Count("DELETE "), "undeploy without confirm must not touch the tenant")
	assert.Zero(t, mock.Count("POST "))
}

func TestUndeployWithConfirm(t *testing.T) {
	mock := cpitest.NewTenant(t, map[string]*cpitest.Artifact{"A": {Runtime: &cpitest.Runtime{Status: "STARTED"}, UndeployAfter: 1}})
	resp := session(t, mock, t.TempDir(), call(1, "undeploy", map[string]any{"artifact_ids": []string{"A"}, "confirm": true}))
	res := toolResult(t, resp["1"])
	assert.True(t, res.StructuredContent.OK, res.StructuredContent.Error)
	assert.Equal(t, 1, mock.Count("DELETE "))
}

func TestParameterTools(t *testing.T) {
	a := &cpitest.Artifact{Type: "Integration", DesignVersion: "1", Parameters: map[string]string{"Host": "old"}}
	mock := cpitest.NewTenant(t, map[string]*cpitest.Artifact{"A": a})
	resp := session(t, mock, t.TempDir(),
		call(1, "set_parameters", map[string]any{"artifact_id": "A", "parameters": map[string]string{"Host": "new"}}),
	)
	res := toolResult(t, resp["1"])
	assert.True(t, res.StructuredContent.OK, res.StructuredContent.Error)
	assert.Equal(t, "new", a.Parameters["Host"])

	resp = session(t, mock, t.TempDir(), call(2, "get_parameters", map[string]any{"artifact_id": "A"}))
	assert.Contains(t, toolResult(t, resp["2"]).Content[0].Text, `"value":"new"`)
}

func TestPDDeployDefaultsToDryRun(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "pd", "P1"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "pd", "P1", "String.properties"), []byte("K=V\n"), 0644))
	mock := cpitest.NewTenant(t, nil)
	resp := session(t, mock, root, call(1, "pd_deploy", map[string]any{"resources_path": "pd", "full_sync": true}))
	res := toolResult(t, resp["1"])
	assert.Equal(t, true, res.StructuredContent.Result.(map[string]any)["dryRun"])
	assert.Zero(t, mock.Count("POST "))
	assert.Zero(t, mock.Count("PUT "))
	assert.Zero(t, mock.Count("DELETE "))
}

func TestResolvePath(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "a"), 0755))
	p, err := resolvePath(root, "a")
	require.NoError(t, err)
	assert.Equal(t, "a", filepath.Base(p))
	for _, bad := range []string{"..", "../x", "/etc", "a/../../x"} {
		_, err := resolvePath(root, bad)
		assert.Error(t, err, bad)
	}
	// a symlink pointing outside the root is rejected too
	require.NoError(t, os.Symlink("/etc", filepath.Join(root, "link")))
	_, err = resolvePath(root, "link")
	assert.Error(t, err)
}

func TestMessageLogTools(t *testing.T) {
	now := time.Now()
	mock := cpitest.NewTenant(t, nil)
	mock.MessageLogSteps = [][]cpitest.MessageLog{
		{{Guid: "g1", Artifact: "A", Status: "PROCESSING", Start: now}},
		{{Guid: "g1", Artifact: "A", Status: "FAILED", Start: now, End: now, ErrorText: "Mapping failed at line 3",
			Headers: map[string]string{"OrderId": "4711"}}},
	}
	srv := NewServer("cpicli", "test", Instructions, Tools(Config{Exe: mock.Executer(), Root: t.TempDir(), LogPollInterval: time.Millisecond}))
	var out bytes.Buffer
	in := strings.Join([]string{
		call(1, "list_message_logs", map[string]any{"artifact_id": "A", "since": "5m", "wait_seconds": 5}),
	}, "\n") + "\n"
	require.NoError(t, srv.Serve(context.Background(), strings.NewReader(in), &out))
	var r rpcResp
	require.NoError(t, json.Unmarshal(out.Bytes(), &r))
	res := toolResult(t, r)
	assert.True(t, res.StructuredContent.OK, res.StructuredContent.Error)
	logs := res.StructuredContent.Result.(map[string]any)["logs"].([]any)
	assert.Equal(t, "FAILED", logs[0].(map[string]any)["status"])
	assert.Equal(t, "Mapping failed at line 3", logs[0].(map[string]any)["errorText"], "errors are included by default")

	resp := session(t, mock, t.TempDir(),
		call(2, "get_message_log", map[string]any{"message_guid": "g1"}),
		call(3, "list_message_logs", map[string]any{"statuses": []string{"BROKEN"}}),
		call(4, "list_message_logs", map[string]any{"since": "yesterday-ish"}),
	)
	detail := toolResult(t, resp["2"])
	assert.True(t, detail.StructuredContent.OK)
	assert.Contains(t, detail.Content[0].Text, `"OrderId"`)
	assert.Equal(t, "usage", toolResult(t, resp["3"]).StructuredContent.ErrorCategory)
	assert.Equal(t, "usage", toolResult(t, resp["4"]).StructuredContent.ErrorCategory)
}

func TestContentTools(t *testing.T) {
	now := time.Now()
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	w, _ := zw.Create("META-INF/MANIFEST.MF")
	_, _ = w.Write([]byte("Manifest-Version: 1.0\n"))
	require.NoError(t, zw.Close())

	mock := cpitest.NewTenant(t, map[string]*cpitest.Artifact{
		"A": {Type: "Integration", DesignVersion: "1", EndpointURL: "https://tenant/http/a", Zip: zipBuf.Bytes(),
			ValidationResult: "Check execution result: Failed - receiver missing",
			Runtime:          &cpitest.Runtime{Version: "1", Status: "ERROR"}, ErrorInfo: "boom",
			Resources: map[string]cpitest.Resource{"s.groovy": {Type: "groovy", Content: []byte("x=1")}}},
	})
	mock.MessageLogSteps = [][]cpitest.MessageLog{{{Guid: "g1", Artifact: "A", Status: "FAILED", Start: now, End: now,
		Attachments: map[string]string{"in.xml": "<in/>"}, StoreEntries: map[string]string{"e1": "<persisted/>"},
		Steps: []cpitest.Step{{StepID: "s1", ModelStepID: "Mapping_1", Status: "FAILED", Error: "bad"}}}}}
	root := t.TempDir()

	resp := session(t, mock, root,
		call(1, "validate_artifact", map[string]any{"artifact_id": "A"}),
		call(2, "list_service_endpoints", map[string]any{"artifact_id": "A"}),
		call(3, "list_runtime_artifacts", map[string]any{"statuses": []string{"ERROR"}}),
		call(4, "get_resource", map[string]any{"artifact_id": "A", "name": "s.groovy", "type": "groovy"}),
		call(5, "download_artifact", map[string]any{"artifact_id": "A", "dir": "work/A"}),
		call(6, "download_artifact", map[string]any{"artifact_id": "A", "dir": "../outside"}),
		call(7, "get_message_steps", map[string]any{"message_guid": "g1"}),
		call(8, "get_message_attachment", map[string]any{"attachment_id": "att-g1-in.xml"}),
		call(9, "get_message_store_entry", map[string]any{"entry_id": "e1", "max_bytes": 5}),
		call(10, "get_resource", map[string]any{"artifact_id": "A", "name": "s.groovy", "type": "groovy", "max_bytes": 99999999}),
	)
	v := toolResult(t, resp["1"])
	assert.Equal(t, "failed", v.StructuredContent.ErrorCategory)
	assert.Contains(t, v.Content[0].Text, "receiver missing")
	assert.Contains(t, toolResult(t, resp["2"]).Content[0].Text, "https://tenant/http/a")
	assert.Contains(t, toolResult(t, resp["3"]).Content[0].Text, `"errorInfo":"boom"`)
	assert.Contains(t, toolResult(t, resp["4"]).Content[0].Text, `"text":"x=1"`)
	assert.True(t, toolResult(t, resp["5"]).StructuredContent.OK)
	assert.FileExists(t, filepath.Join(root, "work", "A", "META-INF", "MANIFEST.MF"))
	assert.Equal(t, "usage", toolResult(t, resp["6"]).StructuredContent.ErrorCategory)
	assert.Contains(t, toolResult(t, resp["7"]).Content[0].Text, `"modelStepId":"Mapping_1"`)
	assert.Contains(t, toolResult(t, resp["8"]).Content[0].Text, `"text":"<in/>"`, "XML stays readable (no HTML escaping)")
	entry := toolResult(t, resp["9"]).Content[0].Text
	assert.Contains(t, entry, `"truncated":true`)
	assert.Equal(t, "usage", toolResult(t, resp["10"]).StructuredContent.ErrorCategory)
}
