package ops

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindPDDependencies(t *testing.T) {
	root := t.TempDir()
	flow := filepath.Join(root, "Orders", "Send_Email")
	writeFile(t, filepath.Join(flow, "META-INF", "MANIFEST.MF"), "Bundle-SymbolicName: Send_Email; singleton:=true\nSAP-BundleType: IntegrationFlow\n")
	writeFile(t, filepath.Join(flow, "src/main/resources/scenarioflows/integrationflow/Send_Email.iflw"), `<bpmn2:definitions>
  <bpmn2:process id="Process_1">
    <bpmn2:callActivity id="CallActivity_3" name="Mapping">
      <ifl:property><key>mappinguri</key><value>pd:OMS:now_email:Binary</value></ifl:property>
    </bpmn2:callActivity>
    <bpmn2:callActivity id="CallActivity_7" name="Dynamic">
      <ifl:property><key>mappinguri</key><value>pd:${property.partner}:mapping:Binary</value></ifl:property>
    </bpmn2:callActivity>
    <bpmn2:callActivity id="CallActivity_9"><ifl:property><key>x</key><value>pd:ONE_OMS:endpoint:String</value></ifl:property></bpmn2:callActivity>
  </bpmn2:process>
</bpmn2:definitions>`)
	writeFile(t, filepath.Join(flow, "src/main/resources/script/pd.groovy"), `def service = ITApiFactory.getService(PartnerDirectoryService.class, null)
def a = service.getParameter("timeout", "ONE_OMS", String.class)
def b = service.getParameter("template", pid, String.class)`)
	pd := filepath.Join(root, "pd")
	writeFile(t, filepath.Join(pd, "ONE_OMS", "String.properties"), "endpoint=x\n")

	res, err := FindPDDependencies(context.Background(), root, PDDependenciesFilter{ResourcesPath: pd})
	require.NoError(t, err)
	assert.ElementsMatch(t, []PDReference{
		{Pid: "OMS", ID: "now_email", Kind: "Binary", Artifact: "Send_Email", Package: "Orders", File: "Orders/Send_Email/src/main/resources/scenarioflows/integrationflow/Send_Email.iflw", Step: "CallActivity_3"},
		{Pid: "ONE_OMS", ID: "endpoint", Kind: "String", Artifact: "Send_Email", Package: "Orders", File: "Orders/Send_Email/src/main/resources/scenarioflows/integrationflow/Send_Email.iflw", Step: "CallActivity_9"},
		{Pid: "ONE_OMS", ID: "timeout", Artifact: "Send_Email", Package: "Orders", File: "Orders/Send_Email/src/main/resources/script/pd.groovy"},
		{Pid: "", ID: "template", Artifact: "Send_Email", Package: "Orders", File: "Orders/Send_Email/src/main/resources/script/pd.groovy"},
	}, res.References)
	require.Len(t, res.Dynamic, 1)
	assert.Equal(t, "CallActivity_7", res.Dynamic[0].Step)
	assert.Equal(t, "pd:${property.partner}:mapping:Binary", res.Dynamic[0].Expression)
	assert.Equal(t, []string{"OMS"}, res.UnknownPids, "pd:OMS is referenced but the PID is ONE_OMS")

	res, err = FindPDDependencies(context.Background(), root, PDDependenciesFilter{Pid: "ONE_OMS", ID: "endpoint"})
	require.NoError(t, err)
	assert.Len(t, res.References, 1)
}
