package cpitest

import (
	"archive/zip"
	"bytes"
	"cmp"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"html"
	"math/big"
	mrand "math/rand/v2"
	"sort"
	"strings"
	"time"

	"github.com/cpars-innovation/cpicli/pkg/iflow"
)

// DemoTiers are the variants of the demo landscape.
var DemoTiers = []string{"dev", "test", "prod"}

// demoReceiver is a receiver channel of a demo flow.
type demoReceiver struct {
	Name, Adapter string
	Props         map[string]string
	// Async receivers (ProcessDirect, JMS) hang off the end event; the
	// others are called by a request-reply step.
	Async bool
}

// demoFlow describes one integration flow of the demo landscape.
type demoFlow struct {
	ID, Name, Package, Version string
	SenderAdapter              string // empty: started by a timer
	SenderProps                map[string]string
	Receivers                  []demoReceiver
	Scripts                    map[string]string
	Params                     map[string]string
	ModifiedBy                 string
	Deployed                   bool
	EndpointPath               string // HTTPS sender path (runtime endpoint)
}

const demoKeysScript = `import com.sap.gateway.ip.core.customdev.util.Message

def Message processData(Message message) {
    def order = message.getHeaders().get("OrderNo") ?: message.getProperty("OrderNo")
    def log = messageLogFactory.getMessageLog(message)
    if (log != null && order != null) {
        log.addCustomHeaderProperty("OrderNo", order.toString())
    }
    message.setHeader("SAP_ApplicationID", order)
    return message
}
`

const demoMapScript = `import com.sap.gateway.ip.core.customdev.util.Message

def Message processData(Message message) {
    def body = message.getBody(String)
    message.setBody(body.replace("<Order>", "<Invoice>").replace("</Order>", "</Invoice>"))
    return message
}
`

// demoFlows returns the flows of a tier: dev is ahead of test, test ahead of prod.
func demoFlows(tier string) []demoFlow {
	host := map[string]string{"dev": "dev", "test": "qa", "prod": "www"}[tier]
	routeVersion := map[string]string{"dev": "1.0.4", "test": "1.0.3", "prod": "1.0.2"}[tier]
	flows := []demoFlow{
		{ID: "Orders_In", Name: "Orders inbound", Package: "Orders", Version: "1.2.0", SenderAdapter: "HTTPS",
			SenderProps: map[string]string{"urlPath": "/orders/in"}, EndpointPath: "/http/orders/in",
			Receivers: []demoReceiver{{Name: "OrdersRoute", Adapter: "ProcessDirect", Async: true, Props: map[string]string{"address": "/orders/route"}}},
			Scripts:   map[string]string{"setKeys.groovy": demoKeysScript}, Deployed: true},
		{ID: "Orders_Route", Name: "Orders routing", Package: "Orders", Version: routeVersion, SenderAdapter: "ProcessDirect",
			SenderProps: map[string]string{"address": "/orders/route"},
			Receivers: []demoReceiver{
				{Name: "ERP", Adapter: "HTTP", Props: map[string]string{"httpAddressWithoutQuery": "{{ERP_URL}}", "credentialName": "ERP_User"}},
				{Name: "Billing", Adapter: "JMS", Async: true, Props: map[string]string{"QueueName_outbound": "Orders_Billing"}},
			},
			Params: map[string]string{"ERP_URL": "https://erp-" + host + ".example.com/sap/opu/odata/orders"}, Deployed: true},
		{ID: "Billing_In", Name: "Billing inbound", Package: "Billing", Version: "1.1.0", SenderAdapter: "JMS",
			SenderProps: map[string]string{"QueueName_inbound": "Orders_Billing"},
			Receivers:   []demoReceiver{{Name: "BillingPost", Adapter: "ProcessDirect", Async: true, Props: map[string]string{"address": "/billing/post"}}},
			Scripts:     map[string]string{"setKeys.groovy": demoKeysScript}, Deployed: true},
		{ID: "Billing_Post", Name: "Billing posting", Package: "Billing", Version: "1.3.1", SenderAdapter: "ProcessDirect",
			SenderProps: map[string]string{"address": "/billing/post"},
			Receivers: []demoReceiver{{Name: "Finance", Adapter: "HTTP", Props: map[string]string{
				"httpAddressWithoutQuery": "{{FINANCE_URL}}", "credentialName": "Finance_OAuth", "privateKeyAlias": "finance_client"}}},
			Scripts: map[string]string{"toInvoice.groovy": demoMapScript},
			Params:  map[string]string{"FINANCE_URL": "https://finance-" + host + ".example.com/api/invoices", "Timeout": "60000"}, Deployed: true},
		{ID: "Partner_Notify", Name: "Partner notification", Package: "Partners", Version: "1.0.0",
			Receivers: []demoReceiver{{Name: "PartnerSFTP", Adapter: "SFTP", Props: map[string]string{
				"host": "sftp.partner.example.com", "path": "/inbound", "credentialName": "Partner_SFTP"}}},
			Params: map[string]string{"Partner": "ACME"}, Deployed: true},
	}
	switch tier {
	case "dev":
		// a new flow not promoted yet: orders come back from the returns portal
		// with a new correlation ID and the same OrderNo
		flows = append(flows, demoFlow{ID: "Returns_In", Name: "Returns inbound", Package: "Orders", Version: "0.1.0", SenderAdapter: "HTTPS",
			SenderProps: map[string]string{"urlPath": "/returns/in"}, EndpointPath: "/http/returns/in",
			Receivers: []demoReceiver{{Name: "ReturnsAPI", Adapter: "HTTP", Props: map[string]string{
				"httpAddressWithoutQuery": "https://returns-dev.example.com/api", "credentialName": "Returns_API"}}},
			Scripts: map[string]string{"setKeys.groovy": demoKeysScript}, Deployed: true})
	case "prod":
		// changed on the tenant outside the pipeline (drift)
		for i := range flows {
			if flows[i].ID == "Billing_Post" {
				flows[i].Params["Timeout"] = "120000"
				flows[i].ModifiedBy = "jane.doe@customer.example"
			}
		}
	}
	return flows
}

