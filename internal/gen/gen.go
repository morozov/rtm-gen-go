package gen

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"go/format"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"

	"github.com/morozov/rtm-gen-go/internal/apispec"
	"github.com/morozov/rtm-gen-go/internal/naming"
)

const (
	filePerm = 0o644
	dirPerm  = 0o755
)

// ErrInvalidConfig is returned when the caller supplied an incomplete
// generator configuration.
var ErrInvalidConfig = errors.New("invalid generator config")

// Config describes a single client generation target.
type Config struct {
	OutDir      string
	PackageName string
}

// hiddenArgs lists argument names the generator drops from both
// the client params structs and the CLI flags. Two reasons an
// argument lands here:
//   - The client fills it in automatically during Call (api_key,
//     auth_token, timeline).
//   - Exposing it would break the client: callback wraps the
//     response in a JSONP call, producing a body the JSON decoder
//     cannot parse.
var hiddenArgs = map[string]struct{}{
	"api_key":    {},
	"auth_token": {},
	"timeline":   {},
	"callback":   {},
}

//go:embed core.go.tmpl
var coreTmplSrc string

//go:embed service.go.tmpl
var serviceTmplSrc string

var (
	coreTmpl    = template.Must(template.New("core").Parse(coreTmplSrc))
	serviceTmpl = template.Must(template.New("service").Parse(serviceTmplSrc))
)

// GenerateClient emits the RTM client package into cfg.OutDir from
// the given spec. It creates cfg.OutDir if missing, overwrites
// generated files, and returns the list of files written. It does
// not emit go.mod or any file outside cfg.OutDir.
func GenerateClient(spec apispec.Spec, cfg Config) ([]string, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}
	groups, err := groupByService(spec)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.OutDir, dirPerm); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", cfg.OutDir, err)
	}
	written := make([]string, 0, len(groups)+1)

	corePath := filepath.Join(cfg.OutDir, "client.go")
	coreData := coreData{
		PackageName: cfg.PackageName,
		Services:    serviceRefs(groups),
		Enums:       enumsUsedBy(spec),
	}
	if err := renderGoFile(corePath, coreTmpl, coreData); err != nil {
		return nil, err
	}
	written = append(written, corePath)

	for _, sg := range groups {
		data, err := buildServiceData(cfg.PackageName, sg)
		if err != nil {
			return nil, fmt.Errorf("build service %s: %w", sg.servicePath, err)
		}
		filename := strings.ReplaceAll(sg.servicePath, ".", "_") + ".go"
		path := filepath.Join(cfg.OutDir, filename)
		if err := renderGoFile(path, serviceTmpl, data); err != nil {
			return nil, err
		}
		written = append(written, path)
	}
	return written, nil
}

func validateConfig(cfg Config) error {
	switch {
	case cfg.OutDir == "":
		return fmt.Errorf("OutDir is empty: %w", ErrInvalidConfig)
	case cfg.PackageName == "":
		return fmt.Errorf("PackageName is empty: %w", ErrInvalidConfig)
	}
	return nil
}

type coreData struct {
	PackageName string
	Services    []serviceRef
	Enums       []enumRenderData
}

// enumRenderData is the per-enum view rendered into the client's
// core file: one named string type plus a const block.
type enumRenderData struct {
	TypeName string
	Values   []enumValueRenderData
}

type enumValueRenderData struct {
	GoName    string
	WireValue string
}

type serviceRef struct {
	FieldName string
	TypeName  string
}

type serviceData struct {
	PackageName string
	TypeName    string
	RTMPrefix   string
	Methods     []methodData
}

type methodData struct {
	RTMName          string
	GoName           string
	Description      string
	ParamsType       string
	ResponseType     string // Go type name for the method's response struct
	ResponseGoSource string // Go source for the response struct body
	Required         []argData
	Optional         []argData
	NeedsLogin       bool
	NeedsTimeline    bool
	NeedsSigning     bool
}

