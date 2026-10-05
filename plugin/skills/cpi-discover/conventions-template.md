# Integration conventions: <tenant / landscape name>

<!-- Written by the cpi-discover skill from .cpi/discovery.json (<date>, <n> flows in <m> packages)
     and maintained by the team. The cpi plan, build, test and review skills follow this file.
     Strength: rule = at least 80 % of flows, common = at least 50 %, observed = less. -->

## Naming

| What | Convention | Strength | Examples |
|------|------------|----------|----------|
| Package ID | | | |
| Package name | | | |
| Integration flow ID | | | |
| Integration flow name | | | |
| Externalised parameters | | | |
| Credentials / key aliases | | | |
| Scripts | | | |

## Structure of a flow

- Sender adapters in use (and when to use which):
- Receiver adapters in use:
- Typical step sequence:
- Local integration processes:

## Error handling

- Exception subprocess: (always / when / how it is built, which script)
- Return exception to sender:
- Retries, dead letter handling, alerting:

## Logging and monitoring

- Log level of the flow:
- Custom header properties written to the message log (SAP_ApplicationID, ...):
- Payload logging (attachments, Persist steps) and when it is allowed:

## Scripts

- Language:
- Shared scripts (identical in several flows) and what they do:
- Script collections:

## Configuration and security

- What is externalised:
- Credential naming and who creates them (`cpictl credentials`):
- Certificates and keystore aliases:

## Testing

- Where test messages live (`.cpi/tests/<FlowId>/`):
- Test harness flow (default `CPICTL_Test_Harness`) and its package:
- Test entries (ProcessDirect `/test/<FlowId>` for polling/scheduled flows): allowed and kept / removed before transport / not allowed
- Tracing: allowed on which tenants, reset the log level afterwards?
- Test data rules (no production data, ...):
- Receivers that must not be called from tests:

## Review checklist additions

- (team-specific checks the reviewer must apply)

## Open questions

- 

---
<!-- .cpi/README.md: This folder holds the integration conventions (conventions.md), the
     discovery facts they were derived from (discovery.json, regenerate with `cpictl discover`)
     and the test cases of the flows (tests/). The cpi skills read it. -->
