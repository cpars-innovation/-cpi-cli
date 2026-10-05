# Review checklist for integration flows

Apply `.cpi/conventions.md` first; its "Review checklist additions" are mandatory. Then:

## Tenant checks
- `validate_artifact` PASSED.
- `check_guidelines`: every violation is fixed or justified.
- Deployed version equals the reviewed version (`get_runtime_status`), status STARTED.

## Conventions
- Package, flow, parameter, credential and script names follow conventions.md.
- Shared scripts are reused (identical copy or script collection), not modified copies.

## Design
- Matches the plan; deviations are explained.
- No unreachable steps, every route of a router has a sensible end, default routes exist.
- Exception subprocess present where the conventions require it; the sender gets a defined
  answer on errors; errors are not swallowed silently.
- Idempotency / duplicate handling where messages can be resent.
- Large payloads: no needless conversions to String, streaming where possible.

## Configuration and security
- Environment-specific values are externalised; no hard-coded hosts, paths or credential
  names that differ per environment.
- No secrets, tokens or personal data in scripts, parameters, properties or logs.
- Credentials and key aliases the flow refers to exist (`list_credentials`, `list_keystore`)
  and certificates are not about to expire.
- Sender authorization (role-based / client certificate) as the conventions require.

## Logging and monitoring
- Log level as in the conventions; payload logging only where allowed and with personal data
  considered.
- Custom header properties (e.g. SAP_ApplicationID, business keys) written as the conventions say.

## Scripts and mappings
- Scripts: null checks, no `println`, exceptions with meaningful messages, no unused imports,
  no reading the whole body as String for large messages unless needed.
- Mappings: mandatory fields, value mappings instead of hard-coded code lists.

## Tests
- Test cases exist in `.cpi/tests/<FlowId>/` for the happy path and the error paths; they pass.

## Report format

For each finding: severity (**blocker** / **major** / **minor** / **nit**), location (file and
element id, or tenant object), what is wrong, why it matters, suggested fix. End with a verdict:
ready / ready after fixes / not ready.
