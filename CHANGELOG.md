# Changelog

## Unreleased: cpictl (fork of FlashPipe 3.7.0 via PaulNetze/flashpipe)

### Added
- `--output json` for every command: one result document on stdout, JSON-line logs on stderr.
- Exit code contract: 0 ok, 1 internal, 2 usage/config, 3 auth, 4 tenant HTTP, 5 deploy/validate
  failed, 6 timeout, 7 partial failure.
- `undeploy`, `status`, `packages`, `artifacts`, `params get`, `params set` commands.
- `mcp`: Model Context Protocol server exposing CPI tools to AI agents.
- Deploy results per artifact: `{id, type, taskId, status, version, error}`.
- `pkg/cpi` (API client) and `pkg/ops` (operations) shared by CLI and MCP.

### Fixed
- Deploy no longer reports success for a same-version redeploy while the previous deployment
  is still visible: it polls the BuildAndDeployStatus task, then requires a runtime artifact
  newer than the one before the trigger. A stale ERROR is no longer reported as failure.
- Partner Directory full sync no longer deletes all remote parameters of a partner whose local
  files could not be read; unreadable binary files are no longer treated as deleted.
- `pd-deploy` exits non-zero when parameters fail.
- `config-generate` kept `sync`/`deploy` defaults, the `orchestrator` section and package
  metadata (all were lost or wrong before).
- `deploy.artifactIds` from the config file works (was blocked by the required-flag check).
- Credentials (host, client ID, user) are no longer logged at info level.
- Data race when several deployments log concurrently.

### Changed
- Binary `cpictl`, module `github.com/cpars-innovation/cpicli`, version set at build time.
- configure and orchestrator deploy package by package (config order) with
  `--parallel-deployments` per package, through the same deploy implementation as `deploy`.
- `config-generate --output` is now `--output-file` (old usage still accepted, deprecated).
- `--tmn-host` accepts `https://` and `:port`.
- Tenant integration tests require `-tags integration`; `go test ./...` runs offline.

### Removed
- Usage analytics (Matomo) and the upstream release workflows.
- Orchestrator: local `--debug` flag (use the global one), unreachable remote-config
  credential handling, references to a non-existent `--orchestrator-config` flag.
- Unused Partner Directory `$batch` code and other dead helpers.
- FlashPipe documentation site, screenshots and examples (replaced by `docs/`).
