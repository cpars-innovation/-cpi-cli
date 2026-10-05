# Partner Directory

`pd-snapshot` downloads Partner Directory parameters into files, `pd-deploy` uploads them.
Keep the files in Git to version and promote Partner Directory content between tenants.

## File layout

```
partner-directory/                 # --resources-path (default ./partner-directory)
└── SAP_SYSTEM_001/                # one directory per partner ID (PID)
    ├── String.properties          # string parameters: Id=Value, one per line
    └── Binary/                    # binary parameters, one file per parameter
        ├── Certificate.crt        # file name = parameter ID + extension
        ├── Mapping.xsl
        └── _metadata.json         # optional, see below
```

- `String.properties`: `key=value` lines; `#` comments and empty lines are ignored; values are
  escaped/unescaped like Java properties.
- Binary content types supported by CPI: `xml`, `xsl`, `xsd`, `json`, `txt`, `zip`, `gz`,
  `zlib`, `crt`. The type is taken from the file extension.
- `_metadata.json` is only needed for content types with parameters, e.g.
  `{"Mapping.xsl": "xsl; encoding=UTF-8"}` (key = file name). `pd-snapshot` writes it when needed.

A complete sample is in [examples/partner-directory/](examples/partner-directory).

## pd-snapshot

```bash
cpictl pd-snapshot                                   # all PIDs into ./partner-directory
cpictl pd-snapshot --resources-path ./pd --pids SAP_SYSTEM_001,CUSTOMER_API
cpictl pd-snapshot --replace=false                   # keep existing local values, add new ones
```

## pd-deploy

```bash
cpictl pd-deploy --dry-run                           # show what would change
cpictl pd-deploy                                     # create new, update changed parameters
cpictl pd-deploy --replace=false                     # only create missing parameters
cpictl pd-deploy --pids SAP_SYSTEM_001
cpictl pd-deploy --full-sync --dry-run               # also show what would be deleted
```

| Mode | Effect |
|------|--------|
| default (`--replace`) | Create missing parameters, update parameters whose value differs |
| `--replace=false` | Only create missing parameters |
| `--full-sync` | Additionally delete remote parameters of the *local* PIDs that do not exist locally. PIDs without a local directory are never touched |

### Full sync safety

Full sync only deletes for a PID after both its `String.properties` and its `Binary/`
directory were read successfully. If anything of a PID cannot be read (unreadable file,
`String.properties` that is a directory, a binary file that cannot be opened), full sync is
skipped for that PID and the run fails, instead of treating the PID as empty and deleting
its remote parameters.

### Result and exit code

Any failed parameter, deletion or local read makes `pd-deploy` exit with code 7 (partial
failure); `--output json` lists `created`, `updated`, `unchanged`, `deleted` and `errors`
for string and binary parameters.

## Inspect and change single parameters

```bash
cpictl pd get --pid ONE_OMS                          # tenant values (binaries: type, size, sha256)
cpictl pd get --pid ONE_OMS --key now_email --content
cpictl pd diff --resources-path ./partner-directory  # create / update / unchanged / remote_only
cpictl pd-deploy --keys ONE_OMS:now_email --dry-run  # then without --dry-run
```

- `pd diff` shows what `pd-deploy` would change; `remote_only` parameters are the ones
  `--full-sync` would delete. A PID whose local files cannot be read is an error (exit 7), never
  "empty".
- `pd-deploy --keys PID:ID,...` creates or updates only those parameters and deletes nothing
  (not combinable with `--full-sync` or `--pids`). The content type of a binary comes from
  `_metadata.json`, else from the extension (`.xsl`/`.xslt` → `xsl`, `.xml`, `.json`, `.xsd`);
  other extensions need a `_metadata.json` entry. The result lists `CREATED`, `UPDATED` or
  `UNCHANGED` per key.

Before changing a parameter, check which flows read it (local content, no tenant calls):

```bash
cpictl pd deps --local-dir ./content --resources-path ./partner-directory --pid ONE_OMS
```

It lists literal `pd:<PID>:<ID>:<Binary|String>` references in `.iflw` models (with the step id),
dynamic `pd:${...}` references, `getParameter(id, pid, ...)` calls in Groovy scripts, and
`unknownPids`: referenced PIDs without a local directory (e.g. `pd:OMS:...` when the PID is
`ONE_OMS`).

MCP: `pd_dependencies`, `get_pd_parameters`, `pd_diff`, `pd_deploy` with `keys`.

## Config file

```yaml
pd-snapshot:
  resources-path: ./partner-directory
  replace: true
  pids: [SAP_SYSTEM_001]

pd-deploy:
  resources-path: ./partner-directory
  replace: true
  full-sync: false
  dry-run: false
  pids: [SAP_SYSTEM_001]
```
