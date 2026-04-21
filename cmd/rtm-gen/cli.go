package main

import (
	"flag"
	"fmt"

	"github.com/morozov/rtm-gen-go/internal/apispec"
	"github.com/morozov/rtm-gen-go/internal/gen"
)

func runCLI(args []string) error {
	fs := flag.NewFlagSet("cli", flag.ContinueOnError)
	specPath := fs.String("spec", "api.json", "path to the RTM reflection dump")
	outDir := fs.String("out", "generated/rtm-cli-go", "output directory for the generated module")
	modulePath := fs.String("module", "github.com/morozov/rtm-cli-go", "Go module path to declare in go.mod")
	pkgName := fs.String("package", "rtmcli", "Go package name for the generated library")
	goVersion := fs.String("go", "1.26", "Go version declared in go.mod")
	clientModule := fs.String("client-module", "github.com/morozov/rtm-client-go", "Go module path of the generated client")
	clientPkg := fs.String("client-package", "rtm", "Go package name of the generated client")
	clientVersion := fs.String("client-version", "v0.0.1", "version of the client module to require")
	cobraVersion := fs.String("cobra-version", "1.8.1", "version of github.com/spf13/cobra to require")
	if err := fs.Parse(args); err != nil {
		return err
	}

	spec, err := apispec.Load(*specPath)
	if err != nil {
		return err
	}
	files, err := gen.GenerateCLI(spec, gen.CLIConfig{
		OutDir:            *outDir,
		ModulePath:        *modulePath,
		PackageName:       *pkgName,
		GoVersion:         *goVersion,
		ClientModulePath:  *clientModule,
		ClientPackageName: *clientPkg,
		ClientVersion:     *clientVersion,
		CobraVersion:      *cobraVersion,
	})
	if err != nil {
		return err
	}
	for _, f := range files {
		fmt.Println(f)
	}
	return nil
}
