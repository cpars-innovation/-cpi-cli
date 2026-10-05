// Package cpitest provides an in-memory SAP CPI tenant (httptest server) for
// offline tests of deploy/undeploy flows. It must only be used from tests.
package cpitest

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cpars-innovation/cpicli/pkg/httpclnt"
)

// Runtime is the runtime state of an artifact.
type Runtime struct {
	Version    string
	Status     string
	DeployedOn time.Time // zero: not reported
}

// Artifact scripts the tenant behaviour for one artifact ID.
type Artifact struct {
	Type          string // designtime type, e.g. Integration
	DesignVersion string // empty: designtime artifact does not exist

	// Runtime is what GET IntegrationRuntimeArtifacts returns before a deploy
	// is triggered (nil: 404 not deployed).
	Runtime *Runtime
	// AfterDeploy is returned by runtime GETs after the deploy trigger, one
	// entry per GET; the last entry repeats. nil entries mean 404.
	AfterDeploy []*Runtime
	// TaskStatuses are returned by BuildAndDeployStatus, one per GET; the last
	// repeats. Empty: the status endpoint answers 404.
	TaskStatuses []string
	ErrorInfo    string
	// DeployStatusCode overrides the 202 of the deploy trigger.
	DeployStatusCode int

	// UndeployAfter is the number of runtime GETs after DELETE that still
	// return the artifact (-1: never disappears).
	UndeployAfter int

	// Package and Name are used by the package artifact listing.
	Package string
	Name    string
	// Parameters are the externalised configuration parameters.
	Parameters map[string]string
	// ConfigUpdateStatus overrides the 202 of a parameter update.
	ConfigUpdateStatus int

	// ValidationResult is the body of ValidateIntegrationDesigntimeArtifact
	// (default "Check execution result: Passed").
	ValidationResult string
	// GuidelineStatuses are returned by the execution list after an execute
	// call, one per GET (last repeats); Guidelines are the results.
	GuidelineStatuses []string
	Guidelines        []map[string]any
	guidelineRuns     int
	guidelineGets     int
	// EndpointURL makes the deployed artifact appear in ServiceEndpoints.
	EndpointURL string
	// Resources: name -> type and content; Zip is the $value download
	// (and the content stored by create/update).
	Resources map[string]Resource
	Zip       []byte
	// Uploads counts designtime creates and updates.
	Uploads int

	triggered      bool
	runtimeGets    int
	taskGets       int
	undeployed     bool
	undeployedGets int
}

// Inbound is the behaviour of a runtime endpoint.
type Inbound struct {
	Status      int // default 200
	Response    string
	ContentType string
	// MessageGuid is returned as SAP_MessageProcessingLogID.
	MessageGuid string
	// RequireCSRF makes the endpoint CSRF protected: modifying requests need a
	// token fetched from the endpoint itself.
	RequireCSRF bool
	token       string
}

// ReceivedMessage is a request received by a runtime endpoint.
type ReceivedMessage struct {
	Method, Path string
	Header       http.Header
	Body         string
}

// Package is an integration package of the mock tenant.
type Package struct {
	ID, Name, Version string
}

// Resource is an iFlow resource of the mock tenant.
type Resource struct {
	Type    string
	Content []byte
}

// MessageLog is a message processing log of the mock tenant.
type MessageLog struct {
	Guid, Artifact, Status string
	Start, End             time.Time
	ErrorText              string
	Headers                map[string]string
	Attachments            map[string]string // name -> content (ids att-<guid>-<name>)
	StoreEntries           map[string]string // id -> payload
	Steps                  []Step
}

// Step is a processing step of a message (one run per message).
type Step struct {
	StepID, ModelStepID, Activity, Status, Error string
}

// KeystoreEntry is a certificate of the mock tenant keystore.
type KeystoreEntry struct {
	Alias    string
	NotAfter time.Time
	DER      []byte
}

