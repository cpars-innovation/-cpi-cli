---
name: cpi-test
description: Set up and run tests of a deployed SAP Cloud Integration flow - write test cases with sample messages under .cpi/tests/<FlowId>/, send them with send_test_message, check HTTP status, message status, response and logs, and diagnose failures down to the failing step. Use after a deployment, when the user asks to test a flow, to add test cases, or to find out why messages fail.
---

# Test an integration flow

Tests run against a deployed flow on a **development tenant** and trigger real processing,
including calls to receivers. Check `.cpi/conventions.md` (Testing) for receivers that must
not be called and for test data rules. Never use productive data.

## Test cases

One folder per flow, one YAML file per case, payloads next to it:

```
.cpi/tests/<FlowId>/
├── valid-order.yaml
├── valid-order.xml
└── missing-customer.yaml
```

```yaml
# .cpi/tests/OrderIntake/valid-order.yaml
description: Valid order is accepted and forwarded
request:
  method: POST                      # default POST
  content_type: application/xml
  headers: { X-Test-Case: valid-order }
  body_file: valid-order.xml        # or body: "<inline/>"
  endpoint: ""                      # only if the flow has several endpoints (list_service_endpoints)
expect:
  http_status: 200
  message_status: COMPLETED         # COMPLETED, FAILED, ESCALATED, ... ("" = do not wait)
  response_contains: ["<Status>OK</Status>"]
  error_contains: []                # for negative cases: text expected in the error
  custom_headers: { OrderId: "4711" }   # custom header properties expected in the message log
```

Write cases from the plan's test case list (`.cpi/plans/<FlowId>.md`): the happy path, each
validation rule, each receiver error the flow handles. Use small, synthetic payloads; ask the
user for real-world samples with anonymised data if the mapping needs them.

## Running

For each case:

1. `send_test_message` with artifact_id, body, content_type, headers and
   `wait_seconds: 60` (more for slow receivers). The result has `httpStatus`, `response`,
   `messageGuid` and, after the wait, `log` (status, error text, custom headers, attachments).
2. Compare with `expect`. A case passes only if every expectation holds.
3. On a mismatch:
   - `get_message_steps` with the message GUID: the first failing step and its `modelStepId`
     (the id of the element in the .iflw).
   - `get_message_log` / `get_message_attachment` / `get_message_store_entry` for the payload
     at the logged points.
   - Decide whether the flow or the test is wrong. Fix the flow with the cpi-build loop,
     never weaken an expectation just to make the test pass. Ask the user when the expected
     behaviour is unclear.

Report a table: case, result, HTTP status, message status, message GUID, reason.

## Errors that are not the flow's fault

- `auth` on send_test_message: the runtime credentials lack the role ESBMessaging.send or are
  missing (`CPICTL_RUNTIME_OAUTH_CLIENTID/SECRET`, a service key of plan integration-flow). Ask
  the user; do not retry.
- "has no endpoint": the flow is not deployed or has no HTTP-based sender. Timer or polling
  flows (SFTP, mail, JMS) are tested by placing input where the sender reads it; ask the user
  how and use `list_message_logs` with `since` and `wait_seconds` to observe the run.