// demoIFlow writes the .iflw model of a flow and lays it out.
func demoIFlow(f demoFlow) ([]byte, error) {
	var b strings.Builder
	prop := func(k, v string) string {
		return fmt.Sprintf("<ifl:property><key>%s</key><value>%s</value></ifl:property>", k, html.EscapeString(v))
	}
	props := func(m map[string]string) string {
		var s strings.Builder
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			s.WriteString(prop(k, m[k]))
		}
		return s.String()
	}
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<bpmn2:definitions xmlns:bpmn2="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:bpmndi="http://www.omg.org/spec/BPMN/20100524/DI" xmlns:dc="http://www.omg.org/spec/DD/20100524/DC" xmlns:di="http://www.omg.org/spec/DD/20100524/DI" xmlns:ifl="http:///com.sap.ifl.model/Ifl.xsd" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" id="Definitions_1">
<bpmn2:collaboration id="Collaboration_1" name="Default Collaboration">
<bpmn2:extensionElements>` + prop("namespaceMapping", "") + prop("log", "All events") + prop("returnExceptionToSender", "false") +
		prop("cmdVariantUri", "ctype::IFlowVariant/cname::IFlowConfiguration/version::1.2.4") + `</bpmn2:extensionElements>
`)
	if f.SenderAdapter != "" {
		b.WriteString(`<bpmn2:participant id="Participant_Sender" ifl:type="EndpointSender" name="Sender"><bpmn2:extensionElements>` +
			prop("enableBasicAuthentication", "false") + prop("ifl:type", "EndpointSender") + `</bpmn2:extensionElements></bpmn2:participant>
`)
	}
	b.WriteString(`<bpmn2:participant id="Participant_Process" ifl:type="IntegrationProcess" name="Integration Process" processRef="Process_1"><bpmn2:extensionElements/></bpmn2:participant>
`)
	for i, r := range f.Receivers {
		fmt.Fprintf(&b, `<bpmn2:participant id="Participant_R%d" ifl:type="EndpointRecevier" name="%s"><bpmn2:extensionElements>%s</bpmn2:extensionElements></bpmn2:participant>
