# cpictl

**Build, deploy and operate SAP Cloud Integration (CPI) content from the command line,
CI/CD pipelines and AI agents.**

```bash
cpictl update artifact --artifact-id OrderIntake --package-id Orders --dir-artifact ./OrderIntake
cpictl deploy --artifact-ids OrderIntake
cpictl status --artifact-ids OrderIntake --output json
```

- **CLI and MCP server in one binary.** `cpictl mcp` gives AI agents the same operations,
  with the same safety rules, as the command line.
- **Machine-readable by design.** `--output json` returns one result document on stdout,
  logs go to stderr, and the exit code says *why* something failed.
- **Deployments you can trust.** `deploy` follows the tenant's build/deploy task and waits
  for the *new* runtime artifact. A redeploy is never reported as done while the previous
  version is still running.
- **Safe defaults.** Partner Directory full sync never deletes a partner's parameters when its
  local files cannot be read; destructive MCP tools need explicit confirmation.

## Contents

- [Install](#install)
- [Connect to a tenant](#connect-to-a-tenant)
- [Everyday workflow](#everyday-workflow)
- [Commands](#commands)
- [Output and exit codes](#output-and-exit-codes)
- [AI agents (MCP)](#ai-agents-mcp)
- [Documentation](#documentation)
- [Development](#development)

## Install

Requires Go 1.26 or later.

```bash
git clone https://github.com/cpars-innovation/cpicli.git
cd cpicli
make build                 # -> bin/cpictl
./bin/cpictl --version
```

`make build-all` cross-compiles for Windows, Linux (amd64, arm64) and macOS.

## Connect to a tenant

Create an OAuth client for the Cloud Integration API in SAP BTP
([how](docs/configuration.md#creating-an-oauth-client-in-sap-btp)) and export its values:

```bash
export CPICTL_TMN_HOST=mytenant.it-cpi018.cfapps.eu10-003.hana.ondemand.com
export CPICTL_OAUTH_HOST=mytenant.authentication.eu10.hana.ondemand.com
export CPICTL_OAUTH_CLIENTID='sb-xxxxxxxx!b1234|it!b5678'
export CPICTL_OAUTH_CLIENTSECRET='...'
```

Every setting is available as a flag (`--tmn-host`), as an environment variable
(`CPICTL_TMN_HOST`) and in the config file `$HOME/cpictl.yaml`, which can also hold
defaults per command. Details: [docs/configuration.md](docs/configuration.md).

## Everyday workflow

```bash
# Look around
cpictl packages
cpictl artifacts --package-id Orders

# Change an iFlow locally, upload it (created or updated only if the content differs) and deploy
cpictl update artifact --artifact-id OrderIntake --package-id Orders --dir-artifact ./OrderIntake
cpictl deploy --artifact-ids OrderIntake

# Did it start? If not, the tenant's error message is in the output
cpictl status --artifact-ids OrderIntake

# Send a test message, then wait for its processing log and read the error if it failed
cpictl logs --artifact-id OrderIntake --since 2m --wait 60s --errors
cpictl logs get --message-guid <guid>

# Change externalised parameters and activate them
cpictl params set --artifact-id OrderIntake --param ReceiverHost=orders.example.com
cpictl deploy --artifact-ids OrderIntake --compare-versions=false

# Take it off the runtime again
cpictl undeploy --artifact-ids OrderIntake
```

For many packages at once, use [`orchestrator`](docs/orchestrator.md) (update + deploy from a
directory tree) or [`configure`](docs/configure.md) (parameters from YAML per environment).

## Commands

| Area | Commands |
|------|----------|
| Inspect | `packages`, `artifacts`, `status` |
| Monitoring | `logs`, `logs get` (message processing logs) |
| Designtime | `update artifact`, `update package` |
| Runtime | `deploy`, `undeploy` |
| Parameters | `params get`, `params set`, `configure`, `configure pull` |
| Many packages | `orchestrator`, `config-generate` |
| Git | `sync`, `snapshot`, `snapshot restore` |
| API Management | `sync apiproxy`, `sync apiproduct` |
| Partner Directory | `pd-snapshot`, `pd-deploy` |
| AI agents | `mcp` |

All commands and flags: [docs/commands.md](docs/commands.md) (generated from the CLI).

## Output and exit codes

By default cpictl writes human-readable logs to stderr and nothing to stdout.
With `--output json` stdout receives exactly one JSON document and stderr receives JSON log lines:

```json
{
  "command": "deploy",
  "ok": false,
  "exitCode": 7,
  "error": "1 of 2 artifact(s) failed - Billing: FAILED (designtime artifact Billing does not exist)",
  "result": {
    "results": [
      {"id": "OrderIntake", "type": "Integration", "taskId": "...", "status": "DEPLOYED", "version": "1.0.2"},
      {"id": "Billing", "type": "Integration", "status": "FAILED", "error": "designtime artifact Billing does not exist"}
    ]
  }
}
```

Artifact statuses: `DEPLOYED`, `SKIPPED` (same version already running), `UNDEPLOYED`,
`NOT_DEPLOYED`, `FAILED`, `TIMEOUT`.

| Exit code | Meaning | Typical reaction |
|-----------|---------|------------------|
| 0 | OK | |
| 1 | Unexpected internal error | report a bug |
| 2 | Usage or configuration error | fix flags or config |
| 3 | Authentication failed (401/403, OAuth token) | check credentials and roles |
| 4 | Tenant HTTP error or tenant unreachable | retry, check host |
| 5 | Deployment or validation failed on the tenant | fix the artifact |
| 6 | Timeout | check `status`, raise `--max-check-limit` |
| 7 | Partial failure | inspect the per-item results |

## AI agents (MCP)

`cpictl mcp` is a [Model Context Protocol](https://modelcontextprotocol.io) server on stdio.
Add it to Claude Code:

```bash
claude mcp add cpi \
  -e CPICTL_TMN_HOST=... -e CPICTL_OAUTH_HOST=... \
  -e CPICTL_OAUTH_CLIENTID=... -e CPICTL_OAUTH_CLIENTSECRET=... \
  -- /path/to/bin/cpictl mcp --root /path/to/integration-repo
```

Tools: `list_packages`, `list_artifacts`, `get_runtime_status`, `list_message_logs`,
`get_message_log`, `get_parameters`, `set_parameters`, `upload_artifact`, `deploy`,
`undeploy`, `pd_deploy`.
Every result carries `ok`, an `errorCategory` matching the exit codes, and the structured result.
See [docs/mcp.md](docs/mcp.md).

## Documentation

| | |
|---|---|
| [Configuration](docs/configuration.md) | Connection, config file, environment variables, OAuth client |
| [Command reference](docs/commands.md) | All commands and flags |
| [MCP server](docs/mcp.md) | Agent setup, tools, result format, safety |
| [Orchestrator](docs/orchestrator.md) | Update + deploy many packages, `config-generate` |
| [Configure](docs/configure.md) | Parameters from YAML (`configure`, `configure pull`) |
| [Partner Directory](docs/partner-directory.md) | `pd-snapshot`, `pd-deploy`, full sync |
| [CI/CD](docs/ci.md) | GitHub Actions, Azure Pipelines, scripting with exit codes |
| [Examples](docs/examples) | Ready-to-copy configuration files |
| [Coming from FlashPipe](docs/migrating-from-flashpipe.md) | What changed and how to migrate |

## Development

```bash
go test ./...               # offline against an in-memory mock tenant, a few seconds
go test -race ./...
make test-integration       # writes to a REAL tenant, needs CPICTL_* variables
```

```
cmd/cpictl         main package
internal/cmd       CLI commands, output contract
internal/mcp       MCP server and tools
pkg/ops            operations with structured results (shared by CLI and MCP)
pkg/cpi            SAP Cloud Integration OData API client
pkg/httpclnt       HTTP client: OAuth / Basic Auth, CSRF, typed errors
internal/cpitest   mock tenant for tests
```

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

Apache License 2.0, see [LICENSE](LICENSE). cpictl contains code originally developed as
[FlashPipe](https://github.com/engswee/flashpipe) by Eng Swee Yeoh; see [NOTICE](NOTICE).
Third-party licenses: [licenses/](licenses).
