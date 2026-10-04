# Contributing

## Before you push

```bash
gofmt -l .                        # must print nothing
go vet ./... && go vet -tags integration ./...
go test -race ./...               # offline, < 10 s, no tenant needed
```

CI runs the same checks (`.github/workflows/ci.yml`).

## Rules

- **Never test against a real tenant by default.** Tests use the in-memory mock tenant in
  `internal/cpitest`. Tests that need a tenant go behind `//go:build integration` and are run
  explicitly with `make test-integration` against a development tenant.
- New tenant operations go into `pkg/ops` (structured result + classified error) and are
  then exposed by a CLI command in `internal/cmd` and, where useful, an MCP tool in
  `internal/mcp`. Do not put tenant logic into cobra commands.
- Errors: wrap usage/config problems with `output.Usagef`, partial results with
  `output.Partial`; HTTP errors from `pkg/httpclnt` are classified automatically. The exit
  codes are a contract (`internal/exitcode`) and must not be renumbered.
- Logs go to stderr only; stdout is reserved for results (and the MCP protocol).
- Never log credentials, tokens or client IDs.
- No new runtime dependencies without agreement.

## Generated files

```bash
go test ./internal/cmd -update    # regenerates docs/commands.md and the testdata/*.golden files
```

Review the diff of regenerated files before committing.

## Upstream

`upstream` points to https://github.com/engswee/flashpipe. Changes are taken over by
cherry-pick only; never rebase onto upstream.
