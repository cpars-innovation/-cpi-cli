package cpi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/cpars-innovation/cpicli/pkg/httpclnt"
	"github.com/go-errors/errors"
)

// Content groups Integration Content API calls that are not specific to one
// artifact type: validation, design guidelines, service endpoints, runtime
// overview and artifact resources.
type Content struct {
	exe *httpclnt.HTTPExecuter
}

// NewContent returns an Integration Content client.
func NewContent(exe *httpclnt.HTTPExecuter) *Content {
	return &Content{exe: exe}
}

// postAction calls an OData function import (POST without body) and returns
// the response body.
func postAction(exe *httpclnt.HTTPExecuter, urlPath, callType string, okCodes ...int) ([]byte, error) {
	headers, cookies, err := InitHeadersAndCookies(exe)
	if err != nil {
		return nil, err
	}
	headers["Accept"] = "application/json"
	resp, err := exe.ExecRequestWithCookies(http.MethodPost, urlPath, http.NoBody, headers, cookies)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(okCodes, resp.StatusCode) {
		_, err = exe.LogError(resp, callType)
		return nil, err
	}
	return exe.ReadRespBody(resp)
}

// getValue reads a raw ($value) resource.
func getValue(exe *httpclnt.HTTPExecuter, urlPath, callType string) ([]byte, error) {
	resp, err := readOnlyCallWithBodyAndAcceptType(urlPath, nil, callType, "", exe)
	if err != nil {
		return nil, err
	}
	return exe.ReadRespBody(resp)
}

