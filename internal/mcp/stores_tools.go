package mcp

import (
	"context"
	"encoding/json"
	"time"

	"github.com/cpars-innovation/cpicli/internal/output"
	"github.com/cpars-innovation/cpicli/pkg/cpi"
	"github.com/cpars-innovation/cpicli/pkg/ops"
)

// storeTools are the operations tools for runtime data: data stores,
// variables, JMS, number ranges, log files, idempotent repository and ID
// mapper.
func storeTools(cfg Config, readOnly map[string]any) []Tool {
	dsProps := func(extra props) props {
		p := props{
			"data_store":  str("Data store name"),
			"artifact_id": str("Integration flow of the data store (empty for global stores)"),
			"type":        str("Data store type (only for stores of adapters or steps, e.g. XI, AS4, Aggregator)"),
		}
		for k, v := range extra {
			p[k] = v
		}
		return p
	}
	type dsArgs struct {
		DataStore  string `json:"data_store"`
		ArtifactID string `json:"artifact_id"`
		Type       string `json:"type"`
	}
	key := func(a dsArgs) cpi.DataStoreKey {
		return cpi.DataStoreKey{Name: a.DataStore, IntegrationFlow: a.ArtifactID, Type: a.Type}
	}
	stores := cpi.NewStores(cfg.Exe)
	return []Tool{
		{
			Name: "list_data_stores", Title: "List data stores",
			Description: "Data stores of the runtime with number of entries and overdue entries; overdue_only shows only stores with overdue entries. Use list_data_store_entries for the entries.",
			InputSchema: object(props{"overdue_only": boolean("Only stores with overdue entries")}),
			Annotations: readOnly,
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					OverdueOnly bool `json:"overdue_only"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				list, err := stores.DataStores(a.OverdueOnly)
				return map[string]any{"dataStores": list}, err
			},
		},
		{
			Name: "list_data_store_entries", Title: "List data store entries",
			Description: "Entries of one data store (data_store + artifact_id) or of all stores (optionally of one flow, or written by message_guid): id, status, message GUID, due and retention dates. " +
				"No content: read one with get_data_store_entry (business data).",
			InputSchema: object(dsProps(props{
				"message_guid": str("Only entries written by this message"),
				"overdue_only": boolean("Only overdue entries"),
				"top":          map[string]any{"type": "integer", "minimum": 1, "maximum": 1000, "description": "Maximum entries, default 100"},
			})),
			Annotations: readOnly,
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					dsArgs
					MessageGuid string `json:"message_guid"`
					OverdueOnly bool   `json:"overdue_only"`
					Top         int    `json:"top"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				return ops.ListDataStoreEntries(cfg.Exe, key(a.dsArgs), a.MessageGuid, a.OverdueOnly, a.Top)
			},
		},
		{
			Name: "get_data_store_entry", Title: "Read a data store entry",
			Description: "Content of one data store entry (id from list_data_store_entries). Text inline, binary base64; truncated to max_bytes.",
			InputSchema: object(dsProps(props{"id": str("Entry ID"), "max_bytes": maxBytesSchema()}), "id", "data_store"),
			Annotations: readOnly,
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					dsArgs
					ID       string `json:"id"`
					MaxBytes int    `json:"max_bytes"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				if err := checkMaxBytes(a.MaxBytes); err != nil {
					return nil, err
				}
				return ops.GetDataStoreEntry(cfg.Exe, a.ID, key(a.dsArgs), a.MaxBytes)
			},
		},
		{
			Name: "delete_data_store_entry", Title: "Delete a data store entry",
			Description: "Delete one data store entry (e.g. a stuck or test entry). Requires confirm=true.",
			InputSchema: object(dsProps(props{"id": str("Entry ID"), "confirm": boolean("Must be true: the entry is removed from the tenant")}), "id", "data_store", "confirm"),
			Annotations: map[string]any{"readOnlyHint": false, "destructiveHint": true, "idempotentHint": true, "openWorldHint": true},
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					dsArgs
					ID      string `json:"id"`
					Confirm bool   `json:"confirm"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				if !a.Confirm {
					return nil, output.Usagef("delete_data_store_entry removes %s from %s; call again with confirm=true", a.ID, a.DataStore)
				}
				return ops.DeleteDataStoreEntry(cfg.Exe, a.ID, key(a.dsArgs))
			},
		},
		{
			Name: "list_variables", Title: "List variables",
			Description: "Global variables and variables of integration flows (Write Variables step): name, flow, visibility, last update. artifact_id keeps that flow's and the global ones.",
			InputSchema: object(props{"artifact_id": str("Only this flow's variables (plus global ones)")}),
			Annotations: readOnly,
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					ArtifactID string `json:"artifact_id"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				vars, err := ops.ListVariables(cfg.Exe, a.ArtifactID)
				return map[string]any{"variables": vars}, err
			},
		},
		{
			Name: "get_variable", Title: "Read a variable",
			Description: "Value of a variable (artifact_id empty for a global variable); truncated to max_bytes.",
			InputSchema: object(props{"name": str("Variable name"), "artifact_id": str("Integration flow (empty: global variable)"), "max_bytes": maxBytesSchema()}, "name"),
			Annotations: readOnly,
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					Name       string `json:"name"`
					ArtifactID string `json:"artifact_id"`
					MaxBytes   int    `json:"max_bytes"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				if err := checkMaxBytes(a.MaxBytes); err != nil {
					return nil, err
				}
				return ops.GetVariable(cfg.Exe, a.Name, a.ArtifactID, a.MaxBytes)
			},
		},
		{
			Name: "list_jms_queues", Title: "List JMS queues",
			Description: "JMS queues with number of messages, active and exclusive flags, fullest first. Use it when a JMS-triggered flow does not process or messages pile up.",
			InputSchema: object(props{"name_prefix": str("Only queues whose name starts with this")}),
			Annotations: readOnly,
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					NamePrefix string `json:"name_prefix"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				queues, err := ops.ListQueues(cfg.Exe, a.NamePrefix)
				return map[string]any{"queues": queues}, err
			},
		},
		{
			Name: "get_jms_broker", Title: "Get JMS broker capacity",
			Description: "Capacity and usage of the JMS broker: used and maximum capacity, number of queues, queues in OK/warning/error, high consumer/producer/transaction flags.",
			InputSchema: object(nil),
			Annotations: readOnly,
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				if err := decode(raw, &struct{}{}); err != nil {
					return nil, err
				}
				return stores.Broker()
			},
		},
		{
			Name: "list_number_ranges", Title: "List number ranges",
			Description: "Number range objects with minimum, maximum and current value and rotation.",
			InputSchema: object(nil),
			Annotations: readOnly,
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				if err := decode(raw, &struct{}{}); err != nil {
					return nil, err
				}
				list, err := stores.NumberRanges()
				return map[string]any{"numberRanges": list}, err
			},
		},
		{
			Name: "list_log_files", Title: "List log files",
			Description: "System and HTTP log files of the runtime (newest first), for adapter and connection errors that never reach a message processing log. type: http, trace, ...",
			InputSchema: object(props{"type": str("Log file type, e.g. http or trace"), "since": str("Only files modified after this time (RFC 3339 or duration like 2h)")}),
			Annotations: readOnly,
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					Type  string `json:"type"`
					Since string `json:"since"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				since, err := ops.ParseTimeArg(a.Since, time.Now())
				if err != nil {
					return nil, output.Usagef("invalid since: %v", err)
				}
				return ops.ListLogFiles(cfg.Exe, a.Type, since)
			},
		},
		{
			Name: "get_log_file", Title: "Read the end of a log file",
			Description: "The last tail_bytes of a log file (name and application from list_log_files), starting at a line boundary.",
			InputSchema: object(props{
				"name":        str("Log file name"),
				"application": str("Application of the log file"),
				"tail_bytes":  map[string]any{"type": "integer", "minimum": 1, "maximum": 1048576, "description": "Bytes from the end, default 65536"},
			}, "name", "application"),
			Annotations: readOnly,
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					Name        string `json:"name"`
					Application string `json:"application"`
					TailBytes   int    `json:"tail_bytes"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				if err := checkMaxBytes(a.TailBytes); err != nil {
					return nil, err
				}
				return ops.GetLogFile(cfg.Exe, a.Name, a.Application, a.TailBytes)
			},
		},
		{
			Name: "list_idempotent_entries", Title: "List idempotent repository entries",
			Description: "Entries of the idempotent repository: message or file IDs that were processed and are ignored when they arrive again. Use it when a message or file was silently skipped as duplicate. " +
				"id is the entry (SFTP: <directory>/<file name>, XI: the message ID).",
			InputSchema: object(props{"id": str("Entry ID"), "component": str("Only this component, e.g. SFTP or XI"), "source": str("Only sources containing this text")}),
			Annotations: readOnly,
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					ID        string `json:"id"`
					Component string `json:"component"`
					Source    string `json:"source"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				entries, err := ops.ListIdempotentEntries(cfg.Exe, a.ID, a.Component, a.Source)
				return map[string]any{"entries": entries}, err
			},
		},
		{
			Name: "list_id_mappings", Title: "List ID mapper entries",
			Description: "ID mapper entries (ID Mapping step): the target IDs of source_id, or the source IDs of target_id.",
			InputSchema: object(props{"source_id": str("Source ID"), "target_id": str("Target ID")}),
			Annotations: readOnly,
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					SourceID string `json:"source_id"`
					TargetID string `json:"target_id"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				m, err := ops.ListIDMappings(cfg.Exe, a.SourceID, a.TargetID)
				return map[string]any{"mappings": m}, err
			},
		},
	}
}
