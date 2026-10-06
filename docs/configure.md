# Configuring parameters

Three ways to change externalised parameters of integration flows:

| Command | Use for |
|---------|---------|
| `params get` / `params set` | One artifact, interactive or scripted |
| `configure` | Many artifacts from YAML files (per environment), optionally deploy afterwards |
| `configure pull` | Write the current tenant values into `configure` YAML files |

Parameter changes only take effect at runtime after a deployment.

## params

```bash
cpictl params get --artifact-id MyIFlow
cpictl params set --artifact-id MyIFlow --param ReceiverHost=api.example.com --param Timeout=60
cpictl params set --artifact-id MyIFlow --param Timeout=60 --dry-run
cpictl deploy --artifact-ids MyIFlow --compare-versions=false
```

`params set` writes only values that differ. An unknown key fails the call (exit code 2)
before anything is written. `--compare-versions=false` is needed on the following deploy
because a parameter change does not change the artifact version.

## configure

```bash
cpictl configure --config-path ./config/dev.yml
cpictl configure --config-path ./config/          # all *.yml / *.yaml in the folder (not recursive)
cpictl configure --config-path ./config/dev.yml --dry-run            # diff against the tenant
cpictl configure --config-path ./config/dev.yml --dry-run --offline  # file only, no tenant calls
cpictl configure --config-path ./config/dev.yml --force              # write and deploy everything
```

`configure` compares every artifact's parameters with the tenant first and writes only the keys
that differ. Artifacts marked for deployment are deployed when at least one key changed (always:
a configuration change does not change the version), and otherwise only when the runtime does
not run the designtime version yet (for example after an upload earlier in the pipeline). A
second run on an unchanged tenant therefore writes and deploys nothing. A key that the artifact
does not have (`unknown_key`, usually a typo) fails that artifact before anything is written.

Artifacts without `parameters` (script collections, message and value mappings, flows that only
need a deployment) are not read or written; with `deploy: true` they are deployed unless the
runtime already runs their designtime version. Only integration flows have externalised
parameters: `parameters` on another type is an error for that artifact.
`--force` writes all parameters and deploys all marked artifacts, as earlier versions did. The
MCP tool `config_diff` shows the same comparison.

Example: [examples/configure.yml](examples/configure.yml).

```yaml
deploymentPrefix: ""                # optional, prepended to package and artifact IDs
packages:
  - integrationSuiteId: MyPackage
    displayName: My Package         # informational
    deploy: false                   # true: deploy all configured artifacts of this package
    artifacts:
      - artifactId: MyIFlow
        type: Integration           # Integration, MessageMapping, ScriptCollection, ValueMapping
        version: active             # default active
        deploy: true                # deploy this artifact after configuration
        parameters:
          - key: ReceiverHost
            value: api.example.com
          - key: Timeout
            value: "60"
        batch:                      # optional
          enabled: true             # default true
          batchSize: 90             # parameters per $batch request
```

How it runs:

1. **Configure**: for each artifact the parameters are written, by default with OData
   `$batch` requests (`--batch-size`, default 90). If a batch fails, the parameters are written
   one by one. Keys that do not exist on the artifact are skipped and counted as failed.
2. **Deploy**: artifacts with `deploy: true` (or in a package with `deploy: true`) that were
   configured successfully are deployed package by package, up to `--parallel-deployments`
   (default 3) at a time: always after a parameter change, otherwise only when the runtime
   version differs from the designtime version. Status is checked
   `--deploy-retries` (default 5) times every `--deploy-delay` (default 15) seconds.

| Flag | Description |
|------|-------------|
| `--config-path`, `-c` | File or folder (required) |
| `--deployment-prefix`, `-p` | Overrides `deploymentPrefix`; the final IDs are `prefix + ID` for packages and artifacts |
| `--package-filter`, `--artifact-filter` | Comma-separated IDs (without prefix) to include |
| `--dry-run` | Show what would be changed (compares with the tenant) |
| `--offline` | With `--dry-run`: only the file, without tenant calls |
| `--force` | Write every parameter and deploy every marked artifact, even without changes |
| `--disable-batch` | Always write parameters one by one |

All flags can be set in the global config file under `configure:` (`configPath`,
`deploymentPrefix`, `packageFilter`, `artifactFilter`, `dryRun`, `deployRetries`,
`deployDelaySeconds`, `parallelDeployments`, `batchSize`, `disableBatch`).

With `--output json` the result contains the statistics, the `diff` (per key: `update` with local
and tenant value, `unchanged`, `unknown_key`) and one deployment result per deployed artifact. Failures give exit code 7 when anything succeeded, otherwise 5.

> **Prefix rule differs from the orchestrator.** `configure` builds `prefix + ID` for packages
> and artifacts (prefix `DEV_` → `DEV_MyPackage`, `DEV_MyIFlow`). The orchestrator builds
> `prefix + ID` for packages but `prefix + "_" + ID` for artifacts (prefix `DEV` →
> `DEVMyPackage`, `DEV_MyIFlow`). Choose the prefix per command so the IDs match.

## configure pull

```bash
cpictl configure pull --output-dir ./config/dev                        # all packages
cpictl configure pull --output-dir ./config/dev --package-ids PkgA,PkgB
```

Writes one `<PackageID>.yml` per package in the `configure` format above, with the current
parameter values of all integration flows. Packages that do not exist are skipped with a
warning. Config keys: `configure.pull.outputDir`, `configure.pull.packageIds`.
