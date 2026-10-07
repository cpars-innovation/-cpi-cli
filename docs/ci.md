# CI/CD

cpictl is a single static binary. In a pipeline: build or download it, provide the tenant
credentials as secret environment variables, run commands, and use the
[exit codes](../README.md#exit-codes) and `--output json` results.

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

      - name: Update and deploy
        run: cpictl orchestrator --packages-dir ./packages --deploy-config ./001-deploy-config.yml --output json > result.json

      - name: Show failed artifacts
        if: failure()
        run: jq '.result.deployments[] | select(.status != "DEPLOYED" and .status != "SKIPPED")' result.json
```

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

For a repository that keeps only the parts of each artifact it manages (scripts, mappings, ...)
on top of a snapshot of the tenant, with uploads and parameters in separate steps:

```bash
set -e
cpictl snapshot --incremental --dir-git-repo . --git-skip-commit   # 1. tenant -> repository
./copy-managed-parts.sh                                            # 2. your parts over the snapshot
cpictl orchestrator -d . -c deploy/ --plan                         # 3. optional: what would happen
cpictl orchestrator -d . -c deploy/ --defer-deploy                 # 4. uploads only what changed
cpictl configure -c config/dev.yml --defer-deploy                  # 5. writes only changed parameters
cpictl deploy --pending                                            # 6. deploys each artifact once
```

- Step 4 compares with the snapshot of step 1 instead of downloading every artifact again
  ([orchestrator.md](orchestrator.md#comparison-without-downloads)).
- Steps 4 and 5 do not deploy: they add to `.cpi/pending-deploy.json` what needs a deployment and
  why. An artifact both steps touch is deployed once, with force when either needs it (a
  parameter change or new content with the running version). The runtime keeps running until
  step 6; nothing is undeployed in between.
- Step 6 skips pending artifacts whose runtime already runs the designtime version and that no
  step forced. Failed deployments stay in the file (exit code 5/7); successful ones are removed,
  and the file is deleted when it is empty. `cpictl deploy --pending --plan` shows the list with
  the reasons first.
- The file records the tenant: it cannot be deployed to another one.

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
