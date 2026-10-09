# Transports between tiers

A transport moves tested artifacts from one tier to the next (DEV → TEST → PROD, or any chain of
tiers). The deployment is the usual pipeline against the target tier (`orchestrator`, `configure`,
`deploy --pending`, see [ci.md](ci.md#pipeline-snapshot-update-configure-deploy-once)).
`cpictl transport` prepares it, and [`cpictl compare`](compare.md) shows the differences between
tiers.

```bash
cpictl compare tenant:test tenant:prod --package Orders            # what differs?
cpictl transport deps Orders_In --with-deps                          # what does it need?
cpictl transport check Orders_In --with-deps --source tenant:test --target tenant:prod \
  --target-dir git:prod:packages --configure config/prod.yaml        # will it run on the target?
cpictl transport copy Orders_In --with-deps --from git:test:packages --to packages   # branch per tier
```

## Version matrix (`matrix`)

```bash
cpictl matrix tenant:dev tenant:test tenant:prod --dir packages --differences
```

Per artifact: the `Bundle-Version` in the content tree (`--dir`), and per tier (in promotion
order) the designtime version, draft flag, running version and status, and who changed it last
where the tenant reports it. `behind` lists the tiers whose version is lower than the tier before
them (or Git), or that do not have the artifact yet: the next promotions. A tier is `tenant`,
`tenant:<profile>` or `<name>=tenant:<profile>`. One pass per tenant (package lists and the
runtime list), no downloads; `--output json` returns `tiers` and `rows`.

## What moves and what does not

- **Moves**: the artifact folders as tested on the source tier, with the same `Bundle-Version`
  (versioning `manifest`, see [versioning.md](versioning.md)), and with `--with-deps` the script
  collections and mappings they reference.
- **Stays on each tier**: parameter values (the tier's `configure` file), Partner Directory
  values, credentials and certificates, deploy-config overrides. The check only verifies that they
  are in place on the target.

## Dependencies (`transport deps`)

Artifacts are given by ID or folder name. For each, from the local files:

| Kind | Found by |
|------|----------|
| `artifact` | script collections (`scriptBundleId` of script steps) and mappings or value mappings whose ID is referenced in a step property |
| `flow_call` | flows the artifact sends to through ProcessDirect or JMS (from the [content graph](graph.md)) |
| `credential` | credential names and key aliases of adapters |
| `pd` | Partner Directory parameters read with a literal partner ID |

`--with-deps` adds the referenced artifacts of the repository to the selection (transitively).
Called flows are reported, not added: they are released on their own.

## Pre-checks (`transport check`): will it run on the target?

Against the target tenant (`--target tenant` or `tenant:<profile>`), read only. Each check is
`pass`, `warn`, `fail` or `skip`; any `fail` gives exit code 5.

| Check | Result |
|-------|--------|
| `draft` | **fail**: the artifact is in draft on the target (someone edits it in the Web UI) |
| `exists` | new on the target, or the target's version |
| `drift` | **warn** (with `--target-dir`, the target's content in Git): changed on the target outside the pipeline, by whom where the tenant reports it; adopt the change into Git first |
| `dependency` | **fail**: a referenced script collection or mapping is neither on the target nor in the transport |
| `flow_call` | **warn**: a called flow is not running on the target |
| `credential` | **fail**: a credential or key alias is missing on the target. When a parameter names it (`{{SFTP Credential}}`), the target's value of that parameter is checked; no value is a failure too |
| `keystore` | **warn**: a key alias the flow uses expires on the target within `--expiry-days` (default 30); **fail**: expired |
| `pd` | **fail**: a Partner Directory parameter is missing on the target (`skip` for dynamic partner IDs) |
| `parameters` | the flows' configuration, source vs target: **fail** when a parameter has no value on the target and none travels; **warn** when it is not set on the target, so the source tier's value travels; **warn** when source and target have the same value and it looks environment-specific (a URL or host) |

**Configuration.** The target's values are the target tenant's configured values (when the
artifact is there) overridden by the target's configure file (`--configure`). The source's values
are the source tenant's configured values (`--source tenant:<profile>`), else the `parameters.prop`
of the content tree. Parameter names come from `parameters.propdef`. Values are never printed,
only the parameter names.

**Accepting gaps.** Sometimes the transport goes ahead while someone else provides the security
material or the configuration on the target. `--allow-missing credential,keystore,pd,parameters,dependency`
(MCP `allow_missing`) reports those failures as warnings marked "accepted: provided separately",
so the gap stays visible in the result and the job summary but does not stop the pipeline.

The MCP tool `transport_check` does the same against the server's tenant (run it on the target
tier's server).

## Copy (`transport copy`)

For a branch or repository per tier: copies the artifact folders from `--from` (a directory or
`git:<ref>[:<path>]`) into `--to`, replacing each folder exactly (files deleted in the source are
deleted). `--dry-run` lists `created`, `updated` or `unchanged` per artifact. Commit the result on
the target branch and open a pull request.
