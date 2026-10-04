// Package cpitest provides an in-memory SAP CPI tenant (httptest server) for
// offline tests of deploy/undeploy flows. It must only be used from tests.
package cpitest

import (
	"encoding/json"
	"fmt"
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

	triggered      bool
	runtimeGets    int
	taskGets       int
	undeployed     bool
	undeployedGets int
}

// Package is an integration package of the mock tenant.
type Package struct {
	ID, Name, Version string
}

// Tenant is a mock CPI tenant.
type Tenant struct {
	mu        sync.Mutex
	Artifacts map[string]*Artifact
	Packages  []Package
	// StatusOverride, if non-zero, is returned for every API call (e.g. 401).
	StatusOverride int
	requests       []string
	server         *httptest.Server
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
	reDesign  = regexp.MustCompile(`^/api/v1/(\w+)DesigntimeArtifacts\(Id='([^']+)',Version='active'\)$`)
	reDeploy  = regexp.MustCompile(`^/api/v1/Deploy(\w+)DesigntimeArtifact$`)
	reTask    = regexp.MustCompile(`^/api/v1/BuildAndDeployStatus\(TaskId='task-([^']+)'\)$`)
	reRuntime = regexp.MustCompile(`^/api/v1/IntegrationRuntimeArtifacts\('([^']+)'\)$`)
	rePkgArts = regexp.MustCompile(`^/api/v1/IntegrationPackages\('([^']+)'\)/(\w+)DesigntimeArtifacts$`)
	reConfigs = regexp.MustCompile(`^/api/v1/IntegrationDesigntimeArtifacts\(Id='([^']+)',Version='active'\)/Configurations$`)
	reConfig  = regexp.MustCompile(`^/api/v1/IntegrationDesigntimeArtifacts\(Id='([^']+)',Version='active'\)/\$links/Configurations\('([^']+)'\)$`)
	reErrInfo = regexp.MustCompile(`^/api/v1/IntegrationRuntimeArtifacts\('([^']+)'\)/ErrorInformation/\$value$`)
)

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
	if path == "/api/v1/" { // CSRF token fetch
		w.Header().Set("x-csrf-token", "token")
		return
	}

	switch {
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
