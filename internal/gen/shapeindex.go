package gen

import (
	"sort"

	"github.com/morozov/rtm-gen-go/internal/apispec"
)

// shapeIndex aggregates populated-element shapes seen across
// every method's sample response. Two lookup tables:
//
//   - populatedContainers, keyed by `parentName.containerName`:
//     records the child list of containers that appear populated
//     somewhere. E.g. rtm.tasks.addTags's sample shows
//     <taskseries><tags><tag>coffee</tag>…</tags></taskseries>,
//     registering `taskseries.tags` → [tag(IsArray, text-only)].
//     Used first when enriching an opaque container: if the
//     same (parent, container) path is populated anywhere, that
//     shape wins.
//   - populatedByName, keyed by element name: a union-merged
//     shape for each element seen with attrs or text. Consulted
//     only for elements named in opaqueChildConvention (notes,
//     participants) — cases where no sample populates the
//     container at any path but the expected child element's
//     shape is available from a different context.
type shapeIndex struct {
	populatedContainers map[string][]*shapeChild
	populatedByName     map[string]*shapeNode
	// childrenByParent records the union of child elements seen
	// for each parent element name across every sample. Used to
	// inject children that the current method's sample omits but
	// other methods' samples have populated at the same anchor
	// (e.g. `<rrule>` under `<taskseries>` exists in several
	// write-method samples but is absent from the
	// `rtm.tasks.getList` sample). Children injected this way are
	// marked IsOptional so the emitted Go field is a pointer and
	// drops out of JSON/YAML output when absent.
	childrenByParent map[string][]*shapeChild
	// childSetsByParent records the set of child element names
	// seen under each parent element name, per sample occurrence.
	// A parent is "chain-compatible" iff every pair of observed
	// sets is in subset/superset relation. Non-chain parents
	// (e.g. `<list>`: `{filter}` in lists.* vs `{taskseries}` in
	// tasks.*) are flagged context-dependent and excluded from
	// cross-method child union.
	childSetsByParent map[string][]map[string]bool
	// contextDependentParents is the set of element names whose
	// children vary in incompatible ways across samples — element
	// names where cross-method child union would graft children
	// from a different semantic context onto a node that shouldn't
	// carry them.
	contextDependentParents map[string]struct{}
	// ambiguousNames carries element names whose shape varies
	// across sample occurrences (e.g. `<tag>` appears as
	// text-only inside `<taskseries>` but as attrs-only at the
	// top of `rtm.tags.getList`). Ambiguous names MUST NOT be
	// consulted through populatedByName — the merged shape would
	// be correct for neither context.
	ambiguousNames map[string]struct{}
}

// opaqueChildConvention maps a container element name to the
// name of its repeated child, for containers that RTM never
// populates in any sample. `<notes>` is always self-closed in
// taskseries-bearing samples, but `<note>` appears populated as
// the top-level response of rtm.tasks.notes.add; the convention
// lets the enricher borrow that shape. `<participants>` never
// populates anywhere, but its child is known to be <contact> by
// RTM naming convention (the plural-of-child rule doesn't apply
// here, which is why this table is explicit).
var opaqueChildConvention = map[string]string{
	"notes":        "note",
	"participants": "contact",
}

