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

| Tool | Changes the tenant | Purpose |
|------|--------------------|---------|
| `list_packages` | no | All integration packages |
| `list_artifacts` | no | Designtime artifacts of a package (all four types) |
| `get_runtime_status` | no | Runtime status, version, deployment time; error message for artifacts in ERROR |
| `list_message_logs` | no | Message processing logs by artifact, status, time window, correlation/application ID; error text of failed messages included; `wait_seconds` waits until the messages are final |
| `get_message_log` | no | One message: status, full error text, custom header properties, adapter attributes, attachment list |
| `get_parameters` | no | Externalised parameters of an integration flow |
| `set_parameters` | designtime | Change parameters; only changed values are written, unknown keys fail before anything is written; `dry_run` available |
| `upload_artifact` | designtime | Create or update an artifact from a local directory; reports `CREATED`, `UPDATED` or `UNCHANGED` |
| `deploy` | runtime | Deploy and wait; per-artifact `DEPLOYED`, `SKIPPED`, `FAILED` (with tenant error), `TIMEOUT` |
| `undeploy` | runtime, **destructive** | Remove from runtime and wait; requires `confirm: true` |
| `pd_deploy` | Partner Directory, **destructive with full_sync** | Upload Partner Directory parameters; dry run unless `dry_run: false` |

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

1. `list_artifacts` / `get_runtime_status` to understand the current state.
2. Edit files of the iFlow in the local repository (inside `--root`).
3. `upload_artifact` with the artifact directory.
4. `deploy`; on `failed`, read `error` (the tenant's runtime error), fix and go back to 3.
5. Note the current time, send a test message to the iFlow endpoint, then
   `list_message_logs` with `artifact_id`, `since` = that time and `wait_seconds` (e.g. 60).
   For a `FAILED` message the result contains `errorText`; `get_message_log` adds custom
   header properties and attachments. Fix and go back to 3.
6. `set_parameters` + `deploy` to change externalised configuration.

Message logs contain business data (headers, error texts with payload fragments). Error texts
are truncated (4 KB in lists, 16 KB in `get_message_log`, flagged with `errorTruncated`) to
keep tool results small.

## Safety

- `undeploy` refuses to run without `confirm: true`.
- `pd_deploy` is a dry run unless `dry_run: false`; full sync never deletes the parameters
  of a partner whose local files could not be read.
- Local paths cannot escape `--root`.
- Long-running calls are cancelled on `notifications/cancelled` and when the server stops.
- Credentials are never part of tool results or logs.
