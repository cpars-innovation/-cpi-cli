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
