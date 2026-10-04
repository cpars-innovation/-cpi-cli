# MCP server

`cpictl mcp` runs a [Model Context Protocol](https://modelcontextprotocol.io) server on
stdin/stdout so that AI agents can work with a CPI tenant: list content, upload an iFlow,
deploy it, read the runtime error, fix, repeat.

- Transport: stdio, JSON-RPC 2.0, one message per line.
  Protocol versions `2025-06-18`, `2025-03-26`, `2024-11-05`.
- stdout carries only protocol messages. Logs are JSON lines on stderr.
- Tenant settings are the same as for the CLI (flags, `CPICTL_*` environment variables,
  `cpictl.yaml`), see [configuration.md](configuration.md). One server = one tenant.

## Setup

### Claude Code

```bash
claude mcp add cpi \
  -e CPICTL_TMN_HOST=mytenant.it-cpi018.cfapps.eu10-003.hana.ondemand.com \
  -e CPICTL_OAUTH_HOST=mytenant.authentication.eu10.hana.ondemand.com \
  -e CPICTL_OAUTH_CLIENTID=... \
  -e CPICTL_OAUTH_CLIENTSECRET=... \
  -- /path/to/bin/cpictl mcp --root /path/to/integration-repo
```

### Other clients (Claude Desktop, Cursor, ...)

See [examples/mcp.json](examples/mcp.json):

```json
{
  "mcpServers": {
    "cpi-dev": {
      "command": "/path/to/bin/cpictl",
      "args": ["mcp", "--root", "/path/to/integration-repo"],
      "env": {
        "CPICTL_TMN_HOST": "mytenant.it-cpi018.cfapps.eu10-003.hana.ondemand.com",
        "CPICTL_OAUTH_HOST": "mytenant.authentication.eu10.hana.ondemand.com",
        "CPICTL_OAUTH_CLIENTID": "...",
        "CPICTL_OAUTH_CLIENTSECRET": "..."
      }
    }
  }
}
```

Use one server entry per tenant (e.g. `cpi-dev`, `cpi-qa`). Point agents at a development
tenant; there is no read-only mode yet.

### Server flags

| Flag | Default | Description |
|------|---------|-------------|
| `--root` | `.` | Local paths in tool arguments are resolved against this directory and may not leave it (symlinks are resolved) |
| `--poll-interval` | `10` | Default seconds between deploy/undeploy status checks |
| `--max-checks` | `30` | Default maximum number of status checks per artifact |

## Tools

| Tool | Changes | Purpose |
|------|---------|---------|
| `list_packages` | | All integration packages |
| `list_artifacts` | | Designtime artifacts of a package (all four types) |
| `list_resources` | | Scripts, mappings, schemas, ... of an integration flow |
| `get_resource` | | Content of one resource (text inline, binary base64) |
| `download_artifact` | local files | Extract an artifact into a directory inside `--root` (empty unless `overwrite`) |
| `upload_artifact` | designtime | Create or update an artifact from a local directory; `CREATED`, `UPDATED` or `UNCHANGED` |
| `validate_artifact` | | Tenant check of an integration flow (like *Check* in the Web UI); `PASSED` / `FAILED` with details |
| `check_guidelines` | | Run the activated design guidelines and wait; violations with violated components |
| `get_parameters` | | Externalised parameters of an integration flow |
| `set_parameters` | designtime | Change parameters; only changed values are written, unknown keys fail first; `dry_run` |
| `deploy` | runtime | Deploy and wait; per artifact `DEPLOYED`, `SKIPPED`, `FAILED` (tenant error), `TIMEOUT` |
| `get_runtime_status` | | Runtime status, version, deployment time and error of given artifacts |
| `list_runtime_artifacts` | | All deployed artifacts, filter by status (e.g. `ERROR`) |
| `list_service_endpoints` | | Callable URLs of deployed integration flows (where to send test messages) |
| `list_message_logs` | | Message processing logs by artifact, status, time, IDs; error texts; `wait_seconds` for final status |
| `get_message_log` | | One message: error text, custom header properties, adapter attributes, attachments, persisted messages |
| `get_message_steps` | | Processing steps of a message and the first failing step (`modelStepId`) |
| `get_message_attachment` | | Content of a log attachment |
| `get_message_store_entry` | | Payload persisted by a Persist step |
| `list_credentials` | | User credentials, OAuth2 client credentials, secure parameters: names and metadata, never secrets |
| `list_keystore` | | Keystore entries with validity and days left; `expiring_within_days` flags soon-expiring ones |
| `undeploy` | runtime, **destructive** | Remove from runtime and wait; requires `confirm: true` |
| `pd_deploy` | Partner Directory, **destructive with full_sync** | Upload Partner Directory parameters; dry run unless `dry_run: false` |

Content tools (`get_resource`, `get_message_attachment`, `get_message_store_entry`) return
`{size, text | base64, truncated}`; `max_bytes` (default 64 KB, max 1 MB) limits the size.

Every tool has a JSON schema with `additionalProperties: false`: a misspelt argument is an
error, never silently ignored. `tools/list` returns the schemas and MCP annotations
(`readOnlyHint`, `destructiveHint`).

`deploy` always deploys by default (`compare_versions: false`), unlike the CLI. After
`set_parameters` the version does not change, so skipping by version would leave the old
configuration running.

## Result format

Every tool call returns the same structure, both as `structuredContent` and as JSON text:

```json
{
  "ok": false,
  "errorCategory": "failed",
  "exitCode": 5,
  "error": "1 of 1 artifact(s) failed - MyIFlow: FAILED (artifact MyIFlow deployment unsuccessful, ended with status ERROR. Error message = ...)",
  "result": {
    "results": [
      {"id": "MyIFlow", "type": "Integration", "taskId": "...", "status": "FAILED", "version": "1.0.3", "error": "..."}
    ]
  }
}
```

`errorCategory` mirrors the CLI exit codes and tells the agent what to do next:

| Category | Exit code | Meaning for the agent |
|----------|-----------|-----------------------|
| `usage` | 2 | Arguments are wrong: fix the call (also returned when `confirm` is missing) |
| `auth` | 3 | Credentials or roles: stop and ask the user |
| `tenant_http` | 4 | Tenant error or unreachable: retry later |
| `failed` | 5 | The tenant rejected the content: read `error`, fix the artifact |
| `timeout` | 6 | Not finished in time: check `get_runtime_status` |
| `partial` | 7 | Some items failed: look at the per-item results |

Tool failures are returned as tool results with `isError: true`, not as JSON-RPC errors,
so the model sees them. JSON-RPC errors are only used for protocol problems (unknown tool,
malformed request).

## Typical agent loop

1. `download_artifact` once to get the iFlow into the local repository (inside `--root`),
   or start from files that are already there.
2. Edit the files; `upload_artifact`; `validate_artifact` (and `check_guidelines`).
3. `deploy`; on `failed`, read `error` (the tenant's runtime error), fix and go back to 2.
4. `list_service_endpoints` for the URL; note the time and send a test message.
5. `list_message_logs` with `artifact_id`, `since` = that time and `wait_seconds` (e.g. 60).
   For a `FAILED` message the result contains `errorText`.
6. `get_message_steps` shows the failing step (`modelStepId`, as in the `.iflw` BPMN);
   `get_message_log` lists custom headers, attachments and persisted messages, which
   `get_message_attachment` / `get_message_store_entry` download. Fix and go back to 2.
7. `set_parameters` + `deploy` to change externalised configuration.

Message logs, attachments and persisted messages contain business data. Error texts are
truncated (4 KB in lists, 16 KB in `get_message_log`, flagged with `errorTruncated`) to keep
tool results small. Use a development tenant with test data for agents.

## Safety

- `undeploy` refuses to run without `confirm: true`.
- `pd_deploy` is a dry run unless `dry_run: false`; full sync never deletes the parameters
  of a partner whose local files could not be read.
- Local paths cannot escape `--root`.
- Long-running calls are cancelled on `notifications/cancelled` and when the server stops.
- Credentials are never part of tool results or logs. Security material is read-only: there
  is no tool that creates or changes credentials or keys, so secrets never pass through the
  model. Deploy them with `cpictl credentials` ([security.md](security.md)).
