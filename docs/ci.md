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
      FLASHPIPE_TMN_HOST: ${{ vars.CPI_DEV_TMN_HOST }}
      FLASHPIPE_OAUTH_HOST: ${{ vars.CPI_DEV_OAUTH_HOST }}
      FLASHPIPE_OAUTH_CLIENTID: ${{ secrets.CPI_DEV_OAUTH_CLIENTID }}
      FLASHPIPE_OAUTH_CLIENTSECRET: ${{ secrets.CPI_DEV_OAUTH_CLIENTSECRET }}
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
  - group: cpi-dev            # FLASHPIPE_TMN_HOST, FLASHPIPE_OAUTH_HOST (+ secret client id/secret)

steps:
  - checkout: self
  - checkout: git://Tools/cpicli      # or a GitHub service connection
  - task: GoTool@0
    inputs:
      version: "1.25"
  - script: make -C cpicli build
  - script: |
      ./cpicli/bin/cpictl deploy --artifact-ids "$(ARTIFACT_IDS)" --output json > result.json
    env:
      FLASHPIPE_OAUTH_CLIENTID: $(CPI_OAUTH_CLIENTID)
      FLASHPIPE_OAUTH_CLIENTSECRET: $(CPI_OAUTH_CLIENTSECRET)
```

Secret variables are not exported automatically in Azure Pipelines; map them with `env:` as shown.

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
