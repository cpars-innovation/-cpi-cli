# cpictl

`cpictl` is a fork of [FlashPipe](https://github.com/engswee/flashpipe) maintained by cpars innovation.
It keeps FlashPipe's commands and configuration (`FLASHPIPE_*` environment variables, `flashpipe.yaml`)
and adds safety fixes and agent-friendly output. The original FlashPipe README follows.

Build: `make build` (binary in `bin/cpictl`), or
`go build -ldflags "-X main.Version=$(git describe --tags --always)" -o bin/cpictl ./cmd/cpictl`.

### Agent / CI output contract

Every command accepts `--output text|json` (default `text`, env `FLASHPIPE_OUTPUT`).

- `--output json`: exactly one JSON document on **stdout**
  (`{"command", "ok", "exitCode", "error", "result"}`), logs on **stderr** as JSON lines.
- `--output text`: human-readable logs on stderr, nothing on stdout.
- `deploy` / `undeploy` results contain one entry per artifact:
  `{"id", "type", "taskId", "status", "version", "error"}` with status
  `DEPLOYED | SKIPPED | UNDEPLOYED | NOT_DEPLOYED | FAILED | TIMEOUT`.

| Exit code | Meaning |
|-----------|---------|
| 0 | OK |
| 1 | Unexpected internal error |
| 2 | Usage or configuration error |
| 3 | Authentication failed (401/403, OAuth token) |
| 4 | Tenant HTTP error or tenant unreachable |
| 5 | Deployment / validation failed |
| 6 | Timeout (polling budget exhausted) |
| 7 | Partial failure (some items succeeded) |

### Commands added in cpictl

| Command | Purpose |
|---------|---------|
| `undeploy --artifact-ids A,B` | Remove runtime artifacts and wait until they are gone |
| `status --artifact-ids A,B` | Runtime status, version, deployedOn, error message |
| `packages` / `artifacts --package-id P` | List packages / designtime artifacts |
| `params get --artifact-id A` / `params set --artifact-id A --param k=v` | Read / change externalised parameters |
| `mcp` | MCP server for AI agents (see below) |

### MCP server

`cpictl mcp` runs a Model Context Protocol server on stdio. It uses the same tenant
settings as the CLI (flags, `FLASHPIPE_*` env vars, `flashpipe.yaml`); stdout carries only
the protocol, logs go to stderr as JSON lines. Local paths are confined to `--root`.

Tools: `list_packages`, `list_artifacts`, `get_runtime_status`, `get_parameters`,
`set_parameters`, `upload_artifact`, `deploy`, `undeploy` (requires `confirm: true`),
`pd_deploy` (dry run unless `dry_run: false`). Every tool result has
`{ok, errorCategory, exitCode, error, result}` with the same categories as the exit codes.

Claude Code: `claude mcp add cpi -e FLASHPIPE_TMN_HOST=... -e FLASHPIPE_OAUTH_HOST=... -e FLASHPIPE_OAUTH_CLIENTID=... -e FLASHPIPE_OAUTH_CLIENTSECRET=... -- cpictl mcp --root /path/to/repo`

Generic client config:

```json
{"mcpServers": {"cpi": {"command": "cpictl", "args": ["mcp", "--root", "/path/to/repo"],
  "env": {"FLASHPIPE_TMN_HOST": "...", "FLASHPIPE_OAUTH_HOST": "...",
          "FLASHPIPE_OAUTH_CLIENTID": "...", "FLASHPIPE_OAUTH_CLIENTSECRET": "..."}}}}
```

Code layout: `pkg/cpi` (tenant API client), `pkg/ops` (operations with structured
results, shared by CLI and MCP), `internal/cmd` (CLI), `internal/mcp` (MCP server).

`config-generate` now takes the target file via `--output-file`; a non-format value passed to
`--output` is still accepted as the file path (deprecated).

Tests: `go test ./...` runs offline. Tenant integration tests are behind a build tag and
**write to a real tenant**: `go test -tags integration ./...`.

---
<img src="https://github.com/engswee/flashpipe/raw/main/docs/images/logo/flashpipe_logo_wording.png" alt="FlashPipe Logo" width="200" height="140"/>

## The CI/CD Companion for SAP Integration Suite

[![GitHub license](https://img.shields.io/github/license/engswee/flashpipe)](https://github.com/engswee/flashpipe/blob/main/LICENSE)
[![GitHub release](https://img.shields.io/github/release/engswee/flashpipe.svg)](https://github.com/engswee/flashpipe/releases/latest)
[![Docker Image Size (latest semver)](https://img.shields.io/docker/image-size/engswee/flashpipe)](https://hub.docker.com/r/engswee/flashpipe/tags?page=1&ordering=last_updated)
[![Docker Pulls](https://img.shields.io/docker/pulls/engswee/flashpipe)](https://hub.docker.com/r/engswee/flashpipe/tags?page=1&ordering=last_updated)

### About

_FlashPipe_ is a public [Docker image](https://hub.docker.com/r/engswee/flashpipe) that provides Continuous
Integration (CI) & Continuous Delivery/Deployment (CD) capabilities for SAP Integration Suite.

_FlashPipe_ aims to simplify the Build-To-Deploy cycle for SAP Integration Suite by providing CI/CD capabilities for
automating time-consuming manual tasks.

### Enhanced Capabilities

_FlashPipe_ has been significantly enhanced with powerful new commands for streamlined CI/CD workflows:

#### 🎯 Orchestrator Command

High-level deployment orchestration with integrated workflow management:

- **Complete Lifecycle**: Update and deploy packages and artifacts in a single command
- **Multi-Source Configs**: Load from files, folders, or remote URLs
- **YAML Configuration**: Define all settings in a config file for reproducibility
- **Parallel Deployment**: Deploy multiple artifacts simultaneously (3-5x faster)
- **Environment Support**: Multi-tenant/environment prefixes (DEV, QA, PROD)
- **Selective Processing**: Filter by specific packages or artifacts

```bash
# Simple deployment with YAML config
flashpipe orchestrator --orchestrator-config ./orchestrator.yml

# Or with individual flags
flashpipe orchestrator --update \
  --deployment-prefix DEV \
  --deploy-config ./001-deploy-config.yml \
  --packages-dir ./packages
```

#### ⚙️ Config Generation

Automatically generate deployment configurations from your packages directory:

```bash
# Generate config from package structure
flashpipe config-generate --packages-dir ./packages --output ./deploy-config.yml
```

#### 📁 Partner Directory Management

Snapshot and deploy Partner Directory parameters:

```bash
# Download parameters from SAP CPI
flashpipe pd-snapshot --output ./partner-directory

# Upload parameters to SAP CPI
flashpipe pd-deploy --source ./partner-directory
```

See documentation below for complete details on each command.

### Documentation

For comprehensive documentation on using _FlashPipe_, visit the [GitHub Pages documentation site](https://engswee.github.io/flashpipe/).

#### New Commands Documentation

- **[Orchestrator](docs/orchestrator.md)** - High-level deployment orchestration and workflow management
- **[Orchestrator Quick Start](docs/orchestrator-quickstart.md)** - Get started with orchestrator in 30 seconds
- **[Orchestrator YAML Config](docs/orchestrator-yaml-config.md)** - Complete YAML configuration reference
- **[Configure](docs/configure.md)** - Configure artifact parameters with YAML files
- **[Config Generate](docs/config-generate.md)** - Automatically generate deployment configurations
- **[Partner Directory](docs/partner-directory.md)** - Manage Partner Directory parameters

#### Migration Guides

- **[Orchestrator Migration Guide](docs/orchestrator-migration.md)** - Migrate from standalone CLI to integrated orchestrator

#### Core FlashPipe Documentation

- **[FlashPipe CLI Reference](docs/flashpipe-cli.md)** - Complete CLI command reference
- **[OAuth Client Setup](docs/oauth_client.md)** - Configure OAuth authentication
- **[GitHub Actions Integration](docs/documentation.md)** - CI/CD pipeline examples

#### Examples

Configuration examples are available in [docs/examples/](docs/examples/):
- `orchestrator-config-example.yml` - Orchestrator configuration template
- `flashpipe-cpars-example.yml` - Partner Directory configuration example

#### Developer Documentation

For contributors and maintainers, see [dev-docs/](dev-docs/) for:
- Testing guides and coverage reports
- CLI porting summaries
- Enhancement documentation

### Analytics

_FlashPipe_ collects anonymous usage analytics to help guide ongoing development and improve the tool. No personal or user-identifiable information is collected. Analytics collection is always enabled and not optional. By using _FlashPipe_, you consent to this data collection. If you have concerns, you are encouraged to review the implementation. If you do not agree, please refrain from using the tool.

### Examples Repository
The following repository on GitHub provides examples of different use cases of _FlashPipe_.

[https://github.com/engswee/flashpipe-demo](https://github.com/engswee/flashpipe-demo)

### Versioning
[SemVer](https://semver.org/) is used for versioning.

### Contributing

Contributions from the community are welcome.

To contribute to _FlashPipe_, check the [contribution guidelines page](CONTRIBUTING.md).

### License

_FlashPipe_ is licensed under the terms of Apache License, Version 2.0 - see the [LICENSE](LICENSE) file for details.

### Stargazers over time
[![Stargazers over time](https://starchart.cc/engswee/flashpipe.svg?variant=adaptive)](https://starchart.cc/engswee/flashpipe)


