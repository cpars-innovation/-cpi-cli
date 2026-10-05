package ops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cpars-innovation/cpicli/internal/cpitest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFlow writes files below dir and returns dir.
func writeFlow(t *testing.T, dir string, files map[string]string) string {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}
	return dir
}

func templateFlow(t *testing.T) string {
	files := testIFlowFiles("Template_Sync")
	files["META-INF/MANIFEST.MF"] = "Manifest-Version: 1.0\r\nBundle-SymbolicName: Template_Sync; singleton:=true\r\nBundle-Name: Template for synchronous HTTPS flows with a ver\r\n y long name\r\nBundle-Version: 1.4.2\r\nSAP-BundleType: IntegrationFlow\r\nImport-Package: com.sap.esb.application.services.cxf.interceptor,com.sap\r\n .it.op.agent.api\r\n\r\n"
	files["metainfo.prop"] = "#Store metainfo properties\ndescription=The template\nsource=x\n"
	files[".project"] = "<projectDescription><name>Template_Sync</name></projectDescription>"
	files["src/main/resources/script/log.groovy"] = "// Template_Sync logging\n"
	return writeFlow(t, filepath.Join(t.TempDir(), "Template_Sync"), files)
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	data, err := os.ReadFile(p)
	require.NoError(t, err)
	return string(data)
}

func TestCopyIFlowFromDir(t *testing.T) {
	src := templateFlow(t)
	target := filepath.Join(t.TempDir(), "content", "Sales", "Orders_Sync")
	res, err := CopyIFlow(nil, IFlowCopyOptions{FromDir: src, TargetDir: target, ID: "Orders_Sync",
		Name: "Orders synchronous", Description: "Bestellungen für SAP: S/4", Addresses: []string{"/orders/sync"}})
	require.NoError(t, err)
	assert.Equal(t, "Template_Sync", res.SourceID)
	assert.Equal(t, "1.0.0", res.Version)
	assert.Equal(t, []AddressChange{{Adapter: "HTTPS", Old: "/orders/in", New: "/orders/sync", Where: "model"}}, res.Addresses)

	mf := readFile(t, filepath.Join(target, "META-INF", "MANIFEST.MF"))
	assert.Contains(t, mf, "Bundle-SymbolicName: Orders_Sync; singleton:=true\r\n")
	assert.Contains(t, mf, "Bundle-Name: Orders synchronous\r\n")
	assert.NotContains(t, mf, "y long name", "continuation lines of the old value are removed")
	assert.Contains(t, mf, "Bundle-Version: 1.0.0\r\n")
	assert.Contains(t, mf, "Import-Package: com.sap.esb.application.services.cxf.interceptor,com.sap\r\n .it.op.agent.api\r\n", "other headers unchanged")
	h := parseManifest([]byte(mf))
	assert.Equal(t, "IntegrationFlow", h["SAP-BundleType"])

	model := filepath.Join(target, "src/main/resources/scenarioflows/integrationflow/Orders_Sync.iflw")
	assert.NoFileExists(t, filepath.Join(target, "src/main/resources/scenarioflows/integrationflow/Template_Sync.iflw"))
	f, err := AnalyzeIFlow(os.DirFS(target))
	require.NoError(t, err)
	assert.Equal(t, "Orders_Sync", f.ID)
	assert.Equal(t, []Trigger{{Adapter: "HTTPS", Address: "/orders/sync"}}, f.Triggers)
	assert.Contains(t, readFile(t, model), "<value>/billing/in</value>", "receiver addresses are not touched")

	meta := readFile(t, filepath.Join(target, "metainfo.prop"))
	assert.Contains(t, meta, `description=Bestellungen f\u00fcr SAP: S/4`, "non-ASCII as \\u escapes, as Java properties are read")
	assert.Contains(t, meta, "source=x")
	assert.Equal(t, "<projectDescription><name>Orders_Sync</name></projectDescription>", readFile(t, filepath.Join(target, ".project")))
	assert.Equal(t, []string{"src/main/resources/script/log.groovy (1)"}, res.Remaining)
	assert.Equal(t, 8, res.Files)

	// the source is unchanged
	assert.Contains(t, readFile(t, filepath.Join(src, "META-INF", "MANIFEST.MF")), "Template_Sync; singleton")
}

