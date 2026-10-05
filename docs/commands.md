# Command reference

<!-- Generated from the CLI by `go test ./internal/cmd -run TestCommandReference -update`. Do not edit. -->

Every flag can also be set with an environment variable (`CPICTL_` + flag name in upper case, `-` replaced by `_`) or as a top-level key in the config file. See [configuration.md](configuration.md).

| Command | Description |
|---------|-------------|
| [`artifacts`](#artifacts) | List designtime artifacts of a package |
| [`config-generate`](#config-generate) | Generate or refresh the orchestrator deployment config from a packages directory |
| [`configure`](#configure) | Set artifact parameters from YAML files and optionally deploy |
| [`configure pull`](#configure-pull) | Write current tenant parameter values into configure YAML files |
| [`credentials`](#credentials) | List and deploy user credentials, OAuth2 client credentials and secure parameters |
| [`credentials apply`](#credentials-apply) | Create or update all credentials of a YAML file |
| [`credentials delete`](#credentials-delete) | Delete a credential (requires --confirm) |
| [`credentials list`](#credentials-list) | List credentials (names and metadata, never secrets) |
| [`credentials set-oauth2`](#credentials-set-oauth2) | Create or update an OAuth2 client credential |
| [`credentials set-secure-param`](#credentials-set-secure-param) | Create or update a secure parameter (Neo environment) |
| [`credentials set-user`](#credentials-set-user) | Create or update a user credential |
| [`deploy`](#deploy) | Deploy designtime artifacts and wait for the result |
| [`discover`](#discover) | Inventory existing integration flows to derive conventions |
| [`download`](#download) | Download a designtime artifact and extract it into a directory |
| [`endpoints`](#endpoints) | List the URLs of deployed integration flows |
| [`guidelines`](#guidelines) | Check an integration flow against the design guidelines activated on the tenant |
| [`keystore`](#keystore) | List keystore entries, check expiry, export and import certificates |
| [`keystore export-cert`](#keystore-export-cert) | Export the certificate of a keystore entry as PEM |
| [`keystore import-cert`](#keystore-import-cert) | Import a certificate (PEM or DER) into the tenant keystore |
| [`keystore list`](#keystore-list) | List keystore entries with remaining validity |
| [`log-level`](#log-level) | Set the message processing log level of a deployed integration flow |
| [`logs`](#logs) | Query message processing logs |
| [`logs attachment`](#logs-attachment) | Download a log attachment (ID from 'logs get') |
| [`logs get`](#logs-get) | Show one message: status, error text, custom headers, attachments |
| [`logs payload`](#logs-payload) | Download a persisted message (message store entry ID from 'logs get') |
| [`logs steps`](#logs-steps) | Show the processing steps of a message and the step that failed |
| [`logs trace`](#logs-trace) | List the traced steps of a message (flow on log level TRACE) |
| [`logs trace-message`](#logs-trace-message) | Show payload, headers and exchange properties of a traced step (ID from 'logs trace') |
| [`logs tree`](#logs-tree) | Show the call tree of a trace across flows and its first failure |
| [`mcp`](#mcp) | Run the MCP server (stdio) for AI agents |
| [`orchestrator`](#orchestrator) | Update and deploy many packages from a local directory tree |
| [`packages`](#packages) | List integration packages |
| [`packages create`](#packages-create) | Create an integration package if it does not exist |
| [`params`](#params) | Read or change externalised parameters of an integration flow |
| [`params get`](#params-get) | Show the parameters of an integration flow |
| [`params set`](#params-set) | Set parameters of an integration flow (deploy afterwards to activate) |
| [`pd`](#pd) | Inspect Partner Directory parameters (get, diff, deps) |
| [`pd deps`](#pd-deps) | Show which flows read which Partner Directory parameters (local files) |
| [`pd diff`](#pd-diff) | Compare local Partner Directory files with the tenant |
| [`pd get`](#pd-get) | Show the Partner Directory parameters of a partner ID on the tenant |
| [`pd-deploy`](#pd-deploy) | Upload Partner Directory parameters from local files |
| [`pd-snapshot`](#pd-snapshot) | Download Partner Directory parameters into local files |
| [`profile`](#profile) | Switch between tenants (profiles in $HOME/.cpictl) |
| [`profile current`](#profile-current) | Show the profile that commands use now |
| [`profile list`](#profile-list) | List profiles and mark the active one |
| [`profile use`](#profile-use) | Make a profile the default (stored in $HOME/.cpictl/current); 'use -' clears it |
| [`resources`](#resources) | List the resources (scripts, mappings, schemas, ...) of an integration flow |
| [`resources get`](#resources-get) | Download one resource of an integration flow |
| [`send`](#send) | Send a test message to a deployed integration flow |
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
      --config string               config file (default: the profile, else $CPICTL_CONFIG, else $HOME/cpictl.yaml plus ./cpictl.yaml of the repository)
      --debug                       Show debug logs
      --oauth-clientid string       Client ID for using OAuth
      --oauth-clientsecret string   Client Secret for using OAuth
      --oauth-host string           OAuth token server host
      --oauth-path string           Path for OAuth token server (default "/oauth/token")
      --output string               Output format: text or json. With json the result is written to stdout as one JSON document and logs are written to stderr as JSON lines (default "text")
      --profile string              Profile to use: $HOME/.cpictl/<name>.yaml (default: $CPICTL_PROFILE, else the one chosen with 'cpictl profile use')
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

Phase 1 compares each artifact's parameters with the tenant and writes only the
ones that differ (OData $batch by default, falling back to single requests); an
unknown key fails the artifact before anything is written. Phase 2 deploys the
artifacts marked with deploy: true that had a change, package by package with up
to --parallel-deployments concurrent deployments. --force writes and deploys
everything as before.

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
      --force                      Write all parameters and deploy all marked artifacts, even if the tenant already has the values
      --offline                    With --dry-run: only show the file contents, do not read the tenant
      --package-filter string      Comma-separated list of packages to include (config: configure.packageFilter)
      --parallel-deployments int   Number of parallel deployments (config: configure.parallelDeployments, default: 3)
```

**Examples:**

```
  # Configure artifacts from a config file
  cpictl configure --config-path ./config/dev-config.yml

  # Configure and deploy
  cpictl configure --config-path ./config/prod-config.yml

  # Dry run: compare with the tenant (what would be written and deployed)
  cpictl configure --config-path ./config.yml --dry-run

  # Only show the file, without reading the tenant
  cpictl configure --config-path ./config.yml --dry-run --offline

  # Write everything and redeploy, even if the tenant has the values
  cpictl configure --config-path ./config.yml --force

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

## credentials

List and deploy user credentials, OAuth2 client credentials and secure parameters

```
Manage security material referenced by integration flows.

Secrets are never accepted as flag values (they would end up in shell history
and process lists): use --*-env, --*-file or --*-stdin, or 'credentials apply'
with a YAML file that references environment variables or files.
Listing never returns secrets.
```

## credentials apply

Create or update all credentials of a YAML file

```
Deploy every credential listed in a YAML file. Secrets are references only:

  userCredentials:
    - name: ERP_User
      user: svc_erp
      password: {env: ERP_PASSWORD}
  oauth2Credentials:
    - name: Graph
      tokenServiceUrl: https://login.example.com/oauth/token
      clientId: my-app
      clientSecret: {file: secrets/graph.txt}   # relative to the YAML file
  secureParameters:
    - name: ApiKey
      value: {env: API_KEY}

All secrets are resolved before anything is written. Inline secrets and
unknown keys are rejected.
```

**Usage:** `cpictl credentials apply [flags]`

**Flags:**

```
      --dry-run       Resolve all secrets and validate, without writing
      --file string   Credentials YAML file
```

## credentials delete

Delete a credential (requires --confirm)

**Usage:** `cpictl credentials delete [flags]`

**Flags:**

```
      --confirm       Confirm the deletion
      --kind string   Kind: user, oauth2, secure-param
      --name string   Credential name
```

## credentials list

List credentials (names and metadata, never secrets)

**Usage:** `cpictl credentials list [flags]`

**Flags:**

```
      --kind string   Only this kind: user, oauth2, secure-param
```

## credentials set-oauth2

Create or update an OAuth2 client credential

**Usage:** `cpictl credentials set-oauth2 [flags]`

**Flags:**

```
      --audience string             Audience
      --client-auth string          Client authentication: body or header (default "body")
      --client-id string            Client ID
      --description string          Description
      --dry-run                     Check the input without writing
      --name string                 Credential name
      --resource string             Resource
      --scope string                Scope
      --scope-content-type string   urlencoded or json (default "urlencoded")
      --secret-env string           Read the client secret from this environment variable
      --secret-file string          Read the client secret from this file
      --secret-stdin                Read the client secret from stdin
      --token-url string            Token service URL (https)
```

**Examples:**

```
  cpictl credentials set-oauth2 --name Graph --token-url https://login/token --client-id app --secret-file ./graph.secret
```

## credentials set-secure-param

Create or update a secure parameter (Neo environment)

**Usage:** `cpictl credentials set-secure-param [flags]`

**Flags:**

```
      --description string   Description
      --dry-run              Check the input without writing
      --name string          Parameter name
      --value-env string     Read the value from this environment variable
      --value-file string    Read the value from this file
      --value-stdin          Read the value from stdin
```

## credentials set-user

Create or update a user credential

**Usage:** `cpictl credentials set-user [flags]`

**Flags:**

```
      --company-id string      Company ID (SuccessFactors)
      --description string     Description
      --dry-run                Check the input without writing
      --kind string            default, successfactors or openconnectors (default "default")
      --name string            Credential name (as referenced in the iFlow)
      --password-env string    Read the password from this environment variable
      --password-file string   Read the password from this file
      --password-stdin         Read the password from stdin
      --user string            User name
```

**Examples:**

```
  ERP_PASSWORD=... cpictl credentials set-user --name ERP_User --user svc_erp --password-env ERP_PASSWORD
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
      --allow-downgrade        Deploy even if the designtime version is older than the running version (config: deploy.allowDowngrade)
      --artifact-ids strings   Comma separated list of artifact IDs (config: deploy.artifactIds)
      --artifact-type string   Artifact type. Allowed values: Integration, MessageMapping, ScriptCollection, ValueMapping (config: deploy.artifactType) (default "Integration")
      --compare-versions       Perform version comparison of design time against runtime before deployment (config: deploy.compareVersions) (default true)
      --delay-length int       Delay (in seconds) between each check of artifact deployment status (config: deploy.delayLength) (default 30)
      --max-check-limit int    Max number of times to check for artifact deployment status (config: deploy.maxCheckLimit) (default 10)
```

## discover

Inventory existing integration flows to derive conventions

```
Inventory the packages and integration flows of the tenant (or of a local
directory with --dir) and write the facts as JSON: adapters, steps, exception
subprocesses, log levels, scripts (identical scripts across flows), externalised
parameter keys, credential names, headers and properties that are set, and
naming patterns of packages and flows.

The tenant is only read; every integration flow is downloaded and analysed in
memory. The file is the input for writing a repository's conventions (see the
cpi-discover skill of the Claude Code plugin).
```

**Usage:** `cpictl discover [flags]`

**Flags:**

```
      --dir string            Analyse this local directory instead of the tenant
      --max-iflows int        Stop after this many integration flows (0: all)
      --output-file string    JSON file to write (default ".cpi/discovery.json")
      --package-ids strings   Only these packages (default: all)
```

**Examples:**

```
  cpictl discover --output-file .cpi/discovery.json
  cpictl discover --package-ids SalesOrders,Finance
  cpictl discover --dir ./content   # local repository from 'sync' or 'snapshot', offline
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

## keystore

List keystore entries, check expiry, export and import certificates

## keystore export-cert

Export the certificate of a keystore entry as PEM

**Usage:** `cpictl keystore export-cert [flags]`

**Flags:**

```
      --alias string      Keystore alias
      --keystore string   Keystore: system, backup_admin_system, KeyRenewal, KeyHistory (default "system")
      --out string        Write the PEM to this file instead of stdout
```

## keystore import-cert

Import a certificate (PEM or DER) into the tenant keystore

**Usage:** `cpictl keystore import-cert [flags]`

**Flags:**

```
      --alias string   Keystore alias
      --file string    Certificate file (PEM or DER)
      --update         Replace an existing entry with the same alias
```

**Examples:**

```
  cpictl keystore import-cert --alias partner_acme --file acme.pem
```

## keystore list

List keystore entries with remaining validity

**Usage:** `cpictl keystore list [flags]`

**Flags:**

```
      --expiring-within string   Flag entries expiring within this period (e.g. 30d, 720h)
      --fail-on-expiry           Exit with code 5 if an entry is expired or expiring
      --keystore string          Keystore: system, backup_admin_system, KeyRenewal, KeyHistory (default "system")
```

**Examples:**

```
  cpictl keystore list --expiring-within 30d
  cpictl keystore list --expiring-within 30d --fail-on-expiry   # exit 5 in CI
```

## log-level

Set the message processing log level of a deployed integration flow

```
Set the log level of a deployed integration flow: NONE, INFO, DEBUG or TRACE.
TRACE records payload and headers at every step for 10 minutes, then the tenant
falls back to the previous level; read traces with 'logs trace'. Traces contain
business data: use them on development tenants.

Uses the operations command of the Web UI (there is no OData API for it).
```

**Usage:** `cpictl log-level [flags]`

**Flags:**

```
      --artifact-id string           Integration flow ID (deployed)
      --level string                 NONE, INFO, DEBUG or TRACE
      --node-type string             Runtime node type (default "IFLMAP")
      --runtime-location-id string   Runtime location (edge integration cells use their own) (default "cloudintegration")
```

**Examples:**

```
  cpictl log-level --artifact-id OrderIntake --level TRACE
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
      --header string                   Only messages with this custom header property, name=value (client-side scan; needs --artifact-id or --package-id, and --since)
      --package-id string               Messages of all integration flows of this package (needs --since)
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
  cpictl logs --package-id Orders --since 1h --header OrderId=4711
  cpictl logs get --message-guid AFq478Bblxi4wCjBcDb_G0vAGGZG
  cpictl logs tree --trace-id 0af7651916cd43dd8448eb211c80319c
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

## logs trace

List the traced steps of a message (flow on log level TRACE)

```
List the steps of a message that was processed with log level TRACE and the
trace IDs of the message at each step. Read one with 'logs trace-message'.
Set the level with 'cpictl log-level --level TRACE' (active for 10 minutes).
```

**Usage:** `cpictl logs trace [flags]`

**Flags:**

```
      --message-guid string    Message GUID
      --model-step-id string   Only this element of the iFlow model
```

## logs trace-message

Show payload, headers and exchange properties of a traced step (ID from 'logs trace')

**Usage:** `cpictl logs trace-message [flags]`

**Flags:**

```
      --id string       Trace ID
      --max-bytes int   Maximum bytes returned in the JSON result (default 65536; 0 with --out: unlimited)
      --out string      Write the content to this file instead of stdout / the JSON result
```

## logs tree

Show the call tree of a trace across flows and its first failure

```
Build the call tree of one trace (W3C trace ID, e.g. the traceId of 'cpictl send'):
messages are found by ApplicationMessageId = trace ID, otherwise by scanning the
scope (--artifact-ids or --package-id, and --since) for the trace-id custom header.
Nodes are linked by span-id / parent-span-id (names configurable).
```

**Usage:** `cpictl logs tree [flags]`

**Flags:**

```
      --artifact-ids strings     Scope of the fallback scan
      --max-scan int             Maximum messages scanned (default 200)
      --package-id string        Scope of the fallback scan: all flows of this package
      --parent-property string   Custom header property with the parent span ID (default "parent-span-id")
      --since string             Start of the scan window (duration like 1h or RFC 3339)
      --span-property string     Custom header property with the span ID (default "span-id")
      --trace-id string          Trace ID (32 hex characters)
      --trace-property string    Custom header property with the trace ID (default "trace-id")
      --until string             End of the scan window
```

## mcp

Run the MCP server (stdio) for AI agents

```
Run a Model Context Protocol server on stdin/stdout.

The server uses the same tenant settings as every other command (flags,
CPICTL_* environment variables or cpictl.yaml). stdout carries the
protocol only; logs go to stderr as JSON lines.

Tools: list packages, artifacts, resources and parameters; create packages;
download, upload, validate, guideline check, deploy, undeploy (requires
confirm=true); runtime status and endpoints; send test messages; message logs,
steps, attachments and persisted messages; discovery of existing content;
pd_deploy (dry run unless dry_run=false). See docs/mcp.md.

send_test_message uses the --runtime-* credentials (default: the API
credentials), see 'cpictl send --help'.

Local paths given to tools are resolved against --root and may not leave it.

Limit the tools per server, e.g. for a QA or production tenant:
  --read-only                     no tool that changes the tenant or sends messages
  --tools list_*,get_*            only matching tools
  --disable-tools undeploy,pd_*   everything except these
  --mode discover|operate|develop presets (combined with the above, the most
                                  restrictive wins)
Also as CPICTL_MODE, CPICTL_READ_ONLY, CPICTL_TOOLS, CPICTL_DISABLE_TOOLS. Disabled tools are not
listed and cannot be called; a pattern that matches no tool is an error.
```

**Usage:** `cpictl mcp [flags]`

**Flags:**

```
      --disable-tools strings               Do not offer these tools (names or patterns); wins over --tools
      --max-checks int                      Default maximum number of deploy/undeploy status checks (default 30)
      --mode string                         Preset: discover (read-only), operate (read tools + set_log_level), develop (all tools, no pd_deploy full_sync)
      --poll-interval int                   Default seconds between deploy/undeploy status checks (default 10)
      --read-only                           Offer only tools that do not change the tenant or trigger processing
      --root string                         Directory that local paths of tool calls are confined to (default ".")
      --runtime-oauth-clientid string       OAuth client ID for runtime endpoints (default: the API credentials)
      --runtime-oauth-clientsecret string   OAuth client secret for runtime endpoints
      --runtime-oauth-host string           OAuth token server host for runtime endpoints (default: --oauth-host)
      --runtime-password string             Password for Basic Auth on runtime endpoints
      --runtime-userid string               User ID for Basic Auth on runtime endpoints
      --tools strings                       Offer only these tools (names or patterns such as list_*)
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

## packages create

Create an integration package if it does not exist

```
Create an integration package. An existing package with the same ID is left
unchanged (action EXISTS). To create or update a package from a JSON file use
'update package'.
```

**Usage:** `cpictl packages create [flags]`

**Flags:**

```
      --description string   Description
      --name string          Display name (default: package ID)
      --package-id string    Package ID (letters, digits, '_' and '.')
      --short-text string    Short description (default: name)
```

**Examples:**

```
  cpictl packages create --package-id SalesOrders --name "Sales Orders"
```

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

## pd

Inspect Partner Directory parameters (get, diff, deps)

## pd deps

Show which flows read which Partner Directory parameters (local files)

```
Scan local content for Partner Directory references: pd:<PID>:<ID>:<Binary|String>
in .iflw models, dynamic pd:${...} references and getParameter(id, pid, ...) in
Groovy scripts. With --resources-path, PIDs that are referenced but have no local
directory are listed as unknown. Nothing is read from the tenant.
```

**Usage:** `cpictl pd deps [flags]`

**Flags:**

```
      --id string               Only this parameter ID
      --local-dir string        Local content directory (default ".")
      --pid string              Only this partner ID
      --resources-path string   Local Partner Directory tree (to report unknown PIDs)
```

**Examples:**

```
  cpictl pd deps --local-dir ./content --resources-path ./partner-directory --pid ONE_OMS
```

## pd diff

Compare local Partner Directory files with the tenant

```
Compare the local tree (layout of pd-snapshot) with the tenant: per parameter
create, update, unchanged or remote_only (only on the tenant: pd-deploy --full-sync
would delete it). Exit code 7 when a PID could not be read locally.
```

**Usage:** `cpictl pd diff [flags]`

**Flags:**

```
      --pids strings            Only these partner IDs
      --resources-path string   Path to partner directory parameters (default "./partner-directory")
```

**Examples:**

```
  cpictl pd diff --resources-path ./partner-directory --pids ONE_OMS
```

## pd get

Show the Partner Directory parameters of a partner ID on the tenant

**Usage:** `cpictl pd get [flags]`

**Flags:**

```
      --content         Include binary content
      --key strings     Only these parameter IDs (repeatable)
      --max-bytes int   Maximum bytes returned in the JSON result (default 65536; 0 with --out: unlimited)
      --out string      Write the content to this file instead of stdout / the JSON result
      --pid string      Partner ID
```

**Examples:**

```
  cpictl pd get --pid ONE_OMS
  cpictl pd get --pid ONE_OMS --key now_email --content
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
      --keys strings            Deploy only these parameters, PID:ID (create or update, never delete; not with --full-sync)
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

  # Only one mapping (no other parameter is touched)
  cpictl pd-deploy --keys ONE_OMS:now_email
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

## profile

Switch between tenants (profiles in $HOME/.cpictl)

```
A profile is a personal config file $HOME/.cpictl/<name>.yaml with the connection
of one tenant (same keys as cpictl.yaml, credentials included; keep it chmod 600).

  cpictl profile use qa          make qa the default for every following command
  cpictl --profile dev deploy …  one command against another tenant
  CPICTL_PROFILE=dev             per shell (overrides 'profile use')
  cpictl mcp --profile dev       one MCP server per tenant

Precedence: --config, --profile, CPICTL_PROFILE, CPICTL_CONFIG, 'profile use',
$HOME/cpictl.yaml. A repository's cpictl.yaml is still overlaid; its hosts must
match the profile's.
```

## profile current

Show the profile that commands use now

**Usage:** `cpictl profile current`

## profile list

List profiles and mark the active one

**Usage:** `cpictl profile list`

## profile use

Make a profile the default (stored in $HOME/.cpictl/current); 'use -' clears it

**Usage:** `cpictl profile use <name>`

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

## send

Send a test message to a deployed integration flow

```
Send a test message to an endpoint of a deployed integration flow and print
the HTTP status, the response and the message GUID. Only endpoint URLs that the
tenant lists for the flow (see 'endpoints') are used.

With --wait the command also waits for the message processing log and exits
with code 5 when the message did not complete. The message is processed like any
other, including calls to receivers: use test data on a development tenant.

Runtime endpoints usually need other credentials than the API: set
--runtime-oauth-clientid/--runtime-oauth-clientsecret (CPICTL_RUNTIME_OAUTH_*)
from a service key of plan integration-flow with role ESBMessaging.send.
Without them the API credentials are used.

Flows started by ProcessDirect have no endpoint of their own: --process-direct
sends the message to the test harness flow (--harness), which forwards it to the
given address; --artifact-id is then the flow behind the address and --wait
reports that flow's message (found by correlation ID). See docs/testing.md.
```

**Usage:** `cpictl send [flags]`

**Flags:**

```
      --artifact-id string                  Integration flow ID (deployed)
      --body string                         Message body
      --body-file string                    Read the message body from this file ('-' for stdin)
      --content-type string                 Content-Type of the body
      --harness string                      Test harness flow ID (with --process-direct) (default "CPICTL_Test_Harness")
      --header strings                      Additional header name=value (repeatable)
      --method string                       HTTP method (default "POST")
      --no-trace                            Do not send a W3C traceparent header (by default one is generated unless --header traceparent=... is given; its traceId is in the result)
      --process-direct string               Send through the test harness flow to this ProcessDirect address (flows without an HTTP sender)
      --runtime-oauth-clientid string       OAuth client ID for runtime endpoints (default: the API credentials)
      --runtime-oauth-clientsecret string   OAuth client secret for runtime endpoints
      --runtime-oauth-host string           OAuth token server host for runtime endpoints (default: --oauth-host)
      --runtime-password string             Password for Basic Auth on runtime endpoints
      --runtime-userid string               User ID for Basic Auth on runtime endpoints
      --url string                          Endpoint URL, only needed when the flow has several endpoints
      --wait duration                       Wait up to this long for the message processing log (e.g. 60s)
```

**Examples:**

```
  # Send a file and wait for the outcome
  cpictl send --artifact-id OrderIntake --body-file order.xml --content-type application/xml --wait 60s

  # Body from stdin, extra header
  echo '{"id":1}' | cpictl send --artifact-id OrderIntake --body-file - --header X-Test=1

  # A flow with a ProcessDirect sender, through the test harness
  cpictl send --artifact-id Billing --process-direct /billing/in --body-file invoice.xml --wait 60s
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