// buildShapeIndex parses every method's sample response and
// records what's populated. Parse errors surface as generator
// failures.
func buildShapeIndex(spec apispec.Spec) (*shapeIndex, error) {
	idx := &shapeIndex{
		populatedContainers:     map[string][]*shapeChild{},
		populatedByName:         map[string]*shapeNode{},
		childrenByParent:        map[string][]*shapeChild{},
		childSetsByParent:       map[string][]map[string]bool{},
		contextDependentParents: map[string]struct{}{},
		ambiguousNames:          map[string]struct{}{},
	}
	for _, m := range spec {
		root, err := parseSampleXML(m.Response)
		if err != nil {
			return nil, err
		}
		walkShape(root, func(n *shapeNode) {
			if n.Name != "" {
				for _, c := range n.Children {
					if len(c.Node.Children) > 0 {
						key := n.Name + "." + c.Name
						if _, seen := idx.populatedContainers[key]; !seen {
							idx.populatedContainers[key] = c.Node.Children
						}
					}
					if !hasChildWithName(idx.childrenByParent[n.Name], c.Name) {
						idx.childrenByParent[n.Name] = append(idx.childrenByParent[n.Name], c)
					}
				}
				if len(n.Children) > 0 {
					set := make(map[string]bool, len(n.Children))
					for _, c := range n.Children {
						set[c.Name] = true
					}
					idx.childSetsByParent[n.Name] = append(idx.childSetsByParent[n.Name], set)
				}
			}
			if n.Name == "" {
				return
			}
			if len(n.Attrs) == 0 && !n.HasText {
				return
			}
			if existing, ok := idx.populatedByName[n.Name]; ok {
				if existing.OnlyText != n.OnlyText || existing.HasText != n.HasText {
					idx.ambiguousNames[n.Name] = struct{}{}
				}
				mergeShapeByName(existing, n)
			} else {
				idx.populatedByName[n.Name] = copyShapeMinimal(n)
			}
		})
	}
	// Post-process: flag parents whose observed child-name sets
	// are pairwise chain-incompatible (neither set is a subset of
	// the other). Those are context-dependent parents (e.g.
	// `<list>` seen with `{filter}` in lists.* samples and
	// `{taskseries}` in tasks.*) where cross-method union would
	// inject children from a different semantic context.
	for name, sets := range idx.childSetsByParent {
		if chainCompatible(sets) {
			continue
		}
		idx.contextDependentParents[name] = struct{}{}
	}
	return idx, nil
}

// chainCompatible reports whether every pair of sets is in a
// subset/superset relation, i.e. they can be ordered by
// inclusion. Used to distinguish "same element, optional child
// sometimes present" (chain-compatible) from "same element name,
// different children in different contexts" (incompatible).
func chainCompatible(sets []map[string]bool) bool {
	for i := 0; i < len(sets); i++ {
		for j := i + 1; j < len(sets); j++ {
			if !isSubset(sets[i], sets[j]) && !isSubset(sets[j], sets[i]) {
				return false
			}
		}
	}
	return true
}

