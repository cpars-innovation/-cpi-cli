# CI/CD

cpictl is a single static binary. In a pipeline: build or download it, provide the tenant
credentials as secret environment variables, run commands, and use the
[exit codes](../README.md#output-and-exit-codes) and `--output json` results.

## GitHub Actions

```yaml
name: Deploy to DEV
on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    env:
      CPICTL_TMN_HOST: ${{ vars.CPI_DEV_TMN_HOST }}
      CPICTL_OAUTH_HOST: ${{ vars.CPI_DEV_OAUTH_HOST }}
      CPICTL_OAUTH_CLIENTID: ${{ secrets.CPI_DEV_OAUTH_CLIENTID }}
      CPICTL_OAUTH_CLIENTSECRET: ${{ secrets.CPI_DEV_OAUTH_CLIENTSECRET }}
    steps:
      - uses: actions/checkout@v4                # the integration content repository

      - uses: actions/checkout@v4                # cpictl sources
        with:
          repository: cpars-innovation/cpicli
          path: .cpicli
          token: ${{ secrets.CPICLI_READ_TOKEN }} # only needed while the repo is private
      - uses: actions/setup-go@v5
        with:
          go-version-file: .cpicli/go.mod
      - run: make -C .cpicli build && echo "$PWD/.cpicli/bin" >> "$GITHUB_PATH"

      - name: Check connection and roles
        run: cpictl doctor

      - name: Plan
        run: cpictl orchestrator --packages-dir ./packages --deploy-config ./001-deploy-config.yml --plan

      - name: Update and deploy
        run: cpictl orchestrator --packages-dir ./packages --deploy-config ./001-deploy-config.yml --output json > result.json

      - name: Show failed artifacts
        if: failure()
        run: jq '.result.deployments[] | select(.status != "DEPLOYED" and .status != "SKIPPED")' result.json
```

`doctor` fails the job early (exit code 3 or 4) when the credentials or the connection are
wrong. The plan, the update and the deployments each add a table to the run page
([job summary](#job-summary)).

## Azure Pipelines

```yaml
trigger:
  branches:
    include: [main]

pool:
  vmImage: ubuntu-latest

variables:
  - group: cpi-dev            # CPICTL_TMN_HOST, CPICTL_OAUTH_HOST (+ secret client id/secret)

steps:
  - checkout: self
  - checkout: git://Tools/cpicli      # or a GitHub service connection
  - task: GoTool@0
    inputs:
      version: "1.26"
  - script: make -C cpicli build
  - script: |
      ./cpicli/bin/cpictl deploy --artifact-ids "$(ARTIFACT_IDS)" --output json > result.json
    env:
      CPICTL_OAUTH_CLIENTID: $(CPI_OAUTH_CLIENTID)
      CPICTL_OAUTH_CLIENTSECRET: $(CPI_OAUTH_CLIENTSECRET)
```

Secret variables are not exported automatically in Azure Pipelines; map them with `env:` as shown.

### Versioning per branch

The versioning mode belongs to the pipeline, not to files that are merged between branches
([versioning.md](versioning.md)):

```yaml
variables:
  - name: CPICTL_VERSIONING
    ${{ if in(variables['Build.SourceBranchName'], 'dev', 'test', 'main') }}:
      value: manifest
    ${{ else }}:
      value: keep
```

Azure Pipelines exports non-secret variables as environment variables, so every cpictl call of
the job uses it.

## Pipeline: snapshot, update, configure, deploy once

For a repository with one copy of every artifact in the tenant layout (`packages/`, tracked in
Git, see [snapshot.md](snapshot.md)), with uploads and parameters in separate steps:

```bash
set -e
cpictl snapshot --dir-git-repo packages --git-skip-commit --dry-run \
  --fail-on-local-modified                                          # 1. optional gate: tenant vs repository
cpictl orchestrator -d packages -c deployments --plan              # 2. optional: what would happen
cpictl orchestrator -d packages -c deployments --defer-deploy      # 3. uploads only what changed
cpictl configure -c config/dev.yml --defer-deploy                  # 4. writes only changed parameters
cpictl deploy --pending                                            # 5. deploys each artifact once
```

Pulling tenant changes into the repository is its own job (scheduled, or before a release
branch): `cpictl snapshot --dir-git-repo packages --git-skip-commit`, then review and commit the
diff. Two snapshots in a row give an empty `git status`; artifacts with local edits are skipped as
`local-modified`; deployment copies of the deploy config are not written.

- The orchestrator never writes into `--packages-dir`: IDs, names, `configOverrides` and
  prefixes are applied to a temporary copy, so the working tree stays clean.
- Step 3 compares with the snapshot state (`packages/.cpi/snapshot-state.json`) instead of
  downloading every artifact again ([orchestrator.md](orchestrator.md#comparison-without-downloads)).
- Steps 3 and 4 do not deploy: they add to `.cpi/pending-deploy.json` what needs a deployment and
  why. An artifact both steps touch is deployed once, with force when either needs it (a
  parameter change or new content with the running version). The runtime keeps running until
  step 5; nothing is undeployed in between.
- Step 5 skips pending artifacts whose runtime already runs the designtime version and that no
  step forced. Failed deployments stay in the file (exit code 5/7); successful ones are removed,
  and the file is deleted when it is empty. `cpictl deploy --pending --plan` shows the list with
  the reasons first.
- The file records the tenant: it cannot be deployed to another one.

### Job summary

In GitHub Actions `orchestrator`, `configure`, `deploy`, `undeploy` and `snapshot` append a
markdown summary to `$GITHUB_STEP_SUMMARY` (shown on the run's page): counts, the plan, and one
row per deployment with status, version, rule and error, failures first. Parameter values are
not shown (only artifact, key and change). `--summary FILE` writes it to a file instead (e.g. for
Azure Pipelines: `echo "##vso[task.uploadsummary]$PWD/summary.md"`), `--summary off` disables it.

## Exit codes in scripts

```bash
cpictl deploy --artifact-ids A,B --output json > result.json
case $? in
  0) echo "deployed" ;;
  5|7) jq -r '.result.results[] | select(.status=="FAILED") | "\(.id): \(.error)"' result.json; exit 1 ;;
  6) echo "timeout, check: cpictl status --artifact-ids A,B"; exit 1 ;;
  *) exit 1 ;;
esac
```

## Docker

`build/Dockerfile` packages a Linux binary placed in `output/cpictl`:

```bash
GOOS=linux CGO_ENABLED=0 go build -o output/cpictl ./cmd/cpictl
docker build -f build/Dockerfile -t cpictl .
```
