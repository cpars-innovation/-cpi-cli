package ops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnalyzeIFlowLinks(t *testing.T) {
	fsys := fstest.MapFS{}
	for name, content := range testIFlowFiles("Orders_In") {
		fsys[name] = &fstest.MapFile{Data: []byte(content)}
	}
	model := strings.Replace(testIFlow, "<value>/billing/in</value>", "<value>{{Billing Address}}</value>", 1)
	model = strings.Replace(model, "<key>script</key><value>logPayload.groovy</value>",
		"<key>script</key><value>logPayload.groovy</value></ifl:property><ifl:property><key>x</key><value>pd:SAP_SYSTEM_001:Identity:Binary</value>", 1)
	fsys["src/main/resources/scenarioflows/integrationflow/Orders_In.iflw"] = &fstest.MapFile{Data: []byte(model)}
	fsys["src/main/resources/script/pd.groovy"] = &fstest.MapFile{Data: []byte(`def m = service.getParameter("Mapping", "SAP_SYSTEM_001", String.class)`)}
	fsys["src/main/resources/parameters.prop"] = &fstest.MapFile{Data: []byte("Billing\\ Address=/billing/in\nHost=https\\://s4.example.com\n")}

	f, err := AnalyzeIFlow(fsys)
	require.NoError(t, err)
	assert.Equal(t, []Trigger{{Adapter: "SFTP"}, {Adapter: "ProcessDirect", Address: "{{Billing Address}}"}}, f.Receivers)
	assert.Equal(t, map[string]string{"Billing Address": "/billing/in"}, f.AddressParameters, "only parameters used in addresses")
	assert.Equal(t, []string{"SAP_SYSTEM_001:Identity", "SAP_SYSTEM_001:Mapping"}, f.PDReferences)
	assert.Equal(t, []string{"Billing Address", "Host"}, f.Parameters)
}

func testDiscovery() *Discovery {
	flow := func(id, pkg string) IFlowFacts {
		return IFlowFacts{ID: id, Name: id + " Flow", PackageID: pkg, Triggers: []Trigger{}, Receivers: []Trigger{}}
	}
	orders := flow("Orders_In", "Sales")
	orders.Triggers = []Trigger{{Adapter: "HTTPS", Address: "/orders/in"}}
	orders.Receivers = []Trigger{{Adapter: "ProcessDirect", Address: "{{Target}}"}, {Adapter: "SFTP"}}
	orders.AddressParameters = map[string]string{"Target": "/billing/in"}
	orders.CredentialRefs = []string{"SFTP_User"}
	orders.Scripts = []ScriptFacts{{Name: "log.groovy", Hash: "abc", Language: "groovy", Lines: 5, CustomHeaders: []string{"OrderId"}}}
	orders.HeadersSet = []string{"SAP_ApplicationID"}

	billing := flow("Billing", "Finance")
	billing.Triggers = []Trigger{{Adapter: "ProcessDirect", Address: "/billing/in"}}
	billing.Receivers = []Trigger{{Adapter: "HTTP", Address: "https://S4.example.com:443/sap/opu/odata"}, {Adapter: "JMS", Address: "Q.Invoices"}}
	billing.Scripts = []ScriptFacts{{Name: "logging.groovy", Hash: "abc", Language: "groovy", Lines: 5}}
	billing.PDReferences = []string{"SAP_SYSTEM_001:Identity"}
	billing.CredentialRefs = []string{"SFTP_User", "{{Credential}}"}

	invoices := flow("Invoice_Send", "Finance")
	invoices.Triggers = []Trigger{{Adapter: "JMS", Address: "Q.Invoices"}, {Adapter: "Timer"}}

	// unresolved parameters with the same name must not link flows
	a := flow("A", "Other")
	a.Receivers = []Trigger{{Adapter: "ProcessDirect", Address: "{{Address}}"}}
	b := flow("B", "Other")
	b.Triggers = []Trigger{{Adapter: "ProcessDirect", Address: "{{Address}}"}}

	return &Discovery{GeneratedAt: time.Unix(0, 0).UTC(), Source: "test",
		Packages: []DiscoveredPackage{{ID: "Sales", Name: "Sales Orders"}, {ID: "Finance"}, {ID: "Other"}},
		IFlows:   []IFlowFacts{orders, billing, invoices, a, b}}
}

func edgeSet(edges []GraphEdge) []string {
	var s []string
	for _, e := range edges {
		s = append(s, e.From+" -"+e.Type+"-> "+e.To)
	}
	return s
}

