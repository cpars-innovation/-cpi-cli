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
