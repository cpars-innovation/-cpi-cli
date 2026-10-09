# Partner Directory patterns

The Partner Directory holds partner-specific values outside the flows: string parameters (URLs,
credential names, IDs, switches) and binary parameters (XSLT, XSD, value lists) per partner ID
(PID). A new partner or a changed value is then data (`pd_deploy`), not a new deployment.

Before proposing a change: `pd_dependencies` (which flows already read which parameters),
`get_pd_parameters` / `pd_diff` (what exists), the conventions for PID and parameter names.

## Reading Partner Directory values in a flow

- **Script**: `PartnerDirectoryService` (`com.sap.it.api.pd.PartnerDirectoryService`, from
  `ITApiFactory.getService(PartnerDirectoryService.class, null)`): `getParameter(id, pid, String.class)`
  for string parameters; `getAlternativePartnerId(agency, scheme, alternativeId)` to find the PID
  from an ID in the message. Put the values into exchange properties.
- **Mappings and validation**: a binary parameter as XSLT or XSD with the URI
  `pd:<PID>:<ParameterID>:Binary`.
- **Adapters**: use the properties in the receiver's dynamic fields (`${property.ReceiverAddress}`,
  credential name where the adapter allows a dynamic value).

## Pattern 1: routing table (`router-literals`)

A router with one branch per value of a header (partner, company code, message type), each going
to another receiver:

1. One PID per value (or an alternative partner ID mapping value -> PID).
2. String parameters per PID: receiver address, credential name, ProcessDirect address of a
   partner-specific subflow, flags.
3. In the flow: determine the PID (header, alternative partner ID), read the parameters into
   properties, one dynamic receiver (or a ProcessDirect call to the address from the PD).
4. Keep a router only for branches that really process differently; route those by a PD flag.

Risk: a missing PID must fail clearly (exception subprocess with a useful text), not route to a
default.

## Pattern 2: lookup table in a script (`lookup-table-in-script`)

- Code lists (unit, country, status codes, the same for every partner): value mapping artifact.
- Partner-specific values: Partner Directory string or binary parameters.
- Small fixed technical maps that never change: may stay; say so in the proposal.

## Pattern 3: one flow deployed per partner (`deployment-copies`)

The same artifact folder deployed several times with different `configOverrides` (one ID per
partner, e.g. `UtilitiesBase_MDX_to_EDM_Outbound` and `UtilitiesBase_Herrenberg_MDX_to_EDM_Outbound`):

1. List what the overrides change per copy (URLs, credentials, IDs).
2. Move those values to the Partner Directory, one PID per copy.
3. The flow determines the PID from the message or the sender (sender address per partner may
   remain the only difference: then one flow with several sender paths, or one path with the
   partner in a header).
4. Deploy the single flow, switch the senders, then undeploy the copies (with the user, per
   partner) and remove them from the deploy config.

This is the biggest change of the three: propose it per flow family, with a migration and a
fallback (copies stay deployed until every partner is switched).

## Pattern 4: fixed receiver addresses (`hardcoded-endpoint`)

The same address for every environment and partner: externalise it (`{{ReceiverURL}}`, set per
environment with configure). Different per partner: Partner Directory (pattern 1).
