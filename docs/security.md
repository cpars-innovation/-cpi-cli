# Security material

Integration flows reference credentials and keys by name. cpictl lists them, deploys
them from CI, and watches certificate expiry.

## Rules

- **Secrets are never flag values.** Every secret comes from an environment variable
  (`--password-env NAME`), a file (`--password-file path`) or stdin (`--password-stdin`),
  so it does not end up in shell history, process lists or CI logs.
- **Secrets are never returned.** `credentials list` (and the MCP tools) show names, users,
  client IDs and status only; the tenant API does not return passwords and cpictl does not
  map such fields.
- **Secrets are never logged.** Request bodies are not written to the log, not even with `--debug`.
- **The MCP server is read-only for security material.** Agents can see which credentials
  exist (`list_credentials`, `list_keystore`), but deploying a secret is done by a person or a
  pipeline with `cpictl credentials`.

## Credentials

```bash
cpictl credentials list                         # user, oauth2, secure-param (names and metadata)
cpictl credentials list --kind oauth2

ERP_PASSWORD=... cpictl credentials set-user --name ERP_User --user svc_erp --password-env ERP_PASSWORD
cpictl credentials set-oauth2 --name Graph --token-url https://login.example.com/oauth/token \
  --client-id my-app --secret-file ./graph.secret [--client-auth header] [--scope ...]
printf '%s' "$API_KEY" | cpictl credentials set-secure-param --name ApiKey --value-stdin   # Neo only

cpictl credentials delete --kind user --name ERP_User --confirm
```

`set-*` create the credential or update it if it exists (`action`: `CREATED` / `UPDATED`);
`--dry-run` validates the input and resolves the secret without writing.

### Declarative: credentials apply

```yaml
# credentials.yml (commit this; it contains no secrets)
userCredentials:
  - name: ERP_User
    user: svc_erp
    kind: default            # default, successfactors, openconnectors
    password: {env: ERP_PASSWORD}
oauth2Credentials:
  - name: Graph
    tokenServiceUrl: https://login.example.com/oauth/token
    clientId: my-app
    clientSecret: {file: secrets/graph.txt}   # relative to this file
    clientAuthentication: body                # or header
secureParameters:
  - name: ApiKey
    value: {env: API_KEY}
```

```bash
cpictl credentials apply --file credentials.yml --dry-run
cpictl credentials apply --file credentials.yml --output json
```

All secrets are resolved before anything is written; a missing variable or file stops the
run with exit code 2 and no changes. Inline values (`password: plain`) and unknown keys are
rejected. If some credentials fail to deploy the exit code is 7 and the result lists each one.

Secure parameters exist only in the Neo environment; on Cloud Foundry tenants `credentials
list` shows a warning and `set-secure-param` fails.

## Keystore

```bash
cpictl keystore list                                   # tenant keystore, sorted by expiry
cpictl keystore list --expiring-within 30d             # flag entries expiring within 30 days
cpictl keystore list --expiring-within 30d --fail-on-expiry   # exit 5 if any: use in a nightly job
cpictl keystore list --keystore KeyRenewal

cpictl keystore export-cert --alias sap_cloudintegrationcertificate > tenant.pem
cpictl keystore import-cert --alias partner_acme --file acme.pem            # PEM or DER
cpictl keystore import-cert --alias partner_acme --file acme-2027.pem --update
```

`import-cert` validates the file as an X.509 certificate before uploading, rejects private
keys and refuses to replace an existing alias without `--update`. Key pair generation,
keystore backup/restore and history operations are intentionally not offered.

## Roles

Reading credentials and the keystore and changing them need the corresponding security
roles of the API client (credentials read/write, keystore read/write); missing roles show up
as exit code 3.
