# Command reference

<!-- Generated from the CLI by `go test ./internal/cmd -run TestCommandReference -update`. Do not edit. -->

Every flag can also be set with an environment variable (`CPICTL_` + flag name in upper case, `-` replaced by `_`) or as a top-level key in the config file. See [configuration.md](configuration.md).

| Command | Description |
|---------|-------------|
| [`artifacts`](#artifacts) | List designtime artifacts of a package |
| [`config-generate`](#config-generate) | Generate or refresh the orchestrator deployment config from a packages directory |
| [`configure`](#configure) | Set artifact parameters from YAML files and optionally deploy |
| [`configure pull`](#configure-pull) | Write current tenant parameter values into configure YAML files |
| [`deploy`](#deploy) | Deploy designtime artifacts and wait for the result |
| [`download`](#download) | Download a designtime artifact and extract it into a directory |
| [`endpoints`](#endpoints) | List the URLs of deployed integration flows |
| [`guidelines`](#guidelines) | Check an integration flow against the design guidelines activated on the tenant |
| [`logs`](#logs) | Query message processing logs |
| [`logs attachment`](#logs-attachment) | Download a log attachment (ID from 'logs get') |
| [`logs get`](#logs-get) | Show one message: status, error text, custom headers, attachments |
| [`logs payload`](#logs-payload) | Download a persisted message (message store entry ID from 'logs get') |
| [`logs steps`](#logs-steps) | Show the processing steps of a message and the step that failed |
| [`mcp`](#mcp) | Run the MCP server (stdio) for AI agents |
| [`orchestrator`](#orchestrator) | Update and deploy many packages from a local directory tree |
| [`packages`](#packages) | List integration packages |
| [`params`](#params) | Read or change externalised parameters of an integration flow |
| [`params get`](#params-get) | Show the parameters of an integration flow |
| [`params set`](#params-set) | Set parameters of an integration flow (deploy afterwards to activate) |
| [`pd-deploy`](#pd-deploy) | Upload Partner Directory parameters from local files |
| [`pd-snapshot`](#pd-snapshot) | Download Partner Directory parameters into local files |
| [`resources`](#resources) | List the resources (scripts, mappings, schemas, ...) of an integration flow |
| [`resources get`](#resources-get) | Download one resource of an integration flow |
| [`snapshot`](#snapshot) | Save all integration packages of the tenant to a Git repository |
| [`snapshot restore`](#snapshot-restore) | Create or update integration packages on the tenant from a Git repository |
| [`status`](#status) | Show runtime status, version and errors of artifacts |
| [`sync`](#sync) | Synchronise the artifacts of a package between tenant and Git |
| [`sync apiproduct`](#sync-apiproduct) | Synchronise API Management products between tenant and Git |
| [`sync apiproxy`](#sync-apiproxy) | Synchronise API Management proxies (with dependent artifacts) between tenant and Git |
| [`undeploy`](#undeploy) | Remove artifacts from runtime and wait until they are gone |
| [`update`](#update) | Create or update designtime artifacts and packages |
| [`update artifact`](#update-artifact) | Create or update a designtime artifact from a local directory |
| [`update package`](#update-package) | Create or update an integration package from a JSON file |
| [`validate`](#validate) | Validate an integration flow on the tenant (like Check in the Web UI) |

## Global flags

```
      --config string               config file (default is $HOME/cpictl.yaml)
      --debug                       Show debug logs
      --oauth-clientid string       Client ID for using OAuth
      --oauth-clientsecret string   Client Secret for using OAuth
      --oauth-host string           OAuth token server host
      --oauth-path string           Path for OAuth token server (default "/oauth/token")
      --output string               Output format: text or json. With json the result is written to stdout as one JSON document and logs are written to stderr as JSON lines (default "text")
      --tmn-host string             Tenant host of Cloud Integration (or API portal host for API Management)
      --tmn-password string         Password for Basic Auth
      --tmn-userid string           User ID for Basic Auth
```

## artifacts

List designtime artifacts of a package

**Usage:** `cpictl artifacts [flags]`

**Flags:**

```
      --package-id string   Integration package ID
```

## config-generate

Generate or refresh the orchestrator deployment config from a packages directory

```
Generate or update deployment configuration from package directory structure.

This command scans the packages directory and generates/updates a deployment configuration
file (001-deploy-config.yml) with all discovered packages and artifacts.

Features:
  - Extracts package metadata from {PackageName}.json files
  - Extracts artifact display names from MANIFEST.MF (Bundle-Name)
  - Extracts artifact types from MANIFEST.MF (SAP-BundleType)
  - Preserves existing configuration settings (sync/deploy flags, config overrides)
  - Smart merging of new and existing configurations
  - Filter by specific packages or artifacts
```

**Usage:** `cpictl config-generate [flags]`

**Flags:**

```
      --artifact-filter strings   Comma separated list of artifacts to include (e.g., 'Artifact1,Artifact2')
      --output-file string        Path to output configuration file (default "./001-deploy-config.yml")
      --package-filter strings    Comma separated list of packages to include (e.g., 'Package1,Package2')
      --packages-dir string       Path to packages directory (default "./packages")
```

**Examples:**

```
  # Generate config with defaults
  cpictl config-generate

  # Specify custom directories
  cpictl config-generate --packages-dir ./my-packages --output-file ./my-config.yml

  # Generate config for specific packages only
  cpictl config-generate --package-filter "DeviceManagement,GenericPipeline"

  # Generate config for specific artifacts only
  cpictl config-generate --artifact-filter "MDMEquipmentMutationOutbound,GenericBroadcaster"

  # Combine package and artifact filters
  cpictl config-generate --package-filter "DeviceManagement" --artifact-filter "MDMEquipmentMutationOutbound"
```

## configure

Set artifact parameters from YAML files and optionally deploy

```
Set externalised parameters of many artifacts from YAML files and optionally
deploy them afterwards.

Phase 1 writes the parameters (OData $batch by default, falling back to single
requests), phase 2 deploys artifacts marked with deploy: true, package by
package with up to --parallel-deployments concurrent deployments.

--config-path accepts a file or a folder (all *.yml/*.yaml files, not
recursive). Generate files with the current tenant values with
'cpictl configure pull'. File format: docs/configure.md.

All flags can be set in the config file under 'configure'.
```

**Usage:** `cpictl configure [flags]`

**Flags:**

```
      --artifact-filter string     Comma-separated list of artifacts to include (config: configure.artifactFilter)
      --batch-size int             Number of parameters per batch request (config: configure.batchSize, default: 90)
  -c, --config-path string         Path to configuration YAML file (config: configure.configPath)
      --deploy-delay int           Delay in seconds between deployment status checks (config: configure.deployDelaySeconds, default: 15)
      --deploy-retries int         Number of retries for deployment status checks (config: configure.deployRetries, default: 5)
  -p, --deployment-prefix string   Deployment prefix for artifact IDs (config: configure.deploymentPrefix)
      --disable-batch              Disable batch processing, use individual requests (config: configure.disableBatch)
      --dry-run                    Show what would be done without making changes (config: configure.dryRun)
      --package-filter string      Comma-separated list of packages to include (config: configure.packageFilter)
      --parallel-deployments int   Number of parallel deployments (config: configure.parallelDeployments, default: 3)
```

**Examples:**

```
  # Configure artifacts from a config file
  cpictl configure --config-path ./config/dev-config.yml

  # Configure and deploy
  cpictl configure --config-path ./config/prod-config.yml

  # Dry run to see what would be changed
  cpictl configure --config-path ./config.yml --dry-run

  # Apply deployment prefix
  cpictl configure --config-path ./config.yml --deployment-prefix DEV_

  # Disable batch processing
  cpictl configure --config-path ./config.yml --disable-batch
```

## configure pull

Write current tenant parameter values into configure YAML files

**Usage:** `cpictl configure pull [flags]`

**Flags:**

```
  -o, --output-dir string     Directory for one YAML file per package (default ".")
      --package-ids strings   Package IDs to pull (default: all packages)
```

## deploy

Deploy designtime artifacts and wait for the result

```
Deploy artifact from designtime to
runtime of SAP Integration Suite tenant.

Configuration:
  Settings can be loaded from the global config file (--config) under the
  'deploy' section. CLI flags override config file settings.
```

**Usage:** `cpictl deploy [flags]`

**Flags:**

```
      --artifact-ids strings   Comma separated list of artifact IDs (config: deploy.artifactIds)
      --artifact-type string   Artifact type. Allowed values: Integration, MessageMapping, ScriptCollection, ValueMapping (config: deploy.artifactType) (default "Integration")
      --compare-versions       Perform version comparison of design time against runtime before deployment (config: deploy.compareVersions) (default true)
      --delay-length int       Delay (in seconds) between each check of artifact deployment status (config: deploy.delayLength) (default 30)
      --max-check-limit int    Max number of times to check for artifact deployment status (config: deploy.maxCheckLimit) (default 10)
```

## download

Download a designtime artifact and extract it into a directory

**Usage:** `cpictl download [flags]`

**Flags:**

```
      --artifact-id string     Artifact ID
      --artifact-type string   Artifact type: Integration, MessageMapping, ScriptCollection, ValueMapping (default "Integration")
      --dir string             Target directory (must be empty unless --overwrite)
      --overwrite              Replace the content of a non-empty target directory
      --version string         Designtime version (default "active")
```

**Examples:**

```
  cpictl download --artifact-id OrderIntake --dir ./OrderIntake
  cpictl download --artifact-id OrderMapping --artifact-type MessageMapping --dir ./OrderMapping --overwrite
```

## endpoints

List the URLs of deployed integration flows

**Usage:** `cpictl endpoints [flags]`

**Flags:**

```
      --artifact-id string   Only endpoints of this integration flow
```

**Examples:**

```
  cpictl endpoints --artifact-id OrderIntake
```

## guidelines

Check an integration flow against the design guidelines activated on the tenant

```
Run the design guidelines that the tenant administrator activated against an
integration flow and wait for the result. Violations (not compliant, not
skipped) give exit code 5.
```

**Usage:** `cpictl guidelines [flags]`

**Flags:**

```
      --artifact-id string   Integration flow ID
      --timeout duration     Maximum time to wait for the result (default 2m0s)
      --version string       Designtime version (default "active")
```

**Examples:**

```
  cpictl guidelines --artifact-id OrderIntake --output json
```

## logs

Query message processing logs

```
Query message processing logs (MPL) of the runtime, newest first.

--since/--until take a duration back from now (30m, 2h, 1d) or an RFC 3339
timestamp. --wait polls until at least one matching message exists and all
matching messages reached a final status (COMPLETED, FAILED, ESCALATED,
CANCELLED, DISCARDED, ABANDONED); use it after sending a test message with
--since set to the send time. Exit code 6 if the wait times out.

Statuses: COMPLETED, PROCESSING, RETRY, ESCALATED, FAILED, CANCELLED, DISCARDED, ABANDONED
```

**Usage:** `cpictl logs [flags]`

**Flags:**

```
      --application-message-id string   Application message ID
      --artifact-id string              Integration flow ID
      --correlation-id string           Correlation ID
      --errors                          Include the error text of failed messages
      --since string                    Messages that ended after this time (duration like 1h or RFC 3339)
      --skip int                        Skip the first n messages
      --status strings                  Comma separated statuses, e.g. FAILED,RETRY
      --top int                         Maximum number of messages (max 200) (default 20)
      --until string                    Messages that started before this time (duration like 1h or RFC 3339)
      --wait duration                   Wait up to this long for final messages (e.g. 60s)
```

**Examples:**

```
  cpictl logs --artifact-id OrderIntake --since 1h
  cpictl logs --artifact-id OrderIntake --status FAILED --errors --output json
  cpictl logs --artifact-id OrderIntake --since 2m --wait 60s --errors
  cpictl logs get --message-guid AFq478Bblxi4wCjBcDb_G0vAGGZG
```

## logs attachment

Download a log attachment (ID from 'logs get')

**Usage:** `cpictl logs attachment [flags]`

**Flags:**

```
      --id string       Attachment ID
      --max-bytes int   Maximum bytes returned in the JSON result (default 65536; 0 with --out: unlimited)
      --out string      Write the content to this file instead of stdout / the JSON result
```

## logs get

Show one message: status, error text, custom headers, attachments

**Usage:** `cpictl logs get [flags]`

**Flags:**

```
      --message-guid string   Message GUID
```

## logs payload

Download a persisted message (message store entry ID from 'logs get')

**Usage:** `cpictl logs payload [flags]`

**Flags:**

```
      --id string       Message store entry ID
      --max-bytes int   Maximum bytes returned in the JSON result (default 65536; 0 with --out: unlimited)
      --out string      Write the content to this file instead of stdout / the JSON result
```

## logs steps

Show the processing steps of a message and the step that failed

**Usage:** `cpictl logs steps [flags]`

**Flags:**

```
      --message-guid string   Message GUID
```

## mcp

Run the MCP server (stdio) for AI agents

```
Run a Model Context Protocol server on stdin/stdout.

The server uses the same tenant settings as every other command (flags,
CPICTL_* environment variables or cpictl.yaml). stdout carries the
protocol only; logs go to stderr as JSON lines.

Tools: list/get packages, artifacts, resources and parameters; download, upload,
validate, guideline check, deploy, undeploy (requires confirm=true); runtime
status and endpoints; message logs, steps, attachments and persisted messages;
pd_deploy (dry run unless dry_run=false). See docs/mcp.md.

Local paths given to tools are resolved against --root and may not leave it.
```

**Usage:** `cpictl mcp [flags]`

**Flags:**

```
      --max-checks int      Default maximum number of deploy/undeploy status checks (default 30)
      --poll-interval int   Default seconds between deploy/undeploy status checks (default 10)
      --root string         Directory that local paths of tool calls are confined to (default ".")
```

**Examples:**

```
  # Claude Code / any MCP client configuration
  {"mcpServers": {"cpi": {"command": "cpictl", "args": ["mcp", "--root", "/path/to/repo"],
    "env": {"CPICTL_TMN_HOST": "...", "CPICTL_OAUTH_HOST": "...",
            "CPICTL_OAUTH_CLIENTID": "...", "CPICTL_OAUTH_CLIENTSECRET": "..."}}}}
```

## orchestrator

Update and deploy many packages from a local directory tree

```
Orchestrate the complete deployment lifecycle for SAP CPI artifacts.

This command handles:
  - Updates artifacts in SAP CPI tenant with modified MANIFEST.MF and parameters
  - Deploys artifacts to make them active (in parallel for faster execution)
  - Supports deployment prefixes for multi-environment scenarios
  - Intelligent artifact grouping by type for efficient deployment
  - Filter by specific packages or artifacts
  - Load configs from files, folders, or remote URLs
  - Configure via YAML file for repeatable deployments

Configuration Sources:
  The --deploy-config flag accepts:
  - Single file:      ./001-deploy-config.yml
  - Folder:           ./configs (processes all matching files alphabetically)
  - Remote URL:       https://raw.githubusercontent.com/org/repo/main/config.yml (public, no authentication)

  All flags can also be set in the global config file (--config) under
  the 'orchestrator' key; CLI flags override config file settings.

Operation Modes:
  --update          Update and deploy artifacts (default)
  --update-only     Only update artifacts, don't deploy
  --deploy-only     Only deploy artifacts, don't update

Deployment Strategy:
  1. Update Phase: All packages and artifacts are updated first
  2. Deploy Phase: All artifacts are deployed in parallel
     - Deployments are triggered concurrently per package
     - Status is polled for all deployments simultaneously
     - Configurable parallelism and retry settings

Configuration:
  Settings can be loaded from the global config file (--config) under the
  'orchestrator' section. CLI flags override config file settings.
```

**Usage:** `cpictl orchestrator [flags]`

**Flags:**

```
      --artifact-filter string     Comma-separated list of artifacts to include (config: orchestrator.artifactFilter)
      --config-pattern string      File pattern for config files in folders (config: orchestrator.configPattern) (default "*.y*ml")
  -c, --deploy-config string       Path to deployment config file/folder/URL (config: orchestrator.deployConfig)
      --deploy-delay int           Delay in seconds between deployment status checks (config: orchestrator.deployDelaySeconds, default: 15)
      --deploy-only                Only deploy artifacts, don't update
      --deploy-retries int         Number of retries for deployment status checks (config: orchestrator.deployRetries, default: 5)
  -p, --deployment-prefix string   Deployment prefix for package/artifact IDs (config: orchestrator.deploymentPrefix)
      --keep-temp                  Keep temporary directory after execution (config: orchestrator.keepTemp)
      --merge-configs              Merge multiple configs into single deployment (config: orchestrator.mergeConfigs)
      --package-filter string      Comma-separated list of packages to include (config: orchestrator.packageFilter)
  -d, --packages-dir string        Directory containing packages (config: orchestrator.packagesDir)
      --parallel-deployments int   Number of parallel deployments per package (config: orchestrator.parallelDeployments, default: 3)
      --update                     Update and deploy artifacts
      --update-only                Only update artifacts, don't deploy
```

**Examples:**

```
  # Update and deploy with config from global cpictl.yaml
  cpictl orchestrator --update

  # Load specific config file
  cpictl orchestrator --config ./my-config.yml --update

  # Override settings via CLI flags
  cpictl orchestrator --config ./my-config.yml \
    --deployment-prefix DEV --parallel-deployments 5
```

## packages

List integration packages

**Usage:** `cpictl packages`

## params

Read or change externalised parameters of an integration flow

## params get

Show the parameters of an integration flow

**Usage:** `cpictl params get [flags]`

**Flags:**

```
      --artifact-id string   Integration flow ID
      --version string       Designtime version (default "active")
```

## params set

Set parameters of an integration flow (deploy afterwards to activate)

**Usage:** `cpictl params set [flags]`

**Flags:**

```
      --artifact-id string   Integration flow ID
      --dry-run              Show what would change without writing
      --param stringArray    Parameter as key=value (repeatable)
      --version string       Designtime version (default "active")
```

**Examples:**

```
  cpictl params set --artifact-id MyIFlow --param Host=example.com --param Port=443
```

## pd-deploy

Upload Partner Directory parameters from local files

```
Upload all partner directory parameters from local files to SAP CPI.

This command reads partner directory parameters from a local directory structure
and uploads them to the SAP CPI Partner Directory:

  {PID}/
    String.properties    - String parameters as key=value pairs
    Binary/              - Binary parameters as individual files
      {ParamId}.{ext}    - Binary parameter files
      _metadata.json     - Content type metadata

The deploy operation supports several modes:
  - Replace mode (default): Updates existing parameters with local values
  - Add-only mode: Only creates new parameters, skips existing ones
  - Full sync mode: Deletes remote parameters not present locally (local is source of truth)

See docs/partner-directory.md for the file format and full sync safety rules.
```

**Usage:** `cpictl pd-deploy [flags]`

**Flags:**

```
      --dry-run                 Show what would be changed without making changes
      --full-sync               Delete remote parameters not present locally (local is source of truth)
      --pids strings            Comma separated list of Partner IDs to deploy (e.g., 'PID1,PID2')
      --replace                 Replace existing values (false = add only missing values) (default true)
      --resources-path string   Path to partner directory parameters (default "./partner-directory")
```

**Examples:**

```
  # Deploy in add-only mode (don't update existing parameters)
  cpictl pd-deploy --replace=false

  # Deploy with full sync (delete remote parameters not in local)
  cpictl pd-deploy --full-sync

  # Deploy only specific PIDs
  cpictl pd-deploy --pids "SAP_SYSTEM_001,CUSTOMER_API"

  # Dry run to see what would be changed
  cpictl pd-deploy --dry-run
```

## pd-snapshot

Download Partner Directory parameters into local files

```
Download all partner directory parameters from SAP CPI and save them locally.

This command retrieves both string and binary parameters from the SAP CPI Partner Directory
and organizes them in a local directory structure:

  {PID}/
    String.properties    - String parameters as key=value pairs
    Binary/              - Binary parameters as individual files
      {ParamId}.{ext}    - Binary parameter files
      _metadata.json     - Content type metadata

The snapshot operation supports two modes:
  - Replace mode (default): Overwrites existing local files
  - Add-only mode: Only adds new parameters, preserves existing values

See docs/partner-directory.md for the file format and full sync safety rules.
```

**Usage:** `cpictl pd-snapshot [flags]`

**Flags:**

```
      --pids strings            Comma separated list of Partner IDs to snapshot (e.g., 'PID1,PID2')
      --replace                 Replace existing values (false = add only missing values) (default true)
      --resources-path string   Path to save partner directory parameters (default "./partner-directory")
```

**Examples:**

```
  # Snapshot in add-only mode (don't overwrite existing values)
  cpictl pd-snapshot --replace=false

  # Snapshot only specific PIDs
  cpictl pd-snapshot --pids "SAP_SYSTEM_001,CUSTOMER_API"
```

## resources

List the resources (scripts, mappings, schemas, ...) of an integration flow

**Usage:** `cpictl resources [flags]`

**Flags:**

```
      --artifact-id string   Integration flow ID
      --version string       Designtime version (default "active")
```

**Examples:**

```
  cpictl resources --artifact-id OrderIntake
  cpictl resources get --artifact-id OrderIntake --name script1.groovy --type groovy
```

## resources get

Download one resource of an integration flow

```
Download one resource. In text mode the content is written to stdout (or
--out); with --output json it is part of the result document.
```

**Usage:** `cpictl resources get [flags]`

**Flags:**

```
      --artifact-id string   Integration flow ID
      --max-bytes int        Maximum bytes returned in the JSON result (default 65536; 0 with --out: unlimited)
      --name string          Resource name, e.g. script1.groovy
      --out string           Write the content to this file instead of stdout / the JSON result
      --type string          Resource type, e.g. groovy, xslt, mmap, xsd, wsdl, jar
      --version string       Designtime version (default "active")
```

## snapshot

Save all integration packages of the tenant to a Git repository

```
Snapshot all editable integration packages from SAP Integration Suite
tenant to a Git repository.

Configuration:
  Settings can be loaded from the global config file (--config) under the
  'snapshot' section. CLI flags override config file settings.
```

**Usage:** `cpictl snapshot [flags]`

**Flags:**

```
      --draft-handling string     Handling when artifact is in draft version. Allowed values: SKIP, ADD, ERROR (config: snapshot.draftHandling) (default "SKIP")
      --git-commit-email string   Email used in commit (config: snapshot.gitCommitEmail) (default "41898282+github-actions[bot]@users.noreply.github.com")
      --git-commit-msg string     Message used in commit (config: snapshot.gitCommitMsg) (default "Tenant snapshot of <current time>")
      --git-commit-user string    User used in commit (config: snapshot.gitCommitUser) (default "github-actions[bot]")
      --git-skip-commit           Skip committing changes to Git repository (config: snapshot.gitSkipCommit)
      --sync-package-details      Sync details of Integration Packages (config: snapshot.syncPackageDetails) (default true)
```

## snapshot restore

Create or update integration packages on the tenant from a Git repository

```
Restore all editable integration packages from a Git repository to SAP Integration Suite tenant.

Configuration:
  Settings can be loaded from the global config file (--config) under the
  'restore' section. CLI flags override config file settings.
```

**Usage:** `cpictl snapshot restore`

## status

Show runtime status, version and errors of artifacts

```
Show the runtime status of artifacts. With --artifact-ids only those
artifacts are shown (including NOT_DEPLOYED ones); without, all deployed
artifacts, optionally filtered with --runtime-status (e.g. ERROR).
```

**Usage:** `cpictl status [flags]`

**Flags:**

```
      --artifact-ids strings     Comma separated list of artifact IDs (default: all deployed artifacts)
      --runtime-status strings   Without --artifact-ids: only artifacts in these statuses (STARTED, STARTING, ERROR, STOPPING)
```

**Examples:**

```
  cpictl status --artifact-ids MyIFlow,MyMapping --output json
  cpictl status --runtime-status ERROR
```

## sync

Synchronise the artifacts of a package between tenant and Git

```
Synchronise designtime artifacts between SAP Integration Suite
tenant and a Git repository.

Configuration:
  Settings can be loaded from the global config file (--config) under the
  'sync' section. CLI flags override config file settings.
```

**Usage:** `cpictl sync [flags]`

**Flags:**

```
      --dir-naming-type string          Name artifact directory by ID or Name. Allowed values: ID, NAME (config: sync.dirNamingType) (default "ID")
      --draft-handling string           Handling when artifact is in draft version. Allowed values: SKIP, ADD, ERROR (config: sync.draftHandling) (default "SKIP")
      --package-id string               ID of Integration Package (config: sync.packageId)
      --script-collection-map strings   Comma-separated source-target ID pairs for converting script collection references during sync (config: sync.scriptCollectionMap)
      --sync-package-details            Sync details of Integration Package (config: sync.syncPackageDetails)
```

## sync apiproduct

Synchronise API Management products between tenant and Git

```
Synchronise API Management products between SAP Integration Suite
tenant and a Git repository.

Configuration:
  Settings can be loaded from the global config file (--config) under the
  'sync.apiproduct' section. CLI flags override config file settings.
```

**Usage:** `cpictl sync apiproduct`

## sync apiproxy

Synchronise API Management proxies (with dependent artifacts) between tenant and Git

```
Synchronise API Management proxies (with dependent artifacts) between SAP Integration Suite
tenant and a Git repository.

Configuration:
  Settings can be loaded from the global config file (--config) under the
  'sync.apiproxy' section. CLI flags override config file settings.
```

**Usage:** `cpictl sync apiproxy`

## undeploy

Remove artifacts from runtime and wait until they are gone

```
Undeploy artifacts from the runtime of an SAP Integration Suite tenant.

For each artifact the runtime artifact is deleted and its status is polled
until the tenant no longer knows it (HTTP 404). Artifacts that are not
deployed are reported as NOT_DEPLOYED and are not treated as failures.
Designtime artifacts are not touched.

Configuration:
  Settings can be loaded from the global config file (--config) under the
  'undeploy' section. CLI flags override config file settings.
```

**Usage:** `cpictl undeploy [flags]`

**Flags:**

```
      --artifact-ids strings   Comma separated list of artifact IDs (config: undeploy.artifactIds)
      --delay-length int       Delay (in seconds) between each check of artifact undeployment status (config: undeploy.delayLength) (default 10)
      --max-check-limit int    Max number of times to check for artifact undeployment status (config: undeploy.maxCheckLimit) (default 30)
```

**Examples:**

```
  cpictl undeploy --artifact-ids MyIFlow,MyOtherIFlow
```

## update

Create or update designtime artifacts and packages

```
Create or update artifacts and/or integration package on the
SAP Integration Suite tenant.
```

## update artifact

Create or update a designtime artifact from a local directory

```
Create or update artifacts on the
SAP Integration Suite tenant.

Configuration:
  Settings can be loaded from the global config file (--config) under the
  'update.artifact' section. CLI flags override config file settings.
```

**Usage:** `cpictl update artifact [flags]`

**Flags:**

```
      --artifact-id string              ID of artifact (config: update.artifact.artifactId)
      --artifact-name string            Name of artifact. Defaults to artifact-id value when not provided (config: update.artifact.artifactName)
      --artifact-type string            Artifact type. Allowed values: Integration, MessageMapping, ScriptCollection, ValueMapping (config: update.artifact.artifactType) (default "Integration")
      --dir-artifact string             Directory containing contents of designtime artifact (config: update.artifact.dirArtifact)
      --dir-work string                 Working directory for in-transit files (config: update.artifact.dirWork) (default "/tmp")
      --file-manifest string            Use a different MANIFEST.MF file instead of the default in META-INF/ (config: update.artifact.fileManifest)
      --file-param string               Use a different parameters.prop file instead of the default in src/main/resources/ (config: update.artifact.fileParam)
      --package-id string               ID of Integration Package (config: update.artifact.packageId)
      --package-name string             Name of Integration Package. Defaults to package-id value when not provided (config: update.artifact.packageName)
      --script-collection-map strings   Comma-separated source-target ID pairs for converting script collection references during create/update (config: update.artifact.scriptCollectionMap)
```

## update package

Create or update an integration package from a JSON file

```
Create or update integration package on the
SAP Integration Suite tenant.

Configuration:
  Settings can be loaded from the global config file (--config) under the
  'update.package' section. CLI flags override config file settings.
```

**Usage:** `cpictl update package [flags]`

**Flags:**

```
      --package-file string   Path to location of package file (config: update.package.packageFile)
```

## validate

Validate an integration flow on the tenant (like Check in the Web UI)

**Usage:** `cpictl validate [flags]`

**Flags:**

```
      --artifact-id string   Integration flow ID
      --version string       Designtime version (default "active")
```

**Examples:**

```
  cpictl validate --artifact-id OrderIntake
```