func TestBuildGraph(t *testing.T) {
	g := BuildGraph(testDiscovery())
	edges := edgeSet(g.Edges)

	assert.Contains(t, edges, "package:Sales -contains-> iflow:Orders_In")
	assert.Contains(t, edges, "iflow:Orders_In -exposes-> endpoint:HTTPS:/orders/in")
	assert.Contains(t, edges, "iflow:Orders_In -calls-> endpoint:ProcessDirect:/billing/in")
	assert.Contains(t, edges, "iflow:Billing -exposes-> endpoint:ProcessDirect:/billing/in")
	assert.Contains(t, edges, "iflow:Orders_In -sends_to-> iflow:Billing")
	assert.Contains(t, edges, "iflow:Billing -sends_to-> iflow:Invoice_Send", "JMS queue")
	assert.Contains(t, edges, "iflow:Billing -calls_system-> system:s4.example.com")
	assert.Contains(t, edges, "iflow:Billing -reads_pd-> pd:SAP_SYSTEM_001:Identity")
	assert.Contains(t, edges, "iflow:Orders_In -uses_script-> script:abc")
	assert.Contains(t, edges, "iflow:Billing -uses_script-> script:abc", "identical scripts are one node")
	assert.Contains(t, edges, "script:abc -logs_header-> customheader:OrderId")
	assert.Contains(t, edges, "iflow:Billing -uses_credential-> credential:SFTP_User")
	assert.Contains(t, edges, "iflow:Orders_In -sets_header-> header:SAP_ApplicationID")
	assert.NotContains(t, strings.Join(edges, "\n"), "credential:{{Credential}}")
	assert.NotContains(t, edges, "iflow:A -sends_to-> iflow:B")
	assert.Contains(t, edges, "iflow:A -calls-> endpoint:ProcessDirect:{{Address}}@A")

	for _, e := range g.Edges {
		if e.Type == EdgeCalls && e.From == "iflow:Orders_In" {
			assert.Equal(t, "parameter Target", e.Via)
		}
		if e.Type == EdgeSendsTo && e.From == "iflow:Orders_In" {
			assert.Equal(t, "endpoint:ProcessDirect:/billing/in", e.Via)
			assert.Equal(t, "ProcessDirect", e.Adapter)
		}
	}
	n, ok := g.Node("iflow:Invoice_Send")
	require.True(t, ok)
	assert.Equal(t, "JMS Q.Invoices, Timer", n.Attrs["triggers"])
	assert.Equal(t, "Finance", n.Attrs["package"])
	assert.Equal(t, 5, g.Stats.NodeTypes[NodeIFlow])
	assert.Equal(t, 2, g.Stats.EdgeTypes[EdgeSendsTo])
}

func TestGraphQueries(t *testing.T) {
	g := BuildGraph(testDiscovery())

	t.Run("resolve", func(t *testing.T) {
		n, err := g.Resolve("Billing")
		require.NoError(t, err)
		assert.Equal(t, "iflow:Billing", n.ID)
		n, err = g.Resolve("sales orders")
		require.NoError(t, err)
		assert.Equal(t, "package:Sales", n.ID)
		_, err = g.Resolve("nothing")
		assert.ErrorContains(t, err, "graph_search")
	})

	t.Run("search", func(t *testing.T) {
		res, err := g.Search("billing", nil, 10)
		require.NoError(t, err)
		require.NotEmpty(t, res.Matches)
		assert.Equal(t, "iflow:Billing", res.Matches[0].ID, "exact match first")
		res, err = g.Search("/billing", []string{NodeEndpoint}, 10)
		require.NoError(t, err)
		require.Len(t, res.Matches, 1)
		assert.Equal(t, "endpoint:ProcessDirect:/billing/in", res.Matches[0].ID)
		res, err = g.Search("timer", []string{NodeIFlow}, 10)
		require.NoError(t, err)
		require.Len(t, res.Matches, 1, "attributes are searched")
		_, err = g.Search("x", []string{"flow"}, 10)
		assert.Error(t, err)
	})

	t.Run("neighbors", func(t *testing.T) {
		sub, err := g.Neighbors("Billing", NeighborOptions{Direction: "in", EdgeTypes: []string{EdgeSendsTo}})
		require.NoError(t, err)
		assert.Equal(t, []string{"iflow:Orders_In -sends_to-> iflow:Billing"}, edgeSet(sub.Edges))
		sub, err = g.Neighbors("credential:SFTP_User", NeighborOptions{})
		require.NoError(t, err)
		assert.Len(t, sub.Nodes, 3, "the credential and both flows")
		sub, err = g.Neighbors("Orders_In", NeighborOptions{Direction: "out", EdgeTypes: []string{EdgeSendsTo}, Depth: 3})
		require.NoError(t, err)
		assert.Equal(t, []string{"iflow:Orders_In -sends_to-> iflow:Billing", "iflow:Billing -sends_to-> iflow:Invoice_Send"}, edgeSet(sub.Edges))
		sub, err = g.Neighbors("Orders_In", NeighborOptions{Limit: 2})
		require.NoError(t, err)
		assert.True(t, sub.Truncated)
		assert.Len(t, sub.Edges, 2)
		_, err = g.Neighbors("Orders_In", NeighborOptions{Depth: 4})
		assert.Error(t, err)
	})

	t.Run("path", func(t *testing.T) {
		p, err := g.Path("Orders_In", "Invoice_Send", nil, 0)
		require.NoError(t, err)
		require.True(t, p.Found)
		ids := []string{}
		for _, n := range p.Nodes {
			ids = append(ids, n.ID)
		}
		assert.Equal(t, []string{"iflow:Orders_In", "iflow:Billing", "iflow:Invoice_Send"}, ids)
		p, err = g.Path("A", "B", nil, 0)
		require.NoError(t, err)
		assert.False(t, p.Found, "unresolved addresses do not connect")
		p, err = g.Path("Orders_In", "Billing", []string{EdgeUsesCredential}, 0)
		require.NoError(t, err)
		assert.True(t, p.Found, "shared credential when asked for")
	})
}

func TestLoadGraph(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "graph.json")
	_, err := LoadGraph(file)
	assert.ErrorContains(t, err, "discover")

	// without graph.json, discovery.json is used
	require.NoError(t, WriteDiscovery(testDiscovery(), filepath.Join(dir, "discovery.json")))
	g, err := LoadGraph(file)
	require.NoError(t, err)
	assert.Equal(t, 5, g.Stats.NodeTypes[NodeIFlow])

	require.NoError(t, WriteGraph(BuildGraph(testDiscovery()), file))
	g, err = LoadGraph(file)
	require.NoError(t, err)
	_, err = g.Resolve("Billing")
	require.NoError(t, err, "index rebuilt after loading")
	again, err := LoadGraph(file)
	require.NoError(t, err)
	assert.Same(t, g, again, "cached while the file is unchanged")

	require.NoError(t, os.WriteFile(file, []byte("{"), 0o644))
	_, err = LoadGraph(file)
	assert.Error(t, err)
}