// getResults reads an OData v2 collection ({"d":{"results":[...]}}) into v.
func getResults(exe *httpclnt.HTTPExecuter, urlPath, callType string, v any) error {
	resp, err := readOnlyCall(urlPath, callType, exe)
	if err != nil {
		return err
	}
	body, err := exe.ReadRespBody(resp)
	if err != nil {
		return err
	}
	var data struct {
		D struct {
			Results json.RawMessage `json:"results"`
		} `json:"d"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return errors.Wrap(err, 0)
	}
	if len(data.D.Results) == 0 {
		return nil
	}
	return json.Unmarshal(data.D.Results, v)
}

func artifactKey(id, version string) string {
	if version == "" {
		version = "active"
	}
	return fmt.Sprintf("Id=%s,Version=%s", odataString(id), odataString(version))
}

// Validate runs ValidateIntegrationDesigntimeArtifact and returns the
// response text ("Check execution result: Passed or Failed" plus details).
func (c *Content) Validate(id, version string) (string, error) {
	if version == "" {
		version = "active"
	}
	urlPath := fmt.Sprintf("/api/v1/ValidateIntegrationDesigntimeArtifact?Id=%s&Version=%s",
		queryEscape(odataString(id)), queryEscape(odataString(version)))
	body, err := postAction(c.exe, urlPath, "Validate integration flow", http.StatusAccepted, http.StatusOK)
	return string(body), err
}

// GuidelineExecution is one design guideline execution of an integration flow.
type GuidelineExecution struct {
	ExecutionID     string    `json:"executionId"`
	ArtifactVersion string    `json:"artifactVersion,omitempty"`
	Status          string    `json:"status"`
	ExecutionTime   time.Time `json:"executionTime"`
}

type guidelineExecutionData struct {
	ExecutionId     string `json:"ExecutionId"`
	ArtifactVersion string `json:"ArtifactVersion"`
	ExecutionStatus string `json:"ExecutionStatus"`
	ExecutionTime   string `json:"ExecutionTime"`
}

func (g guidelineExecutionData) toExecution() GuidelineExecution {
	e := GuidelineExecution{ExecutionID: g.ExecutionId, ArtifactVersion: g.ArtifactVersion, Status: g.ExecutionStatus}
	if ms, err := strconv.ParseInt(g.ExecutionTime, 10, 64); err == nil {
		e.ExecutionTime = time.UnixMilli(ms).UTC()
	} else {
		e.ExecutionTime, _ = ParseODataTime(g.ExecutionTime)
	}
	return e
}

// ExecuteGuidelines starts a design guideline check and returns the
// execution ID if the tenant reports it ("" otherwise).
func (c *Content) ExecuteGuidelines(id, version string) (string, error) {
	if version == "" {
		version = "active"
	}
	urlPath := fmt.Sprintf("/api/v1/ExecuteIntegrationDesigntimeArtifactsGuidelines?Id=%s&Version=%s",
		queryEscape(odataString(id)), queryEscape(odataString(version)))
	body, err := postAction(c.exe, urlPath, "Execute design guidelines", http.StatusOK, http.StatusAccepted, http.StatusCreated)
	if err != nil {
		return "", err
	}
	var data struct {
		D guidelineExecutionData `json:"d"`
	}
	if json.Unmarshal(body, &data) == nil {
		return data.D.ExecutionId, nil
	}
	return "", nil
}

// GuidelineExecutions lists the design guideline executions of an iFlow.
func (c *Content) GuidelineExecutions(id, version string) ([]GuidelineExecution, error) {
	var rows []guidelineExecutionData
	urlPath := fmt.Sprintf("/api/v1/IntegrationDesigntimeArtifacts(%s)/DesignGuidelineExecutionResults", artifactKey(id, version))
	if err := getResults(c.exe, urlPath, "Get design guideline executions", &rows); err != nil {
		return nil, err
	}
	out := make([]GuidelineExecution, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.toExecution())
	}
	return out, nil
}

// Guideline is the result of one design guideline.
type Guideline struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Category           string `json:"category,omitempty"`
	Severity           string `json:"severity,omitempty"`
	Applicability      string `json:"applicability,omitempty"`
	Compliance         string `json:"compliance"`
	Skipped            bool   `json:"skipped,omitempty"`
	ExpectedKPI        string `json:"expectedKpi,omitempty"`
	ActualKPI          string `json:"actualKpi,omitempty"`
	ViolatedComponents string `json:"violatedComponents,omitempty"`
}

type guidelineData struct {
	GuidelineId        string `json:"GuidelineId"`
	GuidelineName      string `json:"GuidelineName"`
	Category           string `json:"Category"`
	Severity           string `json:"Severity"`
	Applicability      string `json:"Applicability"`
	Compliance         string `json:"Compliance"`
	IsGuidelineSkipped any    `json:"IsGuidelineSkipped"` // "false" or false
	ExpectedKPI        string `json:"ExpectedKPI"`
	ActualKPI          string `json:"ActualKPI"`
	ViolatedComponents string `json:"ViolatedComponents"`
}

// GuidelineResults returns the per-guideline results of an execution.
func (c *Content) GuidelineResults(id, version, executionID string) (*GuidelineExecution, []Guideline, error) {
	urlPath := fmt.Sprintf("/api/v1/IntegrationDesigntimeArtifacts(%s)/DesignGuidelineExecutionResults(%s)?$expand=DesignGuidelines",
		artifactKey(id, version), odataString(executionID))
	resp, err := readOnlyCall(urlPath, "Get design guideline results", c.exe)
	if err != nil {
		return nil, nil, err
	}
	body, err := c.exe.ReadRespBody(resp)
	if err != nil {
		return nil, nil, err
	}
	// The entity carries DesignGuidelines.results; some tenants answer with a
	// plain results collection of guidelines.
	var data struct {
		D struct {
			guidelineExecutionData
			DesignGuidelines struct {
				Results []guidelineData `json:"results"`
			} `json:"DesignGuidelines"`
			Results []guidelineData `json:"results"`
		} `json:"d"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, nil, errors.Wrap(err, 0)
	}
	rows := data.D.DesignGuidelines.Results
	if len(rows) == 0 {
		rows = data.D.Results
	}
	exec := data.D.guidelineExecutionData.toExecution()
	if exec.ExecutionID == "" {
		exec.ExecutionID = executionID
	}
	out := make([]Guideline, 0, len(rows))
	for _, r := range rows {
		skipped := r.IsGuidelineSkipped == true || r.IsGuidelineSkipped == "true"
		out = append(out, Guideline{ID: r.GuidelineId, Name: r.GuidelineName, Category: r.Category, Severity: r.Severity,
			Applicability: r.Applicability, Compliance: r.Compliance, Skipped: skipped,
			ExpectedKPI: r.ExpectedKPI, ActualKPI: r.ActualKPI, ViolatedComponents: r.ViolatedComponents})
	}
	return &exec, out, nil
}

// EntryPoint is a callable URL of a deployed integration flow.
type EntryPoint struct {
	Name string `json:"name,omitempty"`
	URL  string `json:"url"`
	Type string `json:"type,omitempty"`
}

// ServiceEndpoint is an endpoint provided by a deployed integration flow.
type ServiceEndpoint struct {
	ArtifactID  string       `json:"artifactId"`
	ID          string       `json:"id"`
	Title       string       `json:"title,omitempty"`
	Version     string       `json:"version,omitempty"`
	Protocol    string       `json:"protocol,omitempty"`
	EntryPoints []EntryPoint `json:"entryPoints"`
}

