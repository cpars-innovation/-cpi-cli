# cpictl

`cpictl` is a command line tool and MCP server for **SAP Cloud Integration (CPI)**:
build, deploy, configure and inspect integration content on a tenant from scripts,
CI/CD pipelines and AI agents.

- **One binary, two interfaces**: a regular CLI and an [MCP server](docs/mcp.md) (`cpictl mcp`)
  that share the same operations and the same safety rules.
- **Agent-friendly output**: `--output json` writes one result document to stdout,
  logs go to stderr, and [exit codes](#exit-codes) tell *why* something failed.
- **Correct deployments**: deploy waits for the build/deploy task and for the *new* runtime
  artifact, so a redeploy is never reported as successful while the old version is still running.
- **Safe by default**: Partner Directory full sync never deletes the parameters of a partner
  whose local files could not be read; destructive MCP tools need an explicit confirmation.

`cpictl` started as a fork of [FlashPipe](https://github.com/engswee/flashpipe) (Apache 2.0);
see [NOTICE](NOTICE).

---

## Contents

- [Install](#install)
- [Connect to a tenant](#connect-to-a-tenant)
- [Quick start](#quick-start)
- [Commands](#commands)
- [Output and exit codes](#output-and-exit-codes)
- [MCP server](#mcp-server)
- [Documentation](#documentation)
- [Development](#development)

## Install

Requires Go 1.25 or later.

```bash
git clone https://github.com/cpars-innovation/cpicli.git
cd cpicli
make build            # -> bin/cpictl
./bin/cpictl --version
```

Without make:

```bash
go build -ldflags "-X main.Version=$(git describe --tags --always --dirty)" -o bin/cpictl ./cmd/cpictl
```

Cross-platform builds: `make build-all` (Windows, Linux amd64/arm64, macOS).

## Connect to a tenant

Every command needs the tenant host and either OAuth client credentials (recommended)
or Basic Auth. Use environment variables for secrets:

```bash
export FLASHPIPE_TMN_HOST=mytenant.it-cpi018.cfapps.eu10-003.hana.ondemand.com
export FLASHPIPE_OAUTH_HOST=mytenant.authentication.eu10.hana.ondemand.com
export FLASHPIPE_OAUTH_CLIENTID=sb-xxxxxxxx!b1234|it!b5678
export FLASHPIPE_OAUTH_CLIENTSECRET=...
```

The same settings can be passed as flags (`--tmn-host`, `--oauth-host`, ...) or put in a
config file (`$HOME/flashpipe.yaml` or `--config file.yaml`), which can also hold defaults for
each command. See [docs/configuration.md](docs/configuration.md), including how to create the
OAuth client in SAP BTP.

> The `FLASHPIPE_` prefix and the `flashpipe.yaml` file name are kept for compatibility with
> existing FlashPipe pipelines.

## Quick start

```bash
# What is on the tenant?
cpictl packages
cpictl artifacts --package-id MyPackage

# Upload a local iFlow directory (create or update) and deploy it
cpictl update artifact --artifact-id MyIFlow --package-id MyPackage --dir-artifact ./MyIFlow
cpictl deploy --artifact-ids MyIFlow

# Check the result, read the error if it failed
cpictl status --artifact-ids MyIFlow

# Change externalised parameters, then deploy again to activate them
cpictl params set --artifact-id MyIFlow --param ReceiverHost=api.example.com
cpictl deploy --artifact-ids MyIFlow --compare-versions=false

# Remove it from runtime again
cpictl undeploy --artifact-ids MyIFlow
```

Machine-readable output for scripts:

```bash
cpictl deploy --artifact-ids A,B --output json | jq '.result.results[] | {id, status, error}'
```

## Commands

| Area | Commands |
|------|----------|
| Inspect | `packages`, `artifacts`, `status` |
| Designtime | `update artifact`, `update package` |
| Runtime | `deploy`, `undeploy` |
| Parameters | `params get`, `params set`, `configure`, `configure pull` |
| Multi-package deployments | `orchestrator`, `config-generate` |
| Git synchronisation | `sync`, `snapshot`, `snapshot restore` |
| API Management | `sync apiproxy`, `sync apiproduct` |
| Partner Directory | `pd-snapshot`, `pd-deploy` |
| AI agents | `mcp` |

Full reference with all flags: [docs/commands.md](docs/commands.md) (generated from the CLI).

## Output and exit codes

`--output text` (default) writes human-readable logs to stderr and nothing to stdout.
`--output json` writes exactly one JSON document to stdout and JSON-line logs to stderr:

```json
{
  "command": "deploy",
  "ok": false,
  "exitCode": 7,
  "error": "1 of 2 artifact(s) failed - B: FAILED (designtime artifact B does not exist)",
  "result": {
    "results": [
      {"id": "A", "type": "Integration", "taskId": "...", "status": "DEPLOYED", "version": "1.0.2"},
      {"id": "B", "type": "Integration", "status": "FAILED", "error": "designtime artifact B does not exist"}
    ]
  }
}
```

Deploy/undeploy statuses: `DEPLOYED`, `SKIPPED` (same version already running),
`UNDEPLOYED`, `NOT_DEPLOYED`, `FAILED`, `TIMEOUT`.

### Exit codes

| Code | Meaning | Typical reaction |
|------|---------|------------------|
| 0 | OK | |
| 1 | Unexpected internal error | report a bug |
| 2 | Usage or configuration error | fix flags/config |
| 3 | Authentication failed (401/403, OAuth token) | check credentials and roles |
| 4 | Tenant HTTP error or tenant unreachable | retry later / check host |
| 5 | Deployment or validation failed on the tenant | fix the artifact |
| 6 | Timeout (polling budget exhausted) | check status, increase `--max-check-limit` |
| 7 | Partial failure (some items succeeded) | inspect the per-item results |

## MCP server

`cpictl mcp` exposes the tenant to AI agents over the Model Context Protocol (stdio).
With Claude Code:

```bash
claude mcp add cpi \
  -e FLASHPIPE_TMN_HOST=... -e FLASHPIPE_OAUTH_HOST=... \
  -e FLASHPIPE_OAUTH_CLIENTID=... -e FLASHPIPE_OAUTH_CLIENTSECRET=... \
  -- /path/to/bin/cpictl mcp --root /path/to/your/integration-repo
```

Tools: `list_packages`, `list_artifacts`, `get_runtime_status`, `get_parameters`,
`set_parameters`, `upload_artifact`, `deploy`, `undeploy`, `pd_deploy`.
See [docs/mcp.md](docs/mcp.md) for the tool contract, safety rules and other clients.

## Documentation

| Document | Content |
|----------|---------|
| [docs/configuration.md](docs/configuration.md) | Connection, config file, environment variables, OAuth client setup |
| [docs/commands.md](docs/commands.md) | Generated reference of all commands and flags |
| [docs/mcp.md](docs/mcp.md) | MCP server: setup, tools, result format, safety |
| [docs/orchestrator.md](docs/orchestrator.md) | Multi-package update + deploy, `config-generate` |
| [docs/configure.md](docs/configure.md) | Parameter configuration from YAML (`configure`, `configure pull`) |
| [docs/partner-directory.md](docs/partner-directory.md) | `pd-snapshot` / `pd-deploy`, file layout, full sync |
| [docs/ci.md](docs/ci.md) | Using cpictl in GitHub Actions / Azure Pipelines |
| [docs/examples/](docs/examples) | Ready-to-copy configuration files |
| [CHANGELOG.md](CHANGELOG.md) | Changes since the FlashPipe fork |

## Development

```bash
go test ./...                    # offline, no tenant needed (httptest mock tenant)
go test -race ./...
make test-integration            # WRITES TO A REAL TENANT, needs FLASHPIPE_* env vars
```

Project layout:

```
cmd/cpictl         main package
internal/cmd       CLI commands (cobra), output contract
internal/mcp       MCP server and tool definitions
pkg/ops            operations with structured results, shared by CLI and MCP
pkg/cpi            SAP CPI OData API client
pkg/httpclnt       HTTP client (OAuth / Basic Auth, CSRF, typed HTTP errors)
internal/cpitest   in-memory mock tenant for tests
```

Golden files: `go test ./internal/cmd -update` regenerates the exit-code goldens and
`docs/commands.md`. See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

Apache License 2.0, see [LICENSE](LICENSE) and [NOTICE](NOTICE). Third-party licenses are in
[licenses/](licenses).
