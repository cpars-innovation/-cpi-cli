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

## Parallel downloads and failures

Up to `--parallel` artifacts (default 8) are downloaded at the same time, across all packages: a
package with many flows does not run alone at the end. For large tenants (hundreds of flows) try
`--parallel 16`; if the tenant answers `429` often, lower it. The summary line and the JSON result
show how many artifacts were downloaded or skipped and how long it took
(`artifactsDownloaded`, `artifactsSkipped`, `seconds`). Reads that the tenant throttles (`429`) or that hit a gateway error (`502`-`504`) are
retried up to three times with backoff (`Retry-After` is honoured). A package that fails does not
stop the others: the snapshot of the other packages is written, the state saved and committed,
and the run ends with exit code 7 (partial) and the failed packages in the result:

```json
{"packages": 130, "succeeded": 129, "artifactsDownloaded": 41, "artifactsSkipped": 759, "seconds": 74.2, "failed": ["Legacy: ..."]}
```

A failing artifact no longer stops the other artifacts of its package either; the package is
reported as failed with the artifacts that failed.

All settings can also be set in the config file under `snapshot` (`incremental`, `parallel`,
`stateFile`, ...), see [commands.md](commands.md#snapshot).
