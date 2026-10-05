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
  with one structured result per artifact.
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
- Settings via flags, `CPICTL_*` environment variables and `$HOME/cpictl.yaml`.
