package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cpars-innovation/cpicli/internal/output"
	"github.com/cpars-innovation/cpicli/internal/repo"
	"github.com/cpars-innovation/cpicli/pkg/cpi"
	"github.com/cpars-innovation/cpicli/pkg/httpclnt"
	"github.com/cpars-innovation/cpicli/pkg/ops"
)

// Instructions is sent to the client on initialize.
const Instructions = `Tools for SAP Cloud Integration (CPI) on one tenant.

Build loop: download_artifact (existing flow, once) or write files locally -> create_package (new
package) -> upload_artifact -> validate_artifact -> deploy -> send_test_message (wait_seconds=60)
-> on failure get_trace_tree (traceId of the result: the call tree across flows and firstFailure),
get_message_steps (failing step) and get_message_log / get_message_attachment /
get_message_store_entry for payloads; for step-by-step payloads set_log_level TRACE, send again,
get_message_trace and get_trace_message -> fix the local files and repeat.
Flows without an HTTP sender: ProcessDirect via send_test_message process_direct_address (test
harness flow); timer, SFTP and other polling flows: see the triggers in discover_tenant and use
list_message_logs with since and wait_seconds after triggering them.
Configuration: get_parameters / set_parameters, then deploy to activate; config_diff compares a
configure file with the tenant.
Review: check_guidelines (tenant design guidelines) before a release.
Inspect: list_packages, list_artifacts, list_resources, get_resource (read without download).
Operate: list_runtime_artifacts statuses=["ERROR"], get_runtime_status, list_message_logs,
list_service_endpoints.
Conventions: discover_tenant writes an inventory of existing flows (adapters, steps, error
handling, scripts, naming); follow the conventions of the repository (e.g. .cpi/conventions.md).

Autonomous loops: loop_start before changing anything, loop_status, loop_end when done or stopped.

Every result has {ok, errorCategory, exitCode, error, result}; errorCategory: usage (fix the
arguments), auth (stop, ask the user), tenant_http (retry later), failed (the tenant rejected the
content or the message failed: read error, fix), timeout (check status), partial (see items),
stopped (a loop limit was reached: stop changing things, call loop_end and report).
undeploy requires confirm=true. Partner Directory: pd_dependencies (which flows read a parameter), get_pd_parameters (tenant
values), pd_diff (local vs tenant), then
pd_deploy keys=["PID:ID"] to change one parameter without redeploying flows. pd_deploy is a
dry run unless dry_run=false. send_test_message
triggers real processing, including receiver calls. Security material is read-only here
(list_credentials, list_keystore): secrets never pass through this server.`

// Config configures the CPI tools.
type Config struct {
	Exe *httpclnt.HTTPExecuter
	// Root confines local paths (artifact dirs, partner directory) to this
	// directory tree.
	Root string
	// PollInterval and MaxChecks are the defaults for deploy/undeploy polling.
	PollInterval time.Duration
	MaxChecks    int
	// LogPollInterval is the interval used by list_message_logs wait_seconds.
	LogPollInterval time.Duration
	// NewEndpointExecuter connects to runtime endpoints for send_test_message
	// (nil: the tool reports that runtime credentials are not configured).
	NewEndpointExecuter ops.EndpointExecuterFunc
	// TenantHost is recorded as the source of discover_tenant.
	TenantHost string
	// DenyFullSync makes pd_deploy refuse full_sync (develop mode).
	DenyFullSync bool
	// LogLevels reverts set_log_level changes; created by Tools when nil.
	// Call RevertAll when the server stops.
	LogLevels *LogLevelReverter
}

// Tools returns the CPI tool set.
func Tools(cfg Config) []Tool {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 10 * time.Second
	}
	if cfg.MaxChecks == 0 {
		cfg.MaxChecks = 30
	}
	if cfg.LogPollInterval == 0 {
		cfg.LogPollInterval = 5 * time.Second
	}
	readOnly := map[string]any{"readOnlyHint": true, "openWorldHint": true}
	tenant := ops.NewTenant(cfg.Exe)
	endpoints := cachedEndpointExecuters(cfg.NewEndpointExecuter)
	if cfg.LogLevels == nil {
		cfg.LogLevels = NewLogLevelReverter(cfg.Exe)
	}
	reverter := cfg.LogLevels
	tools := toolList(cfg, readOnly, tenant, endpoints, reverter)
	for i := range tools {
		inner := tools[i].Handler
		tools[i].Handler = func(ctx context.Context, raw json.RawMessage) (any, error) {
			reverter.RunDue()
			return inner(ctx, raw)
		}
	}
	return tools
}

