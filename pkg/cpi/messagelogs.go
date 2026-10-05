package cpi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cpars-innovation/cpicli/pkg/httpclnt"
	"github.com/go-errors/errors"
	"github.com/rs/zerolog/log"
)

// MessageLogs reads message processing logs (MPL) of the runtime.
type MessageLogs struct {
	exe *httpclnt.HTTPExecuter
}

// NewMessageLogs returns a message processing log client.
func NewMessageLogs(exe *httpclnt.HTTPExecuter) *MessageLogs {
	return &MessageLogs{exe: exe}
}

// MessageLogStatuses are the documented values of MessageProcessingLog.Status.
var MessageLogStatuses = []string{"COMPLETED", "PROCESSING", "RETRY", "ESCALATED", "FAILED", "CANCELLED", "DISCARDED", "ABANDONED"}

// MessageLog is one message processing log entry.
type MessageLog struct {
	MessageGuid          string
	CorrelationId        string
	ApplicationMessageId string
	Status               string
	CustomStatus         string
	LogLevel             string
	LogStart             time.Time
	LogEnd               time.Time
	Sender               string
	Receiver             string
	ArtifactId           string
	ArtifactName         string
	ArtifactType         string
	PackageId            string
	AlternateWebLink     string
}

type mplData struct {
	MessageGuid          string `json:"MessageGuid"`
	CorrelationId        string `json:"CorrelationId"`
	ApplicationMessageId string `json:"ApplicationMessageId"`
	Status               string `json:"Status"`
	CustomStatus         string `json:"CustomStatus"`
	LogLevel             string `json:"LogLevel"`
	LogStart             string `json:"LogStart"`
	LogEnd               string `json:"LogEnd"`
	Sender               string `json:"Sender"`
	Receiver             string `json:"Receiver"`
	IntegrationFlowName  string `json:"IntegrationFlowName"`
	AlternateWebLink     string `json:"AlternateWebLink"`
	IntegrationArtifact  *struct {
		Id        string `json:"Id"`
		Name      string `json:"Name"`
		Type      string `json:"Type"`
		PackageId string `json:"PackageId"`
	} `json:"IntegrationArtifact"`
}

func (d *mplData) toLog() MessageLog {
	l := MessageLog{
		MessageGuid: d.MessageGuid, CorrelationId: d.CorrelationId, ApplicationMessageId: d.ApplicationMessageId,
		Status: d.Status, CustomStatus: d.CustomStatus, LogLevel: d.LogLevel, Sender: d.Sender, Receiver: d.Receiver,
		ArtifactId: d.IntegrationFlowName, AlternateWebLink: d.AlternateWebLink,
	}
	if d.IntegrationArtifact != nil {
		if d.IntegrationArtifact.Id != "" {
			l.ArtifactId = d.IntegrationArtifact.Id
		}
		l.ArtifactName, l.ArtifactType, l.PackageId = d.IntegrationArtifact.Name, d.IntegrationArtifact.Type, d.IntegrationArtifact.PackageId
	}
	l.LogStart, _ = ParseODataTime(d.LogStart)
	l.LogEnd, _ = ParseODataTime(d.LogEnd)
	return l
}

// MessageLogQuery selects message processing logs. Empty fields are not
// filtered on.
type MessageLogQuery struct {
	ArtifactID           string
	Statuses             []string // any of MessageLogStatuses
	Since, Until         time.Time
	CorrelationID        string
	ApplicationMessageID string
	Top, Skip            int // Top <= 0: 20
}

// Filter returns the OData $filter expression of q.
func (q MessageLogQuery) Filter() (string, error) {
	var parts []string
	if q.ArtifactID != "" {
		// IntegrationFlowName is mapped to IntegrationArtifact/Id by the tenant
		parts = append(parts, fmt.Sprintf("IntegrationFlowName eq %s", odataString(q.ArtifactID)))
	}
	if len(q.Statuses) > 0 {
		var ors []string
		for _, s := range q.Statuses {
			s = strings.ToUpper(strings.TrimSpace(s))
			if !slices.Contains(MessageLogStatuses, s) {
				return "", fmt.Errorf("invalid message status %q (valid: %s)", s, strings.Join(MessageLogStatuses, ", "))
			}
			ors = append(ors, fmt.Sprintf("Status eq '%s'", s))
		}
		if len(ors) == 1 {
			parts = append(parts, ors[0])
		} else {
			parts = append(parts, "("+strings.Join(ors, " or ")+")")
		}
	}
	if q.CorrelationID != "" {
		parts = append(parts, fmt.Sprintf("CorrelationId eq %s", odataString(q.CorrelationID)))
	}
	if q.ApplicationMessageID != "" {
		parts = append(parts, fmt.Sprintf("ApplicationMessageId eq %s", odataString(q.ApplicationMessageID)))
	}
	if !q.Since.IsZero() {
		parts = append(parts, "LogEnd ge "+odataDateTime(q.Since))
	}
	if !q.Until.IsZero() {
		parts = append(parts, "LogStart le "+odataDateTime(q.Until))
	}
	return strings.Join(parts, " and "), nil
}

