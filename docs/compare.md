# Compare tiers, Git refs and content trees

`cpictl compare <A> <B>` compares two sides per artifact. A side is:

| Side | Meaning |
|------|---------|
| `<directory>` | a content tree (`<package>/<artifact>`, as `snapshot` writes it) |
| `git:<ref>[:<path>]` | a branch, tag or commit of the repository (`--repo`, default `.`), optionally only a sub-folder |
| `tenant` | the configured tenant (`--profile`, config file, environment) |
| `tenant:<profile>` | the tenant of a [profile](configuration.md#profiles-switching-tenants) (`~/.cpictl/<profile>.yaml`) |

```bash
cpictl compare tenant:test tenant:prod --package Orders     # what would a transport change?
cpictl compare packages tenant --diff                       # repository vs tenant (drift)
cpictl compare git:release/2026-10:packages git:main:packages
cpictl compare tenant:dev tenant:test --fail-on-diff         # CI gate, exit code 5 on differences
```

Tenants are only read. Their artifacts are downloaded and normalized exactly as `snapshot` writes
them (LF line endings, `parameters.prop` and `metainfo.prop` without timestamps), so a snapshot and
its tenant compare as `same`.

Per artifact (matched by artifact ID, across package folders):

| Status | Meaning |
|--------|---------|
| `same` | same content, version and parameters |
| `content_differs` | the content differs as an upload compares it (without `Bundle-Version` and `parameters.prop`) |
| `version_differs` | same content, different version |
| `parameters_differ` | same content and version, different `parameters.prop` |
| `only_a`, `only_b` | on one side only |

- **Files**: added (only in B), removed (only in A) or changed; `--diff` adds unified diffs of text
  files (binaries and very large files are marked).
- **Parameters**: per `parameters.prop` key `differs`, `only_a`, `only_b`; values only with
  `--show-values`.
- **Tenant sides**: the designtime version (used as the version: the exported manifest can lag
  behind it), the running version and status, the draft flag, and who changed the artifact last
  where the tenant reports it.

`--package` / `--artifact` (IDs or patterns) limit the comparison and the downloads; `--parallel`
sets the downloads at the same time per tenant (default 8). `--output json` returns `a`, `b`,
`items` and `summary`. The MCP tool `compare` does the same with `tenant`, a directory inside the
server root or `git:<ref>[:<path>]`.
