# Script collections

A script collection is its own artifact (`SAP-BundleType: ScriptCollection`) with scripts in
`src/main/resources/script/`. A script step uses it with the properties `scriptBundleId` (the
collection ID) and `script` (the file name in the collection); a local script has an empty
`scriptBundleId`.

## When

- The same script (or the same function) in two or more flows: always a candidate. The more flows,
  the higher the priority (`duplicate-script` lists them in `related`).
- Scripts that differ only in comments and formatting (`similar-script`): unify first, then move.
- A script equal to one in an existing collection (`use-script-collection`): reference it.
- Large scripts (`large-script`): split into functions; reusable parts go to a collection.

Not every script belongs in a collection: flow-specific logic stays in the flow. A shared script is
a contract: changing it changes every flow that uses it.

## Package or shared

| Scope | Where | When |
|-------|-------|------|
| Package | `<package>/<package>_Scripts` (`scriptCollections.packageCollection`) | the flows using it are in one package; always possible |
| Shared | `scriptCollections.sharedPackage` / `sharedCollection` | the flows are in several packages and the tenant allows references to a collection in another package |

Whether a flow may reference a collection in another package depends on the tenant: check once in
the Web UI (*Add reference > Script collection* lists collections of which packages?) and set
`scriptCollections.crossPackage` in `.cpi/lint.yaml`. Without it, `lint_fix` creates one collection
per package, also for scripts reused across packages; a shared package can follow later.

## Names and structure

- Collection ID from the conventions (default `<package>_Scripts`); the shared one e.g.
  `Shared_Scripts` in a package such as `SharedScripts`.
- File names say what the script does (`setLogProperties.groovy`), not where it came from.
- One function per purpose; parameters through headers/properties, not edits per flow.
- `lint_fix` keeps the file name; a name that already holds other content in the collection gets a
  hash suffix (`Log_3fa2b1.groovy`): rename it while reviewing.

## Deployment

1. The collection is an artifact: add it to the deploy config of its package (orchestrator) and
   deploy it **before** the flows that use it.
2. Then the flows (with raised `Bundle-Version`).
3. After a later change to the collection: redeploy it and test every flow that uses it
   (`lint` / `graph_search` for the script's users). Plan such changes like an API change.

## Test

Before and after the move, the same input must give the same output for every affected flow. Test
at least one flow per package, and every flow whose script variant was unified.