// Query returns matching logs, newest first, and the total number of matches.
func (m *MessageLogs) Query(q MessageLogQuery) ([]MessageLog, int, error) {
	filter, err := q.Filter()
	if err != nil {
		return nil, 0, err
	}
	top := q.Top
	if top <= 0 {
		top = 20
	}
	params := []string{
		"$inlinecount=allpages",
		"$orderby=" + queryEscape("LogEnd desc"),
		"$top=" + strconv.Itoa(top),
	}
	if q.Skip > 0 {
		params = append(params, "$skip="+strconv.Itoa(q.Skip))
	}
	if filter != "" {
		params = append(params, "$filter="+queryEscape(filter))
	}
	urlPath := "/api/v1/MessageProcessingLogs?" + strings.Join(params, "&")
	log.Debug().Msgf("Querying message processing logs: %s", filter)

	resp, err := readOnlyCall(urlPath, "Get message processing logs", m.exe)
	if err != nil {
		return nil, 0, err
	}
	body, err := m.exe.ReadRespBody(resp)
	if err != nil {
		return nil, 0, err
	}
	var data struct {
		D struct {
			Results []mplData `json:"results"`
			Count   string    `json:"__count"`
		} `json:"d"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, 0, errors.Wrap(err, 0)
	}
	logs := make([]MessageLog, 0, len(data.D.Results))
	for i := range data.D.Results {
		logs = append(logs, data.D.Results[i].toLog())
	}
	total, err := strconv.Atoi(data.D.Count)
	if err != nil {
		total = len(logs)
	}
	return logs, total, nil
}

// Get returns one log entry, or nil if the message GUID is unknown.
func (m *MessageLogs) Get(guid string) (*MessageLog, error) {
	resp, err := readOnlyCall(fmt.Sprintf("/api/v1/MessageProcessingLogs(%s)", odataString(guid)), "Get message processing log", m.exe)
	if err != nil {
		if httpclnt.StatusCode(err) == http.StatusNotFound {
			return nil, nil
		}
		return nil, err
	}
	body, err := m.exe.ReadRespBody(resp)
	if err != nil {
		return nil, err
	}
	var data struct {
		D mplData `json:"d"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, errors.Wrap(err, 0)
	}
	l := data.D.toLog()
	return &l, nil
}

// ErrorText returns the error information text of a message ("" if none).
func (m *MessageLogs) ErrorText(guid string) (string, error) {
	urlPath := fmt.Sprintf("/api/v1/MessageProcessingLogs(%s)/ErrorInformation/$value", odataString(guid))
	resp, err := readOnlyCallWithBodyAndAcceptType(urlPath, nil, "Get message error information", "", m.exe)
	if err != nil {
		if code := httpclnt.StatusCode(err); code == http.StatusNotFound || code == http.StatusNoContent {
			return "", nil
		}
		return "", err
	}
	body, err := m.exe.ReadRespBody(resp)
	return string(body), err
}

// NameValue is a custom header property or adapter attribute.
type NameValue struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	Adapter string `json:"adapter,omitempty"`
}

// CustomHeaderProperties returns the custom header properties of a message.
func (m *MessageLogs) CustomHeaderProperties(guid string) ([]NameValue, error) {
	var rows []struct {
		Name  string `json:"Name"`
		Value string `json:"Value"`
	}
	if err := m.list(guid, "CustomHeaderProperties", &rows); err != nil {
		return nil, err
	}
	out := make([]NameValue, 0, len(rows))
	for _, r := range rows {
		out = append(out, NameValue{Name: r.Name, Value: r.Value})
	}
	return out, nil
}

// AdapterAttributes returns the adapter attributes of a message.
func (m *MessageLogs) AdapterAttributes(guid string) ([]NameValue, error) {
	var rows []struct {
		AdapterId string `json:"AdapterId"`
		Name      string `json:"Name"`
		Value     string `json:"Value"`
	}
	if err := m.list(guid, "AdapterAttributes", &rows); err != nil {
		return nil, err
	}
	out := make([]NameValue, 0, len(rows))
	for _, r := range rows {
		out = append(out, NameValue{Name: r.Name, Value: r.Value, Adapter: r.AdapterId})
	}
	return out, nil
}

// MessageAttachment describes an MPL attachment (content is not downloaded).
type MessageAttachment struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	ContentType string    `json:"contentType"`
	Size        int64     `json:"size"`
	TimeStamp   time.Time `json:"timeStamp"`
}

