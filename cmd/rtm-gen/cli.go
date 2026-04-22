package main

import (
	"fmt"

	"github.com/spf13/pflag"

	"github.com/morozov/rtm-gen-go/internal/gen"
)

func runCLI(args []string) error {
	fs := pflag.NewFlagSet("cli", pflag.ContinueOnError)
	specPath := fs.String("spec", "", "path to a local RTM reflection dump (mutually exclusive with --key/--secret)")
	apiKey := fs.String("key", "", "RTM API key for live spec fetch")
	apiSecret := fs.String("secret", "", "RTM API secret for live spec fetch")
	outDir := fs.String("out", "generated/commands", "output directory for the generated commands package")
	pkgName := fs.String("package", "commands", "Go package name for the generated commands")
	clientModule := fs.String("client-module", "github.com/morozov/rtm-cli-go/internal/rtm", "import path of the generated client package")
	clientPkg := fs.String("client-package", "rtm", "Go package name of the generated client")
	if err := fs.Parse(args); err != nil {
		return err
	}

	spec, err := loadSpec(*specPath, *apiKey, *apiSecret)
	if err != nil {
		return err
	}
	files, err := gen.GenerateCLI(spec, gen.CLIConfig{
		OutDir:            *outDir,
		PackageName:       *pkgName,
		ClientModulePath:  *clientModule,
		ClientPackageName: *clientPkg,
	})
	if err != nil {
		return err
	}
	for _, f := range files {
		fmt.Println(f)
	}
	return nil
}
