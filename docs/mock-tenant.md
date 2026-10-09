# Mock tenant

`cpictl mock-tenant` serves an in-memory mock of the SAP CPI APIs that cpictl uses. It is for
local development, demos, end-to-end tests of tools built on cpictl (such as a UI or an agent)
and CI. It never talks to a real tenant, and any credentials are accepted.

```bash
cpictl mock-tenant --tier dev --addr 127.0.0.1:8081
```

It prints the settings to connect:

```
CPICTL_TMN_HOST=http://127.0.0.1:8081
CPICTL_TMN_USERID=mock
CPICTL_TMN_PASSWORD=mock
# or OAuth: CPICTL_OAUTH_HOST=127.0.0.1:8081 CPICTL_OAUTH_CLIENTID=mock CPICTL_OAUTH_CLIENTSECRET=mock
```

State is in memory: a restart resets it. Stop it with Ctrl+C.

## Demo landscape

`--seed demo` (default) loads one tier of a demo landscape; `--seed empty` starts without content.

| Flow | Package | Started by | Sends to |
|------|---------|------------|----------|
| `Orders_In` | Orders | HTTPS `/orders/in` | ProcessDirect `/orders/route` |
| `Orders_Route` | Orders | ProcessDirect `/orders/route` | HTTP ERP (`ERP_User`), JMS queue `Orders_Billing` |
| `Billing_In` | Billing | JMS queue `Orders_Billing` | ProcessDirect `/billing/post` |
| `Billing_Post` | Billing | ProcessDirect `/billing/post` | HTTP finance (`Finance_OAuth`, key `finance_client`) |
| `Partner_Notify` | Partners | timer | SFTP (`Partner_SFTP`) |
| `Returns_In` (dev only) | Orders | HTTPS `/returns/in` | HTTP returns API (`Returns_API`) |