// Attachments returns the attachment metadata of a message.
func (m *MessageLogs) Attachments(guid string) ([]MessageAttachment, error) {
	var rows []struct {
		Id          string `json:"Id"`
		Name        string `json:"Name"`
		ContentType string `json:"ContentType"`
		PayloadSize int64  `json:"PayloadSize"`
		TimeStamp   string `json:"TimeStamp"`
	}
	if err := m.list(guid, "Attachments", &rows); err != nil {
		return nil, err
	}
	out := make([]MessageAttachment, 0, len(rows))
	for _, r := range rows {
		ts, _ := ParseODataTime(r.TimeStamp)
		out = append(out, MessageAttachment{ID: r.Id, Name: r.Name, ContentType: r.ContentType, Size: r.PayloadSize, TimeStamp: ts})
	}
	return out, nil
}

func (m *MessageLogs) list(guid, navigation string, v any) error {
	urlPath := fmt.Sprintf("/api/v1/MessageProcessingLogs(%s)/%s", odataString(guid), navigation)
	resp, err := readOnlyCall(urlPath, "Get message "+navigation, m.exe)
	if err != nil {
		return err
	}
	body, err := m.exe.ReadRespBody(resp)
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

// odataString quotes s as an OData string literal (” escapes ').
func odataString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// odataDateTime formats t as an OData v2 datetime literal (UTC).
func odataDateTime(t time.Time) string {
	return "datetime'" + t.UTC().Format("2006-01-02T15:04:05.000") + "'"
}

// queryEscape escapes a query value with %20 for spaces.
func queryEscape(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

// AttachmentContent downloads the content of an MPL attachment.
// Endpoint: MessageProcessingLogAttachments('{Id}')/$value.
func (m *MessageLogs) AttachmentContent(attachmentID string) ([]byte, error) {
	return getValue(m.exe, fmt.Sprintf("/api/v1/MessageProcessingLogAttachments(%s)/$value", odataString(attachmentID)), "Download log attachment")
}

// MessageStoreEntry is a message persisted by a Persist step or the JMS/data store.
type MessageStoreEntry struct {
	ID             string    `json:"id"`
	MessageStoreID string    `json:"messageStoreId,omitempty"`
	TimeStamp      time.Time `json:"timeStamp"`
	HasAttachments bool      `json:"hasAttachments,omitempty"`
}

// MessageStoreEntries lists the persisted messages of a message.
func (m *MessageLogs) MessageStoreEntries(guid string) ([]MessageStoreEntry, error) {
	var rows []struct {
		Id             string `json:"Id"`
		MessageStoreId string `json:"MessageStoreId"`
		TimeStamp      string `json:"TimeStamp"`
		HasAttachments bool   `json:"HasAttachments"`
	}
	if err := m.list(guid, "MessageStoreEntries", &rows); err != nil {
		return nil, err
	}
	out := make([]MessageStoreEntry, 0, len(rows))
	for _, r := range rows {
		ts, _ := ParseODataTime(r.TimeStamp)
		out = append(out, MessageStoreEntry{ID: r.Id, MessageStoreID: r.MessageStoreId, TimeStamp: ts, HasAttachments: r.HasAttachments})
	}
	return out, nil
}

// MessageStoreEntryContent downloads a persisted message payload.
// Endpoint: MessageStoreEntries('{Id}')/$value.
func (m *MessageLogs) MessageStoreEntryContent(entryID string) ([]byte, error) {
	return getValue(m.exe, fmt.Sprintf("/api/v1/MessageStoreEntries(%s)/$value", odataString(entryID)), "Download message store entry")
}

// MessageRun is one processing run of a message.
type MessageRun struct {
	ID           string    `json:"id"`
	Start        time.Time `json:"start"`
	Stop         time.Time `json:"stop"`
	OverallState string    `json:"overallState,omitempty"`
	LogLevel     string    `json:"logLevel,omitempty"`
}

// MessageRunStep is one processing step of a run.
type MessageRunStep struct {
	StepID      string    `json:"stepId"`
	ModelStepID string    `json:"modelStepId,omitempty"`
	Activity    string    `json:"activity,omitempty"`
	Status      string    `json:"status,omitempty"`
	Error       string    `json:"error,omitempty"`
	BranchID    string    `json:"branchId,omitempty"`
	Start       time.Time `json:"start"`
	Stop        time.Time `json:"stop"`
	// RunID and ChildCount identify the step (key of MessageProcessingLogRunSteps).
	RunID      string `json:"runId,omitempty"`
	ChildCount int    `json:"childCount"`
}

// Runs lists the processing runs of a message.
// Endpoint: MessageProcessingLogs('{guid}')/Runs.
func (m *MessageLogs) Runs(guid string) ([]MessageRun, error) {
	var rows []struct {
		Id, RunStart, RunStop, OverallState, LogLevel string
	}
	if err := m.list(guid, "Runs", &rows); err != nil {
		return nil, err
	}
	out := make([]MessageRun, 0, len(rows))
	for _, r := range rows {
		start, _ := ParseODataTime(r.RunStart)
		stop, _ := ParseODataTime(r.RunStop)
		out = append(out, MessageRun{ID: r.Id, Start: start, Stop: stop, OverallState: r.OverallState, LogLevel: r.LogLevel})
	}
	return out, nil
}

// RunSteps lists the steps of a processing run.
// Endpoint: MessageProcessingLogRuns('{RunId}')/RunSteps.
func (m *MessageLogs) RunSteps(runID string) ([]MessageRunStep, error) {
	var rows []struct {
		StepId, ModelStepId, Activity, Status, Error, BranchId, StepStart, StepStop string
		ChildCount                                                                  int
	}
	urlPath := fmt.Sprintf("/api/v1/MessageProcessingLogRuns(%s)/RunSteps", odataString(runID))
	if err := getResults(m.exe, urlPath, "Get run steps", &rows); err != nil {
		return nil, err
	}
	out := make([]MessageRunStep, 0, len(rows))
	for _, r := range rows {
		start, _ := ParseODataTime(r.StepStart)
		stop, _ := ParseODataTime(r.StepStop)
		out = append(out, MessageRunStep{StepID: r.StepId, ModelStepID: r.ModelStepId, Activity: r.Activity, Status: r.Status,
			Error: r.Error, BranchID: r.BranchId, Start: start, Stop: stop, RunID: runID, ChildCount: r.ChildCount})
	}
	return out, nil
}

// TraceMessage is the message as it was at one step of a traced run (log
// level TRACE).
type TraceMessage struct {
	TraceID     string `json:"traceId"`
	ModelStepID string `json:"modelStepId,omitempty"`
	PayloadSize int64  `json:"payloadSize"`
	MimeType    string `json:"mimeType,omitempty"`
}

// TraceMessages lists the trace messages of a run step.
// Endpoint: MessageProcessingLogRunSteps(RunId='{RunId}',ChildCount={n})/TraceMessages.
func (m *MessageLogs) TraceMessages(runID string, childCount int) ([]TraceMessage, error) {
	var rows []struct {
		TraceId     json.Number
		ModelStepId string
		PayloadSize json.Number
		MimeType    string
	}
	urlPath := fmt.Sprintf("/api/v1/MessageProcessingLogRunSteps(RunId=%s,ChildCount=%d)/TraceMessages", odataString(runID), childCount)
	if err := getResults(m.exe, urlPath, "Get trace messages", &rows); err != nil {
		return nil, err
	}
	out := make([]TraceMessage, 0, len(rows))
	for _, r := range rows {
		size, _ := r.PayloadSize.Int64()
		out = append(out, TraceMessage{TraceID: r.TraceId.String(), ModelStepID: r.ModelStepId, PayloadSize: size, MimeType: r.MimeType})
	}
	return out, nil
}

// traceKey formats a TraceId (Edm.Int64) as OData key.
func traceKey(id string) (string, error) {
	if _, err := strconv.ParseInt(id, 10, 64); err != nil {
		return "", fmt.Errorf("invalid trace ID %q", id)
	}
	return id, nil
}

// TracePayload downloads the payload of a trace message.
// Endpoint: TraceMessages({TraceId})/$value.
func (m *MessageLogs) TracePayload(traceID string) ([]byte, error) {
	key, err := traceKey(traceID)
	if err != nil {
		return nil, err
	}
	return getValue(m.exe, "/api/v1/TraceMessages("+key+")/$value", "Get trace payload")
}

// TraceHeaders returns the headers (Properties) or, with exchange set, the
// exchange properties of a trace message.
// Endpoints: TraceMessages({TraceId})/Properties and /ExchangeProperties.
func (m *MessageLogs) TraceHeaders(traceID string, exchange bool) ([]NameValue, error) {
	key, err := traceKey(traceID)
	if err != nil {
		return nil, err
	}
	nav := "Properties"
	if exchange {
		nav = "ExchangeProperties"
	}
	var rows []struct{ Name, Value string }
	if err := getResults(m.exe, "/api/v1/TraceMessages("+key+")/"+nav, "Get trace "+nav, &rows); err != nil {
		return nil, err
	}
	out := make([]NameValue, 0, len(rows))
	for _, r := range rows {
		out = append(out, NameValue{Name: r.Name, Value: r.Value})
	}
	return out, nil
}
