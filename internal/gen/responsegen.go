package gen

import (
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
	enums := map[string]string{}
	if info, ok := typeTable[method]; ok {
		types = info.Response
		enums = info.ResponseEnums
	}
	if root == nil {
		root = &shapeNode{}
	}
	// Overlay typeTable before emitting so typed paths not
	// represented in the sample still become struct fields.
	// Sort to keep insertion order into root.Children stable —
	// map iteration is randomised and would otherwise flap when
	// two roots both need synthesizing.
	overlayPaths := make([]string, 0, len(types)+len(enums))
	for path := range types {
		overlayPaths = append(overlayPaths, path)
	}
	for path := range enums {
		overlayPaths = append(overlayPaths, path)
	}
	sort.Strings(overlayPaths)
	for _, path := range overlayPaths {
		overlayTypeTablePath(root, path)
	}
	if len(root.Children) == 0 && len(root.Attrs) == 0 {
		return "struct{}", nil
	}
	return renderNodeAsObject(root, nil, types, enums)
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

// isCollectionWrapper reports whether a node exists purely to
// wrap a single kind of child element — no attributes, no text,
// exactly one distinct child name. Such wrappers are flattened
// in the emitted Go type using encoding/xml's `parent>child`
// path syntax so callers write `resp.Contacts[0]` instead of
// `resp.Contacts.Contact[0]`.
func isCollectionWrapper(n *shapeNode) bool {
	return len(n.Attrs) == 0 && !n.HasText && len(n.Children) == 1
}

// renderNodeType returns the Go type expression for a node at the
// given dot-path. The returned string is a complete Go type (e.g.
// `[]struct{...}` or `rtmInt` or `string`).
func renderNodeType(node *shapeNode, path []string, types map[string]fieldType, enums map[string]string) (string, error) {
	// Self-closed empty element with no structure: preserve the
	// raw inner XML so the rare caller that cares can inspect it.
	// Sample-time self-closing doesn't guarantee runtime emptiness.
	if node.SelfClosed && len(node.Attrs) == 0 && len(node.Children) == 0 {
		return "struct {\n\tInnerXML string `xml:\",innerxml\" json:\"-\"`\n}", nil
	}
	// Text-only leaf: a scalar whose type may be overridden by
	// typeTable.
	if node.OnlyText || (node.HasText && len(node.Attrs) == 0 && len(node.Children) == 0) {
		return scalarGoType(path, types, enums), nil
	}
	// Object with attrs and/or children.
	return renderNodeAsObject(node, path, types, enums)
}

func renderNodeAsObject(node *shapeNode, path []string, types map[string]fieldType, enums map[string]string) (string, error) {
	var b strings.Builder
	b.WriteString("struct {\n")
	// Text alongside attrs/children (e.g. <time timezone="..">VAL</time>)
	// surfaces as chardata on an inline Text field. The JSON key
	// keeps RTM's historical `$t` convention so output formatters
	// that re-render via json.Marshal produce stable keys.
	if node.HasText && (len(node.Attrs) > 0 || len(node.Children) > 0) {
		b.WriteString("\tText string `xml:\",chardata\" json:\"$t,omitempty\"`\n")
	}
	for _, attr := range node.Attrs {
		attrPath := append(append([]string{}, path...), attr)
		goType := scalarGoType(attrPath, types, enums)
		b.WriteString("\t")
		b.WriteString(naming.GoField(attr))
		b.WriteString(" ")
		b.WriteString(goType)
		b.WriteString(" `xml:\"")
		b.WriteString(attr)
		b.WriteString(",attr")
		b.WriteString(omitSuffix(goType))
		b.WriteString("\" json:\"")
		b.WriteString(attr)
		b.WriteString(jsonOmitSuffix(goType))
		b.WriteString("\"`\n")
	}
	for _, child := range node.Children {
		if err := renderChildField(&b, child, path, types, enums); err != nil {
			return "", err
		}
	}
	b.WriteString("}")
	return b.String(), nil
}

// renderChildField emits one child field, honouring the
// collection-wrapper flattening rule (see isCollectionWrapper).
// When flattened, the wrapper element still participates in the
// dot-path passed downstream for typeTable lookup: a path like
// `contacts.contact[].id` resolves the same with or without
// flattening. The JSON tag uses the wrapper name so output
// formatters see the user-visible collection name, not the
// inner element name.
func renderChildField(b *strings.Builder, child *shapeChild, path []string, types map[string]fieldType, enums map[string]string) error {
	if !child.IsArray && isCollectionWrapper(child.Node) {
		gc := child.Node.Children[0]
		gcSeg := gc.Name
		if gc.IsArray {
			gcSeg += "[]"
		}
		gcPath := append(append([]string{}, path...), child.Name, gcSeg)
		inner, err := renderNodeType(gc.Node, gcPath, types, enums)
		if err != nil {
			return err
		}
		goType := "[]" + inner
		b.WriteString("\t")
		b.WriteString(naming.GoField(child.Name))
		b.WriteString(" ")
		b.WriteString(goType)
		b.WriteString(" `xml:\"")
		b.WriteString(child.Name)
		b.WriteString(">")
		b.WriteString(gc.Name)
		b.WriteString("\" json:\"")
		b.WriteString(child.Name)
		b.WriteString(",omitempty\"`\n")
		return nil
	}
	childSeg := child.Name
	if child.IsArray {
		childSeg += "[]"
	}
	childPath := append(append([]string{}, path...), childSeg)
	inner, err := renderNodeType(child.Node, childPath, types, enums)
	if err != nil {
		return err
	}
	goType := inner
	switch {
	case child.IsArray:
		goType = "[]" + inner
	case child.IsOptional && strings.HasPrefix(inner, "struct {"):
		// A struct that's present in other methods' samples but
		// absent from this one. Pointer-wrap so json.Marshal can
		// drop it via omitempty when the runtime element doesn't
		// appear; encoding/xml auto-allocates on decode when the
		// element is present.
		goType = "*" + inner
	}
	b.WriteString("\t")
	b.WriteString(naming.GoField(child.Name))
	b.WriteString(" ")
	b.WriteString(goType)
	b.WriteString(" `xml:\"")
	b.WriteString(child.Name)
	b.WriteString("\" json:\"")
	b.WriteString(child.Name)
	b.WriteString(jsonOmitSuffix(goType))
	b.WriteString("\"`\n")
	return nil
}

// scalarGoType returns the Go type for a leaf at the given path.
// Enum-typed paths win first (producing the catalogue alias like
// `Priority`); then typeTable promotes to `rtmBool` / `rtmInt` /
// `rtmTime`; the default is `string`.
func scalarGoType(path []string, types map[string]fieldType, enums map[string]string) string {
	key := strings.Join(path, ".")
	if enumKey, ok := enums[key]; ok {
		if def, ok := enumCatalogue[enumKey]; ok {
			return def.Name
		}
	}
	if t, ok := types[key]; ok {
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

// omitSuffix returns ",omitempty" for Go types where empty is a
// sensible "absent" sentinel — strings, slices, and pointers to
// structs. Typed wrappers (rtmBool, rtmInt, rtmTime) always
// serialise because zero values carry meaning (false, 0, null).
func omitSuffix(goType string) string {
	if goType == "string" || strings.HasPrefix(goType, "[]") || strings.HasPrefix(goType, "*") {
		return ",omitempty"
	}
	return ""
}

// jsonOmitSuffix is the JSON-tag counterpart to omitSuffix. It
// behaves the same except for rtmTime, which uses `omitzero`
// (Go 1.24+) paired with rtmTime.IsZero to drop absent
// timestamps from JSON/YAML output instead of rendering them as
// null. rtmBool and rtmInt keep their zero values (false, 0)
// because those carry meaning distinct from absence.
func jsonOmitSuffix(goType string) string {
	if goType == "rtmTime" {
		return ",omitzero"
	}
	return omitSuffix(goType)
}