func TestCopyIFlowAddressRules(t *testing.T) {
	src := templateFlow(t)
	target := func() string { return filepath.Join(t.TempDir(), "copy") }

	t.Run("an unchanged sender address is refused and nothing is left behind", func(t *testing.T) {
		dir := target()
		_, err := CopyIFlow(nil, IFlowCopyOptions{FromDir: src, TargetDir: dir, ID: "Copy"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "/orders/in")
		assert.NoDirExists(t, dir)
	})
	t.Run("keep", func(t *testing.T) {
		res, err := CopyIFlow(nil, IFlowCopyOptions{FromDir: src, TargetDir: target(), ID: "Copy", KeepAddresses: true})
		require.NoError(t, err)
		assert.Equal(t, "kept", res.Addresses[0].Where)
		assert.Contains(t, strings.Join(res.Changes, "\n"), "metainfo.prop: description of the source removed")
	})
	t.Run("OLD=NEW", func(t *testing.T) {
		res, err := CopyIFlow(nil, IFlowCopyOptions{FromDir: src, TargetDir: target(), ID: "Copy", Addresses: []string{"/orders/in=/copy"}})
		require.NoError(t, err)
		assert.Equal(t, "/copy", res.Addresses[0].New)
	})
	t.Run("unknown OLD", func(t *testing.T) {
		_, err := CopyIFlow(nil, IFlowCopyOptions{FromDir: src, TargetDir: target(), ID: "Copy", Addresses: []string{"/nope=/copy"}})
		assert.ErrorContains(t, err, "no sender address")
	})
	t.Run("same ID", func(t *testing.T) {
		_, err := CopyIFlow(nil, IFlowCopyOptions{FromDir: src, TargetDir: target(), ID: "Template_Sync", Addresses: []string{"/x"}})
		assert.ErrorContains(t, err, "equals the source ID")
	})
	t.Run("invalid ID", func(t *testing.T) {
		_, err := CopyIFlow(nil, IFlowCopyOptions{FromDir: src, TargetDir: target(), ID: "Bad ID"})
		assert.ErrorContains(t, err, "invalid flow ID")
	})
	t.Run("target inside the source", func(t *testing.T) {
		_, err := CopyIFlow(nil, IFlowCopyOptions{FromDir: src, TargetDir: filepath.Join(src, "copy"), ID: "Copy", Addresses: []string{"/x"}})
		assert.ErrorContains(t, err, "inside")
		assert.NoDirExists(t, filepath.Join(src, "copy"))
	})
	t.Run("target not empty", func(t *testing.T) {
		dir := writeFlow(t, target(), map[string]string{"keep.txt": "x"})
		_, err := CopyIFlow(nil, IFlowCopyOptions{FromDir: src, TargetDir: dir, ID: "Copy", Addresses: []string{"/x"}})
		assert.ErrorContains(t, err, "not empty")
		assert.FileExists(t, filepath.Join(dir, "keep.txt"))
	})
	t.Run("not a flow", func(t *testing.T) {
		other := writeFlow(t, t.TempDir(), map[string]string{"META-INF/MANIFEST.MF": "Bundle-SymbolicName: Map\nSAP-BundleType: MessageMapping\n"})
		_, err := CopyIFlow(nil, IFlowCopyOptions{FromDir: other, TargetDir: target(), ID: "Copy"})
		assert.ErrorContains(t, err, "not an integration flow")
	})
}

const twoSenderModel = `<bpmn2:definitions xmlns:bpmn2="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:ifl="http:///com.sap.ifl.model/Ifl.xsd">
  <bpmn2:collaboration id="Collaboration_1">
    <bpmn2:messageFlow id="MessageFlow_1" name="HTTPS">
      <bpmn2:extensionElements>
        <ifl:property>
          <key>ComponentType</key>
          <value>HTTPS</value>
        </ifl:property>
        <ifl:property><key>cmdVariantUri</key><value>ctype::AdapterVariant/cname::sap:HTTPS/direction::Sender</value></ifl:property>
        <ifl:property>
          <key>urlPath</key>
          <value>{{Inbound Path}}</value>
        </ifl:property>
        <ifl:property><key>direction</key><value>Sender</value></ifl:property>
      </bpmn2:extensionElements>
    </bpmn2:messageFlow>
    <bpmn2:messageFlow id="MessageFlow_2" name="ProcessDirect">
      <bpmn2:extensionElements>
        <ifl:property><key>ComponentType</key><value>ProcessDirect</value></ifl:property>
        <ifl:property><key>cmdVariantUri</key><value>ctype::AdapterVariant/cname::sap:ProcessDirect/direction::Sender</value></ifl:property>
        <ifl:property><key>address</key><value>/test/Template&amp;Co</value></ifl:property>
        <ifl:property><key>direction</key><value>Sender</value></ifl:property>
      </bpmn2:extensionElements>
    </bpmn2:messageFlow>
  </bpmn2:collaboration>
</bpmn2:definitions>`

func TestCopyIFlowParameterAndSeveralSenders(t *testing.T) {
	src := writeFlow(t, filepath.Join(t.TempDir(), "Tpl"), map[string]string{
		"META-INF/MANIFEST.MF": "Bundle-SymbolicName: Tpl\nBundle-Name: Tpl\nSAP-BundleType: IntegrationFlow\n",
		"src/main/resources/scenarioflows/integrationflow/Main.iflw": twoSenderModel,
		"src/main/resources/parameters.prop":                         "#comment\nInbound\\ Path=/tpl/in\nHost=https\\://example.com\n",
	})
	_, err := CopyIFlow(nil, IFlowCopyOptions{FromDir: src, TargetDir: filepath.Join(t.TempDir(), "x"), ID: "New", Addresses: []string{"/new"}})
	assert.ErrorContains(t, err, "OLD=NEW", "a single address is ambiguous with two senders")

	target := filepath.Join(t.TempDir(), "New")
	res, err := CopyIFlow(nil, IFlowCopyOptions{FromDir: src, TargetDir: target, ID: "New",
		Addresses: []string{"{{Inbound Path}}=/new/in:v1", "/test/Template&Co=/test/New"}})
	require.NoError(t, err)
	assert.ElementsMatch(t, []AddressChange{
		{Adapter: "HTTPS", Old: "{{Inbound Path}}", New: "/new/in:v1", Where: "parameter Inbound Path"},
		{Adapter: "ProcessDirect", Old: "/test/Template&Co", New: "/test/New", Where: "model"},
	}, res.Addresses)

	params := readFile(t, filepath.Join(target, "src/main/resources/parameters.prop"))
	assert.Equal(t, "#comment\nInbound\\ Path=/new/in\\:v1\nHost=https\\://example.com\n", params)
	model := readFile(t, filepath.Join(target, "src/main/resources/scenarioflows/integrationflow/Main.iflw"))
	assert.Contains(t, model, "<value>{{Inbound Path}}</value>", "the model keeps the parameter")
	assert.Contains(t, model, "<value>/test/New</value>")
	assert.FileExists(t, filepath.Join(target, "src/main/resources/scenarioflows/integrationflow/Main.iflw"), "a model not named after the ID keeps its name")
	f, err := AnalyzeIFlow(os.DirFS(target))
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"Inbound Path": "/new/in:v1"}, f.AddressParameters)
}

