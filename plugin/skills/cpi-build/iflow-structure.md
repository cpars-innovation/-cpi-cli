# Integration flow files

What `download_artifact` extracts and `upload_artifact` expects:

```
<FlowId>/
├── META-INF/MANIFEST.MF              Bundle-SymbolicName = flow ID, Bundle-Name = display name,
│                                     Bundle-Version, SAP-BundleType: IntegrationFlow
├── metainfo.prop                     description
└── src/main/resources/
    ├── scenarioflows/integrationflow/<Name>.iflw    the model (BPMN 2.0 XML)
    ├── parameters.prop               externalised parameter values (Java properties, key=value;
    │                                 spaces in keys escaped as "\ ")
    ├── parameters.propdef            parameter definitions
    ├── script/                       Groovy (.groovy, .gsh) and JavaScript (.js) scripts
    ├── mapping/                      message mappings (.mmap), XSLT (.xsl, .xslt)
    ├── xsd/, wsdl/, json/, edmx/     schemas and service definitions
    └── ...
```

## The model (.iflw)

- `bpmn2:collaboration` holds the flow settings (properties such as `log`,
  `returnExceptionToSender`), the participants (sender and receiver systems) and the
  `bpmn2:messageFlow` elements, which are the **adapters** (`ComponentType` HTTPS, SOAP, SFTP,
  HTTP, ...; `direction` Sender/Receiver).
- `bpmn2:process` elements are the integration process and local processes. Steps are
  `bpmn2:callActivity` / `bpmn2:serviceTask` / events / gateways connected by
  `bpmn2:sequenceFlow` (`sourceRef`, `targetRef`, and `bpmn2:incoming`/`bpmn2:outgoing` in the steps).
- An exception subprocess is a `bpmn2:subProcess` whose type is `ErrorEventSubProcessTemplate`.
- Every element is configured by `<ifl:property><key>…</key><value>…</value></ifl:property>`
  entries in its `bpmn2:extensionElements`. `cmdVariantUri` names the component and version,
  e.g. `ctype::FlowstepVariant/cname::GroovyScript/version::1.1.2`
  (Enricher = Content Modifier, GroovyScript, MessageMapping, XSLTMapping, ...).
- `bpmndi:BPMNDiagram` holds the layout (shapes and edges with coordinates). Every element and
  sequence flow needs a shape/edge there, or the Web UI cannot display the flow.
- `{{Key}}` in a property value refers to an externalised parameter in parameters.prop.
- `get_message_steps` returns `modelStepId`: the `id` attribute of the failing element.

## Editing safely

- Change property values in place; keep `id`s unique and references (`sourceRef`, `targetRef`,
  `incoming`, `outgoing`, `processRef`, diagram `bpmnElement`) consistent.
- To add a step, copy a complete element of the same type from a flow of this tenant, including
  its diagram shape, give it a new unique id and rewire the sequence flows.
- Keep component versions (`componentVersion`, `cmdVariantUri`) as copied; the tenant's
  check (`validate_artifact`) reports unsupported combinations.
- Contents of table properties (headers and properties of a Content Modifier) are escaped XML
  inside the value (`&lt;row&gt;…`): keep the escaping.

## Renaming a copied flow

Change the folder name, `Bundle-SymbolicName` and `Bundle-Name` in MANIFEST.MF, the model's file
name if the conventions say so, and the sender address (HTTPS `urlPath`, SOAP address) so it
does not clash with the original flow, which would fail the deployment.
