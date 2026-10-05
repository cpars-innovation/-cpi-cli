---
name: cpi-plan
description: Plan a new or changed SAP Cloud Integration flow before building it - clarify the interface (sender, receiver, protocols, payloads, mapping, error handling, volumes), decide the package, IDs and steps according to .cpi/conventions.md, and write the design to .cpi/plans/<FlowId>.md. Use when the user describes an integration requirement or asks for a design, an estimate or "how would we build ...".
---

# Plan an integration flow

The goal is a design that the cpi-build skill can implement without guessing, and that the
user has agreed to. Do not touch the tenant in this phase except for read-only lookups.

## 1. Read the context

- `.cpi/conventions.md` (if missing, suggest running the cpi-discover skill first; without it,
  say which choices are your assumptions).
- Similar existing flows: `.cpi/discovery.json` lists adapters and steps per flow. Find flows
  with the same sender/receiver adapters and reuse their structure. Name them in the plan.
  `graph_search` finds flows, addresses and systems quickly (e.g. the receiver host, a
  ProcessDirect address); `graph_neighbors` shows which flows already call a system or share a
  credential.
- Changing an existing flow: `graph_neighbors` with `direction: "in"`, `edge_types:
  ["sends_to"]` lists the flows that call it; a changed address or payload affects them. Name
  them in the plan.
- Existing packages (`list_packages`) and credentials (`list_credentials`) that the flow can use.

## 2. Clarify

Ask only what you cannot find out yourself, grouped in one message. Typical gaps:

- Trigger: who sends (system, protocol, sync/async), or a timer / polling?
- Receiver(s): systems, protocols, authentication (which credential or certificate exists?).
- Payloads: formats, sample messages or schemas (ask for files), mapping rules.
- Behaviour on errors: what the sender must get back, retries, alerting.
- Volumes and sizes, ordering, idempotency (duplicates).
- Which receivers must not be called from tests.

## 3. Write `.cpi/plans/<FlowId>.md`

```markdown
# <Flow name> (<FlowId>)
Package: <PackageId> (exists | new)    Status: draft | agreed
## Purpose
## Interface
Sender: <adapter, address/path, auth>      Receiver(s): <adapter, address, credential>
## Message flow
1. <step> - <component type, e.g. Content Modifier / Groovy script / Message Mapping> - why
## Error handling and logging      (follow conventions.md; name the deviations)
## Externalised parameters        (key, meaning, value per environment if known)
## Security material needed       (credentials/certificates to create with `cpictl credentials`)
## Test cases                     (input -> expected HTTP status, message status, output)
## Reference flows                (existing flows this design copies from)
## Open points
```

- IDs and names must follow conventions.md; spell them out, the builder will not invent them.
- Prefer components and scripts the tenant already uses over new ones. Every deviation from
  a convention needs a reason in the plan.
- List test cases for the happy path and at least one error case per receiver and per
  validation rule. The cpi-test skill turns them into executable cases.

## 4. Agree

Present the plan in a few lines (interface, steps, open points) and ask for confirmation.
Set `Status: agreed` only after the user said so. Building starts with the cpi-build skill.