// descHereBoilerplate matches RTM's "See here for more details"
// boilerplate anchor phrase. Captures the href so we can record
// a footnote reference. An empirical sweep found 112 of 124
// anchors in the reflection spec follow this pattern — replacing
// the whole phrase with a compact footnote marker (or empty
// text, when no builder is supplied) keeps the surrounding prose
// readable.
var descHereBoilerplate = regexp.MustCompile(`(?i)\s*<a\s+[^>]*?href="([^"]+)"[^>]*>\s*(?:see\s+)?here\s*</a>\s+for\s+more\s+details\s*\.?`)

// descAnchor matches any `<a href="...">content</a>` pair and
// captures the href and the inner content. Applied after
// descHereBoilerplate, so only non-boilerplate anchors (12 of
// 124 in the reflection spec, carrying meaningful content like
// "Smart Add" or "rtm.time.parse") remain for it to process.
var descAnchor = regexp.MustCompile(`(?is)<a\s+[^>]*?href="([^"]+)"[^>]*>(.*?)</a>`)

// descHTMLFormatTag matches a whitelist of HTML formatting tags
// that RTM sprinkles into descriptions (<b>, <code>, <br>, …).
// Tags are replaced with a space so word boundaries survive
// stripping. Literal references to XML elements like `<list>`
// or `<script>` are not in the whitelist and pass through
// unchanged.
var descHTMLFormatTag = regexp.MustCompile(`(?i)</?(?:a|b|i|em|strong|p|code|br|span|div)(?:\s[^>]*)?/?>`)

// descWhitespace collapses any run of whitespace (including
// embedded newlines) to a single space.
var descWhitespace = regexp.MustCompile(`\s+`)

// descSpaceBeforePunct removes a single stray space before
// common punctuation — a byproduct of replacing inline tags
// with spaces (e.g. `<code>foo</code>,` → `foo ,`). RTM never
// intends a space in those positions.
var descSpaceBeforePunct = regexp.MustCompile(`\s+([,.;:])`)

// reference is one footnote entry attached to a cobra command.
type reference struct {
	N   int
	URL string
}

// refBuilder collects the per-command footnote references as
// anchors are encountered in a description. Each distinct
// resolved URL gets one number; duplicates reuse the existing
// number. Numbers count up from 1 in first-appearance order.
type refBuilder struct {
	byURL map[string]int
	refs  []reference
}

func newRefBuilder() *refBuilder {
	return &refBuilder{byURL: map[string]int{}}
}

// mark returns the footnote number for href, assigning a fresh
// one if this is the first time href is seen. href is resolved
// through the redirects map before being keyed.
func (b *refBuilder) mark(href string) int {
	url := resolveDocsURL(href)
	if n, ok := b.byURL[url]; ok {
		return n
	}
	n := len(b.refs) + 1
	b.byURL[url] = n
	b.refs = append(b.refs, reference{N: n, URL: url})
	return n
}

// references returns the ordered slice of references gathered so
// far. The slice is nil when no anchors were encountered.
func (b *refBuilder) references() []reference {
	return b.refs
}

// normalizeDescription returns a single-line, whitespace-
// collapsed, HTML-stripped form of a reflection description,
// safe to embed in a Go doc comment or a cobra Short field.
//
// When b is non-nil, anchors become `[^N]` footnote markers and
// the builder accumulates the href → number mapping. When b is
// nil (client godoc context), anchors lose their tags: boilerplate
// "See here for more details." phrases are dropped; other anchors
// keep their inner content verbatim. Either mode strips known
// HTML formatting tags and decodes HTML entities.
//
// Order of operations matters: the "See here for more details"
// phrase is handled while anchor tags are still present so the
// regex can use them as anchor points; known formatting tags
// come out next (with space substitution); HTML entities are
// decoded last so that `&lt;list&gt;` reads as literal `<list>`
// in the output rather than being mistaken for a tag to strip.
func normalizeDescription(s string, b *refBuilder) string {
	s = descHereBoilerplate.ReplaceAllStringFunc(s, func(match string) string {
		if b == nil {
			return ""
		}
		sub := descHereBoilerplate.FindStringSubmatch(match)
		return fmt.Sprintf("[^%d]", b.mark(sub[1]))
	})
	s = descAnchor.ReplaceAllStringFunc(s, func(match string) string {
		sub := descAnchor.FindStringSubmatch(match)
		href, content := sub[1], sub[2]
		if b == nil {
			return content
		}
		return fmt.Sprintf("%s[^%d]", content, b.mark(href))
	})
	s = descHTMLFormatTag.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = descWhitespace.ReplaceAllString(s, " ")
	s = descSpaceBeforePunct.ReplaceAllString(s, "$1")
	return strings.TrimSpace(s)
}

