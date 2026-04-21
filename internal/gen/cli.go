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
	ModulePath        string
	PackageName       string
	GoVersion         string
	ClientModulePath  string
	ClientPackageName string
	ClientVersion     string
	CobraVersion      string
}

//go:embed cli_gomod.tmpl
var cliGomodTmplSrc string

//go:embed cli_main.go.tmpl
var cliMainTmplSrc string

//go:embed cli_root.go.tmpl
var cliRootTmplSrc string

//go:embed cli_service.go.tmpl
var cliServiceTmplSrc string

var (
	cliGomodTmpl   = template.Must(template.New("cli_gomod").Parse(cliGomodTmplSrc))
	cliMainTmpl    = template.Must(template.New("cli_main").Parse(cliMainTmplSrc))
	cliRootTmpl    = template.Must(template.New("cli_root").Parse(cliRootTmplSrc))
	cliServiceTmpl = template.Must(template.New("cli_service").Parse(cliServiceTmplSrc))
)

// GenerateCLI emits a self-contained Go cobra-based CLI module into
// cfg.OutDir from the given spec. It creates cfg.OutDir and the
// cmd/rtm binary subdirectory if missing, overwrites generated
// files, and returns the list of files written.
func GenerateCLI(spec apispec.Spec, cfg CLIConfig) ([]string, error) {
	if err := validateCLIConfig(cfg); err != nil {
		return nil, err
	}
	groups, err := groupByService(spec)
	if err != nil {
		return nil, err
	}
	cmdDir := filepath.Join(cfg.OutDir, "cmd", "rtm")
	if err := os.MkdirAll(cmdDir, dirPerm); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", cmdDir, err)
	}

	written := make([]string, 0, len(groups)+3)

	gomodPath := filepath.Join(cfg.OutDir, "go.mod")
	if err := writeRaw(gomodPath, cliGomodTmpl, cfg); err != nil {
		return nil, err
	}
	written = append(written, gomodPath)

	mainPath := filepath.Join(cmdDir, "main.go")
	mainData := struct {
		CLIModulePath string
		PackageAlias  string
	}{CLIModulePath: cfg.ModulePath, PackageAlias: cfg.PackageName}
	if err := renderGoFile(mainPath, cliMainTmpl, mainData); err != nil {
		return nil, err
	}
	written = append(written, mainPath)

	rootPath := filepath.Join(cfg.OutDir, "root.go")
	rootData := buildCLIRootData(cfg, groups)
	if err := renderGoFile(rootPath, cliRootTmpl, rootData); err != nil {
		return nil, err
	}
	written = append(written, rootPath)

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
	case cfg.ModulePath == "":
		return fmt.Errorf("ModulePath is empty: %w", ErrInvalidConfig)
	case cfg.PackageName == "":
		return fmt.Errorf("PackageName is empty: %w", ErrInvalidConfig)
	case cfg.GoVersion == "":
		return fmt.Errorf("GoVersion is empty: %w", ErrInvalidConfig)
	case cfg.ClientModulePath == "":
		return fmt.Errorf("ClientModulePath is empty: %w", ErrInvalidConfig)
	case cfg.ClientPackageName == "":
		return fmt.Errorf("ClientPackageName is empty: %w", ErrInvalidConfig)
	case cfg.ClientVersion == "":
		return fmt.Errorf("ClientVersion is empty: %w", ErrInvalidConfig)
	case cfg.CobraVersion == "":
		return fmt.Errorf("CobraVersion is empty: %w", ErrInvalidConfig)
	}
	return nil
}

type cliRootData struct {
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
	Builder            string
	FieldName          string
	Methods            []cliMethodData
	NeedsClientImport  bool
}

type cliMethodData struct {
	CLIName    string
	GoName     string
	ParamsType string
	Builder    string
	Required   []cliArg
	Optional   []cliArg
}

type cliArg struct {
	FlagName string
	VarName  string
	GoField  string
}

func buildCLIRootData(cfg CLIConfig, groups []serviceGroup) cliRootData {
	data := cliRootData{
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

	var required, optional []cliArg
	for _, a := range m.Arguments {
		if _, skip := autoInjected[a.Name]; skip {
			continue
		}
		ca := cliArg{
			FlagName: strings.ReplaceAll(a.Name, "_", "-"),
			VarName:  naming.GoLocal(a.Name),
			GoField:  naming.GoField(a.Name),
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
		ParamsType: params,
		Builder:    builder,
		Required:   required,
		Optional:   optional,
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
