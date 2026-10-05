# Configuration

## Where settings come from

For every flag, the first of these that is set wins:

1. Command line flag, e.g. `--tmn-host`
2. Environment variable: `CPICTL_` + flag name in upper case with `-` replaced by `_`,
   e.g. `CPICTL_TMN_HOST`, `CPICTL_ARTIFACT_IDS`, `CPICTL_OUTPUT`
3. Top-level key in the config file with the flag name, e.g. `tmn-host: ...`
4. Command section in the config file, e.g. `deploy.artifactIds` (the key is shown in each
   flag's help text as `(config: ...)`)
5. The flag's default

The config file is the one given with `--config`, else `$CPICTL_CONFIG`, else
`$HOME/cpictl.yaml`.
A missing default file is fine; a file given with `--config` that cannot be parsed is a
usage error (exit code 2).

> **Secrets:** you may keep `oauth-clientsecret`, `tmn-password` and the runtime secrets in a
> personal config file (like `~/.netrc` or `~/.aws/credentials`): make it readable only by you
> (`chmod 600`; cpictl warns otherwise) and never commit it. In CI and shared setups use
> environment variables instead. The file is read as plain YAML: `${VAR}` is **not** expanded.

### One file per tenant

```bash
mkdir -p ~/.cpictl && chmod 700 ~/.cpictl
# ~/.cpictl/dev.yaml, ~/.cpictl/qa.yaml: tmn-host, oauth-host, oauth-clientid, oauth-clientsecret, ...
chmod 600 ~/.cpictl/*.yaml

export CPICTL_CONFIG=~/.cpictl/dev.yaml      # default for this shell
cpictl packages
cpictl --config ~/.cpictl/qa.yaml status --runtime-status ERROR
```

For the MCP server, point each server entry at its file instead of listing variables:
`"args": ["mcp", "--root", ".", "--config", "/home/me/.cpictl/dev.yaml"]` (or
`"env": {"CPICTL_CONFIG": "..."}`).

Flags that are marked as required (for example `sync --package-id`) must come from the
command line, the environment or a top-level key; a command section key is read too late
for cobra's required-flag check.

## Tenant connection

| Flag | Environment variable | Description |
|------|----------------------|-------------|
| `--tmn-host` | `CPICTL_TMN_HOST` | Tenant management host of Cloud Integration (or API portal host for APIM), without `https://` |
| `--oauth-host` | `CPICTL_OAUTH_HOST` | Token server host, e.g. `<subdomain>.authentication.eu10.hana.ondemand.com` |
| `--oauth-clientid` | `CPICTL_OAUTH_CLIENTID` | OAuth client ID |
| `--oauth-clientsecret` | `CPICTL_OAUTH_CLIENTSECRET` | OAuth client secret |
| `--oauth-path` | `CPICTL_OAUTH_PATH` | Token path, default `/oauth/token` |
| `--tmn-userid` | `CPICTL_TMN_USERID` | Basic Auth user (alternative to OAuth) |
| `--tmn-password` | `CPICTL_TMN_PASSWORD` | Basic Auth password |

Either the three OAuth values or the Basic Auth pair are required, except for
`config-generate`, which works offline. The host may also be given
as `https://host` or `host:port`. Plain `http://` is only honoured for `localhost` /
`127.0.0.1` (used by the offline tests); for any other host https is used.

### Runtime endpoints (test messages)

`send` and the MCP tool `send_test_message` call the flow's own endpoint, which normally needs
a different service key than the API: *SAP Process Integration Runtime*, plan
**integration-flow**, with the role `ESBMessaging.send`.

| Flag | Environment variable | Description |
|------|----------------------|-------------|
| `--runtime-oauth-clientid` | `CPICTL_RUNTIME_OAUTH_CLIENTID` | OAuth client ID of the integration-flow key |
| `--runtime-oauth-clientsecret` | `CPICTL_RUNTIME_OAUTH_CLIENTSECRET` | Its secret |
| `--runtime-oauth-host` | `CPICTL_RUNTIME_OAUTH_HOST` | Token server host, default `--oauth-host` |
| `--runtime-userid` / `--runtime-password` | `CPICTL_RUNTIME_USERID` / `_PASSWORD` | Basic Auth instead of OAuth |

Without these settings the API credentials are used (works when they also carry
`ESBMessaging.send`, e.g. with Basic Auth). Missing roles are exit code 3.

## Global flags

| Flag | Description |
|------|-------------|
| `--config` | Config file (default `$HOME/cpictl.yaml`) |
| `--output text\|json` | Result format, see [README](../README.md#output-and-exit-codes) |
| `--debug` | Debug logging: request URLs and response errors; request bodies are never logged (they can contain secrets) |

## Config file example

A complete annotated file is in [examples/cpictl.yaml](examples/cpictl.yaml). Short version:

```yaml
tmn-host: mytenant.it-cpi018.cfapps.eu10-003.hana.ondemand.com
oauth-host: mytenant.authentication.eu10.hana.ondemand.com
# oauth-clientid / oauth-clientsecret: via CPICTL_OAUTH_CLIENTID / CPICTL_OAUTH_CLIENTSECRET

deploy:
  artifactType: Integration
  delayLength: 15        # seconds between status checks
  maxCheckLimit: 20

undeploy:
  delayLength: 10
  maxCheckLimit: 30

orchestrator:
  packagesDir: ./packages
  deployConfig: ./001-deploy-config.yml
  parallelDeployments: 3

pd-deploy:
  resources-path: ./partner-directory
  full-sync: false
```

Sections used by the commands: `deploy`, `undeploy`, `configure` (and `configure.pull`),
`orchestrator`, `update.artifact`, `update.package`, `sync`, `sync.apiproxy`,
`sync.apiproduct`, `snapshot`, `restore`, `pd-snapshot`, `pd-deploy`.

## Creating an OAuth client in SAP BTP

cpictl uses the OAuth 2.0 client credentials flow against the Cloud Integration API.

1. In the BTP cockpit, open the subaccount and the Cloud Foundry space that holds Integration Suite.
2. **Services → Instances and Subscriptions → Create**:
   - Service: *SAP Process Integration Runtime*, plan: **api**
   - Grant type: **Client Credentials**
   - Roles: the role collection/roles needed for designtime read/edit and deployment
     (for example `WorkspacePackagesRead`, `WorkspacePackagesEdit`, `WorkspaceArtifactsDeploy`,
     `MonitoringDataRead`) plus Partner Directory configuration roles for `pd-*` commands.
     Role names differ between tenant generations; check the role list offered for the plan.
3. Open the instance and **Create** a service key.
4. From the key:
   - `url` → host only → `tmn-host`
   - `tokenurl` → host only → `oauth-host` (the path is `/oauth/token`, the default of `--oauth-path`)
   - `clientid` → `CPICTL_OAUTH_CLIENTID`
   - `clientsecret` → `CPICTL_OAUTH_CLIENTSECRET`

For API Management (`sync apiproxy`, `sync apiproduct`) create a key for the *API Management,
API portal* service (plan `apiportal-apiaccess`) instead and use its host values.

Missing roles show up as exit code 3 (HTTP 403).
