package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/morozov/rtm-gen-go/internal/apispec"
	"github.com/morozov/rtm-gen-go/internal/fetch"
	"github.com/morozov/rtm-gen-go/internal/gen"
)

func runClient(args []string) error {
	fs := flag.NewFlagSet("client", flag.ContinueOnError)
	specPath := fs.String("spec", "", "path to a local RTM reflection dump (mutually exclusive with -key/-secret)")
	apiKey := fs.String("key", "", "RTM API key for live spec fetch (requires -secret; mutually exclusive with -spec)")
	apiSecret := fs.String("secret", "", "RTM API secret for live spec fetch")
	outDir := fs.String("out", "generated/rtm-client-go", "output directory for the generated module")
	modulePath := fs.String("module", "github.com/morozov/rtm-client-go", "Go module path to declare in go.mod")
	pkgName := fs.String("package", "rtm", "Go package name for the generated code")
	goVersion := fs.String("go", "1.26", "Go version declared in go.mod")
	if err := fs.Parse(args); err != nil {
		return err
	}

	spec, err := loadSpec(*specPath, *apiKey, *apiSecret)
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

func loadSpec(specPath, apiKey, apiSecret string) (apispec.Spec, error) {
	hasFile := specPath != ""
	hasCreds := apiKey != "" || apiSecret != ""
	switch {
	case !hasFile && !hasCreds:
		return nil, fmt.Errorf("either -spec <path> or -key and -secret must be supplied")
	case hasFile && hasCreds:
		return nil, fmt.Errorf("-spec and -key/-secret are mutually exclusive")
	case hasCreds && (apiKey == "" || apiSecret == ""):
		return nil, fmt.Errorf("live fetch requires both -key and -secret")
	case hasFile:
		return apispec.Load(specPath)
	default:
		return fetch.Fetch(context.Background(), apiKey, apiSecret, "")
	}
}
