package gen

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/morozov/rtm-gen-go/internal/apispec"
	"github.com/morozov/rtm-gen-go/internal/naming"
)

// SchemaMajor is the current response JSON Schema contract's
// major version. It is baked into every emitted `$id` URL and
// into the output directory layout on the GitHub Pages site.
// Bump deliberately when a response-shape change is not
// additive (field removal, type narrowing, optional → required).
const SchemaMajor = 1

// schemaBaseURL prefixes every emitted `$id`. The trailing
// `/schemas/v<SchemaMajor>` segment is appended per-method.
const schemaBaseURL = "https://morozov.github.io/rtm-cli-go/schemas"

// jsonSchemaDraft is the `$schema` URL every emitted schema
// declares. Draft 2020-12 is the current stable draft.
const jsonSchemaDraft = "https://json-schema.org/draft/2020-12/schema"

// sharedDefName maps an RTM element name to the $defs key under
// which its shape is registered when encountered in a method's
// response tree. Only elements whose shape is stable across
// every occurrence in every sample appear here; context-
// dependent shapes (e.g. <list> in lists.* vs tasks.*) stay
// inlined per method. Enum aliases get their own entries via
// ensureEnumDef.
var sharedDefName = map[string]string{
	"taskseries": "Taskseries",
	"task":       "Task",
	"contact":    "Contact",
	"note":       "Note",
	"rrule":      "Rrule",
}

// GenerateSchemas emits one JSON Schema (draft 2020-12) file
// per method in spec into cfg.OutDir, named
// `<rtm.method.name>.json`. Returns the paths of written files.
func GenerateSchemas(spec apispec.Spec, cfg SchemaConfig) ([]string, error) {
	if cfg.OutDir == "" {
		return nil, fmt.Errorf("OutDir is empty: %w", ErrInvalidConfig)
	}
	idx, err := buildShapeIndex(spec)
	if err != nil {
		return nil, fmt.Errorf("build shape index: %w", err)
	}
	if err := os.MkdirAll(cfg.OutDir, dirPerm); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", cfg.OutDir, err)
	}
	names := make([]string, 0, len(spec))
	for n := range spec {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]string, 0, len(names)+1)
	for _, name := range names {
		m := spec[name]
		data, err := emitMethodSchema(m, idx)
		if err != nil {
			return nil, fmt.Errorf("schema for %s: %w", name, err)
		}
		data = append(data, '\n')
		path := filepath.Join(cfg.OutDir, name+".json")
		if err := os.WriteFile(path, data, filePerm); err != nil {
			return nil, fmt.Errorf("write %s: %w", path, err)
		}
		out = append(out, path)
	}
	// VERSION sidecar: the integer schema major. Consumers (e.g.
	// the gh-pages publish workflow) read this to build the
	// target `schemas/v<MAJOR>/` directory without having to grep
	// Go source for the constant.
	versionPath := filepath.Join(cfg.OutDir, "VERSION")
	if err := os.WriteFile(versionPath, []byte(fmt.Sprintf("%d\n", SchemaMajor)), filePerm); err != nil {
		return nil, fmt.Errorf("write %s: %w", versionPath, err)
	}
	out = append(out, versionPath)
	return out, nil
}

// SchemaConfig parameterises GenerateSchemas.
type SchemaConfig struct {
	OutDir string
}

// emitMethodSchema returns the JSON Schema document for one
// RTM method's response as a pretty-printed byte slice.
func emitMethodSchema(m apispec.Method, idx *shapeIndex) ([]byte, error) {
	shape, err := parseSampleXML(m.Response)
	if err != nil {
		return nil, fmt.Errorf("parse sample: %w", err)
	}
	enrichOpaqueContainers(shape, idx)

	types, enums := typesAndEnumsFor(m.Name)

	// Apply the same overlay the Go-struct emitter uses: each
	// typed path mentioned in typeTable may synthesize missing
	// intermediate nodes and, critically, mark children that
	// carry `[]` in their path as IsArray. Without this step the
	// schema emitter's path segments would miss the `[]` suffix
	// and scalarGoType wouldn't find any typed entries.
	overlayPaths := make([]string, 0, len(types)+len(enums))
	for p := range types {
		overlayPaths = append(overlayPaths, p)
	}
	for p := range enums {
		overlayPaths = append(overlayPaths, p)
	}
	sort.Strings(overlayPaths)
	for _, p := range overlayPaths {
		overlayTypeTablePath(shape, p)
	}

	goName, err := naming.GoMethod(m.Name)
	if err != nil {
		return nil, fmt.Errorf("derive Go name: %w", err)
	}
	segs := strings.Split(m.Name, ".")
	servicePath := strings.Join(segs[1:len(segs)-1], ".")
	serviceType, err := naming.GoService(servicePath)
	if err != nil {
		return nil, fmt.Errorf("derive service name: %w", err)
	}
	responseType := strings.TrimSuffix(serviceType, "Service") + goName + "Response"

	defs := map[string]any{}
	body := objectSchema(shape, nil, types, enums, defs)
	if body == nil {
		// Empty response — still emit a valid (empty-object) schema.
		body = map[string]any{
			"type":                 "object",
			"additionalProperties": false,
		}
	}
	root := asMap(body)

	root["$schema"] = jsonSchemaDraft
	root["$id"] = fmt.Sprintf("%s/v%d/%s.json", schemaBaseURL, SchemaMajor, m.Name)
	root["title"] = responseType
	if desc := normalizeDescription(m.Description, nil); desc != "" {
		root["description"] = desc
	}
	if len(defs) > 0 {
		root["$defs"] = defs
	}

	return json.MarshalIndent(root, "", "  ")
}

