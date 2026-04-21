package main

import (
	"flag"
	"fmt"

	"github.com/morozov/rtm-gen-go/internal/apispec"
	"github.com/morozov/rtm-gen-go/internal/gen"
)

func runClient(args []string) error {
	fs := flag.NewFlagSet("client", flag.ContinueOnError)
	specPath := fs.String("spec", "api.json", "path to the RTM reflection dump")
	outDir := fs.String("out", "generated/rtm-client-go", "output directory for the generated module")
	modulePath := fs.String("module", "github.com/morozov/rtm-client-go", "Go module path to declare in go.mod")
	pkgName := fs.String("package", "rtm", "Go package name for the generated code")
	goVersion := fs.String("go", "1.26", "Go version declared in go.mod")
	if err := fs.Parse(args); err != nil {
		return err
	}

	spec, err := apispec.Load(*specPath)
	if err != nil {
		return err
	}
	files, err := gen.GenerateClient(spec, gen.Config{
		OutDir:      *outDir,
		ModulePath:  *modulePath,
		PackageName: *pkgName,
		GoVersion:   *goVersion,
	})
	if err != nil {
		return err
	}
	for _, f := range files {
		fmt.Println(f)
	}
	return nil
}