The flows have real `.iflw` models with diagrams (laid out by `iflow layout`), scripts that write
the `OrderNo` custom header property, externalized parameters, and a day of message processing
logs: orders run `Orders_In → Orders_Route → Billing_In → Billing_Post` with the same correlation
ID; about one posting in twenty fails with the same error; `Partner_Notify` fails now and then; on
dev some orders come back through `Returns_In` with a new correlation ID and the same `OrderNo`
(see [following a message without a tracer](monitoring.md#following-a-message-without-a-tracer)).

The tiers differ the way real ones do:

| | dev | test | prod |
|--|-----|------|------|
| `Orders_Route` | 1.0.4 | 1.0.3 | 1.0.2 |
| `Returns_In` | 0.1.0 | – | – |
| `Billing_Post` | draft (edited in the Web UI) | 1.3.1 | 1.3.1, `Timeout` changed on the tenant |
| `Returns_API` credential | yes | yes | missing |
| `finance_client` certificate | expires in 300 days | 200 days | **20 days** |
| parameter hosts | `*-dev.example.com` | `*-qa.example.com` | `*-www.example.com` |

So `matrix`, `compare`, `transport check` and `logs summary` all have something to show:

```bash
# one mock per tier, and a profile per tier (~/.cpictl/<tier>.yaml with tmn-host, tmn-userid, tmn-password)
cpictl mock-tenant --tier dev  --addr 127.0.0.1:8081 &
cpictl mock-tenant --tier test --addr 127.0.0.1:8082 &
cpictl mock-tenant --tier prod --addr 127.0.0.1:8083 &

cpictl matrix tenant:dev tenant:test tenant:prod
cpictl compare tenant:test tenant:prod
cpictl --profile dev snapshot --dir-git-repo . --dir-artifacts packages --draft-handling ADD
cpictl transport check Returns_In Orders_Route --dir packages --source tenant:dev --target tenant:prod
cpictl --profile dev logs summary --since 24h
```

## Landscapes (`--seed-dir`)

`--seed-dir <dir>` loads your own landscape instead of the demo, one tier per mock:

```bash
cpictl mock-tenant --seed-dir ./landscapes/retail-b --tier prod-eu --addr 127.0.0.1:8084
```

A landscape directory holds the content in the layout `cpictl snapshot` writes, plus
`landscape.yaml` with everything that differs per tier and what the content does not contain:

```
retail-b/
  landscape.yaml
  packages/<Package>/<Package>.json          # optional: {"d": {"Id", "Name", "Version"}}
  packages/<Package>/<Artifact>/META-INF/MANIFEST.MF
  packages/<Package>/<Artifact>/src/main/resources/...
```

So a snapshot of a real tenant (or `cpictl iflow copy` / templates) is a valid starting point.
The artifact ID comes from `Bundle-SymbolicName`, the name from `Bundle-Name`, the version from
`Bundle-Version`, the type from `SAP-BundleType`.

```yaml
name: retail-b                      # required
description: S/4 orders to Salesforce, invoices to SFTP
keyHeaders: [SalesOrder]            # custom header properties written by the flows' scripts
history: 24h                        # message history generated at start (default 24h)
seed: 7                             # optional: change the generated traffic and failures

tiers:                              # required, in pipeline order; --tier picks one
  - name: dev
    drafts: [SF_Order_Upsert]       # being edited in the Web UI: designtime "Active"
  - name: prod-eu
    versions: { S4_Orders_Out: 1.0.2 }                 # designtime and running version
    parameters: { S4_Orders_Out: { Host: s4-prd.example.com } }   # configured on this tenant
    modifiedBy: { Order_Route: m.okafor@customer.example }       # changed on the tenant (drift)
    exclude: [Returns_In]           # does not exist on this tier
    notDeployed: [SF_Account_Sync]  # exists, not running
    missingCredentials: [SF_OAuth]
    keystore: [{ alias: sf_client, expiresInDays: 12 }]
    partnerDirectory: { Shop_DE/endpoint: "https://shop-de.example.com" }
    systems: { s4: { failRate: 0.1 } }                  # overrides per tier
    trafficScale: 2                 # more messages on this tier

systems:                            # receivers the flows call, matched by host
  s4: { match: "s4-*.example.com", latencyMs: 180, failRate: 0.02, status: 500,
        error: "SO {key}: plant 1010 locked ({host})" }

credentials:                        # names only; secrets are never real
  - { name: S4_User, kind: basic, user: RFC_SO }       # kind: basic | oauth2 | secure
  - { name: SF_OAuth, kind: oauth2, tokenUrl: "https://login.salesforce.com/oauth2/token" }
keystore: [{ alias: sf_client, expiresInDays: 300 }]
partnerDirectory: { Shop_DE/format: JSON }

traffic:                            # messages that start at a flow
  - start: S4_Orders_Out            # HTTPS sender or timer flow
    perHour: 40
    key: "SO{n}"                    # business key; {n} counts up from keyStart
    keyStart: 1000
    followUps:                      # the transaction comes back later with a new correlation ID
      - { start: Returns_In, every: 4, after: 20m, tiers: [dev] }
```

Unknown fields, unknown artifacts, credentials, systems or tiers are refused at start (exit 2).
The built-in demo is a landscape of this format (`internal/cpitest/landscapes/demo`).

## Live behaviour

- A deploy starts the designtime version (a draft deploys as the version running before); the
  build task reports `SUCCESS`; an undeploy removes the runtime artifact.
- Uploads (create, update) record `ModifiedBy: mock-user` and the time, and the tenant derives
  what a real one does from the archive: the resources (scripts, mappings, schemas, the model)
  and the externalised parameters from `parameters.prop`. A new upload keeps the configured value
  of a parameter that still exists. Like a real tenant, the upload does not take the version from
  `MANIFEST.MF` (a new artifact gets 1.0.0); with `--versioning manifest` cpictl sets
  `Bundle-Version` after the upload ([versioning.md](versioning.md)).
- A deploy registers the flow's HTTPS (`/http/<urlPath>`) and SOAP (`/cxf/<address>`) senders as
  runtime endpoints, listed in `ServiceEndpoints`; an undeploy removes them. So flows uploaded
  into `--seed empty` can receive messages.
- A message sent to a flow's endpoint (`cpictl send --artifact-id Orders_In`) is answered with
  HTTP 200 and creates a `COMPLETED` message processing log.
- Message log queries honour the status and time filters, newest first.
- Data stores and log files answer with empty lists; APIs the mock does not know answer 404.

## From other containers

cpictl accepts plain `http://` only for loopback hosts, so credentials never travel unencrypted.
To reach the mock from another container, serve HTTPS with a generated certificate and give the
client the CA:

```bash
cpictl mock-tenant --tier prod --addr 0.0.0.0:8443 --tls \
  --tls-hosts mock-prod,localhost --ca-out /certs/mock-ca.pem --public-url https://mock-prod:8443
# client container
SSL_CERT_FILE=/certs/mock-ca.pem CPICTL_TMN_HOST=https://mock-prod:8443 CPICTL_TMN_USERID=mock CPICTL_TMN_PASSWORD=mock cpictl status
```

`--public-url` is the base of the flows' endpoint URLs in `ServiceEndpoints` (what `send` calls);
by default the listen address.

Docker Compose example:

```yaml
services:
  mock-dev:
    image: <your image with cpictl>
    command: ["cpictl", "mock-tenant", "--tier", "dev", "--addr", "0.0.0.0:8443", "--tls",
              "--tls-hosts", "mock-dev", "--ca-out", "/certs/mock-dev.pem", "--public-url", "https://mock-dev:8443"]
    volumes: ["certs:/certs"]
volumes:
  certs: {}
```