// ServiceEndpoints lists the endpoints of deployed iFlows (artifactID "" = all).
func (c *Content) ServiceEndpoints(artifactID string) ([]ServiceEndpoint, error) {
	urlPath := "/api/v1/ServiceEndpoints?$expand=EntryPoints"
	if artifactID != "" {
		urlPath += "&$filter=" + queryEscape("Name eq "+odataString(artifactID))
	}
	var rows []struct {
		Name        string `json:"Name"`
		Id          string `json:"Id"`
		Title       string `json:"Title"`
		Version     string `json:"Version"`
		Protocol    string `json:"Protocol"`
		EntryPoints struct {
			Results []struct {
				Name string `json:"Name"`
				Url  string `json:"Url"`
				Type string `json:"Type"`
			} `json:"results"`
		} `json:"EntryPoints"`
	}
	if err := getResults(c.exe, urlPath, "Get service endpoints", &rows); err != nil {
		return nil, err
	}
	out := make([]ServiceEndpoint, 0, len(rows))
	for _, r := range rows {
		ep := ServiceEndpoint{ArtifactID: r.Name, ID: r.Id, Title: r.Title, Version: r.Version, Protocol: r.Protocol, EntryPoints: []EntryPoint{}}
		for _, e := range r.EntryPoints.Results {
			ep.EntryPoints = append(ep.EntryPoints, EntryPoint{Name: e.Name, URL: e.Url, Type: e.Type})
		}
		out = append(out, ep)
	}
	return out, nil
}

// RuntimeArtifacts lists deployed artifacts, optionally only those with one
// of the given statuses (e.g. ERROR).
func (c *Content) RuntimeArtifacts(statuses []string) ([]RuntimeArtifact, error) {
	urlPath := "/api/v1/IntegrationRuntimeArtifacts"
	if len(statuses) > 0 {
		filter := ""
		for i, s := range statuses {
			if i > 0 {
				filter += " or "
			}
			filter += "Status eq " + odataString(s)
		}
		urlPath += "?$filter=" + queryEscape(filter)
	}
	var rows []struct {
		Id, Version, Name, Type, DeployedBy, DeployedOn, Status string
	}
	if err := getResults(c.exe, urlPath, "Get runtime artifacts", &rows); err != nil {
		return nil, err
	}
	out := make([]RuntimeArtifact, 0, len(rows))
	for _, r := range rows {
		on, _ := ParseODataTime(r.DeployedOn)
		out = append(out, RuntimeArtifact{Id: r.Id, Version: r.Version, Name: r.Name, Type: r.Type, Status: r.Status, DeployedBy: r.DeployedBy, DeployedOn: on})
	}
	return out, nil
}

// Resource is a file inside an integration flow (script, mapping, schema, ...).
type Resource struct {
	Name           string `json:"name"`
	Type           string `json:"type"`
	ReferencedType string `json:"referencedType,omitempty"`
	Size           int64  `json:"size,omitempty"`
	SizeUnit       string `json:"sizeUnit,omitempty"`
}

// Resources lists the resources of an integration flow.
func (c *Content) Resources(id, version string) ([]Resource, error) {
	var rows []struct {
		Name                   string `json:"Name"`
		ResourceType           string `json:"ResourceType"`
		ReferencedResourceType string `json:"ReferencedResourceType"`
		ResourceSize           int64  `json:"ResourceSize"`
		ResourceSizeUnit       string `json:"ResourceSizeUnit"`
	}
	urlPath := fmt.Sprintf("/api/v1/IntegrationDesigntimeArtifacts(%s)/Resources", artifactKey(id, version))
	if err := getResults(c.exe, urlPath, "Get resources", &rows); err != nil {
		return nil, err
	}
	out := make([]Resource, 0, len(rows))
	for _, r := range rows {
		out = append(out, Resource{Name: r.Name, Type: r.ResourceType, ReferencedType: r.ReferencedResourceType, Size: r.ResourceSize, SizeUnit: r.ResourceSizeUnit})
	}
	return out, nil
}

// ResourceContent downloads one resource of an integration flow.
func (c *Content) ResourceContent(id, version, name, resourceType string) ([]byte, error) {
	urlPath := fmt.Sprintf("/api/v1/IntegrationDesigntimeArtifacts(%s)/Resources(Name=%s,ResourceType=%s)/$value",
		artifactKey(id, version), odataString(name), odataString(resourceType))
	return getValue(c.exe, urlPath, "Download resource")
}

// DownloadArtifact returns the zip content of a designtime artifact.
func DownloadArtifact(exe *httpclnt.HTTPExecuter, artifactType, id, version string) ([]byte, error) {
	if version == "" {
		version = "active"
	}
	return getContent(id, version, artifactType, exe)
}