`, i, r.Name, prop("ifl:type", "EndpointRecevier"))
	}
	if f.SenderAdapter != "" {
		p := map[string]string{"ComponentType": f.SenderAdapter, "direction": "Sender", "Name": f.SenderAdapter,
			"cmdVariantUri": "ctype::AdapterVariant/cname::sap:" + f.SenderAdapter + "/tp::" + f.SenderAdapter + "/direction::Sender"}
		for k, v := range f.SenderProps {
			p[k] = v
		}
		fmt.Fprintf(&b, `<bpmn2:messageFlow id="MessageFlow_Sender" name="%s" sourceRef="Participant_Sender" targetRef="StartEvent_1"><bpmn2:extensionElements>%s</bpmn2:extensionElements></bpmn2:messageFlow>
`, f.SenderAdapter, props(p))
	}
	for i, r := range f.Receivers {
		p := map[string]string{"ComponentType": r.Adapter, "direction": "Receiver", "Name": r.Adapter,
			"cmdVariantUri": "ctype::AdapterVariant/cname::sap:" + r.Adapter + "/tp::" + r.Adapter + "/direction::Receiver"}
		for k, v := range r.Props {
			p[k] = v
		}
		source := fmt.Sprintf("ServiceTask_%d", i)
		if r.Async {
			source = "EndEvent_1"
		}
		fmt.Fprintf(&b, `<bpmn2:messageFlow id="MessageFlow_R%d" name="%s" sourceRef="%s" targetRef="Participant_R%d"><bpmn2:extensionElements>%s</bpmn2:extensionElements></bpmn2:messageFlow>
`, i, r.Adapter, source, i, props(p))
	}
	b.WriteString(`</bpmn2:collaboration>
<bpmn2:process id="Process_1" name="Integration Process">
<bpmn2:extensionElements>` + prop("transactionTimeout", "30") + prop("cmdVariantUri", "ctype::FlowElementVariant/cname::IntegrationProcess/version::1.2.1") +
		prop("transactionalHandling", "Not Required") + `</bpmn2:extensionElements>
`)
	if f.SenderAdapter != "" {
		b.WriteString(`<bpmn2:startEvent id="StartEvent_1" name="Start"><bpmn2:extensionElements>` + prop("componentVersion", "1.0") +
			prop("cmdVariantUri", "ctype::FlowstepVariant/cname::MessageStartEvent/version::1.0") + `</bpmn2:extensionElements><bpmn2:messageEventDefinition/></bpmn2:startEvent>
`)
	} else {
		b.WriteString(`<bpmn2:startEvent id="StartEvent_1" name="Every hour"><bpmn2:extensionElements>` + prop("scheduleKey", "hourly") +
			prop("cmdVariantUri", "ctype::FlowstepVariant/cname::intermediatetimer/version::1.0") + `</bpmn2:extensionElements><bpmn2:timerEventDefinition/></bpmn2:startEvent>
`)
	}
	steps := []string{"StartEvent_1"}
	scripts := make([]string, 0, len(f.Scripts))
	for name := range f.Scripts {
		scripts = append(scripts, name)
	}
	sort.Strings(scripts)
	for i, name := range scripts {
		id := fmt.Sprintf("CallActivity_%d", i)
		fmt.Fprintf(&b, `<bpmn2:callActivity id="%s" name="%s"><bpmn2:extensionElements>%s%s%s%s</bpmn2:extensionElements></bpmn2:callActivity>
`, id, strings.TrimSuffix(name, ".groovy"), prop("scriptFunction", ""), prop("scriptBundleId", ""), prop("script", name),
			prop("cmdVariantUri", "ctype::FlowstepVariant/cname::GroovyScript/version::1.1.2"))
		steps = append(steps, id)
	}
	for i, r := range f.Receivers {
		if r.Async {
			continue
		}
		id := fmt.Sprintf("ServiceTask_%d", i)
		fmt.Fprintf(&b, `<bpmn2:serviceTask id="%s" name="Call %s"><bpmn2:extensionElements>%s%s</bpmn2:extensionElements></bpmn2:serviceTask>
`, id, r.Name, prop("activityType", "ExternalCall"), prop("cmdVariantUri", "ctype::FlowstepVariant/cname::ExternalCall/version::1.0.4"))
		steps = append(steps, id)
	}
	b.WriteString(`<bpmn2:endEvent id="EndEvent_1" name="End"><bpmn2:extensionElements>` + prop("componentVersion", "1.1") +
		prop("cmdVariantUri", "ctype::FlowstepVariant/cname::MessageEndEvent/version::1.1") + `</bpmn2:extensionElements><bpmn2:messageEventDefinition/></bpmn2:endEvent>
