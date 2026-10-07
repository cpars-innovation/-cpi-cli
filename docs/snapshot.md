# Snapshot

`cpictl snapshot` saves every editable package of a tenant into a Git repository (one folder per
package, one per artifact) and commits it. It is the backup of what is on the tenant, including
changes made in the Web UI.

```bash
cpictl snapshot --dir-git-repo ./tenant-backup                    # full: download everything
cpictl snapshot --dir-git-repo ./tenant-backup --incremental      # only what changed
cpictl snapshot --dir-git-repo ./tenant-backup --parallel 8 --git-skip-commit
```

## Incremental snapshots

A full snapshot downloads every artifact and writes only what differs. `--incremental` also skips
the **download** of an artifact when all of these are unchanged since the last snapshot:

| Signal | Source | Catches |
|--------|--------|---------|
| designtime version | package listing (no extra call) | uploads, *Save as version* |
| `ModifiedAt` | package listing (no extra call) | Web UI edits without a new version |
| SHA-256 of the configured parameters | one small call per integration flow | *Configure* changes |
| SHA-256 of the local copy (`META-INF`, `src/main/resources`) | local files | a changed or deleted local folder |

An artifact for which the tenant does not report `ModifiedAt`, or whose configuration cannot be
read, is always downloaded. The signatures are kept in `.cpi/snapshot-state.json` in the
repository (`--state-file` to move it); commit it with the snapshot so that the next run, also in
CI, can compare. Every run (full or incremental) refreshes it.

Recommended: `--incremental` for frequent runs (e.g. hourly) and a full run regularly (e.g.
nightly) as the safety net, in case the tenant changes something none of the signals sees.
Deleting the state file makes the next incremental run a full one.

## Parallel packages and failures

Packages are processed `--parallel` at a time (default 4); artifacts within a package one after
another. Reads that the tenant throttles (`429`) or that hit a gateway error (`502`-`504`) are
retried up to three times with backoff (`Retry-After` is honoured). A package that fails does not
stop the others: the snapshot of the other packages is written, the state saved and committed,
and the run ends with exit code 7 (partial) and the failed packages in the result:

```json
{"packages": 42, "succeeded": 41, "artifactsSkipped": 380, "failed": ["Legacy: ..."]}
```

All settings can also be set in the config file under `snapshot` (`incremental`, `parallel`,
`stateFile`, ...), see [commands.md](commands.md#snapshot).
