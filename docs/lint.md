# Lint and improvements

`cpictl lint` checks the integration flows of a content tree (the tenant layout
`packages/<package>/<artifact>`, as `snapshot` writes it) and reports findings with a rule, a
severity and a suggestion. It reads local files only and is fast enough for every pull request on
a repository with a thousand flows. `--fix` applies the mechanical fixes. The `cpi-improve` skill
turns the findings into proposals and applies them with an agent.

```bash
cpictl lint                                             # all flows below packages/
cpictl lint --package UtilitiesBaseEDM --output json
cpictl lint --changed --since origin/main --fail-on warning     # pull request gate
cpictl lint --update-baseline                           # accept today's findings as known
cpictl lint --fix --dry-run                             # what --fix would change
cpictl lint --fix --package UtilitiesBaseEDM            # apply it
cpictl lint --list-rules
```

## Rules

| Group | Rule | Severity | Finds | Fix |
|-------|------|----------|-------|-----|
| reuse | `duplicate-script` | warning | the same script in several flows | moves it to a script collection |
| | `use-script-collection` | warning | a script equal to one in an existing collection | references the collection |
| | `similar-script` | info | scripts that differ only in comments and formatting | |
| | `duplicate-mapping` | warning | the same message mapping or XSLT in several flows | |
| | `missing-script-collection` | warning | a referenced collection not in the repository | |
| partner-directory | `router-literals` | warning | a router deciding on 4+ literal values of one header or property | |
| | `lookup-table-in-script` | info | a script with 10+ literal map entries or case branches | |
| | `deployment-copies` | info | one folder deployed under several IDs with different `configOverrides` (from the deploy config) | |
| dead-weight | `unconnected-step` | warning | a step not reachable from the start event | removes it |
| | `unused-script` | warning | a script file no step uses | deletes it |
| | `unused-resource` | info | a mapping, schema or WSDL nothing references | |
| | `unused-parameter` | warning | a `parameters.prop` key no `{{key}}` uses | |
| | `noop-content-modifier` | warning | a content modifier without headers, properties and body | removes it (`--fix-rules`) |
| | `property-never-read` | info | an exchange property set but never read in the flow | |
| simplify | `consecutive-content-modifiers` | info | content modifiers in a row | |
| | `converter-roundtrip` | warning | XML to JSON and straight back (or the reverse) | |
| | `trivial-script` | info | a script that only sets literal headers or properties | |
| | `large-script` | info | a script over 300 lines | |
| robustness | `no-exception-subprocess` | warning | an integration process without exception subprocess | |
| | `swallowed-exception` | warning | an empty `catch` block | |
| performance | `body-as-string` | info | `getBody(String)` (whole payload in memory) | |
| | `payload-attachment` | warning | the payload written into the message processing log | |
| | `logging-in-loop` | warning | log attachments inside a loop | |
| | `println` | info | output to stdout | |
| configuration | `hardcoded-endpoint` | warning | a receiver address that is not a `{{parameter}}` or `${...}` | |
| | `hardcoded-url-in-script` | warning | a URL in a script (namespaces excluded) | |
| | `hardcoded-secret` | error | a password, token or key assigned in a script or parameter | |
| hygiene | `outdated-component` | info | a step or adapter older than the newest version used in the repository | |
| | `default-step-name` | info | "Groovy Script 1", "Content Modifier 2", ... | |
| | `naming` | warning | a flow ID not matching `naming.iflowId` | |

All flows are read for the cross-flow rules (duplicates, versions); `--package`, `--artifact` and
`--changed` select which flows are reported. `deployment-copies` needs the deploy config
(`--deploy-config`, else `orchestrator.deployConfig`). The agent's guide per rule (when to act, how
to change it, risk) is the skill's
[improvement catalogue](../plugin/skills/cpi-improve/improvement-catalogue.md).

## Settings: `.cpi/lint.yaml`

```yaml
rules:                      # severity per rule: off, info, warning, error
  default-step-name: off
  hardcoded-endpoint: error
settings:
  duplicateMinFlows: 2      # duplicate-script, similar-script, duplicate-mapping
  routerMinValues: 4        # router-literals
  lookupMinEntries: 10      # lookup-table-in-script
  largeScriptLines: 300     # large-script
scriptCollections:
  packageCollection: "{package}_Scripts"
  crossPackage: false       # true: flows may reference a collection in another package
  sharedPackage: SharedScripts
  sharedCollection: Shared_Scripts
naming:
  iflowId: "^[A-Z][A-Za-z0-9]*(_[A-Za-z0-9]+)+$"
ignore:                     # findings that stay (say why in a comment)
  - artifact: Legacy_*
    rule: "*"
```

`--rules` points to another file. An unknown rule or severity is an error (exit code 2).

## CI: baseline and gate

`--fail-on warning` exits with code 5 when a **new** finding has at least that severity. New means
not in the baseline (`.cpi/lint-baseline.json`, `--baseline`): `cpictl lint --update-baseline`
records today's findings, so a large existing repository can start gating pull requests without
fixing everything first. Commit the baseline; update it when findings are fixed or accepted.

```bash
cpictl lint --changed --since origin/main --fail-on warning
```

In GitHub Actions the new findings also go to the job summary.

## Fixes

`--fix` changes local files only (`--dry-run` lists the changes):

| Rule | Change |
|------|--------|
| `duplicate-script` | the script goes into the package's collection (`<package>_Scripts`, created with manifest and `metainfo.prop` when missing), or with `crossPackage: true` and flows in several packages into the shared collection; every script step that used the local file references the collection (`scriptBundleId`); the local copies are deleted |
| `use-script-collection` | the script step references the existing collection that holds the same script; the local copy is deleted |
| `unused-script` | the file is deleted |
| `unconnected-step` | the step, its sequence flows and its diagram shapes are removed |
| `noop-content-modifier` | only with `--fix-rules noop-content-modifier` (or `all`): the step is removed and its neighbours connected |

Afterwards: review the diff, `cpictl version bump --changed`, add new script collections to the
deploy config and deploy them **before** the flows, then deploy and test the flows. Whether flows
may reference a collection in another package depends on the tenant: check it once before setting
`crossPackage: true`.

## With an agent: the `cpi-improve` skill

The skill runs `lint` (MCP tool) for the scope you name, groups the findings into proposals (one
script collection per cluster of shared scripts, one Partner Directory design per routing table or
flow family, a dead-weight sweep per package, ...), writes them to `.cpi/improvements/<date>-<scope>.md`
and asks which to apply. Approved proposals are applied one at a time on the development tenant:
`lint_fix` for the mechanical ones, cpi-build for the rest, then `bump_versions`, upload
(dependencies first), deploy, cpi-test for every affected flow and cpi-review.

```text
> Check the package UtilitiesBaseEDM for improvements.
> Apply P1 and P3.
```

MCP tools: `lint` (read, local files) and `lint_fix` (local files, `dry_run`), see [mcp.md](mcp.md).