`)
	steps = append(steps, "EndEvent_1")
	for i := 0; i+1 < len(steps); i++ {
		fmt.Fprintf(&b, `<bpmn2:sequenceFlow id="SequenceFlow_%d" sourceRef="%s" targetRef="%s"/>
`, i, steps[i], steps[i+1])
	}
	b.WriteString(`</bpmn2:process>
<bpmndi:BPMNDiagram id="BPMNDiagram_1" name="Default Collaboration Diagram"><bpmndi:BPMNPlane bpmnElement="Collaboration_1" id="BPMNPlane_1"></bpmndi:BPMNPlane></bpmndi:BPMNDiagram>
</bpmn2:definitions>
`)
	out, _, err := iflow.Layout([]byte(b.String()), iflow.LayoutOptions{Mode: iflow.LayoutFull})
	return out, err
}

// demoZip builds the artifact archive of a flow.
func demoZip(f demoFlow, model []byte) ([]byte, map[string]Resource, error) {
	files := map[string][]byte{
		"META-INF/MANIFEST.MF": []byte("Manifest-Version: 1.0\r\nBundle-ManifestVersion: 2\r\nBundle-Name: " + f.Name +
			"\r\nBundle-SymbolicName: " + f.ID + "; singleton:=true\r\nBundle-Version: " + f.Version +
			"\r\nSAP-BundleType: IntegrationFlow\r\nSAP-NodeType: IFLMAP\r\nSAP-RuntimeProfile: iflmap\r\nOrigin-Bundle-Name: " + f.Name +
			"\r\nOrigin-Bundle-SymbolicName: " + f.ID + "\r\n\r\n"),
		"metainfo.prop": []byte("description=" + f.Name + " (demo)\n"),
		"src/main/resources/scenarioflows/integrationflow/" + f.ID + ".iflw": model,
	}
	resources := map[string]Resource{f.ID + ".iflw": {Type: "iflw", Content: model}}
	if len(f.Params) > 0 {
		var p, def strings.Builder
		keys := make([]string, 0, len(f.Params))
		for k := range f.Params {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		def.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="no"?><parameters><param_references/>`)
		for _, k := range keys {
			fmt.Fprintf(&p, "%s=%s\n", k, strings.ReplaceAll(f.Params[k], ":", `\:`))
			fmt.Fprintf(&def, `<parameter><name>%s</name><type>xsd:string</type><isRequired>true</isRequired></parameter>`, k)
		}
		def.WriteString(`</parameters>`)
		files["src/main/resources/parameters.prop"] = []byte(p.String())
		files["src/main/resources/parameters.propdef"] = []byte(def.String())
	}
	for name, content := range f.Scripts {
		files["src/main/resources/script/"+name] = []byte(content)
		resources[name] = Resource{Type: "groovy", Content: []byte(content)}
	}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, n := range names {
		w, err := zw.Create(n)
		if err != nil {
			return nil, nil, err
		}
		if _, err := w.Write(files[n]); err != nil {
			return nil, nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, nil, err
	}
	return buf.Bytes(), resources, nil
}

func demoCert(alias string, notAfter time.Time) (KeystoreEntry, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return KeystoreEntry{}, err
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: alias + ".example.com", Organization: []string{"Demo"}},
		NotBefore: notAfter.AddDate(-1, 0, 0), NotAfter: notAfter}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	return KeystoreEntry{Alias: alias, NotAfter: notAfter, DER: der}, err
}