type argData struct {
	Name        string
	GoName      string
	Description string
	GoType      string // primitive ("string"/"bool"/"int64"/"[]string") or enum alias name
	WireFunc    string // "" (no conversion) | "rtmFormatBool" | "rtmFormatInt" | "rtmJoinStringSlice" | "string" (enum cast)
}

type serviceGroup struct {
	servicePath string
	typeName    string
	fieldName   string
	methods     []apispec.Method
}

func groupByService(spec apispec.Spec) ([]serviceGroup, error) {
	byPath := make(map[string][]apispec.Method)
	for _, m := range spec {
		parts := strings.Split(m.Name, ".")
		if len(parts) < 3 || parts[0] != "rtm" {
			return nil, fmt.Errorf("method name %q is not rtm.<service>[.<sub>].<method>", m.Name)
		}
		sp := strings.Join(parts[1:len(parts)-1], ".")
		byPath[sp] = append(byPath[sp], m)
	}
	paths := make([]string, 0, len(byPath))
	for p := range byPath {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	out := make([]serviceGroup, 0, len(paths))
	for _, p := range paths {
		typeName, err := naming.GoService(p)
		if err != nil {
			return nil, fmt.Errorf("service path %q: %w", p, err)
		}
		methods := append([]apispec.Method{}, byPath[p]...)
		sort.Slice(methods, func(i, j int) bool { return methods[i].Name < methods[j].Name })
		out = append(out, serviceGroup{
			servicePath: p,
			typeName:    typeName,
			fieldName:   strings.TrimSuffix(typeName, "Service"),
			methods:     methods,
		})
	}
	return out, nil
}

func serviceRefs(groups []serviceGroup) []serviceRef {
	out := make([]serviceRef, len(groups))
	for i, g := range groups {
		out[i] = serviceRef{FieldName: g.fieldName, TypeName: g.typeName}
	}
	return out
}

// enumsUsedBy walks typeTable for methods present in spec and
// returns the subset of enumCatalogue entries actually referenced
// as an arg or response enum. Keeps the generated client free of
// enum declarations no emitted code uses.
func enumsUsedBy(spec apispec.Spec) []enumRenderData {
	used := map[string]struct{}{}
	present := map[string]struct{}{}
	for _, m := range spec {
		present[m.Name] = struct{}{}
	}
	for method, info := range typeTable {
		if _, ok := present[method]; !ok {
			continue
		}
		for _, key := range info.ArgEnums {
			used[key] = struct{}{}
		}
		for _, key := range info.ResponseEnums {
			used[key] = struct{}{}
		}
	}
	keys := make([]string, 0, len(used))
	for k := range used {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]enumRenderData, 0, len(keys))
	for _, k := range keys {
		def := enumCatalogue[k]
		vals := make([]enumValueRenderData, len(def.Values))
		for i, v := range def.Values {
			vals[i] = enumValueRenderData{GoName: def.GoNames[i], WireValue: v}
		}
		out = append(out, enumRenderData{TypeName: def.Name, Values: vals})
	}
	return out
}

func buildServiceData(pkgName string, sg serviceGroup) (serviceData, error) {
	data := serviceData{
		PackageName: pkgName,
		TypeName:    sg.typeName,
		RTMPrefix:   "rtm." + sg.servicePath,
		Methods:     make([]methodData, 0, len(sg.methods)),
	}
	for _, m := range sg.methods {
		md, err := buildMethodData(sg.typeName, m)
		if err != nil {
			return serviceData{}, err
		}
		data.Methods = append(data.Methods, md)
	}
	return data, nil
}

