package gen

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/morozov/rtm-gen-go/internal/apispec"
	"github.com/morozov/rtm-gen-go/internal/naming"
)

// CLIConfig describes a CLI-generation target.
type CLIConfig struct {
	OutDir            string
	PackageName       string
	ClientModulePath  string
	ClientPackageName string
}

//go:embed cli_register.go.tmpl
var cliRegisterTmplSrc string

//go:embed cli_service.go.tmpl
var cliServiceTmplSrc string

var (
	cliRegisterTmpl = template.Must(template.New("cli_register").Parse(cliRegisterTmplSrc))
	cliServiceTmpl  = template.Must(template.New("cli_service").Parse(cliServiceTmplSrc))
)

// GenerateCLI emits the cobra commands package into cfg.OutDir from
// the given spec. It creates cfg.OutDir if missing, overwrites
// generated files, and returns the list of files written. It does
// not emit go.mod, main.go, or any file outside cfg.OutDir.
func GenerateCLI(spec apispec.Spec, cfg CLIConfig) ([]string, error) {
	if err := validateCLIConfig(cfg); err != nil {
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

	registerPath := filepath.Join(cfg.OutDir, "register.go")
	registerData := buildCLIRegisterData(cfg, groups)
	if err := renderGoFile(registerPath, cliRegisterTmpl, registerData); err != nil {
		return nil, err
	}
	written = append(written, registerPath)

	for _, sg := range groups {
		data, err := buildCLIServiceData(cfg, sg)
		if err != nil {
			return nil, fmt.Errorf("build CLI service %s: %w", sg.servicePath, err)
		}
		filename := strings.ReplaceAll(sg.servicePath, ".", "_") + ".go"
		path := filepath.Join(cfg.OutDir, filename)
		if err := renderGoFile(path, cliServiceTmpl, data); err != nil {
			return nil, err
		}
		written = append(written, path)
	}
	return written, nil
}

func validateCLIConfig(cfg CLIConfig) error {
	switch {
	case cfg.OutDir == "":
		return fmt.Errorf("OutDir is empty: %w", ErrInvalidConfig)
	case cfg.PackageName == "":
		return fmt.Errorf("PackageName is empty: %w", ErrInvalidConfig)
	case cfg.ClientModulePath == "":
		return fmt.Errorf("ClientModulePath is empty: %w", ErrInvalidConfig)
	case cfg.ClientPackageName == "":
		return fmt.Errorf("ClientPackageName is empty: %w", ErrInvalidConfig)
	}
	return nil
}

type cliRegisterData struct {
	PackageName        string
	ClientPackageAlias string
	ClientModulePath   string
	AllServices        []cliServiceRef
	NestedWiring       []nestedPair
	TopLevelVars       []string
}

type cliServiceRef struct {
	VarName string
	Builder string
}

type nestedPair struct {
	Parent string
	Child  string
}

type cliServiceData struct {
	PackageName        string
	ClientPackageAlias string
	ClientModulePath   string
	ServicePath        string
	CLIName            string
	Short              string
	Builder            string
	FieldName          string
	Methods            []cliMethodData
	NeedsClientImport  bool
}

type cliMethodData struct {
	CLIName    string
	GoName     string
	Short      string
	ParamsType string
	Builder    string
	Required   []cliArg
	Optional   []cliArg
	References []reference
}

type cliArg struct {
	FlagName     string
	VarName      string
	GoField      string
	Description  string
	GoType       string // "string" | "bool" | "int64"
	FlagRegister string // "StringVar" | "BoolVar" | "Int64Var"
	DefaultLit   string // `""` | `false` | `0`
}

// defaultLiteralFor returns the Go source literal used as the
// default value when registering a cobra flag of the given Go
// type. Matches the zero value of the type.
func defaultLiteralFor(goType string) string {
	switch goType {
	case "bool":
		return "false"
	case "int64":
		return "0"
	default:
		return `""`
	}
}

func buildCLIRegisterData(cfg CLIConfig, groups []serviceGroup) cliRegisterData {
	data := cliRegisterData{
		PackageName:        cfg.PackageName,
		ClientPackageAlias: cfg.ClientPackageName,
		ClientModulePath:   cfg.ClientModulePath,
	}
	byPath := make(map[string]cliServiceRef, len(groups))
	for _, g := range groups {
		varName, builder := cliBuilderNames(g)
		ref := cliServiceRef{VarName: varName, Builder: builder}
		data.AllServices = append(data.AllServices, ref)
		byPath[g.servicePath] = ref
	}
	for _, g := range groups {
		ref := byPath[g.servicePath]
		if !strings.Contains(g.servicePath, ".") {
			data.TopLevelVars = append(data.TopLevelVars, ref.VarName)
			continue
		}
		parentPath := g.servicePath[:strings.LastIndex(g.servicePath, ".")]
		parent, ok := byPath[parentPath]
		if !ok {
			// Parent service isn't defined; attach to root so the tree is
			// still reachable.
			data.TopLevelVars = append(data.TopLevelVars, ref.VarName)
			continue
		}
		data.NestedWiring = append(data.NestedWiring, nestedPair{
			Parent: parent.VarName,
			Child:  ref.VarName,
		})
	}
	return data
}

func buildCLIServiceData(cfg CLIConfig, sg serviceGroup) (cliServiceData, error) {
	_, builder := cliBuilderNames(sg)
	data := cliServiceData{
		PackageName:        cfg.PackageName,
		ClientPackageAlias: cfg.ClientPackageName,
		ClientModulePath:   cfg.ClientModulePath,
		ServicePath:        sg.servicePath,
		CLIName:            serviceLeaf(sg.servicePath),
		Short:              fmt.Sprintf("rtm.%s.* methods", sg.servicePath),
		Builder:            builder,
		FieldName:          sg.fieldName,
	}
	for _, m := range sg.methods {
		md, err := buildCLIMethodData(sg, m)
		if err != nil {
			return cliServiceData{}, err
		}
		if md.ParamsType != "" {
			data.NeedsClientImport = true
		}
		data.Methods = append(data.Methods, md)
	}
	return data, nil
}

func buildCLIMethodData(sg serviceGroup, m apispec.Method) (cliMethodData, error) {
	goName, err := naming.GoMethod(m.Name)
	if err != nil {
		return cliMethodData{}, err
	}
	cliPath, err := naming.CLICommand(m.Name)
	if err != nil {
		return cliMethodData{}, err
	}
	if len(cliPath) == 0 {
		return cliMethodData{}, fmt.Errorf("empty CLI path for %q: %w", m.Name, ErrInvalidConfig)
	}
	cliName := cliPath[len(cliPath)-1]

	// Per spec 006, footnote numbering is per-command and shared
	// across the method description and all flag descriptions.
	// Normalize the method description first so any anchors there
	// claim [^1]/[^2]/... before the flags do.
	b := newRefBuilder()
	short := normalizeDescription(m.Description, b)

	var required, optional []cliArg
	for _, a := range m.Arguments {
		if _, skip := autoInjected[a.Name]; skip {
			continue
		}
		goType, flagReg, _ := argTypeInfo(argTypeFor(m.Name, a.Name))
		ca := cliArg{
			FlagName:     strings.ReplaceAll(a.Name, "_", "-"),
			VarName:      naming.GoLocal(a.Name),
			GoField:      naming.GoField(a.Name),
			Description:  normalizeDescription(a.Description, b),
			GoType:       goType,
			FlagRegister: flagReg,
			DefaultLit:   defaultLiteralFor(goType),
		}
		if a.Optional {
			optional = append(optional, ca)
		} else {
			required = append(required, ca)
		}
	}
	params := ""
	if len(required)+len(optional) > 0 {
		params = strings.TrimSuffix(sg.typeName, "Service") + goName + "Params"
	}
	builder := "new" + strings.TrimSuffix(sg.typeName, "Service") + goName + "Cmd"
	return cliMethodData{
		CLIName:    cliName,
		GoName:     goName,
		Short:      short,
		ParamsType: params,
		Builder:    builder,
		Required:   required,
		Optional:   optional,
		References: b.references(),
	}, nil
}

func cliBuilderNames(sg serviceGroup) (varName, builder string) {
	base := strings.TrimSuffix(sg.typeName, "Service")
	return lowerFirst(base) + "Cmd", "new" + base + "Cmd"
}

func serviceLeaf(servicePath string) string {
	if i := strings.LastIndex(servicePath, "."); i >= 0 {
		return servicePath[i+1:]
	}
	return servicePath
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}
