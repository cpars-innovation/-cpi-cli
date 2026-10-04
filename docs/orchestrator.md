# Orchestrator and config-generate

`cpictl orchestrator` updates and deploys many packages and artifacts from a local
directory tree in one run, driven by a deployment config file. `cpictl config-generate`
creates or refreshes that config file from the directory tree.

## Directory layout

```
packages/                          # --packages-dir
└── MyPackage/                     # packageDir
    ├── MyPackage.json             # optional package metadata ({"d": {"Id", "Name", "Description", "ShortText"}})
    └── MyIFlow/                   # artifactDir
        ├── META-INF/MANIFEST.MF
        └── src/main/resources/
            ├── parameters.prop
            └── scenarioflows/integrationflow/MyIFlow.iflw
```

This is the layout written by `cpictl snapshot` / `sync` (Git directory naming by ID).

## Deployment config

Example: [examples/deploy-config.yml](examples/deploy-config.yml).

```yaml
deploymentPrefix: DEV            # optional, see "Prefixes"
packages:
  - integrationSuiteId: MyPackage
    packageDir: MyPackage        # relative to --packages-dir
    displayName: My Package      # optional
    description: ...             # optional, defaults to the display name
    short_text: ...              # optional, defaults to the display name
    sync: true                   # default true: update package + artifacts on the tenant
    deploy: true                 # default true: deploy the package's artifacts
    artifacts:
      - artifactId: MyIFlow
        artifactDir: MyIFlow     # relative to packageDir
        displayName: My iFlow
        type: IntegrationFlow    # SAP-BundleType from MANIFEST.MF, see below
        sync: true               # default true
        deploy: true             # default true
        configOverrides:         # optional, merged into parameters.prop before upload
          ReceiverHost: dev.example.com
          Timeout: 30
```

`type` accepts the MANIFEST `SAP-BundleType` values (`IntegrationFlow`, `MessageMapping`,
`ScriptCollection`, `ValueMapping`; case-insensitive) or the API names (`Integration`, ...).
Unknown values are treated as integration flows.

`configOverrides` are only applied when the artifact has a `parameters.prop`
(`src/main/resources/`, `src/main/resources/script/` or the artifact root).

## Running

```bash
cpictl orchestrator --packages-dir ./packages --deploy-config ./001-deploy-config.yml
```

| Flag | Default | Description |
|------|---------|-------------|
| `--packages-dir`, `-d` | | Root of the package directories |
| `--deploy-config`, `-c` | | Config file, folder or public URL |
| `--update` / `--update-only` / `--deploy-only` | update + deploy | Operation mode |
| `--deployment-prefix`, `-p` | | Overrides `deploymentPrefix` of the config(s) |
| `--package-filter`, `--artifact-filter` | | Comma-separated IDs to include (IDs without prefix) |
| `--config-pattern` | `*.y*ml` | File pattern when `--deploy-config` is a folder |
| `--merge-configs` | `false` | Merge all config files into one run |
| `--parallel-deployments` | `3` | Concurrent deployments per package |
| `--deploy-retries` | `5` | Status checks per artifact |
| `--deploy-delay` | `15` | Seconds between status checks |
| `--keep-temp` | `false` | Keep the temporary working directory |

All of these can be set in the global config file under `orchestrator:` (camelCase keys, e.g.
`packagesDir`, `deployConfig`, `parallelDeployments`, `mode: update-only`).

### What happens

**Phase 1, update** (skipped with `--deploy-only`), per package with `sync: true`:

1. The package is created or updated (ID, name, description, short text).
2. Each artifact with `sync: true` is copied to a temporary directory; `MANIFEST.MF` gets the
   final ID and name (`Bundle-SymbolicName`, `Bundle-Name`), `configOverrides` are merged into
   `parameters.prop`, and the artifact is created or updated on the tenant if its content differs.
   If the content changed but the version did not, the running artifact is undeployed so that
   phase 2 deploys the new content.

**Phase 2, deploy** (skipped with `--update-only`): artifacts with `deploy: true` (whose update
did not fail) are deployed package by package, up to `--parallel-deployments` at a time. An
artifact whose runtime version already equals the designtime version is skipped.

With `--output json` the result contains the statistics and one deployment result per
artifact. Failures give exit code 7 when anything succeeded, otherwise 5.

### Config sources

- **File**: one config.
- **Folder**: all files matching `--config-pattern`, recursively, in alphabetical order.
  Without `--merge-configs` each file is processed on its own with its own prefix.
  With `--merge-configs` all packages are processed in one run; each file's prefix is applied
  to its packages, `--deployment-prefix` is ignored and duplicate final package IDs are an error.
- **URL**: `https://...` (public only; authentication is not supported).

### Prefixes

With prefix `DEV`:

| | Without prefix | With prefix |
|---|---|---|
| Package ID | `MyPackage` | `DEVMyPackage` |
| Package name | `My Package` | `DEV - My Package` |
| Artifact ID | `MyIFlow` | `DEV_MyIFlow` |

Prefixes may contain letters, digits and `_`. Note that `configure` uses a different rule
(prefix + ID for packages *and* artifacts, no `_`), see [configure.md](configure.md).

## config-generate

```bash
cpictl config-generate --packages-dir ./packages --output-file ./001-deploy-config.yml
```

- Scans `--packages-dir`: every sub-directory is a package, every sub-directory of a package
  is an artifact.
- Package name, description and short text come from `<Package>/<Package>.json`;
  artifact display name and type from `MANIFEST.MF` (`Bundle-Name`, `SAP-BundleType`).
- If the output file exists, its settings are preserved: `deploymentPrefix`, the
  `orchestrator` section, `sync`/`deploy` flags, display names, types and `configOverrides`.
  Missing `sync`/`deploy` keep their default `true`.
- Packages and artifacts that no longer exist on disk are removed from the file.
- `--package-filter` / `--artifact-filter` restrict the scan.

`config-generate` works offline: it needs no tenant settings.