// argTypeInfo resolves an argType into the triple of Go source
// fragments the generator's templates need: the Go type name for
// a Params struct field, the cobra flag register to call, and
// the helper that stringifies a Go value back to RTM's wire
// format. The untyped default (argTypeString) returns empty
// WireFunc to signal "use the value directly".
func argTypeInfo(t argType) (goType, flagRegister, wireFunc string) {
	switch t {
	case argTypeBool:
		return "bool", "BoolVar", "rtmFormatBool"
	case argTypeInt:
		return "int64", "Int64Var", "rtmFormatInt"
	case argTypeStringSlice:
		return "[]string", "StringSliceVar", "rtmJoinStringSlice"
	default:
		return "string", "StringVar", ""
	}
}

func argTypeFor(method, argName string) argType {
	if info, ok := typeTable[method]; ok {
		if t, ok := info.Arguments[argName]; ok {
			return t
		}
	}
	return argTypeString
}

// argEnumFor returns the enum catalogue key an argument is bound
// to, or "" if the argument isn't an enum. Enum args take priority
// over `Arguments` entries — the catalogue supplies both the Go
// alias type and the legal-values set.
func argEnumFor(method, argName string) string {
	info, ok := typeTable[method]
	if !ok {
		return ""
	}
	return info.ArgEnums[argName]
}

func buildMethodData(serviceType string, m apispec.Method) (methodData, error) {
	goName, err := naming.GoMethod(m.Name)
	if err != nil {
		return methodData{}, err
	}
	var required, optional []argData
	for _, a := range m.Arguments {
		if _, skip := hiddenArgs[a.Name]; skip {
			continue
		}
		goType, _, wireFunc := argTypeInfo(argTypeFor(m.Name, a.Name))
		if enumKey := argEnumFor(m.Name, a.Name); enumKey != "" {
			def := enumCatalogue[enumKey]
			goType = def.Name
			wireFunc = "string" // enum → wire string is a native Go conversion
		}
		ad := argData{
			Name:        a.Name,
			GoName:      naming.GoField(a.Name),
			Description: normalizeDescription(a.Description, nil),
			GoType:      goType,
			WireFunc:    wireFunc,
		}
		if a.Optional {
			optional = append(optional, ad)
		} else {
			required = append(required, ad)
		}
	}
	params := ""
	if len(required)+len(optional) > 0 {
		params = strings.TrimSuffix(serviceType, "Service") + goName + "Params"
	}
	responseType := strings.TrimSuffix(serviceType, "Service") + goName + "Response"
	shape, err := parseSampleXML(m.Response)
	if err != nil {
		return methodData{}, fmt.Errorf("parse sample response for %s: %w", m.Name, err)
	}
	responseSrc, err := emitResponseType(m.Name, shape)
	if err != nil {
		return methodData{}, fmt.Errorf("emit response type for %s: %w", m.Name, err)
	}
	return methodData{
		RTMName:          m.Name,
		GoName:           goName,
		Description:      normalizeDescription(m.Description, nil),
		ParamsType:       params,
		ResponseType:     responseType,
		ResponseGoSource: responseSrc,
		Required:         required,
		Optional:         optional,
		NeedsLogin:       m.NeedsLogin,
		NeedsTimeline:    m.NeedsTimeline,
		NeedsSigning:     m.NeedsSigning,
	}, nil
}

func renderGoFile(path string, tmpl *template.Template, data any) error {
	var raw bytes.Buffer
	if err := tmpl.Execute(&raw, data); err != nil {
		return fmt.Errorf("execute template for %s: %w", path, err)
	}
	formatted, err := format.Source(raw.Bytes())
	if err != nil {
		return fmt.Errorf("format %s: %w\n--- source ---\n%s", path, err, raw.String())
	}
	if err := os.WriteFile(path, formatted, filePerm); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