func typesAndEnumsFor(method string) (map[string]fieldType, map[string]string) {
	info, ok := typeTable[method]
	if !ok {
		return map[string]fieldType{}, map[string]string{}
	}
	return info.Response, info.ResponseEnums
}

// schemaForNode dispatches on node shape: scalar text-only
// leaves, opaque empty wrappers, or object-with-attrs-or-
// children.
func schemaForNode(node *shapeNode, path []string, types map[string]fieldType, enums map[string]string, defs map[string]any) any {
	// Opaque empty wrappers carry only `xml:",innerxml" json:"-"`
	// in Go — JSON sees an empty object.
	if node.SelfClosed && len(node.Attrs) == 0 && len(node.Children) == 0 {
		return map[string]any{
			"type":                 "object",
			"additionalProperties": false,
		}
	}
	if node.OnlyText || (node.HasText && len(node.Attrs) == 0 && len(node.Children) == 0) {
		return scalarSchema(path, types, enums, defs)
	}
	return objectSchema(node, path, types, enums, defs)
}

// objectSchema builds the {"type":"object", ...} fragment for a
// node that carries attrs, children, or mixed content. Shared
// element shapes are hoisted into defs and referenced.
func objectSchema(node *shapeNode, path []string, types map[string]fieldType, enums map[string]string, defs map[string]any) any {
	if len(node.Children) == 0 && len(node.Attrs) == 0 && !node.HasText {
		return nil
	}

	// Shared shape? Register under its $defs name and return a $ref.
	// The top-level response (path is empty) is not deduplicated.
	// Build the $def using the caller's full dot-path so
	// scalarGoType's type-table lookups resolve.
	if len(path) > 0 {
		if defName, ok := sharedDefName[node.Name]; ok {
			if _, already := defs[defName]; !already {
				// Stub first to break cycles if the shape ever
				// references itself indirectly.
				defs[defName] = map[string]any{"type": "object"}
				defs[defName] = buildObjectFragment(node, path, types, enums, defs)
			}
			return map[string]any{"$ref": "#/$defs/" + defName}
		}
	}
	return buildObjectFragment(node, path, types, enums, defs)
}

// buildObjectFragment emits the inline {"type":"object", ...}
// body. Shared-shape hoisting happens one layer up in
// objectSchema.
func buildObjectFragment(node *shapeNode, path []string, types map[string]fieldType, enums map[string]string, defs map[string]any) any {
	props := map[string]any{}
	var required []string

	if node.HasText && (len(node.Attrs) > 0 || len(node.Children) > 0) {
		// RTM's mixed-content convention surfaces as the `$t` key.
		props["$t"] = map[string]any{"type": "string"}
	}

	for _, attr := range node.Attrs {
		attrPath := append(append([]string{}, path...), attr)
		goType := scalarGoType(attrPath, types, enums)
		props[attr] = scalarSchemaForGoType(goType, defs, enums, attrPath)
		if requiredByTag(goType) {
			required = append(required, attr)
		}
	}

	for _, child := range node.Children {
		schemaForChild(child, path, types, enums, defs, props, &required)
	}

	obj := map[string]any{
		"type":                 "object",
		"properties":           props,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		sort.Strings(required)
		obj["required"] = required
	}
	return obj
}

