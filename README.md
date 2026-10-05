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
  local files cannot be read; destructive MCP tools need explicit confirmation; secrets are
  never taken from flags, never returned and never logged.

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

Download a binary for your platform from the
[releases](https://github.com/cpars-innovation/cpicli/releases), or install with Go 1.26 or later:

```bash
go install github.com/cpars-innovation/cpicli/cmd/cpictl@latest   # or @v0.1.0
cpictl --version
```

The binary lands in `$(go env GOPATH)/bin` (usually `~/go/bin`), which must be on your `PATH`.
While the repository is private, tell Go to fetch it directly with your Git credentials:
`go env -w GOPRIVATE=github.com/cpars-innovation/*`.

From source:

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

Working with several tenants? Put each one into a profile (`~/.cpictl/dev.yaml`, `qa.yaml`, ...)
and switch with `cpictl profile use qa` or `--profile dev`
([profiles](docs/configuration.md#profiles-switching-tenants)).

Every setting is available as a flag (`--tmn-host`), as an environment variable
(`CPICTL_TMN_HOST`) and in the config file `$HOME/cpictl.yaml` (or the file in
`CPICTL_CONFIG`, e.g. one per tenant), which can also hold defaults per command. A
`cpictl.yaml` in a repository (without secrets) sets that repository's tenant and defaults
([project file](docs/configuration.md#project-file)). Instead of
exporting variables you can put the connection into that file; keep it `chmod 600`. Details: [docs/configuration.md](docs/configuration.md).

## Everyday workflow

```bash
# Look around
cpictl packages
cpictl artifacts --package-id Orders

# Get an iFlow into a local folder, change it, upload it (only if the content differs), check and deploy
cpictl download --artifact-id OrderIntake --dir ./OrderIntake
cpictl update artifact --artifact-id OrderIntake --package-id Orders --dir-artifact ./OrderIntake
cpictl validate --artifact-id OrderIntake
cpictl deploy --artifact-ids OrderIntake

# Did it start? If not, the tenant's error message is in the output
cpictl status --artifact-ids OrderIntake

# Send a test message, wait for the processing log, find the failing step
cpictl send --artifact-id OrderIntake --body-file order.xml --content-type application/xml --wait 60s
cpictl logs steps --message-guid <guid>

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
| Inspect | `packages`, `artifacts`, `status`, `endpoints`, `resources`, `discover`, `drift` |
| Runtime data | `datastore`, `variables`, `jms`, `number-ranges`, `log-files`, `idempotent`, `id-mappings` |
| Testing and monitoring | `send`, `log-level`, `logs`, `logs get`, `logs steps`, `logs trace`, `logs trace-message`, `logs attachment`, `logs payload` |
| Quality | `validate`, `guidelines` |
| Designtime | `download`, `packages create`, `update artifact`, `update package` |
| Runtime | `deploy`, `undeploy` |
| Parameters | `params get`, `params set`, `configure`, `configure pull` |
| Many packages | `orchestrator`, `config-generate` |
| Git | `sync`, `snapshot`, `snapshot restore` |
| API Management | `sync apiproxy`, `sync apiproduct` |
| Partner Directory | `pd-snapshot`, `pd-deploy` |
| Security | `credentials` (list, set-user, set-oauth2, set-secure-param, apply, delete), `keystore` (list, export-cert, import-cert) |
| AI agents | `mcp` |

All commands and flags: [docs/commands.md](docs/commands.md) (generated from the CLI).

## Output and exit codes

By default cpictl writes human-readable logs to stderr and nothing to stdout (except
commands that download content, such as `resources get`, which print the content).
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
| 5 | Deployment, validation or test message failed on the tenant | fix the artifact |
| 6 | Timeout | check `status`, raise `--max-check-limit` |
| 7 | Partial failure | inspect the per-item results |
| 8 | Loop stopped (MCP only): a build loop limit was reached | stop, report the loop summary |

## AI agents (MCP)

`cpictl mcp` is a [Model Context Protocol](https://modelcontextprotocol.io) server on stdio.
Add it to Claude Code:

```bash
claude mcp add cpi \
  -e CPICTL_TMN_HOST=... -e CPICTL_OAUTH_HOST=... \
  -e CPICTL_OAUTH_CLIENTID=... -e CPICTL_OAUTH_CLIENTSECRET=... \
  -- /path/to/bin/cpictl mcp --root /path/to/integration-repo
```

Or share the setup with your team: commit a `.mcp.json` to the root of the integration content
repository. It contains no secrets; Claude Code fills in `${VAR}` from each developer's
environment:

```json
{
  "mcpServers": {
    "cpi-dev": {
      "command": "${CPICTL_BIN:-cpictl}",
      "args": ["mcp", "--root", "."],
      "env": {
        "CPICTL_TMN_HOST": "${CPI_DEV_TMN_HOST}",
        "CPICTL_OAUTH_HOST": "${CPI_DEV_OAUTH_HOST}",
        "CPICTL_OAUTH_CLIENTID": "${CPI_DEV_OAUTH_CLIENTID}",
        "CPICTL_OAUTH_CLIENTSECRET": "${CPI_DEV_OAUTH_CLIENTSECRET}",
        "CPICTL_RUNTIME_OAUTH_CLIENTID": "${CPI_DEV_RUNTIME_OAUTH_CLIENTID}",
        "CPICTL_RUNTIME_OAUTH_CLIENTSECRET": "${CPI_DEV_RUNTIME_OAUTH_CLIENTSECRET}"
      }
    }
  }
}
```

Limit what agents may do per server with `--read-only`, `--tools list_*,get_*` or
`--disable-tools undeploy,pd_deploy` ([details](docs/mcp.md#limiting-tools)).
Full example with a read-only QA tenant: [docs/examples/claude-code.mcp.json](docs/examples/claude-code.mcp.json);
for other clients (Claude Desktop, Cursor, ...): [docs/examples/mcp.json](docs/examples/mcp.json).
The runtime credentials are only needed for `send_test_message`
([configuration.md](docs/configuration.md#runtime-endpoints-test-messages)).

Tools cover the whole loop: `create_package`, `download_artifact`, `upload_artifact`,
`validate_artifact`, `check_guidelines`, `deploy`, `send_test_message`, `get_runtime_status`,
`list_message_logs`, `get_message_log`, `get_message_steps`, `get_message_attachment`,
`get_message_store_entry`, parameters, resources, `discover_tenant`, `list_credentials`,
`list_keystore`, `undeploy` and `pd_deploy`. Security material is read-only over MCP; secrets never pass
through the agent.
Every result carries `ok`, an `errorCategory` matching the exit codes, and the structured result.
See [docs/mcp.md](docs/mcp.md).

### Claude Code plugin

The repository is also a Claude Code plugin marketplace. The plugin `cpi` adds the MCP server,
skills to discover your tenant's conventions and to plan, build, test and review flows, and a
read-only reviewer agent:

```text
/plugin marketplace add cpars-innovation/cpicli
/plugin install cpi@cpicli
```

Then run the `cpi-discover` skill once per repository. See [docs/plugin.md](docs/plugin.md).

## Documentation

| | |
|---|---|
| [Configuration](docs/configuration.md) | Connection, config file, environment variables, OAuth client |
| [Command reference](docs/commands.md) | All commands and flags |
| [MCP server](docs/mcp.md) | Agent setup, tools, result format, safety |
| [Claude Code plugin](docs/plugin.md) | Skills, reviewer agent, tenant conventions |
| [Orchestrator](docs/orchestrator.md) | Update + deploy many packages, `config-generate` |
| [Testing](docs/testing.md) | Test messages by trigger type, test harness, test entries, tracing |
| [Monitoring and checks](docs/monitoring.md) | Message logs, steps, attachments, payloads, validation, guidelines |
| [Configure](docs/configure.md) | Parameters from YAML (`configure`, `configure pull`) |
| [Security material](docs/security.md) | Credentials, `credentials apply`, keystore and certificate expiry |
| [Partner Directory](docs/partner-directory.md) | `pd-snapshot`, `pd-deploy`, full sync |
| [CI/CD](docs/ci.md) | GitHub Actions, Azure Pipelines, scripting with exit codes |
| [Examples](docs/examples) | Ready-to-copy configuration files |
| [Coming from FlashPipe](docs/migrating-from-flashpipe.md) | What changed and how to migrate |

## Development

```bash
go test ./...               # offline against an in-memory mock tenant, a few seconds
go test -race ./...
make cover                  # cross-package coverage report
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
