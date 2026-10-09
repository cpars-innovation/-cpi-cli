package ops

import (
	"bytes"
	"encoding/xml"
	"io"
	"sort"
	"strings"
)

// FlowModel is the structure of an .iflw (BPMN) model as the linter needs
// it: the steps of each process with their configuration and connections.
type FlowModel struct {
	// Elements by ID: steps, events, gateways, subprocesses (not the
	// collaboration's participants and message flows).
	Elements map[string]*FlowElement
	// Flows are the sequence flows.
	Flows []*SequenceFlow
	// Adapters are the sender and receiver channels (message flows).
	Adapters []*FlowElement
	// Order is the document order of the element IDs.
	Order []string
}

// FlowElement is one BPMN element with its ifl:property values.
type FlowElement struct {
	ID, Tag, Name string
	// Container is the ID of the process or subprocess that holds it.
	Container string
	Props     map[string]string
	// Incoming and Outgoing are sequence flow IDs.
	Incoming, Outgoing []string
	// TriggeredByEvent marks an event subprocess (exception subprocess).
	TriggeredByEvent bool
}

// SequenceFlow connects two elements.
type SequenceFlow struct {
	ID, Source, Target, Container, Name string
	Condition                           string
	Props                               map[string]string
}

// Kind is the component name of an element (GroovyScript, Enricher, ...)
// from cmdVariantUri, else its activityType, else the BPMN tag.
func (e *FlowElement) Kind() string {
	if _, cname := variant(e.Props["cmdVariantUri"]); cname != "" {
		return cname
	}
	if t := e.Props["activityType"]; t != "" {
		return t
	}
	return e.Tag
}

// Version is the component version of an element.
func (e *FlowElement) Version() string { return e.Props["componentVersion"] }

// IsStep reports whether an element is a processing step (not an event,
// gateway or container).
func (e *FlowElement) IsStep() bool {
	return e.Tag == "callActivity" || e.Tag == "serviceTask"
}

var containerTags = map[string]bool{"process": true, "subProcess": true}