func TestCopyIFlowFromTenant(t *testing.T) {
	mock := cpitest.NewTenant(t, map[string]*cpitest.Artifact{
		"Orders_In": {Type: "Integration", DesignVersion: "1.0.3", Package: "Sales", Zip: zipFiles(t, testIFlowFiles("Orders_In"))},
	})
	target := filepath.Join(t.TempDir(), "Orders_Copy")
	res, err := CopyIFlow(mock.Executer(), IFlowCopyOptions{FromID: "Orders_In", TargetDir: target, ID: "Orders_Copy", Addresses: []string{"/orders/copy"}})
	require.NoError(t, err)
	assert.Equal(t, "tenant:Orders_In", res.From)
	assert.FileExists(t, filepath.Join(target, "src/main/resources/scenarioflows/integrationflow/Orders_Copy.iflw"))
	for _, r := range mock.Requests() {
		assert.Regexp(t, `^GET `, r, "the tenant is only read")
	}

	_, err = CopyIFlow(mock.Executer(), IFlowCopyOptions{FromID: "Missing", TargetDir: filepath.Join(t.TempDir(), "m"), ID: "M"})
	assert.ErrorContains(t, err, "not found")
	_, err = CopyIFlow(mock.Executer(), IFlowCopyOptions{FromID: "Orders_In", FromDir: target, TargetDir: t.TempDir(), ID: "M"})
	assert.ErrorContains(t, err, "either")
}

func TestManifestWrapping(t *testing.T) {
	long := strings.Repeat("ä", 50) // 100 bytes
	lines := wrapManifestLine("Bundle-Name: " + long)
	for _, l := range lines {
		assert.LessOrEqual(t, len(l), 72)
	}
	joined := lines[0]
	for _, l := range lines[1:] {
		require.True(t, strings.HasPrefix(l, " "))
		joined += l[1:]
	}
	assert.Equal(t, "Bundle-Name: "+long, joined)
	h := parseManifest(setManifestHeaders([]byte("Manifest-Version: 1.0\n"), map[string]string{"Bundle-Name": long}))
	assert.Equal(t, long, h["Bundle-Name"])
}
