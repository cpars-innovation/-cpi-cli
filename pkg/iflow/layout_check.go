package iflow

import (
	"fmt"
	"math"
	"sort"

	"github.com/beevik/etree"
)

// Layout issue kinds.
const (
	IssueMissing  = "missing"  // a step or line is not drawn
	IssueOverlap  = "overlap"  // shapes overlap
	IssueOutside  = "outside"  // a shape lies outside its pool or subprocess
	IssueCrossing = "crossing" // a line runs through a step
	IssueCramped  = "cramped"  // shapes nearly touch
)

// LayoutIssue is a problem of the diagram (not of the flow).
type LayoutIssue struct {
	Kind    string `json:"kind"`
	Element string `json:"element"`
	Message string `json:"message"`
}

// minGap is the space below which neighbouring shapes count as cramped.
const minGap = 10

// CheckLayout lists the problems of a diagram: steps and lines that are not
// drawn, overlapping shapes, shapes outside their pool or subprocess, lines
// through steps and shapes that nearly touch.
func CheckLayout(data []byte) ([]LayoutIssue, error) {
	doc := NewDocument()
	if err := doc.ReadFromBytes(data); err != nil {
		return nil, err
	}
	l := newLayouter(LayoutOptions{}, doc)
	if findFirst(doc.Root(), "BPMNPlane") == nil {
		return []LayoutIssue{{Kind: IssueMissing, Message: "the model has no diagram"}}, nil
	}
	var issues []LayoutIssue
	add := func(kind, id, format string, args ...any) {
		issues = append(issues, LayoutIssue{Kind: kind, Element: id, Message: fmt.Sprintf(format, args...)})
	}
	name := func(id string) string {
		if el := l.byID[id]; el != nil {
			if n := el.SelectAttrValue("name", ""); n != "" {
				return fmt.Sprintf("%q", n)
			}
		}
		return id
	}

	// containers: pools and subprocesses with their nodes
	type container struct {
		id    string
		bound *rect
		nodes []string
		flows []*etree.Element
	}
	var containers []*container
	poolOf := map[string]string{}
	if collab := child(doc.Root(), "collaboration"); collab != nil {
		for _, p := range collab.ChildElements() {
			id := p.SelectAttrValue("id", "")
			switch p.Tag {
			case "participant":
				if ref := p.SelectAttrValue("processRef", ""); ref != "" {
					poolOf[ref] = id
				}
				if _, ok := l.old[id]; !ok {
					add(IssueMissing, id, "%s is not drawn", name(id))
				}
			case "messageFlow":
				if l.edges[id] == nil {
					add(IssueMissing, id, "the line %s is not drawn", name(id))
				}
			}
		}
	}
	var collect func(el *etree.Element, bound *rect)
	collect = func(el *etree.Element, bound *rect) {
		c := &container{id: el.SelectAttrValue("id", ""), bound: bound}
		containers = append(containers, c)
		for _, ch := range el.ChildElements() {
			id := ch.SelectAttrValue("id", "")
			switch {
			case ch.Tag == "sequenceFlow":
				c.flows = append(c.flows, ch)
				if l.edges[id] == nil {
					add(IssueMissing, id, "the line %s is not drawn", name(id))
				}
			case id == "" || nonNodes[ch.Tag]:
			default:
				r, ok := l.old[id]
				if !ok {
					add(IssueMissing, id, "%s is not drawn", name(id))
					continue
				}
				c.nodes = append(c.nodes, id)
				if ch.Tag == "subProcess" {
					rr := r
					collect(ch, &rr)
				}
			}
		}
	}
	for _, p := range doc.Root().ChildElements() {
		if p.Tag != "process" {
			continue
		}
		var bound *rect
		if r, ok := l.old[poolOf[p.SelectAttrValue("id", "")]]; ok {
			bound = &r
		}
		collect(p, bound)
	}

	for _, c := range containers {
		for i, a := range c.nodes {
			ra := l.old[a]
			if c.bound != nil && !inside(ra, *c.bound) {
				add(IssueOutside, a, "%s lies outside its %s", name(a), map[bool]string{true: "pool", false: "subprocess"}[poolOf[c.id] != ""])
			}
			for _, b := range c.nodes[i+1:] {
				rb := l.old[b]
				switch {
				case overlaps(ra, rb, 0):
					add(IssueOverlap, a, "%s overlaps %s", name(a), name(b))
				case overlaps(ra, rb, minGap/2):
					add(IssueCramped, a, "%s and %s nearly touch", name(a), name(b))
				}
			}
		}
		for _, f := range c.flows {
			id := f.SelectAttrValue("id", "")
			edge := l.edges[id]
			if edge == nil {
				continue
			}
			pts := waypoints(edge)
			src, tgt := f.SelectAttrValue("sourceRef", ""), f.SelectAttrValue("targetRef", "")
			for _, n := range c.nodes {
				if n == src || n == tgt {
					continue
				}
				r := shrink(l.old[n], 2)
				for i := 1; i < len(pts); i++ {
					if segmentHits(pts[i-1], pts[i], r) {
						add(IssueCrossing, id, "a line from %s to %s runs through %s", name(src), name(tgt), name(n))
						break
					}
				}
			}
		}
	}

	// senders and receivers must not cover a pool
	var pools, endpoints []string
	for id := range l.old {
		if l.byID[id] != nil && l.byID[id].Tag == "participant" {
			if l.byID[id].SelectAttrValue("processRef", "") != "" {
				pools = append(pools, id)
			} else {
				endpoints = append(endpoints, id)
			}
		}
	}
	sort.Strings(pools)
	sort.Strings(endpoints)
	for i, e := range endpoints {
		for _, p := range pools {
			if overlaps(l.old[e], l.old[p], 0) {
				add(IssueOverlap, e, "%s overlaps the pool %s", name(e), name(p))
			}
		}
		for _, f := range endpoints[i+1:] {
			if overlaps(l.old[e], l.old[f], 0) {
				add(IssueOverlap, e, "%s overlaps %s", name(e), name(f))
			}
		}
	}
	return issues, nil
}

func inside(r, b rect) bool {
	return r.x >= b.x-1 && r.y >= b.y-1 && r.right() <= b.right()+1 && r.bottom() <= b.bottom()+1
}

// overlaps reports whether a and b, grown by pad, intersect.
func overlaps(a, b rect, pad float64) bool {
	return a.x-pad < b.right()+pad && b.x-pad < a.right()+pad && a.y-pad < b.bottom()+pad && b.y-pad < a.bottom()+pad
}

func shrink(r rect, d float64) rect { return rect{r.x + d, r.y + d, r.w - 2*d, r.h - 2*d} }

// segmentHits reports whether the segment p-q passes through r
// (Liang-Barsky clipping).
func segmentHits(p, q point, r rect) bool {
	if r.w <= 0 || r.h <= 0 {
		return false
	}
	dx, dy := q.x-p.x, q.y-p.y
	t0, t1 := 0.0, 1.0
	for _, c := range [][2]float64{{-dx, p.x - r.x}, {dx, r.right() - p.x}, {-dy, p.y - r.y}, {dy, r.bottom() - p.y}} {
		pp, qq := c[0], c[1]
		if pp == 0 {
			if qq < 0 {
				return false
			}
			continue
		}
		t := qq / pp
		if pp < 0 {
			t0 = math.Max(t0, t)
		} else {
			t1 = math.Min(t1, t)
		}
		if t0 > t1 {
			return false
		}
	}
	return t1-t0 > 1e-9
}
