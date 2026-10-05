# Monitoring and checks

## Before deploying

```bash
cpictl validate --artifact-id OrderIntake          # tenant check, exit 5 if it fails
cpictl guidelines --artifact-id OrderIntake        # activated design guidelines, exit 5 on violations
```

`guidelines` needs design guidelines activated on the tenant by an administrator; otherwise
the tenant answers with an error.

## Runtime

```bash
cpictl status --artifact-ids OrderIntake,Billing   # given artifacts, including NOT_DEPLOYED
cpictl status --runtime-status ERROR               # all deployed artifacts in ERROR, with error text
cpictl endpoints --artifact-id OrderIntake         # URLs to send test messages to
```

## Test messages

```bash
# Send a file to the flow's endpoint and wait for the processing log (exit 5 if it did not complete)
cpictl send --artifact-id OrderIntake --body-file order.xml --content-type application/xml --wait 60s
echo '{"id":1}' | cpictl send --artifact-id OrderIntake --body-file - --header X-Test=1 --output json
```

Only URLs that the tenant lists for the flow (`endpoints`) are used; `--url` chooses one
when there are several. The result contains the HTTP status, the response (64 KB inline),
the message GUID (`SAP_MessageProcessingLogID`) and, with `--wait`, the message log. Exit
codes: 5 for a non-2xx answer or a message that is not COMPLETED, 3 for 401/403, 6 when the
log is not final in time. The message is processed like any other, including calls to
receivers. Credentials: [configuration.md](configuration.md#runtime-endpoints-test-messages).

For flows without an HTTP sender (ProcessDirect, timers, polling adapters) see
[testing.md](testing.md); it also covers tracing (`log-level`, `logs trace`).

## Message processing logs

```bash
cpictl logs --artifact-id OrderIntake --since 1h                    # newest first, max 200
cpictl logs --status FAILED,ESCALATED --since 1d --errors           # with error texts
cpictl logs --correlation-id <id>
cpictl logs --artifact-id OrderIntake --since 2m --wait 60s --errors # wait until final (exit 6 on timeout)
```

`--since` / `--until`: duration back from now (`30m`, `2h`, `1d`) or RFC 3339 timestamp.
Statuses: `COMPLETED`, `PROCESSING`, `RETRY`, `ESCALATED`, `FAILED`, `CANCELLED`,
`DISCARDED`, `ABANDONED`.

One message:

```bash
cpictl logs get --message-guid <guid>        # error text, custom header properties, adapter attributes,
                                             # attachments, persisted messages (with their IDs)
cpictl logs steps --message-guid <guid>      # processing steps and the failing step (ModelStepId)
cpictl logs attachment --id <attachment-id> [--out file]
cpictl logs payload --id <message-store-entry-id> [--out file]
```

In text mode `logs attachment` / `logs payload` / `resources get` write the content to stdout
(pipe or redirect it); with `--output json` it is part of the result (text, or base64 for binary,
limited by `--max-bytes`, default 64 KB).

### Searching by custom header

The tenant cannot filter message processing logs by custom header properties, so cpictl scans:

```bash
cpictl logs --artifact-id OrderIntake --since 2h --header OrderId=4711
cpictl logs --package-id Orders --since 1h --header OrderId=4711 --top 5
```

A scan always needs `--artifact-id` or `--package-id` and `--since` (the tenant is shared). It
reads at most `--top` × 10 messages (max 500); `scanned` and `truncated` in the result say how
far it got. `--package-id` alone lists the messages of all flows of the package.

### Following a message across flows

`cpictl send` (MCP `send_test_message`) sends a W3C `traceparent` header (unless `--no-trace` or
your own `--header traceparent=...`) and returns its `traceId`. Flows with a tracer script write
`trace-id`, `span-id` and `parent-span-id` as custom header properties; `logs tree` rebuilds the
call tree from them and points at the first failure:

```bash
cpictl logs tree --trace-id 0af7651916cd43dd8448eb211c80319c
cpictl logs tree --trace-id <id> --package-id Orders --since 30m   # scan if not found directly
```

Messages are found by ApplicationMessageId = trace ID first (set `SAP_ApplicationID` to the trace
ID in the tracer), otherwise by scanning the scope (`--max-scan`, default 200). Other property
names: `--trace-property`, `--span-property`, `--parent-property`. Tenants whose sender adapters
need business headers (e.g. `sap-client`) get them with `--header` on `send`.

## Resources of an iFlow

```bash
cpictl resources --artifact-id OrderIntake
cpictl resources get --artifact-id OrderIntake --name script1.groovy --type groovy
cpictl download --artifact-id OrderIntake --dir ./OrderIntake     # whole artifact, extracted
```

## API coverage

| Area | Endpoints |
|------|-----------|
| Logs | `MessageProcessingLogs` (query, single), `ErrorInformation/$value`, `CustomHeaderProperties`, `AdapterAttributes`, `Attachments` |
| Payloads and steps | `MessageProcessingLogAttachments('{Id}')/$value`, `MessageProcessingLogs('{guid}')/MessageStoreEntries`, `MessageStoreEntries('{Id}')/$value`, `MessageProcessingLogs('{guid}')/Runs`, `MessageProcessingLogRuns('{Id}')/RunSteps` |
| Content | `ValidateIntegrationDesigntimeArtifact`, `ExecuteIntegrationDesigntimeArtifactsGuidelines`, `DesignGuidelineExecutionResults`, `ServiceEndpoints`, `IntegrationRuntimeArtifacts`, `Resources`, artifact `$value` |

The endpoints in the second row belong to SAP's Message Store and trace APIs and are not
part of the Message Processing Logs API specification; `logs get` treats them as optional
and reports problems in `warnings` instead of failing.

Required roles: monitoring read access for logs; reading persisted messages and
attachments can require additional roles (e.g. access to message content).
