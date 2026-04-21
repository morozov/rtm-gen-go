package gen

import (
	"fmt"
	"sort"
	"strings"

	"github.com/morozov/rtm-gen-go/internal/naming"
)

// emitResponseType returns the Go source fragment for a method's
// response struct (the body, starting with `struct {`). It walks
// the shape tree parsed from the method's reflection sample
// response, overlays any typeTable paths the sample didn't
// surface (RTM's samples are incomplete in places — `sort_order`
// on lists is the canonical miss), and applies typeTable type
// overrides on scalar leaves.
func emitResponseType(method string, root *shapeNode) (string, error) {
	types := map[string]fieldType{}
	if info, ok := typeTable[method]; ok {
		types = info.Response
	}
	if root == nil {
		root = &shapeNode{}
	}
	// Overlay typeTable before emitting so typed paths not
	// represented in the sample still become struct fields.
	for path := range types {
		overlayTypeTablePath(root, path)
	}
	if len(root.Children) == 0 && len(root.Attrs) == 0 {
		return "struct{}", nil
	}
	return renderNodeAsObject(root, nil, types)
}

// overlayTypeTablePath ensures the shape tree rooted at root has
// the structure required to carry a typeTable path. Missing
// intermediate elements are synthesized; missing terminals are
// added as attributes (which render identically to text-content
// child elements in JSON, so the distinction doesn't matter for
// unmarshalling).
func overlayTypeTablePath(root *shapeNode, path string) {
	segs := strings.Split(path, ".")
	cur := root
	for i, seg := range segs {
		isArray := strings.HasSuffix(seg, "[]")
		name := strings.TrimSuffix(seg, "[]")
		last := i == len(segs)-1
		if last {
			if hasAttr(cur, name) || hasChild(cur, name) {
				return
			}
			cur.Attrs = append(cur.Attrs, name)
			sort.Strings(cur.Attrs)
			return
		}
		child := findChild(cur, name)
		if child == nil {
			child = &shapeChild{Name: name, Node: &shapeNode{Name: name}}
			cur.Children = append(cur.Children, child)
		}
		if isArray {
			child.IsArray = true
		}
		cur = child.Node
	}
}

func hasAttr(n *shapeNode, name string) bool {
	for _, a := range n.Attrs {
		if a == name {
			return true
		}
	}
	return false
}

func hasChild(n *shapeNode, name string) bool {
	for _, c := range n.Children {
		if c.Name == name {
			return true
		}
	}
	return false
}

func findChild(n *shapeNode, name string) *shapeChild {
	for _, c := range n.Children {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// renderNodeType returns the Go type expression for a node at the
// given dot-path. The returned string is a complete Go type (e.g.
// `[]struct{...}` or `rtmInt` or `string`).
func renderNodeType(node *shapeNode, path []string, types map[string]fieldType) (string, error) {
	// Self-closed empty element: pass-through untyped. RTM's JSON
	// renders these as `[]` when empty and `{"child":[…]}` when
	// populated — the shape is inconsistent, so we leave it as a
	// raw blob for the caller to inspect if they care.
	if node.SelfClosed && len(node.Attrs) == 0 && len(node.Children) == 0 {
		return "json.RawMessage", nil
	}
	// Text-only leaf: a scalar whose type may be overridden by
	// typeTable.
	if node.OnlyText || (node.HasText && len(node.Attrs) == 0 && len(node.Children) == 0) {
		return scalarGoType(path, types), nil
	}
	// Object with attrs and/or children.
	return renderNodeAsObject(node, path, types)
}

func renderNodeAsObject(node *shapeNode, path []string, types map[string]fieldType) (string, error) {
	var b strings.Builder
	b.WriteString("struct {\n")
	// Text alongside attrs/children (e.g. <time timezone="..">VAL</time>)
	// surfaces as a `$t` JSON key in RTM's XML-to-JSON bridge.
	if node.HasText && (len(node.Attrs) > 0 || len(node.Children) > 0) {
		b.WriteString("\tText string `json:\"$t,omitempty\"`\n")
	}
	for _, attr := range node.Attrs {
		attrPath := append(append([]string{}, path...), attr)
		goType := scalarGoType(attrPath, types)
		b.WriteString("\t")
		b.WriteString(naming.GoField(attr))
		b.WriteString(" ")
		b.WriteString(goType)
		b.WriteString(" `json:\"")
		b.WriteString(attr)
		b.WriteString(jsonTagSuffix(goType))
		b.WriteString("\"`\n")
	}
	for _, child := range node.Children {
		childSeg := child.Name
		if child.IsArray {
			childSeg += "[]"
		}
		childPath := append(append([]string{}, path...), childSeg)
		inner, err := renderNodeType(child.Node, childPath, types)
		if err != nil {
			return "", err
		}
		goType := inner
		if child.IsArray {
			goType = "[]" + inner
		}
		b.WriteString("\t")
		b.WriteString(naming.GoField(child.Name))
		b.WriteString(" ")
		b.WriteString(goType)
		b.WriteString(" `json:\"")
		b.WriteString(child.Name)
		b.WriteString(jsonTagSuffix(goType))
		b.WriteString("\"`\n")
	}
	b.WriteString("}")
	return b.String(), nil
}

// scalarGoType returns the Go type for a leaf at the given path.
// The default is `string`; typeTable entries promote individual
// fields to `rtmBool` / `rtmInt` / `rtmTime`.
func scalarGoType(path []string, types map[string]fieldType) string {
	if t, ok := types[strings.Join(path, ".")]; ok {
		switch t {
		case fieldTypeBool:
			return "rtmBool"
		case fieldTypeInt:
			return "rtmInt"
		case fieldTypeTime:
			return "rtmTime"
		}
	}
	return "string"
}

// jsonTagSuffix returns ",omitempty" for Go types where
// omitempty is safe (strings; pointers; slices; raw messages).
// Typed wrappers (rtmBool, rtmInt, rtmTime) always serialise —
// even zero values carry meaning.
func jsonTagSuffix(goType string) string {
	if goType == "string" || strings.HasPrefix(goType, "[]") || goType == "json.RawMessage" {
		return ",omitempty"
	}
	return ""
}

// typeDeclaration returns a full `type Name = <source>` or
// `type Name <source>` declaration suitable for embedding in a
// generated Go file.
func typeDeclaration(name, source string) string {
	return fmt.Sprintf("type %s %s", name, source)
}
