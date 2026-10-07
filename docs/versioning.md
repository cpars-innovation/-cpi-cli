# Versioning

For promotion `dev -> test -> prod`, the same content should carry the same version on every
tenant. cpictl does that by keeping the version in the repository: `Bundle-Version` in each
artifact's `META-INF/MANIFEST.MF`. With `--versioning manifest`, upload sets exactly that
version on the tenant and deploy refuses anything lower than what is running.

```
change the flow -> cpictl version bump --changed -> PR to dev -> pipeline (versioning manifest)
                   (raises Bundle-Version of               upload: designtime = Bundle-Version
                    the changed artifacts)                 deploy: guard on, same version on
                                                           dev, test and prod
```

## Modes

The mode is set per pipeline (or per branch) with `--versioning` or the environment variable
`CPICTL_VERSIONING`. It applies to `update artifact`, `sync` (to the tenant), `snapshot restore`,
`orchestrator`, `configure`, `deploy` and the MCP server (`upload_artifact`, `deploy`).

| Mode | Upload | Deploy (designtime version lower than running) |
|------|--------|-----------------------------------------------|
| `manifest` | The designtime version becomes `Bundle-Version` of the repository. Upload fails without one. | `FAILED`: raise the version in the repository. Only `allowDowngrade` overrides it. |
| `keep` | The tenant keeps its versions (as before). | Deployed: no guard. |
| `tenant-bump` | For repositories without versions: every upload that changes content sets `max(designtime, runtime) + 1` (patch). | `FAILED` (cannot happen after a bump). |
| not set | The tenant keeps its versions. | `FAILED` unless the designtime artifact was changed after the running deployment ([configure.md](configure.md#older-designtime-versions)). |

Every artifact logs the version it got and why, on upload
(`Version of X: 1.0.16 (manifest: Bundle-Version of the repository (tenant had 1.0.15))`,
JSON `version`, `versionReason`, `versionSet`) and on deploy
(`Deploying artifact X with version 1.0.16 (versioning manifest)`, JSON `versioning`, `rule`).

### Per package or artifact

Deployment files (orchestrator) and configure files accept `versioning` on a package or an
artifact; the artifact's wins, then the package's, then `--versioning`:

```yaml
packages:
  - integrationSuiteId: Legacy
    versioning: tenant-bump        # this package has no versions in the repository yet
    artifacts:
      - artifactId: OldFlow
        versioning: keep           # exception for one artifact
```

Use this only for exceptions that hold on every branch. These files are merged between branches,
so the mode of a branch belongs to its pipeline (`CPICTL_VERSIONING`), not to the files.

## Bumping versions: `cpictl version bump`

```bash
cpictl version bump --changed --dir packages                 # patch, changed artifacts only
cpictl version bump --changed --level minor --package UtilitiesBaseEDM
cpictl version bump --artifact UtilitiesBase_MDX_to_EDM_Outbound --dry-run
```

With `--changed`, an artifact is bumped when its directory changed since the commit that last
set its `Bundle-Version` (`git log -G '^Bundle-Version:'` on its manifest). Committed, staged,
unstaged and untracked changes count. It is left alone when:

- nothing changed since that commit (`unchanged`),
- its version was already raised since that commit, e.g. a second run before committing
  (`already_bumped`),
- it was never committed (`new`: its version is its first version).

Only the `Bundle-Version` line of the manifest changes. Without `--changed`, all selected
artifacts are bumped. The command needs no tenant access and prints `old -> new` per artifact.
Agents use the MCP tool `bump_versions` (`changed: true`) before they open a pull request.

## Export and download

`sync --target git`, `snapshot` and `download` (MCP `download_artifact`) used to write the
download's `Bundle-Version` into the repository. That is not the artifact's version (often
`1.0.0`). Now:

- `sync --target git` / `snapshot` keep the repository's `Bundle-Version`, or take the tenant's
  designtime version when it is higher (a version saved in the Web UI). New artifacts get the
  tenant's designtime version.
- `download` writes the tenant's designtime version.

When cpictl compares local content with the tenant (upload, export), `Bundle-Version` is not
compared: a different version alone is no content change. In `manifest` mode a version that
differs from the tenant's is set without uploading the content again.

## Switching a repository to `manifest`

One-time step: put the current versions into the repository, at least as high as what runs on
any tenant now.

1. Find the running versions per tenant, for example
   `cpictl --profile prod status --output json`, `cpictl --profile test status ...`.
2. Set `Bundle-Version` of each artifact to the highest of them (or one above). Edit the
   manifests or run `cpictl version bump --artifact X` from a known version. Commit.
3. Set `CPICTL_VERSIONING=manifest` in the pipelines of the branches that should use it.

After that, raise versions with `cpictl version bump --changed` before each pull request. A
version that is lower than the running one then always means stale content, and
`--allow-downgrade` / `allowDowngrade` is not needed any more.

## Limits

- The version is set with the API action `IntegrationDesigntimeArtifactSaveAsVersion`, which the
  public API offers for integration flows. For message mappings, value mappings and script
  collections, `manifest` and `tenant-bump` work only if the tenant takes the version from the
  uploaded manifest; otherwise the upload reports that the version cannot be set.
- Whether the tenant takes `Bundle-Version` from an uploaded manifest by itself is not assumed:
  cpictl reads the designtime version after the upload and sets it when it differs.
- Content changes without a version bump in `manifest` mode: the upload warns
  (`run 'cpictl version bump --changed'`) and, as before, undeploys the running artifact with
  the same version so that the new content is deployed.