// SeedDemo fills the tenant with a demo landscape for one tier (dev, test or
// prod): packages, flows with real models, parameters, deployments,
// credentials, keystore entries, Partner Directory parameters and a day of
// message processing logs with failures, custom headers and steps. The
// tiers differ the way real ones do: dev is ahead (a newer version, a new
// flow, a draft), prod has a change made on the tenant and an expiring
// certificate, and misses a credential the new flow needs.
func SeedDemo(m *Tenant, tier string, now time.Time) error {
	if !strings.Contains(" "+strings.Join(DemoTiers, " ")+" ", " "+tier+" ") {
		return fmt.Errorf("unknown demo tier %q (%s)", tier, strings.Join(DemoTiers, ", "))
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Packages = []Package{{ID: "Orders", Name: "Orders", Version: "1.0.0"}, {ID: "Billing", Name: "Billing", Version: "1.0.0"},
		{ID: "Partners", Name: "Partners", Version: "1.0.0"}}
	if m.Artifacts == nil {
		m.Artifacts = map[string]*Artifact{}
	}
	if m.Inbound == nil {
		m.Inbound = map[string]*Inbound{}
	}
	for i, f := range demoFlows(tier) {
		model, err := demoIFlow(f)
		if err != nil {
			return fmt.Errorf("%s: %w", f.ID, err)
		}
		zipData, resources, err := demoZip(f, model)
		if err != nil {
			return err
		}
		a := &Artifact{Type: "Integration", DesignVersion: f.Version, Package: f.Package, Name: f.Name, Zip: zipData, Resources: resources,
			Parameters: map[string]string{}, ModifiedAt: now.Add(-time.Duration(48+i*24) * time.Hour), ModifiedBy: "ci-pipeline"}
		for k, v := range f.Params {
			a.Parameters[k] = v
		}
		if f.ModifiedBy != "" {
			a.ModifiedBy, a.ModifiedAt = f.ModifiedBy, now.Add(-5*time.Hour)
		}
		if f.Deployed {
			a.Runtime = &Runtime{Version: f.Version, Status: "STARTED", DeployedOn: a.ModifiedAt}
		}
		if f.EndpointPath != "" {
			a.EndpointURL = strings.TrimSuffix(cmp.Or(m.EndpointBase, m.URL()), "/") + f.EndpointPath
			m.Inbound[f.EndpointPath] = &Inbound{Artifact: f.ID, Response: "<ack/>", ContentType: "application/xml"}
		}
		m.Artifacts[f.ID] = a
	}
	if tier == "dev" {
		// someone is editing in the Web UI: a draft, the runtime keeps the last version
		m.Artifacts["Billing_Post"].DesignVersion = "Active"
		m.Artifacts["Billing_Post"].ModifiedBy, m.Artifacts["Billing_Post"].ModifiedAt = "dev.user@customer.example", now.Add(-30*time.Minute)
	}

	m.Credentials = map[string]map[string]map[string]any{
		"UserCredentials": {
			"ERP_User":     {"Name": "ERP_User", "Kind": "default", "User": "RFC_ORDERS", "Password": "secret"},
			"Partner_SFTP": {"Name": "Partner_SFTP", "Kind": "default", "User": "acme", "Password": "secret"},
		},
		"OAuth2ClientCredentials": {
			"Finance_OAuth": {"Name": "Finance_OAuth", "TokenServiceUrl": "https://finance.example.com/oauth/token", "ClientId": "cpi", "ClientSecret": "secret"},
		},
	}
	if tier != "prod" {
		m.Credentials["UserCredentials"]["Returns_API"] = map[string]any{"Name": "Returns_API", "Kind": "default", "User": "returns", "Password": "secret"}
	}
	expiry := map[string]time.Duration{"dev": 300 * 24 * time.Hour, "test": 200 * 24 * time.Hour, "prod": 20 * 24 * time.Hour}[tier]
	for _, c := range []struct {
		alias string
		in    time.Duration
	}{{"finance_client", expiry}, {"sap_cloud_root", 3 * 365 * 24 * time.Hour}} {
		e, err := demoCert(c.alias, now.Add(c.in).Truncate(time.Second))
		if err != nil {
			return err
		}
		m.Keystore = append(m.Keystore, e)
	}
	m.PDStrings = map[string]string{"ACME/SFTP_Directory": "/inbound/" + tier, "ACME/Format": "CSV"}

	if m.Raw == nil {
		m.Raw = map[string]any{}
	}
	m.Raw["/api/v1/DataStores"] = []map[string]any{}
	m.Raw["/api/v1/LogFiles"] = []map[string]any{}

	m.FilterMessageLogs, m.Live = true, true
	m.MessageLogSteps = [][]MessageLog{demoMessageLogs(tier, now)}
	return nil
}

// demoMessageLogs generates a day of messages, newest first: orders run
// Orders_In -> Orders_Route -> (JMS) Billing_In -> Billing_Post with the
// OrderNo custom header; some postings fail; on dev some orders come back
// through Returns_In with a new correlation ID; Partner_Notify runs hourly.
func demoMessageLogs(tier string, now time.Time) []MessageLog {
	rng := mrand.New(mrand.NewPCG(42, uint64(len(tier))))
	var logs []MessageLog
	seq := 0
	guid := func() string {
		seq++
		return fmt.Sprintf("AG%s%024d", strings.ToUpper(tier[:1]), seq)
	}
	order := 4500017000
	for h := 24; h >= 1; h-- {
		hourStart := now.Add(-time.Duration(h) * time.Hour)
		for i := 0; i < 5; i++ {
			order++
			t := hourStart.Add(time.Duration(i*11+rng.IntN(5)) * time.Minute)
			corr := fmt.Sprintf("C-%s-%d", tier, order)
			key := map[string]string{"OrderNo": fmt.Sprint(order)}
			pkg := map[string]string{"Orders_In": "Orders", "Orders_Route": "Orders", "Billing_In": "Billing", "Billing_Post": "Billing"}
			in := MessageLog{Guid: guid(), Artifact: "Orders_In", Package: pkg["Orders_In"], Status: "COMPLETED", CorrelationID: corr,
				ApplicationID: fmt.Sprint(order), Start: t, End: t.Add(900 * time.Millisecond), Headers: key}
			route := MessageLog{Guid: guid(), Artifact: "Orders_Route", Package: "Orders", Status: "COMPLETED", CorrelationID: corr,
				Predecessor: in.Guid, Start: t.Add(100 * time.Millisecond), End: t.Add(800 * time.Millisecond)}
			bin := MessageLog{Guid: guid(), Artifact: "Billing_In", Package: "Billing", Status: "COMPLETED", CorrelationID: corr,
				Start: t.Add(2 * time.Second), End: t.Add(3 * time.Second), Headers: key}
			post := MessageLog{Guid: guid(), Artifact: "Billing_Post", Package: "Billing", Status: "COMPLETED", CorrelationID: corr,
				Predecessor: bin.Guid, Start: t.Add(2100 * time.Millisecond), End: t.Add(2900 * time.Millisecond)}
			switch r := rng.IntN(20); {
			case r == 0:
				post.Status = "FAILED"
				post.ErrorText = fmt.Sprintf("com.sap.gateway.core.ip.component.odata.exception.OsciException: HTTP 500 from finance-%s.example.com: posting period 2026-10 is closed for order %d (request %s)",
					tier, order, guid())
				post.Steps = []Step{{StepID: "s1", ModelStepID: "CallActivity_0", Activity: "toInvoice", Status: "COMPLETED"},
					{StepID: "s2", ModelStepID: "ServiceTask_0", Activity: "Call Finance", Status: "FAILED", Error: post.ErrorText}}
				bin.Status = "COMPLETED"
			case r == 1 && tier != "dev":
				route.Status = "RETRY"
				route.ErrorText = "java.net.SocketTimeoutException: Read timed out calling erp"
			}
			logs = append(logs, in, route, bin, post)
			if tier == "dev" && order%4 == 0 {
				rt := t.Add(20 * time.Minute)
				logs = append(logs, MessageLog{Guid: guid(), Artifact: "Returns_In", Package: "Orders", Status: "COMPLETED",
					CorrelationID: fmt.Sprintf("C-%s-R%d", tier, order), Start: rt, End: rt.Add(time.Second), Headers: key})
			}
		}
		pt := hourStart.Add(58 * time.Minute)
		notify := MessageLog{Guid: guid(), Artifact: "Partner_Notify", Package: "Partners", Status: "COMPLETED",
			CorrelationID: fmt.Sprintf("C-%s-N%d", tier, h), Start: pt, End: pt.Add(4 * time.Second)}
		if h%9 == 0 {
			notify.Status, notify.ErrorText = "FAILED", "com.jcraft.jsch.JSchException: Auth fail for user acme at sftp.partner.example.com"
		}
		logs = append(logs, notify)
	}
	sort.SliceStable(logs, func(i, j int) bool { return logs[i].Start.After(logs[j].Start) })
	return logs
}
