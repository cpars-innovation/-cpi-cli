package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const lintFlowModel = `<?xml version="1.0" encoding="UTF-8"?>
<bpmn2:definitions xmlns:bpmn2="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:ifl="http:///com.sap.ifl.model/Ifl.xsd">
<bpmn2:collaboration id="Collaboration_1"/>
<bpmn2:process id="Process_1" name="Integration Process">
<bpmn2:extensionElements><ifl:property><key>cmdVariantUri</key><value>ctype::FlowElementVariant/cname::IntegrationProcess/version::1.2.0</value></ifl:property></bpmn2:extensionElements>
<bpmn2:startEvent id="Start" name="Start"><bpmn2:outgoing>F1</bpmn2:outgoing></bpmn2:startEvent>
<bpmn2:callActivity id="S1" name="Log"><bpmn2:extensionElements>
<ifl:property><key>activityType</key><value>Script</value></ifl:property>
<ifl:property><key>scriptBundleId</key><value/></ifl:property>
<ifl:property><key>script</key><value>Log.groovy</value></ifl:property>
</bpmn2:extensionElements><bpmn2:incoming>F1</bpmn2:incoming><bpmn2:outgoing>F2</bpmn2:outgoing></bpmn2:callActivity>
<bpmn2:endEvent id="End" name="End"><bpmn2:incoming>F2</bpmn2:incoming></bpmn2:endEvent>
<bpmn2:sequenceFlow id="F1" sourceRef="Start" targetRef="S1"/>
<bpmn2:sequenceFlow id="F2" sourceRef="S1" targetRef="End"/>
</bpmn2:process>
</bpmn2:definitions>
`

func writeLintFlow(t *testing.T, root, pkg, id string) {
	dir := filepath.Join(root, pkg, id)
	for name, content := range map[string]string{
		"META-INF/MANIFEST.MF": "Manifest-Version: 1.0\nBundle-SymbolicName: " + id + "\nBundle-Version: 1.0.0\nSAP-BundleType: IntegrationFlow\n",
		"src/main/resources/scenarioflows/integrationflow/" + id + ".iflw": lintFlowModel,
		"src/main/resources/script/Log.groovy":                             "def Message processData(Message message) {\n  return message\n}\n",
	} {
		p := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}
}

func TestLintCommand(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "packages")
	writeLintFlow(t, repo, "EDM", "Outbound")
	writeLintFlow(t, repo, "EDM", "Inbound")
	deploys := filepath.Join(root, "deployments")
	require.NoError(t, os.MkdirAll(deploys, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(deploys, "edm.yaml"), []byte(`packages:
  - integrationSuiteId: EDM
    packageDir: EDM
    artifacts:
      - {artifactId: Outbound, artifactDir: Outbound, type: IntegrationFlow, configOverrides: {Host: a}}
      - {artifactId: Herrenberg_Outbound, artifactDir: Outbound, type: IntegrationFlow, configOverrides: {Host: h}}
`), 0o644))
	baseline := filepath.Join(root, ".cpi", "lint-baseline.json")
	lint := func(extra ...string) (cliRun, lintResult) {
		args := append([]string{"lint", "--dir", repo, "--baseline", baseline, "--deploy-config", deploys, "--rules", filepath.Join(root, "none.yaml"), "--output", "json"}, extra...)
		r := runMain(t, args...)
		var env struct {
			Result lintResult `json:"result"`
		}
		if r.stdout != "" {
			require.NoError(t, json.Unmarshal([]byte(r.stdout), &env), r.stdout)
		}
		return r, env.Result
	}
	r, res := lint()
	require.Equal(t, 0, r.code, r.stderr)
	rules := map[string]int{}
	for _, f := range res.Findings {
		rules[f.Rule]++
	}
	assert.Equal(t, 2, rules["duplicate-script"])
	assert.Equal(t, 1, rules["deployment-copies"], "Outbound deployed twice with different configOverrides")
	assert.Equal(t, 2, rules["no-exception-subprocess"])

	r, _ = lint("--fail-on", "warning")
	assert.Equal(t, 5, r.code, "new warnings")
	r, _ = lint("--update-baseline")
	require.Equal(t, 0, r.code, r.stderr)
	assert.FileExists(t, baseline)
	r, _ = lint("--fail-on", "warning")
	assert.Equal(t, 0, r.code, "all known")
	r, _ = lint("--fail-on", "nope")
	assert.Equal(t, 2, r.code)

	r, res = lint("--fix")
	require.Equal(t, 0, r.code, r.stderr)
	require.NotNil(t, res.Fix)
	assert.Equal(t, []string{"EDM/EDM_Scripts"}, res.Fix.Collections)
	assert.FileExists(t, filepath.Join(repo, "EDM", "EDM_Scripts", "src", "main", "resources", "script", "Log.groovy"))
	for _, f := range res.Findings {
		assert.NotEqual(t, "duplicate-script", f.Rule, "fixed")
	}

	r = runMain(t, "lint", "--list-rules", "--output", "json")
	require.Equal(t, 0, r.code)
	assert.Contains(t, r.stdout, `"router-literals"`)
}