func isSubset(a, b map[string]bool) bool {
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// copyShapeMinimal returns a new shape node carrying the leaf
// attributes and flags of n, excluding Children. Used when
// borrowing a cross-method shape — the borrower synthesises
// its own child list from index lookups or convention.
func copyShapeMinimal(n *shapeNode) *shapeNode {
	out := &shapeNode{
		Name:       n.Name,
		HasText:    n.HasText,
		OnlyText:   n.OnlyText,
		SelfClosed: n.SelfClosed,
	}
	if len(n.Attrs) > 0 {
		out.Attrs = append([]string{}, n.Attrs...)
	}
	return out
}

// mergeShapeByName folds from into into: attrs union, HasText
// OR'd, OnlyText and SelfClosed kept only when every observed
// occurrence preserved them.
func mergeShapeByName(into, from *shapeNode) {
	seen := map[string]struct{}{}
	for _, a := range into.Attrs {
		seen[a] = struct{}{}
	}
	for _, a := range from.Attrs {
		if _, ok := seen[a]; !ok {
			into.Attrs = append(into.Attrs, a)
			seen[a] = struct{}{}
		}
	}
	sort.Strings(into.Attrs)
	if from.HasText {
		into.HasText = true
	}
	if !from.OnlyText {
		into.OnlyText = false
	}
	if !from.SelfClosed {
		into.SelfClosed = false
	}
}

// enrichOpaqueContainers walks root and fills every opaque
// container (no attrs, no children, no text) using the index:
//   - populatedContainers first, keyed by parent+container name;
//   - opaqueChildConvention + populatedByName as fallback.
//
// Containers matching neither stay opaque and render as an
// InnerXML capture wrapper downstream.
func enrichOpaqueContainers(root *shapeNode, idx *shapeIndex) {
	walkShape(root, func(n *shapeNode) {
		for _, c := range n.Children {
			cn := c.Node
			if len(cn.Children) > 0 || len(cn.Attrs) > 0 || cn.HasText {
				continue
			}
			if n.Name != "" {
				key := n.Name + "." + c.Name
				if kids, ok := idx.populatedContainers[key]; ok {
					cn.Children = cloneChildren(kids, idx)
					cn.SelfClosed = false
					continue
				}
			}
			if childName, ok := opaqueChildConvention[c.Name]; ok {
				if _, ambig := idx.ambiguousNames[childName]; ambig {
					continue
				}
				if shape, ok := idx.populatedByName[childName]; ok {
					cn.Children = []*shapeChild{{
						Name:    childName,
						IsArray: true,
						Node:    copyShapeMinimal(shape),
					}}
					cn.SelfClosed = false
				}
			}
		}
	})
	unionMissingChildren(root, idx)
}

// unionMissingChildren walks root and, for every non-ambiguous
// node, injects any child seen under the same parent-name in
// other samples that is absent from this shape. Injected
// children are marked IsOptional so the emitted Go field is a
// pointer (for struct-valued children) and drops cleanly from
// output when the runtime element is absent. This closes the
// sample-coverage gap where some methods document a parent
// element without one of its populated children (e.g.
// `rtm.tasks.getList`'s sample has no `<rrule>` inside
// `<taskseries>`, but several write methods' samples do).
func unionMissingChildren(root *shapeNode, idx *shapeIndex) {
	walkShape(root, func(n *shapeNode) {
		if n.Name == "" {
			return
		}
		if _, ambig := idx.ambiguousNames[n.Name]; ambig {
			return
		}
		if _, ctx := idx.contextDependentParents[n.Name]; ctx {
			return
		}
		for _, c := range idx.childrenByParent[n.Name] {
			if hasChildWithName(n.Children, c.Name) || hasAttr(n, c.Name) {
				continue
			}
			cloned := cloneChild(c, idx)
			cloned.IsOptional = true
			n.Children = append(n.Children, cloned)
		}
	})
}

func hasChildWithName(xs []*shapeChild, name string) bool {
	for _, c := range xs {
		if c.Name == name {
			return true
		}
	}
	return false
}

// cloneChild returns a deep copy of c with the same leaf
// attributes and flags, and a one-level-deep copy of Children.
func cloneChild(c *shapeChild, idx *shapeIndex) *shapeChild {
	out := &shapeChild{
		Name:    c.Name,
		IsArray: c.IsArray,
		Node:    copyShapeMinimal(c.Node),
	}
	for _, gc := range c.Node.Children {
		out.Node.Children = append(out.Node.Children, &shapeChild{
			Name:    gc.Name,
			IsArray: gc.IsArray,
			Node:    copyShapeMinimal(gc.Node),
		})
	}
	if _, ambig := idx.ambiguousNames[c.Name]; !ambig {
		if richer, ok := idx.populatedByName[c.Name]; ok {
			mergeShapeByName(out.Node, richer)
		}
	}
	return out
}

// walkShape calls visit on n and every descendant node,
// recursing through each child's Node.
func walkShape(n *shapeNode, visit func(*shapeNode)) {
	visit(n)
	for _, c := range n.Children {
		walkShape(c.Node, visit)
	}
}

// cloneChildren returns a deep-copied children slice via
// cloneChild, which also enriches each leaf shape from
// populatedByName when safe. This lets a method whose sample
// shows a minimal form (e.g. rtm.groups.getList's
// `<contact id="1"/>`) still surface richer attrs (fullname,
// username) seen in other samples (rtm.contacts.*).
func cloneChildren(kids []*shapeChild, idx *shapeIndex) []*shapeChild {
	out := make([]*shapeChild, len(kids))
	for i, c := range kids {
		out[i] = cloneChild(c, idx)
	}
	return out
}
