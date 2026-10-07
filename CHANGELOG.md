# Changelog

All notable changes to cpictl. Coming from FlashPipe? See
[docs/migrating-from-flashpipe.md](docs/migrating-from-flashpipe.md).

## Unreleased

- OpenCode: MCP setup (`opencode.json`) and `cpictl skills install --agent opencode`
  (`.opencode/skills`, `--user`: `~/.config/opencode/skills`). Cursor setup expanded (global
  file, `${env:...}`, tool approval). Examples with profiles and with environment variables in
  [docs/examples/agents](docs/examples/agents); see [docs/agents.md](docs/agents.md).

- Defaults unified: deployment status every 10 s, up to 30 checks (5 minutes) for `deploy`
  (before 30 s x 10), `orchestrator` and `configure` (before 15 s x 5, which timed out slow
  deployments after 75 s); 5 deployments at the same time per package (before 3). All defaults
  in one table: [docs/configuration.md](docs/configuration.md#defaults).
- Job summary: `orchestrator`, `configure`, `deploy`, `undeploy` and `snapshot` append a markdown
  summary (counts, plan, deployments with errors first) to `$GITHUB_STEP_SUMMARY`, or to
  `--summary FILE`; `--summary off` disables it. See [docs/ci.md](docs/ci.md#job-summary).
- `cpictl doctor` (MCP: `doctor`): checks configuration, connection and authentication, and per
  API area whether the credentials can use it (403: missing role, 404: not offered). Read only.
- MCP: `upload_artifacts` uploads several artifacts in one call (8 at a time); `get_parameters`
  takes `artifact_ids`. `list_packages` and `list_artifacts` results are reused for
  `--cache-ttl` seconds (default 60, `cached: true`; `refresh: true` re-reads); every tool that
  changes the tenant clears the cache.
- Parallel: `orchestrator --parallel` (default 8) uploads and compares artifacts at the same time
  across all packages (packages are still created first); `configure --parallel` (8) reads the
  parameters of all artifacts at the same time and writes the changes one after another;
  `configure pull --parallel` (8) reads packages and artifacts in parallel; `drift --parallel`
  (8, before: 4).
- Deploy once per pipeline: `orchestrator --defer-deploy` and `configure --defer-deploy` add what
  needs a deployment (and why) to `.cpi/pending-deploy.json`; `cpictl deploy --pending` deploys
  each artifact once, with force when any step needs it, and keeps only failures in the file.
  `deploy --plan` (also with `--artifact-ids`) predicts without triggering. See
  [docs/ci.md](docs/ci.md#pipeline-snapshot-update-configure-deploy-once).
- `orchestrator`: a content change with the running version is no longer undeployed during the
  upload; phase 2 deploys it with force, so the runtime keeps running in between.
- Plan mode: `orchestrator --plan` reports per artifact whether it would be created, updated or
  left unchanged and whether it would be deployed, and why; nothing is written. `configure
  --plan` (= `--dry-run`) now also says which artifacts would be deployed and why (`plan` in the
  result). MCP: `upload_artifact` and `deploy` take `dry_run`.
- `orchestrator`: compares existing artifacts with the snapshot state (`.cpi/snapshot-state.json`,
  written by `snapshot` earlier in the pipeline) instead of downloading each one again. Artifacts
  the state does not cover, or that changed on the tenant since the snapshot, are downloaded as
  before. `--snapshot-state` picks the file (`off`: always download), `--verify-download`
  downloads anyway. New statistics: `comparedWithSnapshot`, `downloadedForComparison`,
  `artifactsChanged`, `artifactsUnchanged`. `snapshot` records the tenant host and a content
  hash per artifact for that.
- Every command and the MCP server retry tenant reads (GET) answered with 429 or 502-504 up to
  three times with backoff (before: `snapshot` only). `--read-retries` (`CPICTL_READ_RETRIES`)
  sets the count, `0` turns it off. Writes are never retried.
- `cpictl stats`: local usage statistics. Every command and MCP tool call appends name, source,
  exit code, duration and time to `~/.cpictl/stats.jsonl` (no arguments, hosts or names; nothing
  is sent anywhere; compacted at 1 MiB; `CPICTL_STATS=off` disables it).
- MCP: every tool result has `durationMs`; the server log has one line per finished call.

## 0.2.4

- Every command logs the time it took at the end (`⏱ snapshot finished in 4m12s`, JSON log field
  `durationMs`); the `--output json` envelope has a new field `durationMs`.
- `snapshot`: `--parallel` (default now 8) limits the artifacts downloaded at the same time across
  all packages instead of the packages: the artifacts of a large package are downloaded in
  parallel too, so it no longer runs alone at the end. A failing artifact does not stop the rest
  of its package. The result reports `artifactsDownloaded`, `artifactsSkipped` and `seconds`.

## 0.2.3

- `snapshot`: `--incremental` skips the download of artifacts whose designtime version,
  `ModifiedAt`, configured parameters (SHA-256) and local copy (SHA-256) are unchanged since the
  last snapshot (state in `.cpi/snapshot-state.json`, committed with the snapshot; artifacts
  without `ModifiedAt` are always downloaded). Packages run `--parallel` (default 4); a failing
  package no longer aborts the snapshot (exit code 7, the rest is written and committed).
  Throttled or gateway-failed reads (429, 502-504) are retried with backoff. See
  [docs/snapshot.md](docs/snapshot.md).

## 0.2.2

- Versioning for promotion (`dev -> test -> prod`, same content = same version), see
  [docs/versioning.md](docs/versioning.md):
  - `--versioning manifest|keep|tenant-bump` (env `CPICTL_VERSIONING`) on `update artifact`,
    `sync`, `snapshot restore`, `orchestrator`, `configure`, `deploy` and `mcp`; `versioning` per
    package or artifact in deployment and configure files for exceptions. `manifest`: upload sets
    the designtime version to `Bundle-Version` of the repository (SaveAsVersion when the tenant
    did not take it), deploy refuses lower versions (no timestamp exception). `keep`: tenant
    versions, no downgrade guard. `tenant-bump`: max(designtime, runtime)+1 on content changes.
    Upload and deploy log the version per artifact and why (`versionReason`, `versioning`).
  - `cpictl version bump [--changed] [--level] [--package] [--artifact] [--dry-run]` and MCP
    `bump_versions`: raise `Bundle-Version` of artifacts changed since the commit that last set
    it; no tenant access.
  - Export (`sync --target git`, `snapshot`) keeps the repository's `Bundle-Version` (or takes a
    higher designtime version) instead of the download's (usually 1.0.0); `download` writes the
    designtime version. Content comparisons ignore `Bundle-Version`.
  - `manifest` details: content that differs from the tenant's while `Bundle-Version` equals the
    tenant's designtime or running version is refused before anything is written (bump it); the
    version set with SaveAsVersion is read back; a lower target version is attempted with a
    warning and a refusal is reported with the version to exceed; the orchestrator deploys only
    the directory's `Bundle-Version` (rule `guard` otherwise).
  - Rules in upload and deploy results and logs: `manifest`, `bump`, `keep`, `tenant`, `guard`
    (plus `version`, `modified after deployment`, `allowDowngrade` without a mode), with a reason.
- Orchestrator multi-deploy: rewriting `Bundle-SymbolicName` for a final artifact ID keeps its
  attributes (`; singleton:=true` was dropped) and removes continuation lines of the old
  `Bundle-Name`; values are wrapped at 72 bytes.
- MCP `upload_artifact` returns the result (version, rule) together with an error.

- Deploy (`deploy`, `configure`, `orchestrator`, MCP `deploy`): a designtime version older than
  the running one is deployed when the designtime artifact was changed after the running version
  was deployed (designtime `ModifiedAt` later than runtime `DeployedOn`; e.g. the runtime came
  from a manual deployment of an older build with a bumped version). Otherwise, or when a
  timestamp is missing, it is still refused. Every result names the deciding rule
  (`version`, `modified after deployment`, `allowDowngrade`) on its log line and in `rule`.
- `configure`: `allowDowngrade: true|false` per artifact and per package in the configure file
  (artifact wins, then package, then `--allow-downgrade` / `configure.allowDowngrade` /
  `deploy.allowDowngrade`); before, configure could not allow a downgrade at all. The
  modification time is read before parameters are written, so a parameter change never makes
  older content look new. The refusal names the `allowDowngrade` key.
- `resources` / MCP `list_resources`: `ResourceSize` sent as a string (`"1234"`) or with
  decimals no longer fails (`cannot unmarshal string into ... ResourceSize`).
- Partner Directory (`pd diff`, `pd get`, `pd-snapshot`, `pd-deploy`, MCP `pd_diff`,
  `get_pd_parameters`): all pages are read. The tenant returns at most 30 binary parameters per
  page; listings stopped after the first page, so further binaries showed as missing or
  `create`, and snapshots could miss them. Paging now follows `__next` or `$skip` until
  `__count` is reached.

## 0.2.1

- `configure`: artifacts without `parameters` (script collections, mappings, flows that only need
  a deployment) no longer fail with 404 (`Integration design time artifact not found`): their
  parameters are not read, and with `deploy: true` they are deployed unless the runtime already
  runs the designtime version. Artifacts whose parameters are unchanged are now also deployed
  when the designtime version is newer than the runtime (they were skipped). `parameters` on a
  type other than Integration is reported as an error before anything is written; `config_diff`
  skips artifacts without parameters.
- Skills: cpi-test starts every run with the static checks (`validate_artifact`,
  `check_guidelines`, runtime version) and reports them; cpi-build runs `check_guidelines`
  after the validation.
- Package IDs: only letters and digits are accepted (`packages create`, MCP `create_package`);
  the tenant refuses `_`, `-` and `.`. Examples and the cpi-discover skill no longer suggest them.

## 0.2.0

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

## 0.1.0

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
