package ops

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/cpars-innovation/cpicli/internal/output"
	"github.com/cpars-innovation/cpicli/pkg/cpi"
	"github.com/cpars-innovation/cpicli/pkg/httpclnt"
)

// TraceProperties are the custom header property names a tracer writes
// (W3C trace context in the message processing log).
type TraceProperties struct {
	Trace, Span, Parent string
}

// DefaultTraceProperties match the CPIMessageTracer convention.
var DefaultTraceProperties = TraceProperties{Trace: "trace-id", Span: "span-id", Parent: "parent-span-id"}

// TraceTreeQuery selects the messages of one trace.
type TraceTreeQuery struct {
	TraceID string
	// Scope for the fallback scan when no message carries the trace ID as
	// ApplicationMessageId.
	Scope      ScanScope
	Properties TraceProperties
	// MaxScan caps the fallback scan (default 200).
	MaxScan int
}

// TraceNode is one message of a trace.
type TraceNode struct {
	MessageGuid    string       `json:"messageGuid"`
	ArtifactID     string       `json:"artifactId,omitempty"`
	PackageID      string       `json:"packageId,omitempty"`
	Status         string       `json:"status"`
	LogStart       *time.Time   `json:"logStart,omitempty"`
	LogEnd         *time.Time   `json:"logEnd,omitempty"`
	SpanID         string       `json:"spanId,omitempty"`
	ParentSpanID   string       `json:"parentSpanId,omitempty"`
	ErrorText      string       `json:"errorText,omitempty"`
	ErrorTruncated bool         `json:"errorTruncated,omitempty"`
	Children       []*TraceNode `json:"children"`
}

// TraceFailure points at the earliest failed message of a trace.
type TraceFailure struct {
	MessageGuid string `json:"messageGuid"`
	ArtifactID  string `json:"artifactId,omitempty"`
	ErrorText   string `json:"errorText,omitempty"`
}

// TraceTree is the call tree of a trace.
type TraceTree struct {
	TraceID string `json:"traceId"`
	// Source is how the messages were found: application_message_id or scan.
	Source       string        `json:"source"`
	Messages     int           `json:"messages"`
	Roots        []*TraceNode  `json:"roots"`
	FirstFailure *TraceFailure `json:"firstFailure,omitempty"`
	Scanned      int           `json:"scanned,omitempty"`
	Truncated    bool          `json:"truncated,omitempty"`
}

var traceIDPattern32 = regexp.MustCompile(`^[0-9a-f]{32}$`)

