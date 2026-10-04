package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/cpars-innovation/cpicli/internal/output"
	"github.com/cpars-innovation/cpicli/internal/repo"
	"github.com/cpars-innovation/cpicli/pkg/cpi"
	"github.com/cpars-innovation/cpicli/pkg/httpclnt"
	"github.com/cpars-innovation/cpicli/pkg/ops"
)

// Instructions is sent to the client on initialize.
const Instructions = `Tools for SAP Cloud Integration (CPI) on one tenant.
Typical loop: download_artifact (once) -> edit files -> upload_artifact -> validate_artifact
-> deploy -> get_runtime_status; list_service_endpoints gives the URL for a test message;
after sending it, list_message_logs (since=send time, wait_seconds), then get_message_log,
get_message_steps (failing step) and get_message_attachment / get_message_store_entry
for payloads; fix the local files and repeat. check_guidelines reports design issues. set_parameters changes
externalised parameters; deploy afterwards to activate them.
Every result has {ok, errorCategory, exitCode, error, result}; errorCategory is one of
usage (fix the arguments), auth, tenant_http, failed, timeout, partial.
undeploy requires confirm=true. pd_deploy is a dry run unless dry_run=false.`

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

	return []Tool{
		{
			Name: "list_packages", Title: "List integration packages",
			Description: "List all integration packages on the tenant.",
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
			Description: "List designtime artifacts (integration flows, mappings, script collections, value mappings) of a package.",
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
			Description: "Runtime status (STARTED, STARTING, ERROR or not deployed), version and deployment time of artifacts. Includes the error message for artifacts in ERROR.",
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
				"To test an iFlow: note the time, send the test message, then call with artifact_id, since=<that time> and wait_seconds to wait until the message reached a final status.",
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
			}),
			Annotations: readOnly,
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					ArtifactID           string   `json:"artifact_id"`
					Statuses             []string `json:"statuses"`
					Since                string   `json:"since"`
					Until                string   `json:"until"`
					CorrelationID        string   `json:"correlation_id"`
					ApplicationMessageID string   `json:"application_message_id"`
					Top                  int      `json:"top"`
					Skip                 int      `json:"skip"`
					IncludeErrors        *bool    `json:"include_errors"`
					WaitSeconds          int      `json:"wait_seconds"`
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
			Description: "Details of one message: status, full error text, custom header properties, adapter attributes and attachment list.",
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
			Description: "Step-level trace of a message (runs and steps with status and error) and the first failing step. modelStepId identifies the step in the iFlow model (BPMN).",
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
			Description: "Content of a log attachment (attachment id from get_message_log). Text is returned inline, binary as base64; truncated to max_bytes.",
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
			Description: "Payload persisted by a Persist step (entry id from messageStoreEntries of get_message_log). Truncated to max_bytes.",
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
			Name: "list_runtime_artifacts", Title: "List deployed artifacts",
			Description: "All deployed artifacts with status, version and deployment time; filter by runtime status (e.g. [\"ERROR\"] to find broken deployments, with their error message).",
			InputSchema: object(props{"statuses": map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": []string{"STARTED", "STARTING", "ERROR", "STOPPING"}}}}),
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
			Description: "Callable URLs of deployed integration flows (where to send test messages).",
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
			Description: "Run the tenant's check of an integration flow (like Check in the Web UI) before deploying. status PASSED or FAILED with details; FAILED is errorCategory failed.",
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
			Description: "Run the design guidelines activated on the tenant against an integration flow and wait for the result; returns violations (not compliant, not skipped) with the violated components.",
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
			Description: "Scripts, mappings, schemas and other resources contained in an integration flow (name and type).",
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
			Description: "Content of one resource (e.g. a Groovy script or XSLT) from the tenant; text inline, binary as base64.",
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
			Description: "Download a designtime artifact from the tenant and extract it into a local directory (inside the server root) to edit it; upload it again with upload_artifact. The directory must be empty unless overwrite=true.",
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
			Name: "get_parameters", Title: "Get configuration parameters",
			Description: "Externalised configuration parameters of an integration flow.",
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
			Description: "Set externalised parameters of an integration flow. Only changed values are written; unknown keys fail the call before anything is written. Deploy afterwards to activate.",
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
			Name: "upload_artifact", Title: "Upload artifact from a local directory",
			Description: "Create the designtime artifact or update it when the local content differs (action CREATED, UPDATED or UNCHANGED). The directory must contain META-INF/MANIFEST.MF and src/main/resources. Does not deploy.",
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
			Description: "Deploy designtime artifacts to runtime and wait for the outcome. Returns one result per artifact: status DEPLOYED, SKIPPED, FAILED (with the tenant error message) or TIMEOUT. A redeploy is only reported as DEPLOYED once the runtime shows the new deployment.",
			InputSchema: object(props{
				"artifact_ids":          strArray("Artifact IDs"),
				"artifact_type":         enum(`Artifact type, default "Integration"`, cpi.ArtifactTypes...),
				"compare_versions":      boolean("Skip artifacts whose version is already running (default false: always deploy)"),
				"poll_interval_seconds": integer("Seconds between status checks"),
				"max_checks":            integer("Maximum number of status checks per artifact"),
			}, "artifact_ids"),
			Annotations: map[string]any{"readOnlyHint": false, "destructiveHint": false, "idempotentHint": true, "openWorldHint": true},
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					ArtifactIDs     []string `json:"artifact_ids"`
					ArtifactType    string   `json:"artifact_type"`
					CompareVersions bool     `json:"compare_versions"`
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
				results := ops.Deploy(ctx, tenant, artifacts, opts)
				return map[string]any{"results": results}, ops.Err(results)
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
			Description: "Upload Partner Directory parameters from a local directory ({PID}/String.properties, {PID}/Binary/). Runs as a dry run unless dry_run=false. full_sync deletes remote parameters of the managed PIDs that do not exist locally.",
			InputSchema: object(props{
				"resources_path": str("Local partner directory root, relative to the server root"),
				"pids":           strArray("Restrict to these partner IDs"),
				"replace":        boolean("Update existing parameters whose value differs (default true)"),
				"full_sync":      boolean("Delete remote parameters not present locally (default false)"),
				"dry_run":        boolean("Only report what would change (default true)"),
			}, "resources_path"),
			Annotations: map[string]any{"readOnlyHint": false, "destructiveHint": true, "idempotentHint": true, "openWorldHint": true},
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					ResourcesPath string   `json:"resources_path"`
					PIDs          []string `json:"pids"`
					Replace       *bool    `json:"replace"`
					FullSync      bool     `json:"full_sync"`
					DryRun        *bool    `json:"dry_run"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				dir, err := resolvePath(cfg.Root, a.ResourcesPath)
				if err != nil {
					return nil, err
				}
				opts := ops.PDDeployOptions{Replace: true, FullSync: a.FullSync, DryRun: true, PIDs: a.PIDs}
				if a.Replace != nil {
					opts.Replace = *a.Replace
				}
				if a.DryRun != nil {
					opts.DryRun = *a.DryRun
				}
				return ops.PDDeploy(cpi.NewPartnerDirectory(cfg.Exe), repo.NewPartnerDirectory(dir), opts)
			},
		},
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