// Tenant is a mock CPI tenant.
type Tenant struct {
	// Credentials: collection (UserCredentials, OAuth2ClientCredentials,
	// SecureParameters) -> name -> stored properties (including secrets,
	// which are never returned, like on a real tenant).
	Credentials map[string]map[string]map[string]any
	// SecureParametersUnavailable answers 404 for SecureParameters (Cloud Foundry).
	SecureParametersUnavailable bool
	Keystore                    []KeystoreEntry
	// MessageLogSteps: each MessageProcessingLogs query returns the next step
	// (the last step repeats). Single-message endpoints look up all steps.
	MessageLogSteps [][]MessageLog
	// LastMessageLogQuery is the raw query string of the last MPL query.
	LastMessageLogQuery string
	mplQueries          int

	// Inbound configures runtime endpoints (paths starting with /http/ or
	// /cxf/, as in ServiceEndpoints URLs) that receive test messages.
	Inbound map[string]*Inbound
	// Received are the messages received by runtime endpoints.
	Received []ReceivedMessage

	mu        sync.Mutex
	Artifacts map[string]*Artifact
	Packages  []Package
	// StatusOverride, if non-zero, is returned for every API call (e.g. 401).
	StatusOverride int
	// NoCSRF disables CSRF enforcement (by default modifying Basic Auth
	// requests need the token and session cookie from a "Fetch" request).
	NoCSRF     bool
	csrfToken  string
	csrfSerial int
	requests   []string
	server     *httptest.Server
}

// NewTenant starts a mock tenant; it is closed when the test ends.
func NewTenant(t *testing.T, artifacts map[string]*Artifact) *Tenant {
	t.Helper()
	m := &Tenant{Artifacts: artifacts}
	m.server = httptest.NewServer(http.HandlerFunc(m.handle))
	t.Cleanup(m.server.Close)
	return m
}

// HostPort returns the host and port of the mock server.
func (m *Tenant) HostPort() (string, int) {
	return httpclnt.GetHostPort(m.server.URL)
}

// Executer returns an HTTP executer (Basic Auth) pointing at the mock.
func (m *Tenant) Executer() *httpclnt.HTTPExecuter {
	host, port := m.HostPort()
	return httpclnt.New("", "", "", "", "user", "secret", host, "http", port, false)
}

// ExpireCSRF invalidates the current CSRF token (as a tenant does when the
// session ends); the next fetch issues a new one.
func (m *Tenant) ExpireCSRF() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.csrfToken = ""
}

// CSRFFetches returns the number of CSRF token fetch requests.
func (m *Tenant) CSRFFetches() int {
	n := 0
	for _, r := range m.Requests() {
		if r == "GET /api/v1/" {
			n++
		}
	}
	return n
}

// Requests returns "METHOD path" for every request received.
func (m *Tenant) Requests() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.requests...)
}

// Count returns how many requests started with prefix ("METHOD /path").
func (m *Tenant) Count(prefix string) int {
	n := 0
	for _, r := range m.Requests() {
		if strings.HasPrefix(r, prefix) {
			n++
		}
	}
	return n
}

var (
	reDesign          = regexp.MustCompile(`^/api/v1/(\w+)DesigntimeArtifacts\(Id='([^']+)',Version='active'\)$`)
	reDeploy          = regexp.MustCompile(`^/api/v1/Deploy(\w+)DesigntimeArtifact$`)
	reTask            = regexp.MustCompile(`^/api/v1/BuildAndDeployStatus\(TaskId='task-([^']+)'\)$`)
	reRuntime         = regexp.MustCompile(`^/api/v1/IntegrationRuntimeArtifacts\('([^']+)'\)$`)
	rePkgArts         = regexp.MustCompile(`^/api/v1/IntegrationPackages\('([^']+)'\)/(\w+)DesigntimeArtifacts$`)
	reConfigs         = regexp.MustCompile(`^/api/v1/IntegrationDesigntimeArtifacts\(Id='([^']+)',Version='active'\)/Configurations$`)
	reConfig          = regexp.MustCompile(`^/api/v1/IntegrationDesigntimeArtifacts\(Id='([^']+)',Version='active'\)/\$links/Configurations\('([^']+)'\)$`)
	reMPL             = regexp.MustCompile(`^/api/v1/MessageProcessingLogs\('([^']+)'\)(.*)$`)
	reAttValue        = regexp.MustCompile(`^/api/v1/MessageProcessingLogAttachments\('([^']+)'\)/\$value$`)
	reStoreValue      = regexp.MustCompile(`^/api/v1/MessageStoreEntries\('([^']+)'\)/\$value$`)
	reRunSteps        = regexp.MustCompile(`^/api/v1/MessageProcessingLogRuns\('([^']+)'\)/RunSteps$`)
	reGuidelineList   = regexp.MustCompile(`^/api/v1/IntegrationDesigntimeArtifacts\(Id='([^']+)',Version='active'\)/DesignGuidelineExecutionResults$`)
	reGuidelineResult = regexp.MustCompile(`^/api/v1/IntegrationDesigntimeArtifacts\(Id='([^']+)',Version='active'\)/DesignGuidelineExecutionResults\('([^']+)'\)$`)
	reResources       = regexp.MustCompile(`^/api/v1/IntegrationDesigntimeArtifacts\(Id='([^']+)',Version='active'\)/Resources(\(Name='([^']+)',ResourceType='([^']+)'\)/\$value)?$`)
	reDesignValue     = regexp.MustCompile(`^/api/v1/(\w+)DesigntimeArtifacts\(Id='([^']+)',Version='active'\)/\$value$`)
	reCredential      = regexp.MustCompile(`^/api/v1/(UserCredentials|OAuth2ClientCredentials|SecureParameters)(?:\('(.+)'\))?$`)
	reKeystoreCert    = regexp.MustCompile(`^/api/v1/KeystoreEntries\('([0-9A-Fa-f]+)'\)/Certificate/\$value$`)
	reCertImport      = regexp.MustCompile(`^/api/v1/CertificateResources\('([0-9A-Fa-f]+)'\)/\$value$`)
	rePackage         = regexp.MustCompile(`^/api/v1/IntegrationPackages\('([^']+)'\)$`)
	reDesignCreate    = regexp.MustCompile(`^/api/v1/(\w+)DesigntimeArtifacts$`)
	reErrInfo         = regexp.MustCompile(`^/api/v1/IntegrationRuntimeArtifacts\('([^']+)'\)/ErrorInformation/\$value$`)
)

