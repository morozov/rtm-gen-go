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

// autoInjected lists argument names the generator hides from user-
// facing params structs because the client fills them in automatically
// during Call.
var autoInjected = map[string]struct{}{
	"api_key":    {},
	"auth_token": {},
	"timeline":   {},
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
	coreData := coreData{PackageName: cfg.PackageName, Services: serviceRefs(groups)}
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
	RTMName       string
	GoName        string
	Description   string
	ParamsType    string
	Required      []argData
	Optional      []argData
	NeedsLogin    bool
	NeedsTimeline bool
	NeedsSigning  bool
}

// descHereBoilerplate matches RTM's "See here for more details"
// links — always `<a>here</a>` or `<a>See here</a>` followed by
// "for more details." An empirical sweep found 112 of 124
// anchors in the reflection spec follow this pattern, each
// pointing at an RTM web page that agents and terminal users
// cannot follow. Stripping the phrase entirely leaves cleaner
// prose.
var descHereBoilerplate = regexp.MustCompile(`(?i)\s*<a[^>]*>\s*(?:see\s+)?here\s*</a>\s+for\s+more\s+details\s*\.?`)

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

// normalizeDescription returns a single-line, whitespace-
// collapsed, HTML-stripped form of a reflection description,
// safe to embed in a Go doc comment or a cobra Short field.
//
// Order of operations matters: the "See here for more details"
// phrase is stripped while anchor tags are still present so the
// regex can use them as anchor points; known formatting tags
// come out next (with space substitution); HTML entities are
// decoded last so that `&lt;list&gt;` reads as literal `<list>`
// in the output rather than being mistaken for a tag to strip.
func normalizeDescription(s string) string {
	s = descHereBoilerplate.ReplaceAllString(s, "")
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

func buildMethodData(serviceType string, m apispec.Method) (methodData, error) {
	goName, err := naming.GoMethod(m.Name)
	if err != nil {
		return methodData{}, err
	}
	var required, optional []argData
	for _, a := range m.Arguments {
		if _, skip := autoInjected[a.Name]; skip {
			continue
		}
		ad := argData{
			Name:        a.Name,
			GoName:      naming.GoField(a.Name),
			Description: normalizeDescription(a.Description),
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
	return methodData{
		RTMName:       m.Name,
		GoName:        goName,
		Description:   normalizeDescription(m.Description),
		ParamsType:    params,
		Required:      required,
		Optional:      optional,
		NeedsLogin:    m.NeedsLogin,
		NeedsTimeline: m.NeedsTimeline,
		NeedsSigning:  m.NeedsSigning,
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
