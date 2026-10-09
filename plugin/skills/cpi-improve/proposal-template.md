# Improvements: <scope>

Date: <yyyy-mm-dd> · Scope: <all packages | package X | flows ...> · lint: <counts per severity>

## Summary

| ID | Proposal | Flows | Benefit | Risk | Effort | Fix | Status |
|----|----------|-------|---------|------|--------|-----|--------|
| P1 | Shared logging script into `EDM_Scripts` | 23 | one place to change logging | low | S | lint_fix | proposed |
| P2 | Routing table of `Route_by_Partner` into the Partner Directory | 1 (+12 receivers) | new partner without deployment | medium | M | manual | proposed |

Status: proposed, approved, done, rejected, blocked.

## P1 - <title>

- **Findings:** rules and counts (`duplicate-script` x23, ...)
- **Flows:** list (package/flow)
- **Change:** what exactly changes, file by file or step by step
- **Benefit:** why it is worth it
- **Risk:** what can break, who else is affected (callers from `graph_neighbors`)
- **Deploy order:** e.g. script collection first, then the flows
- **Test plan:** which test cases, which flows, expected result (same as before)
- **Fix:** `lint_fix rules=[duplicate-script] packages=[EDM]` or manual (cpi-build)
- **Status:** proposed

## Kept as is

| Finding | Flow | Why |
|---------|------|-----|
| `hardcoded-endpoint` | Legacy_SAP_PI | migrated next quarter, no change until then |
