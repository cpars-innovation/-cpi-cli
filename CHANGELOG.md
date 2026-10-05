# Changelog

All notable changes to cpictl. Coming from FlashPipe? See
[docs/migrating-from-flashpipe.md](docs/migrating-from-flashpipe.md).

## 0.1.0 (unreleased)

First release.

- CLI for SAP Cloud Integration: inspect (`packages`, `artifacts`, `status`), designtime
  (`update artifact`, `update package`), runtime (`deploy`, `undeploy`), parameters
  (`params`, `configure`), multi-package deployments (`orchestrator`, `config-generate`),
  Git (`sync`, `snapshot`), API Management and Partner Directory commands.
- `--output json` result documents and a stable exit code contract.
- Deployments confirmed via the BuildAndDeployStatus task and a fresh runtime artifact,
  with one structured result per artifact (including designtime and runtime version).
- Runtime data: data stores (list, entries, get, delete with confirm), variables, JMS queues and
  broker, number ranges, log files (tail), idempotent repository, ID mapper (CLI and MCP).
- `set_log_level` reverts to INFO after `revert_after_minutes` and on server shutdown; trace tools
  report a missing role (403) as `status: "missing_role"` instead of an auth error.
- Build loops: `loop_start` / `loop_status` / `loop_end` with server-enforced limits
  (iterations, repeated errors, wall clock, deploys); exit code 8 / `stopped`. `cpictl mcp --mode
  discover|operate|develop`.
- Tracing across flows: `send` / `send_test_message` send a W3C `traceparent` and return the
  `traceId`; `logs tree` / `get_trace_tree` build the call tree and the first failure;
  `logs --header name=value` / `custom_header` and `--package-id` search by custom header
  (scoped, capped client-side scans). Message logs include `applicationMessageType`,
  `packageName` and `predecessorMessageGuid`.
- `configure` writes only changed parameters and redeploys only changed artifacts (`--force`,
  `--dry-run` diff, `--offline`); MCP `config_diff`. `pd deps` / `pd_dependencies` show which
  flows read which Partner Directory parameters.
- Partner Directory: `pd get` / `get_pd_parameters`, `pd diff` / `pd_diff`, and
  `pd-deploy --keys PID:ID` / `pd_deploy keys` for single-parameter changes.
- Downgrade guard: a designtime version older than the running one is not deployed unless
  `--allow-downgrade` / `allow_downgrade`.
- Message processing logs: `logs` (query, `--wait` for final status, error texts),
  `logs get` (error text, custom headers, adapter attributes, attachments, persisted messages),
  `logs steps` (failing step), `logs attachment`, `logs payload`.
- `validate` and `guidelines` (tenant check and design guidelines), `endpoints`,
  `resources`, `download`, `status --runtime-status ERROR`.
- Security material: `credentials` (list, set-user, set-oauth2, set-secure-param, declarative
  `apply`, delete) with secrets from env/file/stdin only; `keystore` (list with expiry check,
  export-cert, import-cert).
- `send` (test messages to a flow's endpoint or, through a test harness flow, to a
  ProcessDirect address; waits for the processing log), `log-level` (e.g. TRACE),
  `logs trace` / `logs trace-message` (payload and headers per step), `discover`
  (inventory of existing flows for conventions), `packages create`.
- CSRF tokens handled once per session with transparent refresh and retry.
- `cpictl mcp --read-only / --tools / --disable-tools`: limit the tools per server.
- MCP server (`cpictl mcp`) with tools for listing, status, message logs, parameters,
  upload, validation, deploy, undeploy, Partner Directory deploy and read-only security
  material.
- Claude Code plugin `cpi` (marketplace in this repository): skills cpi-discover, cpi-plan,
  cpi-build, cpi-test, cpi-review and the read-only cpi-reviewer agent; conventions live in
  each content repository under `.cpi/`.
- Profiles: `~/.cpictl/<name>.yaml`, `cpictl profile list|use|current`, `--profile`,
  `CPICTL_PROFILE`; every tenant command logs the profile and host it uses.
- Project file: `cpictl.yaml` in the repository (no secrets; home credentials are never sent
  to hosts it sets), merged over `$HOME/cpictl.yaml`.
- `CPICTL_CONFIG` selects the config file; a warning when a config file with secrets is
  readable by others.
- Settings via flags, `CPICTL_*` environment variables and `$HOME/cpictl.yaml`.
