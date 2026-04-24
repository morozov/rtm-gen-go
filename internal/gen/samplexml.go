package gen

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

// shapeNode is the parsed structure of an RTM reflection sample
// response. It records element names, attribute names, and
// child-element cardinality (array vs single) — the pieces the
// generator needs to build a typed response struct. Sample
// *values* are explicitly not captured; per spec 008, types come
// from typeTable, not from samples.
type shapeNode struct {
	Name       string
	Attrs      []string      // attribute names, sorted
	Children   []*shapeChild // in first-appearance order
	HasText    bool          // non-empty text content alongside attrs/children
	SelfClosed bool          // every instance seen was a self-closing empty element
	OnlyText   bool          // element has text content and nothing else
}

type shapeChild struct {
	Name    string
	Node    *shapeNode
	IsArray bool
	// IsOptional is true when this child was injected via
	// cross-method union (see shapeindex.go) rather than observed
	// in the current method's own sample. Optional struct-valued
	// children render as pointers so an absent element drops out
	// of JSON/YAML output cleanly.
	IsOptional bool
}

// parseSampleXML turns an RTM reflection `response` XML fragment
// into a shape tree. Methods that return multiple top-level
// elements (e.g. push.getSubscriptions with <subscriptions> +
// <topics>) are supported — the returned root node holds every
// top-level element as one of its children.
func parseSampleXML(frag string) (*shapeNode, error) {
	frag = strings.TrimSpace(frag)
	if frag == "" {
		return &shapeNode{}, nil
	}
	// Wrap so encoding/xml sees a single root regardless of
	// whether the sample has one or many top-level elements.
	// The synthetic wrapper is skipped via depth-tracking.
	dec := xml.NewDecoder(strings.NewReader("<_root>" + frag + "</_root>"))
	root := &shapeNode{}
	stack := []*shapeNode{root}
	sibCounts := []map[string]int{{}}
	depth := 0

	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse sample XML: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if depth == 1 {
				// Synthetic wrapper — contribute nothing.
				continue
			}
			parent := stack[len(stack)-1]
			name := t.Name.Local
			var child *shapeChild
			for _, c := range parent.Children {
				if c.Name == name {
					child = c
					break
				}
			}
			if child == nil {
				child = &shapeChild{Name: name, Node: &shapeNode{Name: name, SelfClosed: true}}
				parent.Children = append(parent.Children, child)
			}
			sibCounts[len(sibCounts)-1][name]++
			if sibCounts[len(sibCounts)-1][name] >= 2 {
				child.IsArray = true
			}
			node := child.Node
			// Merge attributes from this occurrence.
			seen := map[string]struct{}{}
			for _, a := range node.Attrs {
				seen[a] = struct{}{}
			}
			for _, a := range t.Attr {
				if _, ok := seen[a.Name.Local]; !ok {
					node.Attrs = append(node.Attrs, a.Name.Local)
					seen[a.Name.Local] = struct{}{}
				}
			}
			stack = append(stack, node)
			sibCounts = append(sibCounts, map[string]int{})

		case xml.CharData:
			if depth < 2 {
				// Whitespace between top-level elements inside
				// the synthetic wrapper. Ignore.
				continue
			}
			// RTM's samples use literal `...` inside parent
			// elements to mean "more of the same child below".
			// That's sample shorthand, not real text content —
			// strip it before deciding whether the element has
			// meaningful chardata.
			text := strings.TrimSpace(strings.ReplaceAll(string(t), "...", ""))
			if text != "" {
				stack[len(stack)-1].HasText = true
			}

		case xml.EndElement:
			depth--
			if depth == 0 {
				// Closing the synthetic wrapper.
				continue
			}
			if len(stack) == 0 {
				return nil, fmt.Errorf("unbalanced XML: end without start")
			}
			node := stack[len(stack)-1]
			if len(node.Children) > 0 || len(node.Attrs) > 0 || node.HasText {
				node.SelfClosed = false
			}
			if node.HasText && len(node.Attrs) == 0 && len(node.Children) == 0 {
				node.OnlyText = true
			}
			stack = stack[:len(stack)-1]
			sibCounts = sibCounts[:len(sibCounts)-1]
		}
	}

	// RTM's reflection samples wrap the payload in <rsp stat="…">…</rsp>.
	// The generated client's unwrap() strips both "rsp" and "stat" before
	// decoding, so leaving them in the shape would emit a dead
	// Rsp{Stat string} field on every response type and hide real children
	// one level below where overlayTypeTablePath looks for them.
	if len(root.Children) == 1 && root.Children[0].Name == "rsp" {
		rsp := root.Children[0].Node
		root.Children = rsp.Children
		root.Attrs = root.Attrs[:0]
		for _, a := range rsp.Attrs {
			if a == "stat" {
				continue
			}
			root.Attrs = append(root.Attrs, a)
		}
		root.HasText = rsp.HasText
	}

	sortAllAttrs(root)
	return root, nil
}

func sortAllAttrs(n *shapeNode) {
	sort.Strings(n.Attrs)
	for _, c := range n.Children {
		sortAllAttrs(c.Node)
	}
}
