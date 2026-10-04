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
- Message processing logs: `logs` (query, `--wait` for final status, error texts) and
  `logs get` (error text, custom headers, adapter attributes, attachments).
- MCP server (`cpictl mcp`) with tools for listing, status, message logs, parameters,
  upload, deploy, undeploy and Partner Directory deploy.
- Settings via flags, `CPICTL_*` environment variables and `$HOME/cpictl.yaml`.