// ParseFlowModel reads an .iflw model.
func ParseFlowModel(data []byte) (*FlowModel, error) {
	m := &FlowModel{Elements: map[string]*FlowElement{}}
	dec := xml.NewDecoder(bytes.NewReader(data))
	type frame struct {
		tag     string
		el      *FlowElement
		flow    *SequenceFlow
		key     string
		text    strings.Builder
		props   map[string]string
		capture bool
	}
	var stack []*frame
	var containers []string
	owner := func() map[string]string {
		// property -> extensionElements -> owner
		for i := len(stack) - 1; i >= 0; i-- {
			if stack[i].el != nil {
				return stack[i].el.Props
			}
			if stack[i].flow != nil {
				return stack[i].flow.Props
			}
		}
		return nil
	}
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			f := &frame{tag: t.Name.Local}
			attr := func(name string) string {
				for _, a := range t.Attr {
					if a.Name.Local == name {
						return a.Value
					}
				}
				return ""
			}
			id := attr("id")
			for _, fr := range stack {
				if fr.tag == "extensionElements" {
					id = "" // content of a property value, not a model element
				}
			}
			container := ""
			if n := len(containers); n > 0 {
				container = containers[n-1]
			}
			switch {
			case f.tag == "sequenceFlow":
				f.flow = &SequenceFlow{ID: id, Source: attr("sourceRef"), Target: attr("targetRef"), Container: container, Name: attr("name"), Props: map[string]string{}}
				m.Flows = append(m.Flows, f.flow)
			case f.tag == "participant" || f.tag == "messageFlow":
				f.el = &FlowElement{ID: id, Tag: f.tag, Name: attr("name"), Props: map[string]string{}}
				if f.tag == "messageFlow" {
					m.Adapters = append(m.Adapters, f.el)
				}
			case id != "" && (containerTags[f.tag] || container != "") && f.tag != "incoming" && f.tag != "outgoing":
				f.el = &FlowElement{ID: id, Tag: f.tag, Name: attr("name"), Container: container, Props: map[string]string{},
					TriggeredByEvent: attr("triggeredByEvent") == "true"}
				m.Elements[id] = f.el
				m.Order = append(m.Order, id)
			}
			if containerTags[f.tag] && id != "" {
				containers = append(containers, id)
			}
			f.capture = f.tag == "key" || f.tag == "value" || f.tag == "incoming" || f.tag == "outgoing" || f.tag == "conditionExpression"
			stack = append(stack, f)
		case xml.CharData:
			if n := len(stack); n > 0 && stack[n-1].capture {
				stack[n-1].text.Write(t)
			}
		case xml.EndElement:
			n := len(stack)
			if n == 0 {
				continue
			}
			f := stack[n-1]
			stack = stack[:n-1]
			text := f.text.String()
			switch f.tag {
			case "key", "value":
				if n >= 2 {
					p := stack[n-2]
					if p.props == nil {
						p.props = map[string]string{}
					}
					p.props[f.tag] = text
				}
			case "property":
				if props := owner(); props != nil {
					props[f.props["key"]] = f.props["value"]
				}
			case "incoming", "outgoing":
				for i := len(stack) - 1; i >= 0; i-- {
					if el := stack[i].el; el != nil {
						if f.tag == "incoming" {
							el.Incoming = append(el.Incoming, strings.TrimSpace(text))
						} else {
							el.Outgoing = append(el.Outgoing, strings.TrimSpace(text))
						}
						break
					}
				}
			case "conditionExpression":
				for i := len(stack) - 1; i >= 0; i-- {
					if fl := stack[i].flow; fl != nil {
						fl.Condition = strings.TrimSpace(text)
						break
					}
				}
			}
			if containerTags[f.tag] && len(containers) > 0 && f.el != nil && containers[len(containers)-1] == f.el.ID {
				containers = containers[:len(containers)-1]
			}
		}
	}
	// incoming/outgoing from the sequence flows (some models omit them)
	for _, fl := range m.Flows {
		if el := m.Elements[fl.Source]; el != nil && !contains(el.Outgoing, fl.ID) {
			el.Outgoing = append(el.Outgoing, fl.ID)
		}
		if el := m.Elements[fl.Target]; el != nil && !contains(el.Incoming, fl.ID) {
			el.Incoming = append(el.Incoming, fl.ID)
		}
	}
	return m, nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Flow returns a sequence flow by ID.
func (m *FlowModel) Flow(id string) *SequenceFlow {
	for _, f := range m.Flows {
		if f.ID == id {
			return f
		}
	}
	return nil
}

// Unreachable returns the elements of each process that cannot be reached
// from a start event of the same process (event subprocesses and their
// content count as reachable; the containers themselves are skipped).
func (m *FlowModel) Unreachable() []*FlowElement {
	byContainer := map[string][]*FlowElement{}
	for _, id := range m.Order {
		el := m.Elements[id]
		byContainer[el.Container] = append(byContainer[el.Container], el)
	}
	var out []*FlowElement
	for container, els := range byContainer {
		if container == "" {
			continue
		}
		seen := map[string]bool{}
		var queue []string
		for _, el := range els {
			if el.Tag == "startEvent" || el.Tag == "boundaryEvent" || (el.Tag == "subProcess" && el.TriggeredByEvent) {
				queue = append(queue, el.ID)
			}
		}
		if len(queue) == 0 {
			continue // no start: nothing to judge
		}
		for len(queue) > 0 {
			id := queue[0]
			queue = queue[1:]
			if seen[id] {
				continue
			}
			seen[id] = true
			el := m.Elements[id]
			if el == nil {
				continue
			}
			for _, out := range el.Outgoing {
				if f := m.Flow(out); f != nil {
					queue = append(queue, f.Target)
				}
			}
			// a boundary event belongs to the activity it is attached to
		}
		for _, el := range els {
			if !seen[el.ID] && !containerTags[el.Tag] {
				out = append(out, el)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Next returns the single element an element leads to (nil when it has no
// or several outgoing flows).
func (m *FlowModel) Next(el *FlowElement) *FlowElement {
	if len(el.Outgoing) != 1 {
		return nil
	}
	f := m.Flow(el.Outgoing[0])
	if f == nil {
		return nil
	}
	return m.Elements[f.Target]
}