// schemaForChild adds one child's property + maybe-required
// entry, applying the same collection-wrapper flattening rule
// the Go emitter uses.
func schemaForChild(child *shapeChild, path []string, types map[string]fieldType, enums map[string]string, defs map[string]any, props map[string]any, required *[]string) {
	if !child.IsArray && isCollectionWrapper(child.Node) {
		gc := child.Node.Children[0]
		gcSeg := gc.Name
		if gc.IsArray {
			gcSeg += "[]"
		}
		gcPath := append(append([]string{}, path...), child.Name, gcSeg)
		itemSchema := schemaForNode(gc.Node, gcPath, types, enums, defs)
		props[child.Name] = map[string]any{
			"type":  "array",
			"items": itemSchema,
		}
		// Slices with omitempty → not required.
		return
	}

	childSeg := child.Name
	if child.IsArray {
		childSeg += "[]"
	}
	childPath := append(append([]string{}, path...), childSeg)

	inner := schemaForNode(child.Node, childPath, types, enums, defs)
	if child.IsArray {
		props[child.Name] = map[string]any{
			"type":  "array",
			"items": inner,
		}
		return
	}
	props[child.Name] = inner

	// Non-array struct children with no IsOptional (i.e. no
	// pointer) have no ,omitempty in the Go tag → required.
	// Scalar-valued children (text-only elements) get the same
	// treatment as attrs of the same goType.
	switch {
	case child.IsOptional:
		// Pointer + omitempty → optional.
	case child.Node.OnlyText || (child.Node.HasText && len(child.Node.Attrs) == 0 && len(child.Node.Children) == 0):
		goType := scalarGoType(childPath, types, enums)
		if requiredByTag(goType) {
			*required = append(*required, child.Name)
		}
	case child.Node.SelfClosed && len(child.Node.Attrs) == 0 && len(child.Node.Children) == 0:
		// Opaque InnerXML wrapper uses json:"-"; property was not
		// added — drop it.
		delete(props, child.Name)
	default:
		// Concrete struct child — always marshalled, always present.
		*required = append(*required, child.Name)
	}
}

// scalarSchema returns the schema fragment for a leaf value at
// the given path, consulting types and enums as scalarGoType
// does for Go emission.
func scalarSchema(path []string, types map[string]fieldType, enums map[string]string, defs map[string]any) any {
	goType := scalarGoType(path, types, enums)
	return scalarSchemaForGoType(goType, defs, enums, path)
}

// scalarSchemaForGoType translates a Go type name (as emitted
// by scalarGoType) to a JSON Schema fragment. Enum aliases are
// hoisted into $defs and referenced.
func scalarSchemaForGoType(goType string, defs map[string]any, enums map[string]string, path []string) any {
	switch goType {
	case "rtmInt":
		return map[string]any{"type": "integer"}
	case "rtmBool":
		return map[string]any{"type": "boolean"}
	case "rtmTime":
		return map[string]any{"type": "string", "format": "date-time"}
	case "string":
		return map[string]any{"type": "string"}
	}
	// Enum alias.
	key := strings.Join(path, ".")
	if enumKey, ok := enums[key]; ok {
		if def, ok := enumCatalogue[enumKey]; ok && def.Name == goType {
			ensureEnumDef(defs, def)
			return map[string]any{"$ref": "#/$defs/" + def.Name}
		}
	}
	// Fallback: treat as string.
	return map[string]any{"type": "string"}
}

// ensureEnumDef inserts an enum alias's $defs entry if not yet
// present.
func ensureEnumDef(defs map[string]any, def enumDef) {
	if _, already := defs[def.Name]; already {
		return
	}
	values := make([]any, len(def.Values))
	for i, v := range def.Values {
		values[i] = v
	}
	defs[def.Name] = map[string]any{
		"type": "string",
		"enum": values,
	}
}

// requiredByTag mirrors jsonOmitSuffix: an attr or scalar child
// is required iff its Go tag carries no ,omitempty / ,omitzero.
// Strings and slices always emit omitempty; rtmTime emits
// omitzero; typed integer/bool wrappers and enum aliases do not,
// so they're required.
func requiredByTag(goType string) bool {
	if goType == "string" || strings.HasPrefix(goType, "[]") || strings.HasPrefix(goType, "*") {
		return false
	}
	if goType == "rtmTime" {
		return false
	}
	return true
}

// asMap coerces a schema fragment (always a map) to a writable
// map. schemaForNode and objectSchema return `any`; hoisting
// keys onto it requires a concrete type.
func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	// Should never happen for a non-nil object schema.
	return map[string]any{}
}
