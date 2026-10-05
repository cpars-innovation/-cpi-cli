# Testing integration flows

`cpictl send` (MCP: `send_test_message`) sends a message to a deployed flow and waits for its
processing log. How the message gets into the flow depends on the flow's trigger.
`cpictl discover` lists the triggers of every flow (`iflows[].triggers`: adapter and address).

| Trigger | How to test | Change to the flow |
|---------|-------------|--------------------|
| HTTPS, SOAP, OData, REST sender | Send directly to the flow's endpoint | none |
| ProcessDirect sender | Send through the **test harness** to the flow's address | none |
| Timer, "run once" | Deploying is the trigger: deploy, then wait for the log | none |
| Timer, scheduled | Trigger by redeploying with a run-once schedule on the development tenant, or a test entry | parameter or test entry |
| SFTP, FTP, mail, JMS, AMQP, Kafka (polling / queue) | Put input where the flow reads it, or a **test entry** | none, or test entry |

All tests run on a development tenant and trigger real processing, including calls to
receivers. Use synthetic data.

## Direct: HTTP-based senders

```bash
cpictl send --artifact-id OrderIntake --body-file order.xml --content-type application/xml --wait 60s
```

The runtime answers with the message GUID (`SAP_MessageProcessingLogID`), so the result has the
HTTP status, the response and the final message log.

## Test harness: ProcessDirect senders

A flow started by ProcessDirect can only be called by other flows on the same tenant. The test
harness is one small flow on the development tenant that forwards an HTTP request to any
ProcessDirect address:

```
HTTPS sender (/cpictl/test) -> Request Reply -> ProcessDirect receiver, address ${header.CpictlTargetAddress}
```

```bash
cpictl send --artifact-id Billing --process-direct /billing/in --body-file invoice.xml --wait 60s
```

cpictl sends the address in the header `CpictlTargetAddress` to the harness
(`--harness`, default `CPICTL_Test_Harness`). The flow behind the address runs with the
harness message's correlation ID; with `--wait` cpictl finds that flow's message log by
correlation ID and `--artifact-id`, and reports its status.

Setting up the harness (once per development tenant; the cpi-test skill of the Claude Code
plugin builds it by copying the components from existing flows of the tenant):

- Integration flow ID `CPICTL_Test_Harness`, in a package for test tooling. Do not transport it.
- HTTPS sender: address `/cpictl/test`, user role `ESBMessaging.send`, CSRF protection
  off (or on: cpictl handles the token).
- Request Reply step with a ProcessDirect receiver whose address is
  `${header.CpictlTargetAddress}` (dynamic address). Verify on your tenant that the ProcessDirect
  receiver accepts the dynamic address.
- Runtime configuration, *Allowed Header(s)*: `CpictlTargetAddress` and the headers your test
  cases send (or `*` on the development tenant).

## Test entry: polling and scheduled flows

For flows that poll (SFTP, mail, JMS, ...) or run on a schedule, the flow can get an additional
entry for tests:

- Move the business logic into a **local integration process**.
- The existing integration process (SFTP sender, timer, ...) only calls it (Process Call).
- A second integration process with a **ProcessDirect sender** `/test/<FlowId>` calls the same
  local process. Test it through the harness:
  `cpictl send --artifact-id <FlowId> --process-direct /test/<FlowId> --body-file input.csv --wait 60s`.

Keep it or drop it: a ProcessDirect address is only reachable from flows of the same tenant,
so a permanent test entry exposes nothing and keeps regression tests possible after every
change. Teams whose guidelines forbid test code in productive content remove it before
transport (and the review checks that). Record the decision in the Testing section of
`.cpi/conventions.md`; the build and review skills follow it.

Alternatives: provide the real input (upload a file to the SFTP directory, put a message on the
queue) and observe the run with
`cpictl logs --artifact-id <FlowId> --since 2m --wait 120s`; or, for content you cannot change,
deploy a copy of the flow with an HTTPS sender (it drifts from the original; use it only as a
last resort).

## Timers

A timer set to *Run once* starts the flow on every deployment:

```bash
cpictl deploy --artifact-ids Cleanup --compare-versions=false
cpictl logs --artifact-id Cleanup --since 2m --wait 120s --errors
```

For scheduled timers, check whether the schedule is externalised (`cpictl params get`): set it to
run once on the development tenant, deploy, test, and set it back.

## Tracing

With log level TRACE the tenant records payload and headers at every step:

```bash
cpictl log-level --artifact-id Billing --level TRACE      # active for 10 minutes
cpictl send --artifact-id Billing --process-direct /billing/in --body-file invoice.xml --wait 60s
cpictl logs trace --message-guid <guid>                    # traced steps with trace IDs
cpictl logs trace-message --id <trace-id>                  # payload, headers, exchange properties
```

The log level is set through the Web UI's operations command
`/Operations/com.sap.it.op.tmn.commands.dashboard.webui.IntegrationComponentSetMplLogLevelCommand`
(there is no OData API for it). `--node-type` (default `IFLMAP`) and `--runtime-location-id`
(default `cloudintegration`) select the runtime. Traces contain business data; values of headers
and properties whose names look sensitive are masked in the output.