func toolList(cfg Config, readOnly map[string]any, tenant ops.Tenant, endpoints ops.EndpointExecuterFunc, reverter *LogLevelReverter) []Tool {

	return []Tool{
		{
			Name: "list_packages", Title: "List integration packages",
			Description: "List all integration packages (ID, name, version). Start here to find where artifacts live, then list_artifacts. To add a package use create_package.",
			InputSchema: object(nil),
			Annotations: readOnly,
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				if err := decode(raw, &struct{}{}); err != nil {
					return nil, err
				}
				pkgs, err := ops.ListPackages(cfg.Exe)
				return map[string]any{"packages": pkgs}, err
			},
		},
		{
			Name: "list_artifacts", Title: "List artifacts of a package",
			Description: "List the designtime artifacts of one package: integration flows, message mappings, script collections and value mappings, with version and draft flag. Use it to find artifact IDs; for what is running use list_runtime_artifacts.",
			InputSchema: object(props{"package_id": str("Integration package ID")}, "package_id"),
			Annotations: readOnly,
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					PackageID string `json:"package_id"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				if a.PackageID == "" {
					return nil, output.Usagef("package_id is required")
				}
				arts, err := ops.ListArtifacts(cfg.Exe, a.PackageID)
				return map[string]any{"packageId": a.PackageID, "artifacts": arts}, err
			},
		},
		{
			Name: "get_runtime_status", Title: "Get runtime status",
			Description: "Runtime state of given artifacts: deployed or not, STARTED/STARTING/ERROR, version, deployment time and, for ERROR, the tenant's error message. Use it for known artifacts (e.g. after a deploy timeout); to find all broken deployments use list_runtime_artifacts with statuses [\"ERROR\"].",
			InputSchema: object(props{"artifact_ids": strArray("Artifact IDs")}, "artifact_ids"),
			Annotations: readOnly,
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					ArtifactIDs []string `json:"artifact_ids"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				ids, err := requireIDs(a.ArtifactIDs)
				if err != nil {
					return nil, err
				}
				statuses := ops.GetRuntimeStatus(cfg.Exe, ids)
				var errs []string
				var firstErr error
				for _, s := range statuses {
					if s.Err != nil {
						errs = append(errs, s.ID+": "+s.Error)
						if firstErr == nil {
							firstErr = s.Err
						}
					}
				}
				result := map[string]any{"artifacts": statuses}
				if firstErr != nil {
					return result, fmt.Errorf("status lookup failed for %s: %w", strings.Join(errs, "; "), firstErr)
				}
				return result, nil
			},
		},
		{
			Name: "list_message_logs", Title: "Query message processing logs",
			Description: "Message processing logs (newest first) filtered by artifact, status, time and IDs, with the error text of failed messages. " +
				"Use it to see how an iFlow behaved at runtime: call with artifact_id, since=<time before the messages> and wait_seconds to wait until the messages reached a final status. " +
				"After send_test_message prefer its wait_seconds, which already returns the log of that message. " +
				"custom_header {name, value} finds messages by a custom header property; the tenant cannot filter on it, so it scans (needs artifact_id or package_id, and since; at most top*10, max 500 messages; see scanned/truncated).",
			InputSchema: object(props{
				"artifact_id":            str("Integration flow ID"),
				"statuses":               map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": cpi.MessageLogStatuses}, "description": "Only these statuses"},
				"since":                  str("Messages that ended after this time: RFC 3339 timestamp or duration back from now (30m, 2h, 1d)"),
				"until":                  str("Messages that started before this time (same format as since)"),
				"correlation_id":         str("Correlation ID"),
				"application_message_id": str("Application message ID"),
				"top":                    map[string]any{"type": "integer", "minimum": 1, "maximum": ops.MaxMessageLogs, "description": "Maximum number of messages, default 20"},
				"skip":                   integer("Skip the first n messages"),
				"include_errors":         boolean("Include the error text of failed messages (default true)"),
				"wait_seconds":           map[string]any{"type": "integer", "minimum": 0, "maximum": 600, "description": "Wait up to this long until at least one message matches and all matches are final"},
				"package_id":             str("Messages of all integration flows of this package (needs since)"),
				"custom_header": map[string]any{"type": "object", "description": "Only messages with this custom header property value (client-side scan)", "additionalProperties": false,
					"properties": map[string]any{"name": str("Property name"), "value": str("Property value")}, "required": []string{"name", "value"}},
			}),
			Annotations: readOnly,
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					ArtifactID           string                  `json:"artifact_id"`
					Statuses             []string                `json:"statuses"`
					Since                string                  `json:"since"`
					Until                string                  `json:"until"`
					CorrelationID        string                  `json:"correlation_id"`
					ApplicationMessageID string                  `json:"application_message_id"`
					Top                  int                     `json:"top"`
					Skip                 int                     `json:"skip"`
					IncludeErrors        *bool                   `json:"include_errors"`
					WaitSeconds          int                     `json:"wait_seconds"`
					PackageID            string                  `json:"package_id"`
					CustomHeader         *ops.CustomHeaderFilter `json:"custom_header"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				now := time.Now()
				since, err := ops.ParseTimeArg(a.Since, now)
				if err != nil {
					return nil, output.Usagef("invalid since: %v", err)
				}
				until, err := ops.ParseTimeArg(a.Until, now)
				if err != nil {
					return nil, output.Usagef("invalid until: %v", err)
				}
				q := ops.MessageLogQuery{ArtifactID: a.ArtifactID, Statuses: a.Statuses, Since: since, Until: until,
					CorrelationID: a.CorrelationID, ApplicationMessageID: a.ApplicationMessageID, Top: a.Top, Skip: a.Skip,
					IncludeErrors: a.IncludeErrors == nil || *a.IncludeErrors}
				if a.CustomHeader != nil || a.PackageID != "" {
					if a.WaitSeconds > 0 || a.CorrelationID != "" || a.ApplicationMessageID != "" || a.Skip > 0 {
						return nil, output.Usagef("custom_header and package_id cannot be combined with wait_seconds, correlation_id, application_message_id or skip")
					}
					scope := ops.ScanScope{PackageID: a.PackageID, Since: since, Until: until, Statuses: a.Statuses}
					if a.ArtifactID != "" {
						scope.ArtifactIDs = []string{a.ArtifactID}
					}
					if a.CustomHeader == nil {
						return ops.QueryPackageMessageLogs(cfg.Exe, scope, a.Top)
					}
					return ops.QueryMessageLogsByHeader(ctx, cfg.Exe, scope, *a.CustomHeader, a.Top, q.IncludeErrors)
				}
				if a.WaitSeconds > 0 {
					if a.WaitSeconds > 600 {
						return nil, output.Usagef("wait_seconds must be at most 600")
					}
					return ops.WaitForMessageLogs(ctx, cfg.Exe, q, time.Duration(a.WaitSeconds)*time.Second, cfg.LogPollInterval)
				}
				return ops.QueryMessageLogs(cfg.Exe, q)
			},
		},
		{
			Name: "get_message_log", Title: "Get one message processing log",
			Description: "Details of one message: status, full error text, custom header properties, adapter attributes, attachments and persisted messages. Next: get_message_steps for the failing step, get_message_attachment / get_message_store_entry for payloads.",
			InputSchema: object(props{"message_guid": str("Message GUID from list_message_logs")}, "message_guid"),
			Annotations: readOnly,
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					MessageGuid string `json:"message_guid"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				return ops.GetMessageLog(cfg.Exe, a.MessageGuid, 0)
			},
		},
		{
			Name: "get_message_steps", Title: "Get processing steps of a message",
			Description: "Step-level trace of a message (runs and steps with status and error) and the first failing step. Use it for FAILED messages: modelStepId is the id of the element in the .iflw model to fix.",
			InputSchema: object(props{"message_guid": str("Message GUID")}, "message_guid"),
			Annotations: readOnly,
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					MessageGuid string `json:"message_guid"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				return ops.GetMessageSteps(cfg.Exe, a.MessageGuid)
			},
		},
		{
			Name: "get_message_attachment", Title: "Download a message log attachment",
			Description: "Content of a message log attachment (id from get_message_log attachments), e.g. a payload logged by a script. Text inline, binary base64; truncated to max_bytes.",
			InputSchema: object(props{"attachment_id": str("Attachment ID"), "max_bytes": maxBytesSchema()}, "attachment_id"),
			Annotations: readOnly,
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					AttachmentID string `json:"attachment_id"`
					MaxBytes     int    `json:"max_bytes"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				if err := checkMaxBytes(a.MaxBytes); err != nil {
					return nil, err
				}
				return ops.GetMessageAttachment(cfg.Exe, a.AttachmentID, a.MaxBytes)
			},
		},
		{
			Name: "get_message_store_entry", Title: "Download a persisted message",
			Description: "Payload written by a Persist step (id from messageStoreEntries of get_message_log). Truncated to max_bytes.",
			InputSchema: object(props{"entry_id": str("Message store entry ID"), "max_bytes": maxBytesSchema()}, "entry_id"),
			Annotations: readOnly,
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					EntryID  string `json:"entry_id"`
					MaxBytes int    `json:"max_bytes"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				if err := checkMaxBytes(a.MaxBytes); err != nil {
					return nil, err
				}
				return ops.GetMessageStoreEntry(cfg.Exe, a.EntryID, a.MaxBytes)
			},
		},
		{
			Name: "set_log_level", Title: "Set the log level of a deployed flow",
			Description: "Set the message processing log level of a deployed integration flow: NONE, INFO, DEBUG or TRACE. " +
				"TRACE records payload and headers at every step for the next 10 minutes (then the tenant resets it): set it, send the message again, then get_message_trace. " +
				"Traces contain business data; use on development tenants.",
			InputSchema: object(props{
				"artifact_id":         str("Integration flow ID (deployed)"),
				"level":               enum("Log level", ops.LogLevels...),
				"node_type":           str(`Runtime node type, default "IFLMAP"`),
				"runtime_location_id": str(`Runtime location, default "cloudintegration" (edge integration cells use their own)`),
				"revert_after_minutes": map[string]any{"type": "integer", "minimum": 0, "maximum": 1440,
					"description": "Set the flow back to INFO after this many minutes (on the next tool call, or when the server stops); default 10 for TRACE and DEBUG, 0 = never"},
			}, "artifact_id", "level"),
			Annotations: map[string]any{"readOnlyHint": false, "destructiveHint": false, "idempotentHint": true, "openWorldHint": true},
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					ArtifactID        string `json:"artifact_id"`
					Level             string `json:"level"`
					NodeType          string `json:"node_type"`
					RuntimeLocationID string `json:"runtime_location_id"`
					RevertAfter       *int   `json:"revert_after_minutes"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				req := ops.LogLevelRequest{ArtifactID: a.ArtifactID, Level: a.Level, NodeType: a.NodeType, RuntimeLocationID: a.RuntimeLocationID}
				res, err := ops.SetLogLevel(cfg.Exe, req)
				if err != nil {
					return res, err
				}
				minutes := 0
				if res.Level == "TRACE" || res.Level == "DEBUG" {
					minutes = 10
				}
				if a.RevertAfter != nil {
					minutes = *a.RevertAfter
				}
				if minutes > 0 && res.Level != "INFO" {
					at := reverter.now().UTC().Add(time.Duration(minutes) * time.Minute).Truncate(time.Second)
					res.RevertsAt = &at
					reverter.schedule(req, at)
				} else {
					reverter.schedule(req, time.Time{})
				}
				return res, nil
			},
		},
		{
			Name: "get_message_trace", Title: "List traced steps of a message",
			Description: "Steps of a message that was processed with log level TRACE, with the trace IDs of the message at each step (optionally only one model step). " +
				"Use it to see how the payload changed step by step; read one with get_trace_message. Empty with a hint when the flow was not on TRACE.",
			InputSchema: object(props{
				"message_guid":  str("Message GUID"),
				"model_step_id": str("Only this element of the .iflw model (modelStepId from get_message_steps)"),
			}, "message_guid"),
			Annotations: readOnly,
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					MessageGuid string `json:"message_guid"`
					ModelStepID string `json:"model_step_id"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				return ops.GetMessageTrace(cfg.Exe, a.MessageGuid, a.ModelStepID)
			},
		},
		{
			Name: "get_trace_message", Title: "Read a traced message",
			Description: "Payload, headers and exchange properties of the message at one traced step (trace ID from get_message_trace). Sensitive header values are masked; the payload is truncated to max_bytes.",
			InputSchema: object(props{"trace_id": str("Trace ID from get_message_trace"), "max_bytes": maxBytesSchema()}, "trace_id"),
			Annotations: readOnly,
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					TraceID  string `json:"trace_id"`
					MaxBytes int    `json:"max_bytes"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				if err := checkMaxBytes(a.MaxBytes); err != nil {
					return nil, err
				}
				return ops.GetTraceMessage(cfg.Exe, a.TraceID, a.MaxBytes)
			},
		},
		{
			Name: "get_trace_tree", Title: "Get the call tree of a trace",
			Description: "From one trace ID (traceId of send_test_message, or trace-id custom header) to the call tree across flows (ProcessDirect, JMS, HTTP) and firstFailure, the earliest failed message. " +
				"Finds the messages by ApplicationMessageId first; otherwise scans the given scope (artifact_ids or package_id, and since) for the trace-id custom header, at most max_scan messages. " +
				"Next: get_message_steps for firstFailure.messageGuid.",
			InputSchema: object(props{
				"trace_id":     str("32 hex characters"),
				"artifact_ids": strArray("Scope of the fallback scan"),
				"package_id":   str("Scope of the fallback scan: all flows of this package"),
				"since":        str("Start of the scan window: RFC 3339 or duration back from now (30m, 2h)"),
				"until":        str("End of the scan window"),
				"max_scan":     map[string]any{"type": "integer", "minimum": 1, "maximum": 1000, "description": "Maximum messages scanned, default 200"},
				"properties": map[string]any{"type": "object", "additionalProperties": false, "description": "Custom header names of the tracer (default trace-id, span-id, parent-span-id)",
					"properties": map[string]any{"trace": str("Trace ID property"), "span": str("Span ID property"), "parent": str("Parent span ID property")}},
			}, "trace_id"),
			Annotations: readOnly,
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					TraceID     string   `json:"trace_id"`
					ArtifactIDs []string `json:"artifact_ids"`
					PackageID   string   `json:"package_id"`
					Since       string   `json:"since"`
					Until       string   `json:"until"`
					MaxScan     int      `json:"max_scan"`
					Properties  *struct {
						Trace  string `json:"trace"`
						Span   string `json:"span"`
						Parent string `json:"parent"`
					} `json:"properties"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				now := time.Now()
				since, err := ops.ParseTimeArg(a.Since, now)
				if err != nil {
					return nil, output.Usagef("invalid since: %v", err)
				}
				until, err := ops.ParseTimeArg(a.Until, now)
				if err != nil {
					return nil, output.Usagef("invalid until: %v", err)
				}
				q := ops.TraceTreeQuery{TraceID: a.TraceID, MaxScan: a.MaxScan,
					Scope: ops.ScanScope{ArtifactIDs: a.ArtifactIDs, PackageID: a.PackageID, Since: since, Until: until}}
				if a.Properties != nil {
					q.Properties = ops.TraceProperties{Trace: a.Properties.Trace, Span: a.Properties.Span, Parent: a.Properties.Parent}
				}
				return ops.TraceTreeFor(ctx, cfg.Exe, q)
			},
		},
		{
			Name: "list_runtime_artifacts", Title: "List deployed artifacts",
			Description: "All artifacts deployed on the tenant with status, version and deployment time. Use statuses [\"ERROR\"] for a tenant-wide health check (includes error messages); for specific artifacts use get_runtime_status.",
			InputSchema: object(props{"statuses": map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": []string{"STARTED", "STARTING", "ERROR", "STOPPING"}}, "description": "Only artifacts in these runtime statuses (default: all)"}}),
			Annotations: readOnly,
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					Statuses []string `json:"statuses"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				list, err := ops.ListRuntimeArtifacts(cfg.Exe, a.Statuses)
				return map[string]any{"artifacts": list}, err
			},
		},
		{
			Name: "list_service_endpoints", Title: "List endpoint URLs",
			Description: "Callable URLs of deployed integration flows with HTTP-based senders (HTTPS, SOAP, ...). send_test_message finds the URL itself; use this to show URLs or to choose between several endpoints of one flow.",
			InputSchema: object(props{"artifact_id": str("Only endpoints of this integration flow")}),
			Annotations: readOnly,
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					ArtifactID string `json:"artifact_id"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				eps, err := ops.ListServiceEndpoints(cfg.Exe, a.ArtifactID)
				return map[string]any{"endpoints": eps}, err
			},
		},
		{
			Name: "validate_artifact", Title: "Validate an integration flow",
			Description: "Tenant check of an uploaded integration flow (Check in the Web UI): model errors such as missing mandatory settings. Checks the designtime version on the tenant, not local files: run it after upload_artifact and before deploy. status PASSED or FAILED (errorCategory failed) with details.",
			InputSchema: object(props{"artifact_id": str("Integration flow ID"), "version": str(`Designtime version, default "active"`)}, "artifact_id"),
			Annotations: map[string]any{"readOnlyHint": true, "openWorldHint": true},
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					ArtifactID string `json:"artifact_id"`
					Version    string `json:"version"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				return ops.ValidateArtifact(cfg.Exe, a.ArtifactID, a.Version)
			},
		},
		{
			Name: "check_guidelines", Title: "Check design guidelines",
			Description: "Run the design guidelines activated on the tenant against an integration flow and wait for the result; returns violations with the violated components. Slower than validate_artifact: use it before a review or release, not after every edit.",
			InputSchema: object(props{
				"artifact_id":     str("Integration flow ID"),
				"version":         str(`Designtime version, default "active"`),
				"timeout_seconds": map[string]any{"type": "integer", "minimum": 1, "maximum": 600, "description": "Maximum wait, default 120"},
			}, "artifact_id"),
			Annotations: map[string]any{"readOnlyHint": false, "destructiveHint": false, "idempotentHint": true, "openWorldHint": true},
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					ArtifactID     string `json:"artifact_id"`
					Version        string `json:"version"`
					TimeoutSeconds int    `json:"timeout_seconds"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				timeout := 120 * time.Second
				if a.TimeoutSeconds > 0 {
					timeout = time.Duration(a.TimeoutSeconds) * time.Second
				}
				return ops.CheckGuidelines(ctx, cfg.Exe, a.ArtifactID, a.Version, timeout, cfg.LogPollInterval)
			},
		},
		{
			Name: "list_resources", Title: "List resources of an integration flow",
			Description: "Resources inside an integration flow on the tenant (scripts, mappings, schemas) with name and type. To read one file use get_resource; to edit the flow use download_artifact.",
			InputSchema: object(props{"artifact_id": str("Integration flow ID"), "version": str(`Designtime version, default "active"`)}, "artifact_id"),
			Annotations: readOnly,
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					ArtifactID string `json:"artifact_id"`
					Version    string `json:"version"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				list, err := ops.ListResources(cfg.Exe, a.ArtifactID, a.Version)
				return map[string]any{"artifactId": a.ArtifactID, "resources": list}, err
			},
		},
		{
			Name: "get_resource", Title: "Read a resource of an integration flow",
			Description: "Content of one resource of an integration flow on the tenant (e.g. a Groovy script or XSLT) without downloading the flow; text inline, binary base64.",
			InputSchema: object(props{
				"artifact_id": str("Integration flow ID"),
				"name":        str("Resource name from list_resources"),
				"type":        str("Resource type from list_resources (e.g. groovy, xslt, mmap)"),
				"version":     str(`Designtime version, default "active"`),
				"max_bytes":   maxBytesSchema(),
			}, "artifact_id", "name", "type"),
			Annotations: readOnly,
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					ArtifactID string `json:"artifact_id"`
					Name       string `json:"name"`
					Type       string `json:"type"`
					Version    string `json:"version"`
					MaxBytes   int    `json:"max_bytes"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				if err := checkMaxBytes(a.MaxBytes); err != nil {
					return nil, err
				}
				return ops.GetResource(cfg.Exe, a.ArtifactID, a.Version, a.Name, a.Type, a.MaxBytes)
			},
		},
		{
			Name: "download_artifact", Title: "Download an artifact into a local directory",
			Description: "Download a designtime artifact and extract it into a local directory inside the server root. Use it once before editing an existing artifact; then edit the files and upload_artifact. The directory must be empty unless overwrite=true.",
			InputSchema: object(props{
				"artifact_id":   str("Artifact ID"),
				"artifact_type": enum(`Artifact type, default "Integration"`, cpi.ArtifactTypes...),
				"dir":           str("Target directory, relative to the server root"),
				"overwrite":     boolean("Replace the content of a non-empty directory"),
				"version":       str(`Designtime version, default "active"`),
			}, "artifact_id", "dir"),
			Annotations: map[string]any{"readOnlyHint": false, "destructiveHint": true, "idempotentHint": true, "openWorldHint": true},
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					ArtifactID   string `json:"artifact_id"`
					ArtifactType string `json:"artifact_type"`
					Dir          string `json:"dir"`
					Overwrite    bool   `json:"overwrite"`
					Version      string `json:"version"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				if a.ArtifactType == "" {
					a.ArtifactType = "Integration"
				}
				dir, err := resolvePath(cfg.Root, a.Dir)
				if err != nil {
					return nil, err
				}
				if dir == mustAbs(cfg.Root) {
					return nil, output.Usagef("dir must be a sub-directory of the server root")
				}
				return ops.DownloadArtifactToDir(cfg.Exe, a.ArtifactType, a.ArtifactID, a.Version, dir, a.Overwrite)
			},
		},
		{
			Name: "list_credentials", Title: "List security credentials",
			Description: "Names and metadata of user credentials, OAuth2 client credentials and secure parameters deployed on the tenant (never secrets). Use it to check that the credentials an iFlow references exist. Credentials cannot be created through MCP; ask the user to run 'cpictl credentials'.",
			InputSchema: object(props{"kind": enum("Only this kind", cpi.CredentialKinds...)}),
			Annotations: readOnly,
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					Kind string `json:"kind"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				return ops.ListCredentials(cfg.Exe, a.Kind)
			},
		},
		{
			Name: "list_keystore", Title: "List keystore entries",
			Description: "Certificates and key pairs of a tenant keystore with validity and days left; flags entries expiring within expiring_within_days. Use it to check that a key alias an adapter references exists and is valid.",
			InputSchema: object(props{
				"keystore":             enum(`Keystore, default "system"`, cpi.Keystores...),
				"expiring_within_days": map[string]any{"type": "integer", "minimum": 0, "maximum": 3650, "description": "Flag entries expiring within this many days"},
			}),
			Annotations: readOnly,
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					Keystore           string `json:"keystore"`
					ExpiringWithinDays int    `json:"expiring_within_days"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				return ops.ListKeystore(cfg.Exe, a.Keystore, time.Duration(a.ExpiringWithinDays)*24*time.Hour, false, time.Now())
			},
		},
		{
			Name: "get_parameters", Title: "Get configuration parameters",
			Description: "Externalised parameters ({{...}} placeholders in the model) of an integration flow with their current values. Change them with set_parameters; no upload needed.",
			InputSchema: object(props{"artifact_id": str("Integration flow ID"), "version": str(`Designtime version, default "active"`)}, "artifact_id"),
			Annotations: readOnly,
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					ArtifactID string `json:"artifact_id"`
					Version    string `json:"version"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				if a.ArtifactID == "" {
					return nil, output.Usagef("artifact_id is required")
				}
				params, err := ops.GetConfiguration(cfg.Exe, a.ArtifactID, a.Version)
				return map[string]any{"artifactId": a.ArtifactID, "parameters": params}, err
			},
		},
		{
			Name: "set_parameters", Title: "Set configuration parameters",
			Description: "Set externalised parameters of an integration flow. Only changed values are written; unknown keys fail the call before anything is written. The running version keeps the old values until you deploy.",
			InputSchema: object(props{
				"artifact_id": str("Integration flow ID"),
				"parameters":  map[string]any{"type": "object", "description": "Parameter key -> new value", "additionalProperties": map[string]any{"type": "string"}},
				"version":     str(`Designtime version, default "active"`),
				"dry_run":     boolean("Only report what would change"),
			}, "artifact_id", "parameters"),
			Annotations: map[string]any{"readOnlyHint": false, "destructiveHint": false, "idempotentHint": true, "openWorldHint": true},
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					ArtifactID string            `json:"artifact_id"`
					Parameters map[string]string `json:"parameters"`
					Version    string            `json:"version"`
					DryRun     bool              `json:"dry_run"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				if a.ArtifactID == "" || len(a.Parameters) == 0 {
					return nil, output.Usagef("artifact_id and at least one parameter are required")
				}
				return ops.UpdateConfiguration(cfg.Exe, a.ArtifactID, a.Version, a.Parameters, a.DryRun)
			},
		},
		{
			Name: "create_package", Title: "Create an integration package",
			Description: "Create an integration package if it does not exist (action CREATED or EXISTS; an existing package is never changed). Use it before upload_artifact of an artifact in a new package.",
			InputSchema: object(props{
				"package_id":  str("Package ID: letters, digits, '_' and '.'"),
				"name":        str("Display name, defaults to package_id"),
				"description": str("Description"),
				"short_text":  str("Short description, defaults to name"),
			}, "package_id"),
			Annotations: map[string]any{"readOnlyHint": false, "destructiveHint": false, "idempotentHint": true, "openWorldHint": true},
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					PackageID   string `json:"package_id"`
					Name        string `json:"name"`
					Description string `json:"description"`
					ShortText   string `json:"short_text"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				return ops.CreatePackage(cfg.Exe, ops.PackageRequest{ID: a.PackageID, Name: a.Name, Description: a.Description, ShortText: a.ShortText})
			},
		},
		{
			Name: "upload_artifact", Title: "Upload artifact from a local directory",
			Description: "Create or update a designtime artifact from a local directory (action CREATED, UPDATED or UNCHANGED). The package must exist (create_package); the directory needs META-INF/MANIFEST.MF and src/main/resources. Does not deploy: next validate_artifact, then deploy.",
			InputSchema: object(props{
				"artifact_id": str("Artifact ID (must match Bundle-SymbolicName)"),
				"name":        str("Display name, defaults to artifact_id"),
				"type":        enum("Artifact type", cpi.ArtifactTypes...),
				"package_id":  str("Integration package ID (must exist)"),
				"dir":         str("Local artifact directory, relative to the server root"),
			}, "artifact_id", "type", "package_id", "dir"),
			Annotations: map[string]any{"readOnlyHint": false, "destructiveHint": false, "idempotentHint": true, "openWorldHint": true},
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					ArtifactID string `json:"artifact_id"`
					Name       string `json:"name"`
					Type       string `json:"type"`
					PackageID  string `json:"package_id"`
					Dir        string `json:"dir"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				dir, err := resolvePath(cfg.Root, a.Dir)
				if err != nil {
					return nil, err
				}
				return ops.UploadArtifact(cfg.Exe, ops.UploadRequest{ID: a.ArtifactID, Name: a.Name, Type: a.Type, PackageID: a.PackageID, Dir: dir})
			},
		},
		{
			Name: "deploy", Title: "Deploy artifacts",
			Description: "Deploy designtime artifacts to runtime and wait for the outcome. One result per artifact: DEPLOYED, SKIPPED, FAILED (with the tenant's error message) or TIMEOUT. A redeploy is only DEPLOYED once the runtime shows the new deployment. A designtime version older than the running one is refused (FAILED) unless allow_downgrade. Next: send_test_message to test the flow.",
			InputSchema: object(props{
				"artifact_ids":          strArray("Artifact IDs"),
				"artifact_type":         enum(`Artifact type, default "Integration"`, cpi.ArtifactTypes...),
				"compare_versions":      boolean("Skip artifacts whose version is already running (default false: always deploy)"),
				"allow_downgrade":       boolean("Deploy even if the designtime version is older than the running one (default false: such a deploy FAILS before it is triggered, because the tenant copy was probably not updated)"),
				"poll_interval_seconds": integer("Seconds between status checks"),
				"max_checks":            integer("Maximum number of status checks per artifact"),
			}, "artifact_ids"),
			Annotations: map[string]any{"readOnlyHint": false, "destructiveHint": false, "idempotentHint": true, "openWorldHint": true},
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					ArtifactIDs     []string `json:"artifact_ids"`
					ArtifactType    string   `json:"artifact_type"`
					CompareVersions bool     `json:"compare_versions"`
					AllowDowngrade  bool     `json:"allow_downgrade"`
					PollInterval    *int     `json:"poll_interval_seconds"`
					MaxChecks       *int     `json:"max_checks"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				ids, err := requireIDs(a.ArtifactIDs)
				if err != nil {
					return nil, err
				}
				if a.ArtifactType == "" {
					a.ArtifactType = "Integration"
				}
				if !cpi.IsValidArtifactType(a.ArtifactType) {
					return nil, output.Usagef("invalid artifact_type %q (valid: %s)", a.ArtifactType, strings.Join(cpi.ArtifactTypes, ", "))
				}
				artifacts := make([]ops.Artifact, 0, len(ids))
				for _, id := range ids {
					artifacts = append(artifacts, ops.Artifact{ID: id, Type: a.ArtifactType})
				}
				opts := pollOptions(cfg, a.PollInterval, a.MaxChecks)
				opts.CompareVersions = a.CompareVersions
				opts.AllowDowngrade = a.AllowDowngrade
				results := ops.Deploy(ctx, tenant, artifacts, opts)
				return map[string]any{"results": results}, ops.Err(results)
			},
		},
		{
			Name: "send_test_message", Title: "Send a test message",
			Description: "Send a test message to an endpoint of a deployed integration flow (only URLs the tenant lists for it) and return the HTTP status, the response and the message GUID. " +
				"Flows started by ProcessDirect are reached through the test harness flow with process_direct_address. " +
				"With wait_seconds it also waits for the message processing log and returns it (status, error text, custom headers, attachments). " +
				"This triggers real processing, including calls to receivers: use test data on a development tenant. On failure: get_message_steps with the message GUID.",
			InputSchema: object(props{
				"artifact_id":  str("Integration flow ID (deployed)"),
				"url":          str("Endpoint URL from list_service_endpoints; only needed when the flow has several endpoints"),
				"method":       enum(`HTTP method, default "POST"`, "POST", "PUT", "PATCH", "GET", "DELETE"),
				"body":         str("Message body (text)"),
				"content_type": str("Content-Type of the body, e.g. application/xml"),
				"headers":      map[string]any{"type": "object", "description": "Additional HTTP headers (not Authorization, Cookie or X-CSRF-Token)", "additionalProperties": map[string]any{"type": "string"}},
				"wait_seconds": map[string]any{"type": "integer", "minimum": 0, "maximum": 600, "description": "Wait up to this long for the message processing log to reach a final status"},
				"max_bytes":    maxBytesSchema(),
				"trace":        boolean("Send a W3C traceparent header and return its traceId for get_trace_tree (default true; a traceparent in headers is kept)"),
				"process_direct_address": str("For flows with a ProcessDirect sender: send through the test harness flow to this address (e.g. /billing/in); " +
					"artifact_id is then the flow behind the address and the returned log is that flow's message"),
				"harness": str(`Test harness flow ID, default "` + ops.DefaultHarnessID + `"`),
			}, "artifact_id"),
			Annotations: map[string]any{"readOnlyHint": false, "destructiveHint": false, "idempotentHint": false, "openWorldHint": true},
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					ArtifactID  string            `json:"artifact_id"`
					URL         string            `json:"url"`
					Method      string            `json:"method"`
					Body        string            `json:"body"`
					ContentType string            `json:"content_type"`
					Headers     map[string]string `json:"headers"`
					WaitSeconds int               `json:"wait_seconds"`
					MaxBytes    int               `json:"max_bytes"`
					Address     string            `json:"process_direct_address"`
					Trace       *bool             `json:"trace"`
					Harness     string            `json:"harness"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				if err := checkMaxBytes(a.MaxBytes); err != nil {
					return nil, err
				}
				if a.WaitSeconds < 0 || a.WaitSeconds > 600 {
					return nil, output.Usagef("wait_seconds must be between 0 and 600")
				}
				if endpoints == nil {
					return nil, output.Usagef("sending test messages is not configured for this server")
				}
				return ops.SendTestMessage(ctx, cfg.Exe, endpoints, ops.TestMessage{
					ArtifactID: a.ArtifactID, URL: a.URL, Method: a.Method, Body: []byte(a.Body), ContentType: a.ContentType,
					Headers: a.Headers, MaxBytes: a.MaxBytes, Wait: time.Duration(a.WaitSeconds) * time.Second, PollInterval: cfg.LogPollInterval,
					ProcessDirectAddress: a.Address, Harness: a.Harness, NoTrace: a.Trace != nil && !*a.Trace,
				})
			},
		},
		{
			Name: "undeploy", Title: "Undeploy artifacts",
			Description: "Remove artifacts from runtime and wait until they are gone (status UNDEPLOYED, NOT_DEPLOYED, FAILED or TIMEOUT). Designtime artifacts are kept. Requires confirm=true.",
			InputSchema: object(props{
				"artifact_ids":          strArray("Artifact IDs"),
				"confirm":               boolean("Must be true: stops the integration on the tenant"),
				"poll_interval_seconds": integer("Seconds between status checks"),
				"max_checks":            integer("Maximum number of status checks per artifact"),
			}, "artifact_ids", "confirm"),
			Annotations: map[string]any{"readOnlyHint": false, "destructiveHint": true, "idempotentHint": true, "openWorldHint": true},
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					ArtifactIDs  []string `json:"artifact_ids"`
					Confirm      bool     `json:"confirm"`
					PollInterval *int     `json:"poll_interval_seconds"`
					MaxChecks    *int     `json:"max_checks"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				ids, err := requireIDs(a.ArtifactIDs)
				if err != nil {
					return nil, err
				}
				if !a.Confirm {
					return nil, output.Usagef("undeploy stops %s on the tenant; call again with confirm=true", strings.Join(ids, ", "))
				}
				artifacts := make([]ops.Artifact, 0, len(ids))
				for _, id := range ids {
					artifacts = append(artifacts, ops.Artifact{ID: id})
				}
				results := ops.Undeploy(ctx, tenant, artifacts, pollOptions(cfg, a.PollInterval, a.MaxChecks))
				return map[string]any{"results": results}, ops.Err(results)
			},
		},
		{
			Name: "pd_deploy", Title: "Deploy Partner Directory parameters",
			Description: "Upload Partner Directory parameters from a local directory ({PID}/String.properties, {PID}/Binary/). Runs as a dry run unless dry_run=false. " +
				"keys=[\"PID:ID\"] deploys only those parameters (create or update, nothing else is touched): use it to fix one mapping. " +
				"full_sync deletes remote parameters of the managed PIDs that do not exist locally; check with pd_diff first.",
			InputSchema: object(props{
				"resources_path": str("Local partner directory root, relative to the server root"),
				"pids":           strArray("Restrict to these partner IDs"),
				"replace":        boolean("Update existing parameters whose value differs (default true)"),
				"full_sync":      boolean("Delete remote parameters not present locally (default false)"),
				"dry_run":        boolean("Only report what would change (default true)"),
				"keys":           strArray("Only these parameters, \"PID:ID\" (not with full_sync or pids)"),
			}, "resources_path"),
			Annotations: map[string]any{"readOnlyHint": false, "destructiveHint": true, "idempotentHint": true, "openWorldHint": true},
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					ResourcesPath string   `json:"resources_path"`
					PIDs          []string `json:"pids"`
					Replace       *bool    `json:"replace"`
					FullSync      bool     `json:"full_sync"`
					DryRun        *bool    `json:"dry_run"`
					Keys          []string `json:"keys"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				dir, err := resolvePath(cfg.Root, a.ResourcesPath)
				if err != nil {
					return nil, err
				}
				if a.FullSync && cfg.DenyFullSync {
					return nil, output.Usagef("full_sync is not allowed in this mode: deploy single parameters with keys, or ask the user to run 'cpictl pd-deploy --full-sync'")
				}
				opts := ops.PDDeployOptions{Replace: true, FullSync: a.FullSync, DryRun: true, PIDs: a.PIDs, Keys: a.Keys}
				if a.Replace != nil {
					opts.Replace = *a.Replace
				}
				if a.DryRun != nil {
					opts.DryRun = *a.DryRun
				}
				return ops.PDDeploy(cpi.NewPartnerDirectory(cfg.Exe), repo.NewPartnerDirectory(dir), opts)
			},
		},
		{
			Name: "get_pd_parameters", Title: "Read Partner Directory parameters",
			Description: "String and binary Partner Directory parameters of one partner ID on the tenant (all, or only keys). " +
				"Binaries show content type, size and sha256; their content only with include_content. Use it before pd_deploy to see the tenant's value.",
			InputSchema: object(props{
				"pid":             str("Partner ID"),
				"keys":            strArray("Only these parameter IDs"),
				"include_content": boolean("Return binary content (text inline, else base64; default false)"),
				"max_bytes":       maxBytesSchema(),
			}, "pid"),
			Annotations: readOnly,
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					Pid            string   `json:"pid"`
					Keys           []string `json:"keys"`
					IncludeContent bool     `json:"include_content"`
					MaxBytes       int      `json:"max_bytes"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				if err := checkMaxBytes(a.MaxBytes); err != nil {
					return nil, err
				}
				return ops.GetPDParameters(cpi.NewPartnerDirectory(cfg.Exe), a.Pid, a.Keys, a.IncludeContent, a.MaxBytes)
			},
		},
		{
			Name: "pd_diff", Title: "Compare local Partner Directory files with the tenant",
			Description: "Per parameter: create, update (value, content or content type), unchanged, or remote_only (exists only on the tenant: pd_deploy full_sync would delete it). " +
				"Binaries are compared by content hash and content type. A PID whose local files cannot be read is reported as an error, never as empty.",
			InputSchema: object(props{
				"resources_path": str("Local partner directory root, relative to the server root"),
				"pids":           strArray("Only these partner IDs"),
			}, "resources_path"),
			Annotations: readOnly,
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					ResourcesPath string   `json:"resources_path"`
					PIDs          []string `json:"pids"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				dir, err := resolvePath(cfg.Root, a.ResourcesPath)
				if err != nil {
					return nil, err
				}
				return ops.PDDiff(cpi.NewPartnerDirectory(cfg.Exe), repo.NewPartnerDirectory(dir), a.PIDs)
			},
		},
		{
			Name: "pd_dependencies", Title: "Find which flows use Partner Directory parameters",
			Description: "Scan local content (inside the server root) for Partner Directory references: pd:<PID>:<ID>:<Binary|String> in .iflw models (with the step id), dynamic pd:${...} references, " +
				"and getParameter(id, pid, ...) in Groovy scripts. With resources_path, unknownPids lists referenced PIDs that have no local directory (often a typo in the model). " +
				"Use it before pd_deploy: every listed flow is affected by the change. Reads local files only.",
			InputSchema: object(props{
				"local_dir":      str("Local content directory, relative to the server root"),
				"resources_path": str("Local Partner Directory tree, to report unknown PIDs"),
				"pid":            str("Only references to this partner ID"),
				"id":             str("Only references to this parameter ID"),
			}, "local_dir"),
			Annotations: readOnly,
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					LocalDir      string `json:"local_dir"`
					ResourcesPath string `json:"resources_path"`
					Pid           string `json:"pid"`
					ID            string `json:"id"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				dir, err := resolvePath(cfg.Root, a.LocalDir)
				if err != nil {
					return nil, err
				}
				f := ops.PDDependenciesFilter{Pid: a.Pid, ID: a.ID}
				if a.ResourcesPath != "" {
					if f.ResourcesPath, err = resolvePath(cfg.Root, a.ResourcesPath); err != nil {
						return nil, err
					}
				}
				return ops.FindPDDependencies(ctx, dir, f)
			},
		},
		{
			Name: "config_diff", Title: "Compare a configure file with the tenant",
			Description: "Compare the parameters of a configure YAML file (or folder) with the tenant: per key update (with local and tenant value), unchanged, or unknown_key. " +
				"Shows what 'cpictl configure' would write and which artifacts it would redeploy (only those with a change).",
			InputSchema: object(props{
				"config_path":       str("Configure YAML file or folder, relative to the server root"),
				"package_filter":    strArray("Only these packages (IDs as in the file)"),
				"artifact_filter":   strArray("Only these artifacts (IDs as in the file)"),
				"deployment_prefix": str("Prefix for package and artifact IDs (overrides the file's deploymentPrefix)"),
			}, "config_path"),
			Annotations: readOnly,
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					ConfigPath       string   `json:"config_path"`
					PackageFilter    []string `json:"package_filter"`
					ArtifactFilter   []string `json:"artifact_filter"`
					DeploymentPrefix string   `json:"deployment_prefix"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				path, err := resolvePath(cfg.Root, a.ConfigPath)
				if err != nil {
					return nil, err
				}
				files, err := ops.LoadConfigureFiles(path)
				if err != nil {
					return nil, output.Usage(err)
				}
				return ops.ConfigDiff(cfg.Exe, ops.MergeConfigureFiles(files, a.DeploymentPrefix), ops.ConfigFilter{Packages: a.PackageFilter, Artifacts: a.ArtifactFilter})
			},
		},
		{
			Name: "discover_tenant", Title: "Discover conventions of existing content",
			Description: "Inventory packages and integration flows (adapters, steps, exception subprocesses, log levels, scripts, parameters, credential names, naming patterns) " +
				"and write it as JSON to output_file inside the server root; returns the summary. Read-only on the tenant. " +
				"Use it once to derive the conventions of a tenant; it downloads every flow, so limit with package_ids on large tenants. local_dir analyses a local repository instead.",
			InputSchema: object(props{
				"package_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Only these packages (default: all)"},
				"max_iflows":  map[string]any{"type": "integer", "minimum": 0, "description": "Stop after this many integration flows (default: all)"},
				"output_file": str(`JSON file relative to the server root, default ".cpi/discovery.json"`),
				"local_dir":   str("Analyse this local directory (relative to the server root) instead of the tenant"),
			}),
			Annotations: map[string]any{"readOnlyHint": false, "destructiveHint": false, "idempotentHint": true, "openWorldHint": true},
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					PackageIDs []string `json:"package_ids"`
					MaxIFlows  int      `json:"max_iflows"`
					OutputFile string   `json:"output_file"`
					LocalDir   string   `json:"local_dir"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				if a.OutputFile == "" {
					a.OutputFile = ".cpi/discovery.json"
				}
				out, err := resolvePath(cfg.Root, a.OutputFile)
				if err != nil {
					return nil, err
				}
				var d *ops.Discovery
				if a.LocalDir != "" {
					dir, err := resolvePath(cfg.Root, a.LocalDir)
					if err != nil {
						return nil, err
					}
					d, err = ops.DiscoverDir(ctx, dir)
					if err != nil {
						return nil, err
					}
					d.Source = a.LocalDir
				} else {
					d, err = ops.DiscoverTenant(ctx, cfg.Exe, cfg.TenantHost, ops.DiscoverOptions{PackageIDs: a.PackageIDs, MaxIFlows: a.MaxIFlows})
					if err != nil {
						return nil, err
					}
				}
				if err := ops.WriteDiscovery(d, out); err != nil {
					return nil, err
				}
				return map[string]any{"file": a.OutputFile, "source": d.Source, "summary": d.Summary, "errors": d.Errors}, nil
			},
		},
	}
}

// cachedEndpointExecuters reuses one executer per endpoint URL, so OAuth and
// CSRF tokens survive between test messages.
func cachedEndpointExecuters(newExe ops.EndpointExecuterFunc) ops.EndpointExecuterFunc {
	if newExe == nil {
		return nil
	}
	type entry struct {
		exe  *httpclnt.HTTPExecuter
		path string
	}
	var mu sync.Mutex
	cache := map[string]entry{}
	return func(url string) (*httpclnt.HTTPExecuter, string, error) {
		mu.Lock()
		defer mu.Unlock()
		if e, ok := cache[url]; ok {
			return e.exe, e.path, nil
		}
		exe, path, err := newExe(url)
		if err != nil {
			return nil, "", err
		}
		cache[url] = entry{exe, path}
		return exe, path, nil
	}
}

func pollOptions(cfg Config, interval, maxChecks *int) ops.Options {
	opts := ops.Options{Interval: cfg.PollInterval, MaxChecks: cfg.MaxChecks}
	if interval != nil && *interval >= 0 {
		opts.Interval = time.Duration(*interval) * time.Second
	}
	if maxChecks != nil && *maxChecks > 0 {
		opts.MaxChecks = *maxChecks
	}
	return opts
}

// decode strictly decodes tool arguments; unknown fields are usage errors so
// that a misspelt argument is never silently ignored.
func decode(raw json.RawMessage, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return output.Usagef("invalid arguments: %v", err)
	}
	return nil
}

func requireIDs(ids []string) ([]string, error) {
	var out []string
	for _, id := range ids {
		if id = strings.TrimSpace(id); id != "" {
			out = append(out, id)
		}
	}
	if len(out) == 0 {
		return nil, output.Usagef("artifact_ids must contain at least one ID")
	}
	return out, nil
}

// resolvePath resolves p (relative paths against root) and rejects paths
// outside root.
func resolvePath(root, p string) (string, error) {
	if p == "" {
		return "", output.Usagef("path is required")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	target := p
	if !filepath.IsAbs(target) {
		target = filepath.Join(absRoot, target)
	}
	target = filepath.Clean(target)
	if resolved, err := filepath.EvalSymlinks(target); err == nil {
		target = resolved
	}
	if resolvedRoot, err := filepath.EvalSymlinks(absRoot); err == nil {
		absRoot = resolvedRoot
	}
	rel, err := filepath.Rel(absRoot, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", output.Usagef("path %q is outside the server root %q", p, absRoot)
	}
	return target, nil
}

type props = map[string]any

func object(properties props, required ...string) map[string]any {
	if properties == nil {
		properties = props{}
	}
	schema := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
func boolean(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}
func integer(desc string) map[string]any {
	return map[string]any{"type": "integer", "minimum": 0, "description": desc}
}
func strArray(desc string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "minItems": 1, "description": desc}
}
func enum(desc string, values ...string) map[string]any {
	return map[string]any{"type": "string", "enum": values, "description": desc}
}

func maxBytesSchema() map[string]any {
	return map[string]any{"type": "integer", "minimum": 1, "maximum": 1048576, "description": "Maximum bytes returned, default 65536"}
}

func mustAbs(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return abs
}

func checkMaxBytes(n int) error {
	if n < 0 || n > 1048576 {
		return output.Usagef("max_bytes must be between 1 and 1048576")
	}
	return nil
}
