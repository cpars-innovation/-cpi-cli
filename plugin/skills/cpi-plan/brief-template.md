# Brief: <short title>

<!-- Fill in what you know; leave the rest empty or write "unknown". Fields marked (*) are needed
     before planning can start. IDs and names may stay empty: the planner proposes them from
     .cpi/conventions.md and you confirm. Put sample files next to this brief in
     .cpi/briefs/<short-title>/ and list them below. No secrets or productive personal data. -->

Requester: <name>    Business owner: <name>    Ticket: <ID or link>
Go-live: <date or "none">    Priority: <low | normal | high>

## Purpose (*)

<2-3 sentences: which business process, what happens today, what should happen.>

## IDs, names, descriptions

| | ID | Name | Description |
|---|---|---|---|
| Package (existing or new) | | | |
| Integration flow | | | |
| Further artifacts (mapping, script collection, value mapping) | | | |

## Sender / trigger (*)

- System: <system and its owner>
- Started by: <HTTPS / SOAP / OData / IDoc / ProcessDirect / SFTP / mail / JMS / timer>
- Synchronous (sender waits for an answer) or asynchronous:
- Address or path (if fixed by the sender):
- Authentication of the sender: <role, client certificate, user>
- Timer: schedule and time zone. Polling (SFTP, mail, ...): directory, file pattern, interval,
  post-processing (delete, archive, move):

## Receivers (*)

| System | Adapter / protocol | Operation or path | Host per environment (dev / qa / prod) | Authentication (credential or certificate name, exists?) |
|---|---|---|---|---|
| | | | | |

## Messages (*)

- Formats in and out: <XML, JSON, CSV, IDoc, ...> and encoding
- Samples: <files in .cpi/briefs/<short-title>/; synthetic or anonymised data only>
- Schemas / WSDL / EDMX / OpenAPI:
- Size (typical / max) and volume (messages per day, peak per minute):

## Mapping and logic

- Field mapping: <table, file or rules>
- Lookups and value mappings (code lists):
- Routing, splitting, aggregation, enrichment (extra calls):
- Filtering (which messages are ignored):

## Errors and behaviour

- What the sender gets on an error:
- Retries (where, how often):
- Duplicates (can messages be sent twice? business key for idempotency):
- Ordering required (yes / no):
- Who is alerted, how:

## Monitoring and data protection

- Business keys to show in the message monitor (custom header properties):
- Payload logging allowed? Personal or confidential data in the messages?

## Environments and configuration

- Values that differ per environment (hosts, paths, directories, credential names):
- Known values for the development tenant:

## Tests and acceptance

- Test data available:
- Receivers that must not be called from tests (or only their test systems):
- Acceptance criteria (what must work for go-live):

## Open questions

-