// TraceTreeFor builds the call tree of a trace. It first looks for messages
// whose ApplicationMessageId is the trace ID (one query; the tracer sets
// SAP_ApplicationID = trace ID). Otherwise it scans the given scope for
// messages whose trace property matches. Nodes are linked by span and parent
// span (or, without them, by PredecessorMessageGuid); children are ordered by
// start time.
func TraceTreeFor(ctx context.Context, exe *httpclnt.HTTPExecuter, q TraceTreeQuery) (*TraceTree, error) {
	q.TraceID = strings.ToLower(strings.TrimSpace(q.TraceID))
	if !traceIDPattern32.MatchString(q.TraceID) {
		return nil, output.Usagef("trace_id must be 32 hex characters")
	}
	props := q.Properties
	if props.Trace == "" {
		props.Trace = DefaultTraceProperties.Trace
	}
	if props.Span == "" {
		props.Span = DefaultTraceProperties.Span
	}
	if props.Parent == "" {
		props.Parent = DefaultTraceProperties.Parent
	}
	if q.MaxScan <= 0 {
		q.MaxScan = 200
	}
	tree := &TraceTree{TraceID: q.TraceID, Source: "application_message_id", Roots: []*TraceNode{}}

	byAppID, err := QueryMessageLogs(exe, MessageLogQuery{ApplicationMessageID: q.TraceID, Top: MaxMessageLogs})
	if err != nil {
		return nil, err
	}
	var found []ScannedLog
	if len(byAppID.Logs) > 0 {
		// span IDs are only in the custom header properties: read them for
		// the matches (no scan)
		if found, err = withHeaders(ctx, exe, byAppID.Logs); err != nil {
			return nil, err
		}
	} else {
		if (len(q.Scope.ArtifactIDs) == 0 && q.Scope.PackageID == "") || q.Scope.Since.IsZero() {
			return nil, errScopeRequired()
		}
		res, err := ScanByHeaders(ctx, exe, q.Scope, q.MaxScan, func(h []cpi.NameValue) bool {
			return strings.EqualFold(headerValue(h, props.Trace), q.TraceID)
		})
		if err != nil {
			return nil, err
		}
		tree.Source, tree.Scanned, tree.Truncated, found = "scan", res.Scanned, res.Truncated, res.Matches
	}

	mpl := cpi.NewMessageLogs(exe)
	nodes := make([]*TraceNode, 0, len(found))
	bySpan, byGuid := map[string]*TraceNode{}, map[string]*TraceNode{}
	predecessor := map[*TraceNode]string{}
	for _, l := range found {
		n := &TraceNode{MessageGuid: l.MessageGuid, ArtifactID: l.ArtifactID, PackageID: l.PackageID, Status: l.Status,
			LogStart: l.LogStart, LogEnd: l.LogEnd, Children: []*TraceNode{},
			SpanID: strings.ToLower(headerValue(l.Headers, props.Span)), ParentSpanID: strings.ToLower(headerValue(l.Headers, props.Parent))}
		if slices.Contains(errorMessageStatuses, l.Status) {
			if text, err := mpl.ErrorText(l.MessageGuid); err == nil {
				n.ErrorText, n.ErrorTruncated = truncate(text, 0)
			}
		}
		nodes = append(nodes, n)
		byGuid[n.MessageGuid] = n
		if n.SpanID != "" {
			bySpan[n.SpanID] = n
		}
		predecessor[n] = l.PredecessorMessageGuid
	}
	for _, n := range nodes {
		parent := bySpan[n.ParentSpanID]
		if n.ParentSpanID == "" || parent == nil {
			parent = byGuid[predecessor[n]]
		}
		if parent != nil && parent != n {
			parent.Children = append(parent.Children, n)
		} else {
			tree.Roots = append(tree.Roots, n)
		}
	}
	for _, n := range nodes {
		sortByStart(n.Children)
	}
	sortByStart(tree.Roots)
	tree.Messages = len(nodes)

	var failed []*TraceNode
	for _, n := range nodes {
		if n.Status == "FAILED" || n.Status == "ESCALATED" {
			failed = append(failed, n)
		}
	}
	sortByStart(failed)
	if len(failed) > 0 {
		f := failed[0]
		tree.FirstFailure = &TraceFailure{MessageGuid: f.MessageGuid, ArtifactID: f.ArtifactID, ErrorText: f.ErrorText}
	}
	return tree, nil
}

func sortByStart(nodes []*TraceNode) {
	sort.SliceStable(nodes, func(i, j int) bool {
		a, b := nodes[i].LogStart, nodes[j].LogStart
		switch {
		case a == nil:
			return false
		case b == nil:
			return true
		}
		return a.Before(*b)
	})
}

// NewTraceparent returns a W3C traceparent header value and its trace ID.
func NewTraceparent() (traceparent, traceID string) {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	traceID = hex.EncodeToString(b[:16])
	return "00-" + traceID + "-" + hex.EncodeToString(b[16:]) + "-01", traceID
}

var reTraceparent = regexp.MustCompile(`^[0-9a-f]{2}-([0-9a-f]{32})-[0-9a-f]{16}-[0-9a-f]{2}$`)

// TraceIDOf returns the trace ID of a traceparent value ("" if invalid).
func TraceIDOf(traceparent string) string {
	if m := reTraceparent.FindStringSubmatch(strings.ToLower(strings.TrimSpace(traceparent))); m != nil {
		return m[1]
	}
	return ""
}