var secretFields = map[string]string{"UserCredentials": "Password", "OAuth2ClientCredentials": "ClientSecret", "SecureParameters": "SecureParam"}

func (m *Tenant) handleCredential(w http.ResponseWriter, r *http.Request, mm []string) {
	coll, name := mm[1], strings.ReplaceAll(mm[2], "''", "'")
	if coll == "SecureParameters" && m.SecureParametersUnavailable {
		notFound(w)
		return
	}
	if m.Credentials == nil {
		m.Credentials = map[string]map[string]map[string]any{}
	}
	if m.Credentials[coll] == nil {
		m.Credentials[coll] = map[string]map[string]any{}
	}
	store := m.Credentials[coll]
	public := func(props map[string]any) map[string]any {
		out := map[string]any{}
		for k, v := range props {
			out[k] = v
		}
		out[secretFields[coll]] = nil
		out["SecurityArtifactDescriptor"] = map[string]any{"Type": "CREDENTIALS", "DeployedBy": "tester", "Status": "DEPLOYED"}
		return out
	}
	switch {
	case r.Method == http.MethodGet && mm[2] == "":
		rows := []map[string]any{}
		for _, n := range sortedKeys(store) {
			rows = append(rows, public(store[n]))
		}
		writeJSON(w, map[string]any{"d": map[string]any{"results": rows}})
	case r.Method == http.MethodGet:
		if props, ok := store[name]; ok {
			writeJSON(w, map[string]any{"d": public(props)})
			return
		}
		notFound(w)
	case r.Method == http.MethodPost || r.Method == http.MethodPut:
		var props map[string]any
		if err := json.NewDecoder(r.Body).Decode(&props); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, exists := store[fmt.Sprint(props["Name"])]
		if (r.Method == http.MethodPost && exists) || (r.Method == http.MethodPut && (name == "" || !exists)) {
			w.WriteHeader(http.StatusConflict)
			return
		}
		store[fmt.Sprint(props["Name"])] = props
		w.WriteHeader(http.StatusAccepted)
	case r.Method == http.MethodDelete:
		if _, ok := store[name]; !ok {
			notFound(w)
			return
		}
		delete(store, name)
		w.WriteHeader(http.StatusAccepted)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (m *Tenant) handleInbound(w http.ResponseWriter, r *http.Request) {
	in := m.Inbound[r.URL.Path]
	if in == nil {
		notFound(w)
		return
	}
	if in.RequireCSRF {
		if r.Method == http.MethodGet && strings.EqualFold(r.Header.Get("X-CSRF-Token"), "fetch") {
			if in.token == "" {
				m.csrfSerial++
				in.token = fmt.Sprintf("rt-token-%d", m.csrfSerial)
			}
			w.Header().Set("X-CSRF-Token", in.token)
			return
		}
		if r.Method != http.MethodGet && (in.token == "" || r.Header.Get("X-CSRF-Token") != in.token) {
			w.Header().Set("X-CSRF-Token", "Required")
			w.WriteHeader(http.StatusForbidden)
			return
		}
	}
	body, _ := io.ReadAll(r.Body)
	m.Received = append(m.Received, ReceivedMessage{Method: r.Method, Path: r.URL.Path, Header: r.Header.Clone(), Body: string(body)})
	if in.MessageGuid != "" {
		w.Header().Set("SAP_MessageProcessingLogID", in.MessageGuid)
	}
	if in.ContentType != "" {
		w.Header().Set("Content-Type", in.ContentType)
	}
	status := in.Status
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	_, _ = w.Write([]byte(in.Response))
}

func mplJSON(l MessageLog) map[string]any {
	return map[string]any{
		"MessageGuid": l.Guid, "Status": l.Status, "CustomStatus": l.Status, "LogLevel": "INFO",
		"LogStart": odataDate(l.Start), "LogEnd": odataDate(l.End), "IntegrationFlowName": l.Artifact,
		"IntegrationArtifact": map[string]string{"Id": l.Artifact, "Name": l.Artifact, "Type": "INTEGRATION_FLOW", "PackageId": "P"},
	}
}

func (m *Tenant) findMessageLog(guid string) *MessageLog {
	for j := len(m.MessageLogSteps) - 1; j >= 0; j-- { // latest state first
		step := m.MessageLogSteps[j]
		for i := range step {
			if step[i].Guid == guid {
				return &step[i]
			}
		}
	}
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func odataDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return fmt.Sprintf("/Date(%d)/", t.UnixMilli())
}

func pick[T any](list []T, i int) T {
	if i >= len(list) {
		return list[len(list)-1]
	}
	return list[i]
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func notFound(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(`{"error":{"code":"Not Found","message":{"value":"Requested entity could not be found."}}}`))
}

func (m *Tenant) handle(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	path := r.URL.Path
	m.requests = append(m.requests, r.Method+" "+path)

	if m.StatusOverride != 0 {
		w.WriteHeader(m.StatusOverride)
		return
	}
	if strings.HasPrefix(path, "/http/") || strings.HasPrefix(path, "/cxf/") {
		m.handleInbound(w, r)
		return
	}
	if r.Method == http.MethodGet && path == "/api/v1/" { // CSRF token fetch
		if strings.EqualFold(r.Header.Get("X-CSRF-Token"), "fetch") {
			if m.csrfToken == "" {
				m.csrfSerial++
				m.csrfToken = fmt.Sprintf("token-%d", m.csrfSerial)
			}
			w.Header().Set("X-CSRF-Token", m.csrfToken)
			http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "session-" + m.csrfToken})
		}
		return
	}
	if !m.NoCSRF && r.Method != http.MethodGet && strings.HasPrefix(r.Header.Get("Authorization"), "Basic ") {
		cookie, err := r.Cookie("JSESSIONID")
		if m.csrfToken == "" || r.Header.Get("X-CSRF-Token") != m.csrfToken || err != nil || cookie.Value != "session-"+m.csrfToken {
			w.Header().Set("X-CSRF-Token", "Required")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("CSRF token validation failed"))
			return
		}
	}

	switch {
	case reCredential.MatchString(path):
		m.handleCredential(w, r, reCredential.FindStringSubmatch(path))

	case r.Method == http.MethodGet && path == "/api/v1/KeystoreEntries":
		rows := []map[string]any{}
		for _, e := range m.Keystore {
			rows = append(rows, map[string]any{"Hexalias": strings.ToUpper(hex.EncodeToString([]byte(e.Alias))), "Alias": e.Alias,
				"Type": "Certificate", "KeyType": "RSA", "KeySize": 2048, "ValidNotAfter": odataDate(e.NotAfter), "SubjectDN": "CN=" + e.Alias})
		}
		writeJSON(w, map[string]any{"d": map[string]any{"results": rows}})

	case r.Method == http.MethodGet && reKeystoreCert.MatchString(path):
		alias, _ := hex.DecodeString(reKeystoreCert.FindStringSubmatch(path)[1])
		for _, e := range m.Keystore {
			if e.Alias == string(alias) {
				_, _ = w.Write(e.DER)
				return
			}
		}
		notFound(w)

	case r.Method == http.MethodPut && reCertImport.MatchString(path):
		alias, _ := hex.DecodeString(reCertImport.FindStringSubmatch(path)[1])
		body, _ := io.ReadAll(r.Body)
		der, err := base64.StdEncoding.DecodeString(string(body))
		if err != nil || r.Header.Get("Content-Type") != "application/pkix-cert" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		for i, e := range m.Keystore {
			if e.Alias == string(alias) {
				if r.URL.Query().Get("update") != "true" {
					w.WriteHeader(http.StatusConflict)
					return
				}
				m.Keystore[i].DER = der
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		m.Keystore = append(m.Keystore, KeystoreEntry{Alias: string(alias), DER: der, NotAfter: time.Now().Add(365 * 24 * time.Hour)})
		w.WriteHeader(http.StatusNoContent)

	case r.Method == http.MethodGet && path == "/api/v1/MessageProcessingLogs":
		m.LastMessageLogQuery = r.URL.RawQuery
		var logs []MessageLog
		if len(m.MessageLogSteps) > 0 {
			logs = pick(m.MessageLogSteps, m.mplQueries)
		}
		m.mplQueries++
		results := []map[string]any{}
		for _, l := range logs {
			results = append(results, mplJSON(l))
		}
		writeJSON(w, map[string]any{"d": map[string]any{"results": results, "__count": fmt.Sprint(len(results))}})

	case r.Method == http.MethodGet && reMPL.MatchString(path):
		mm := reMPL.FindStringSubmatch(path)
		l := m.findMessageLog(mm[1])
		if l == nil {
			notFound(w)
			return
		}
		switch mm[2] {
		case "":
			writeJSON(w, map[string]any{"d": mplJSON(*l)})
		case "/ErrorInformation/$value":
			if l.ErrorText == "" {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(l.ErrorText))
		case "/CustomHeaderProperties":
			rows := []map[string]string{}
			for _, k := range sortedKeys(l.Headers) {
				rows = append(rows, map[string]string{"Name": k, "Value": l.Headers[k]})
			}
			writeJSON(w, map[string]any{"d": map[string]any{"results": rows}})
		case "/AdapterAttributes":
			writeJSON(w, map[string]any{"d": map[string]any{"results": []map[string]string{{"AdapterId": "HTTPS", "Name": "Status", "Value": "200"}}}})
		case "/Attachments":
			rows := []map[string]any{}
			for _, n := range sortedKeys(l.Attachments) {
				rows = append(rows, map[string]any{"Id": "att-" + l.Guid + "-" + n, "Name": n, "ContentType": "text/plain",
					"PayloadSize": len(l.Attachments[n]), "TimeStamp": odataDate(l.End)})
			}
			writeJSON(w, map[string]any{"d": map[string]any{"results": rows}})
		case "/MessageStoreEntries":
			rows := []map[string]any{}
			for _, id := range sortedKeys(l.StoreEntries) {
				rows = append(rows, map[string]any{"Id": id, "MessageGuid": l.Guid, "MessageStoreId": "store", "TimeStamp": odataDate(l.End), "HasAttachments": false})
			}
			writeJSON(w, map[string]any{"d": map[string]any{"results": rows}})
		case "/Runs":
			writeJSON(w, map[string]any{"d": map[string]any{"results": []map[string]any{
				{"Id": "run-" + l.Guid, "RunStart": odataDate(l.Start), "RunStop": odataDate(l.End), "OverallState": l.Status}}}})
		default:
			notFound(w)
		}

	case r.Method == http.MethodGet && reAttValue.MatchString(path):
		id := reAttValue.FindStringSubmatch(path)[1]
		for _, step := range m.MessageLogSteps {
			for _, l := range step {
				for name, content := range l.Attachments {
					if "att-"+l.Guid+"-"+name == id {
						_, _ = w.Write([]byte(content))
						return
					}
				}
			}
		}
		notFound(w)

	case r.Method == http.MethodGet && reStoreValue.MatchString(path):
		id := reStoreValue.FindStringSubmatch(path)[1]
		for _, step := range m.MessageLogSteps {
			for _, l := range step {
				if content, ok := l.StoreEntries[id]; ok {
					_, _ = w.Write([]byte(content))
					return
				}
			}
		}
		notFound(w)

	case r.Method == http.MethodGet && reRunSteps.MatchString(path):
		guid := strings.TrimPrefix(reRunSteps.FindStringSubmatch(path)[1], "run-")
		l := m.findMessageLog(guid)
		if l == nil {
			notFound(w)
			return
		}
		rows := []map[string]any{}
		for _, st := range l.Steps {
			rows = append(rows, map[string]any{"StepId": st.StepID, "ModelStepId": st.ModelStepID, "Activity": st.Activity,
				"Status": st.Status, "Error": st.Error, "StepStart": odataDate(l.Start), "StepStop": odataDate(l.End)})
		}
		writeJSON(w, map[string]any{"d": map[string]any{"results": rows}})

	case r.Method == http.MethodPost && path == "/api/v1/ValidateIntegrationDesigntimeArtifact":
		a := m.Artifacts[strings.Trim(r.URL.Query().Get("Id"), "'")]
		if a == nil || a.DesignVersion == "" {
			notFound(w)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		result := a.ValidationResult
		if result == "" {
			result = "Check execution result: Passed"
		}
		_, _ = w.Write([]byte(result))

	case r.Method == http.MethodPost && path == "/api/v1/ExecuteIntegrationDesigntimeArtifactsGuidelines":
		a := m.Artifacts[strings.Trim(r.URL.Query().Get("Id"), "'")]
		if a == nil || a.DesignVersion == "" {
			notFound(w)
			return
		}
		a.guidelineRuns++
		a.guidelineGets = 0
		writeJSON(w, map[string]any{"d": map[string]any{"ExecutionId": fmt.Sprintf("exec-%d", a.guidelineRuns)}})

	case r.Method == http.MethodGet && reGuidelineList.MatchString(path):
		a := m.Artifacts[reGuidelineList.FindStringSubmatch(path)[1]]
		if a == nil {
			notFound(w)
			return
		}
		rows := []map[string]any{}
		if a.guidelineRuns > 0 && len(a.GuidelineStatuses) > 0 {
			rows = append(rows, map[string]any{"ExecutionId": fmt.Sprintf("exec-%d", a.guidelineRuns), "ArtifactVersion": a.DesignVersion,
				"ExecutionStatus": pick(a.GuidelineStatuses, a.guidelineGets), "ExecutionTime": fmt.Sprint(time.Now().UnixMilli())})
			a.guidelineGets++
		}
		writeJSON(w, map[string]any{"d": map[string]any{"results": rows}})

	case r.Method == http.MethodGet && reGuidelineResult.MatchString(path):
		mm := reGuidelineResult.FindStringSubmatch(path)
		a := m.Artifacts[mm[1]]
		if a == nil {
			notFound(w)
			return
		}
		status := ""
		if len(a.GuidelineStatuses) > 0 {
			status = a.GuidelineStatuses[len(a.GuidelineStatuses)-1]
		}
		writeJSON(w, map[string]any{"d": map[string]any{"ExecutionId": mm[2], "ExecutionStatus": status,
			"DesignGuidelines": map[string]any{"results": a.Guidelines}}})

	case r.Method == http.MethodGet && path == "/api/v1/ServiceEndpoints":
		rows := []map[string]any{}
		filter := r.URL.Query().Get("$filter")
		for _, id := range sortedKeys(m.Artifacts) {
			a := m.Artifacts[id]
			if a.EndpointURL == "" || (filter != "" && filter != "Name eq '"+id+"'") {
				continue
			}
			rows = append(rows, map[string]any{"Name": id, "Id": id + "$endpointAddress=x", "Title": id, "Version": a.DesignVersion, "Protocol": "REST",
				"EntryPoints": map[string]any{"results": []map[string]string{{"Name": id, "Url": a.EndpointURL, "Type": "PROD"}}}})
		}
		writeJSON(w, map[string]any{"d": map[string]any{"results": rows}})

	case r.Method == http.MethodGet && path == "/api/v1/IntegrationRuntimeArtifacts":
		filter := r.URL.Query().Get("$filter")
		rows := []map[string]any{}
		for _, id := range sortedKeys(m.Artifacts) {
			rt := m.Artifacts[id].Runtime
			if rt == nil || (filter != "" && !strings.Contains(filter, "'"+rt.Status+"'")) {
				continue
			}
			rows = append(rows, map[string]any{"Id": id, "Version": rt.Version, "Name": id, "Type": "INTEGRATION_FLOW", "Status": rt.Status, "DeployedOn": odataDate(rt.DeployedOn)})
		}
		writeJSON(w, map[string]any{"d": map[string]any{"results": rows}})

	case r.Method == http.MethodGet && reResources.MatchString(path):
		mm := reResources.FindStringSubmatch(path)
		a := m.Artifacts[mm[1]]
		if a == nil {
			notFound(w)
			return
		}
		if mm[2] == "" {
			rows := []map[string]any{}
			for _, n := range sortedKeys(a.Resources) {
				rows = append(rows, map[string]any{"Name": n, "ResourceType": a.Resources[n].Type, "ResourceSize": len(a.Resources[n].Content)})
			}
			writeJSON(w, map[string]any{"d": map[string]any{"results": rows}})
			return
		}
		res, ok := a.Resources[mm[3]]
		if !ok || res.Type != mm[4] {
			notFound(w)
			return
		}
		_, _ = w.Write(res.Content)

	case r.Method == http.MethodGet && reDesignValue.MatchString(path):
		mm := reDesignValue.FindStringSubmatch(path)
		a := m.Artifacts[mm[2]]
		if a == nil || a.Zip == nil || a.Type != mm[1] {
			notFound(w)
			return
		}
		_, _ = w.Write(a.Zip)

	case r.Method == http.MethodGet && rePackage.MatchString(path):
		id := rePackage.FindStringSubmatch(path)[1]
		for _, p := range m.Packages {
			if p.ID == id {
				writeJSON(w, map[string]any{"d": map[string]string{"Id": p.ID, "Name": p.Name, "Version": p.Version}})
				return
			}
		}
		notFound(w)

	case r.Method == http.MethodPost && path == "/api/v1/IntegrationPackages":
		var body struct {
			D struct{ Id, Name string } `json:"d"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.D.Id == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		for _, p := range m.Packages {
			if p.ID == body.D.Id {
				w.WriteHeader(http.StatusConflict)
				return
			}
		}
		m.Packages = append(m.Packages, Package{ID: body.D.Id, Name: body.D.Name, Version: "1.0.0"})
		w.WriteHeader(http.StatusCreated)

	case r.Method == http.MethodGet && path == "/api/v1/IntegrationPackages":
		results := []map[string]string{}
		for _, p := range m.Packages {
			results = append(results, map[string]string{"Id": p.ID, "Name": p.Name, "Version": p.Version})
		}
		writeJSON(w, map[string]any{"d": map[string]any{"results": results}})

	case r.Method == http.MethodGet && rePkgArts.MatchString(path):
		mm := rePkgArts.FindStringSubmatch(path)
		results := []map[string]string{}
		for _, id := range sortedKeys(m.Artifacts) {
			a := m.Artifacts[id]
			if a.Package == mm[1] && a.Type == mm[2] {
				results = append(results, map[string]string{"Id": id, "Name": a.Name, "Version": a.DesignVersion})
			}
		}
		writeJSON(w, map[string]any{"d": map[string]any{"results": results}})

	case r.Method == http.MethodGet && reConfigs.MatchString(path):
		a := m.Artifacts[reConfigs.FindStringSubmatch(path)[1]]
		if a == nil || a.DesignVersion == "" {
			notFound(w)
			return
		}
		results := []map[string]string{}
		for _, k := range sortedKeys(a.Parameters) {
			results = append(results, map[string]string{"ParameterKey": k, "ParameterValue": a.Parameters[k], "DataType": "xsd:string"})
		}
		writeJSON(w, map[string]any{"d": map[string]any{"results": results}})

	case r.Method == http.MethodPut && reConfig.MatchString(path):
		mm := reConfig.FindStringSubmatch(path)
		a := m.Artifacts[mm[1]]
		if a == nil {
			notFound(w)
			return
		}
		if a.ConfigUpdateStatus != 0 {
			w.WriteHeader(a.ConfigUpdateStatus)
			return
		}
		var body struct{ ParameterValue string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		if a.Parameters == nil {
			a.Parameters = map[string]string{}
		}
		a.Parameters[mm[2]] = body.ParameterValue
		w.WriteHeader(http.StatusAccepted)

	case r.Method == http.MethodPost && reDesignCreate.MatchString(path):
		typ := reDesignCreate.FindStringSubmatch(path)[1]
		var body struct{ Id, Name, PackageId, ArtifactContent string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Id == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		zipData, err := base64.StdEncoding.DecodeString(body.ArtifactContent)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if a := m.Artifacts[body.Id]; a != nil && a.DesignVersion != "" {
			w.WriteHeader(http.StatusConflict)
			return
		}
		if m.Artifacts == nil {
			m.Artifacts = map[string]*Artifact{}
		}
		a := m.Artifacts[body.Id]
		if a == nil {
			a = &Artifact{}
			m.Artifacts[body.Id] = a
		}
		a.Type, a.DesignVersion, a.Package, a.Name, a.Zip = typ, "1.0.0", body.PackageId, body.Name, zipData
		a.Uploads++
		w.WriteHeader(http.StatusCreated)

	case r.Method == http.MethodPut && reDesign.MatchString(path):
		mm := reDesign.FindStringSubmatch(path)
		a := m.Artifacts[mm[2]]
		if a == nil || a.DesignVersion == "" || a.Type != mm[1] {
			notFound(w)
			return
		}
		var body struct{ ArtifactContent string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		zipData, err := base64.StdEncoding.DecodeString(body.ArtifactContent)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		a.Zip = zipData
		a.Uploads++
		w.WriteHeader(http.StatusOK)

	case r.Method == http.MethodGet && reDesign.MatchString(path):
		mm := reDesign.FindStringSubmatch(path)
		a := m.Artifacts[mm[2]]
		if a == nil || a.DesignVersion == "" || a.Type != mm[1] {
			notFound(w)
			return
		}
		writeJSON(w, map[string]any{"d": map[string]string{"Id": mm[2], "Version": a.DesignVersion}})

	case r.Method == http.MethodPost && reDeploy.MatchString(path):
		id := strings.Trim(r.URL.Query().Get("Id"), "'")
		a := m.Artifacts[id]
		if a == nil {
			notFound(w)
			return
		}
		if a.DeployStatusCode != 0 {
			w.WriteHeader(a.DeployStatusCode)
			return
		}
		a.triggered, a.runtimeGets, a.taskGets = true, 0, 0
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("task-" + id))

	case r.Method == http.MethodGet && reTask.MatchString(path):
		a := m.Artifacts[reTask.FindStringSubmatch(path)[1]]
		if a == nil || len(a.TaskStatuses) == 0 {
			notFound(w)
			return
		}
		status := pick(a.TaskStatuses, a.taskGets)
		a.taskGets++
		writeJSON(w, map[string]any{"d": map[string]string{"TaskId": "task", "Status": status}})

	case r.Method == http.MethodGet && reErrInfo.MatchString(path):
		a := m.Artifacts[reErrInfo.FindStringSubmatch(path)[1]]
		if a == nil || a.ErrorInfo == "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeJSON(w, map[string]any{"parameter": []string{a.ErrorInfo}})

	case r.Method == http.MethodGet && reRuntime.MatchString(path):
		a := m.Artifacts[reRuntime.FindStringSubmatch(path)[1]]
		if a == nil {
			notFound(w)
			return
		}
		var rt *Runtime
		switch {
		case a.undeployed:
			if a.UndeployAfter >= 0 && a.undeployedGets >= a.UndeployAfter {
				rt = nil
			} else {
				rt = a.Runtime
			}
			a.undeployedGets++
		case a.triggered && len(a.AfterDeploy) > 0:
			rt = pick(a.AfterDeploy, a.runtimeGets)
			a.runtimeGets++
		default:
			rt = a.Runtime
		}
		if rt == nil {
			notFound(w)
			return
		}
		writeJSON(w, map[string]any{"d": map[string]string{
			"Id": reRuntime.FindStringSubmatch(path)[1], "Version": rt.Version, "Status": rt.Status,
			"DeployedOn": odataDate(rt.DeployedOn),
		}})

	case r.Method == http.MethodDelete && reRuntime.MatchString(path):
		a := m.Artifacts[reRuntime.FindStringSubmatch(path)[1]]
		if a == nil || a.Runtime == nil {
			notFound(w)
			return
		}
		a.undeployed, a.undeployedGets = true, 0
		w.WriteHeader(http.StatusAccepted)

	default:
		notFound(w)
	}
}
