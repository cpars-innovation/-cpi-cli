# Changelog

All notable changes to cpictl. Coming from FlashPipe? See
[docs/migrating-from-flashpipe.md](docs/migrating-from-flashpipe.md).

## Unreleased

- Skills: cpi-test starts every run with the static checks (`validate_artifact`,
  `check_guidelines`, runtime version) and reports them; cpi-build runs `check_guidelines`
  after the validation.
- Package IDs: only letters and digits are accepted (`packages create`, MCP `create_package`);
  the tenant refuses `_`, `-` and `.`. Examples and the cpi-discover skill no longer suggest them.
- MCP tool `help`: what the server offers in its mode (available and disabled tools), workflows
  (task -> skill, tools, docs), the cpi skills with their instructions and reference files, and
  the CLI commands; `topic` for details of one tool, skill, skill file or command. Lets clients
  without skill support use the skills.
- `cpictl skills list|show|install`: the skills are built into the binary; `install` copies them
  into `.agents/skills`, `.cursor/skills`, `.gemini/skills` or `.claude/skills` (repository or
  `--user`) without a checkout of this repository.
- `docs/mcp.md` is checked to mention every MCP tool.
- `cpictl iflow copy` (MCP `copy_iflow`): copy an integration flow from the tenant or a local
  folder under a new ID, name, description and version; renames the model and `.project`,
  keeps `;singleton:=true`, wraps manifest lines correctly, and changes every sender address
  (in the model or in `parameters.prop`), refusing to keep one unless asked. See
  [docs/new-flows.md](docs/new-flows.md). The `cpi-build` skill uses it for new flows.
- MCP `upload_artifact` without `name` uses `Bundle-Name` of the manifest, as the CLI does
  (it used the ID).
- Plugin: brief template for new flows (`.cpi/templates/brief.md`, filled in as
  `.cpi/briefs/<name>.md`; cpi-plan asks only for what is missing), a *Templates* section in
  the conventions (one reference flow per pattern, proposed by cpi-discover, agreed by the
  team) and house scripts in `.cpi/templates/scripts/`; cpi-build copies from them.
- Content graph: `discover` (MCP `discover_tenant`) also writes `.cpi/graph.json`: flows,
  packages, endpoints, receiver systems, credentials, scripts, headers, properties and Partner
  Directory parameters as nodes; `sends_to` links flows through matching ProcessDirect and JMS
  addresses (`{{parameter}}` addresses resolved with `parameters.prop`). Query it with
  `cpictl graph search|neighbors|path|build` and the MCP tools `graph_search`, `graph_neighbors`,
  `graph_path` (read a local file only). Discovery now also records receiver addresses,
  the values of address parameters and literal Partner Directory references per flow.
  See [docs/graph.md](docs/graph.md).
- Codex, Cursor and Gemini CLI: MCP configuration examples (`docs/examples/agents`),
  `scripts/install-skills.sh` to copy the skills into `.agents/skills`, `.cursor/skills` or
  `.gemini/skills`, an `AGENTS.md` template, and [docs/agents.md](docs/agents.md).
  `cpi-review` reviews by itself when there is no `cpi-reviewer` agent.

## 0.1.0 (unreleased)

First release.

- CLI for SAP Cloud Integration: inspect (`packages`, `artifacts`, `status`), designtime
  (`update artifact`, `update package`), runtime (`deploy`, `undeploy`), parameters
  (`params`, `configure`), multi-package deployments (`orchestrator`, `config-generate`),
  Git (`sync`, `snapshot`), API Management and Partner Directory commands.
- `--output json` result documents and a stable exit code contract.
- Deployments confirmed via the BuildAndDeployStatus task and a fresh runtime artifact,
  with one structured result per artifact (including designtime and runtime version).
- `drift` (CLI and MCP): local artifacts vs designtime and runtime (in_sync, tenant_newer,
  local_newer, diverged, not_on_tenant). Content comparison no longer needs an external `diff`
  program (it failed on Windows).
- Runtime data: data stores (list, entries, get, delete with confirm), variables, JMS queues and
  broker, number ranges, log files (tail), idempotent repository, ID mapper (CLI and MCP).
- `set_log_level` reverts to INFO after `revert_after_minutes` and on server shutdown; trace tools
  report a missing role (403) as `status: "missing_role"` instead of an auth error.
- Build loops: `loop_start` / `loop_status` / `loop_end` with server-enforced limits
  (iterations, repeated errors, wall clock, deploys); exit code 8 / `stopped`. `cpictl mcp --mode
  discover|operate|develop|full`.
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
